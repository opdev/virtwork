// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/config"
)

var _ = Describe("SQLiteAuditor", func() {
	var (
		auditor *audit.SQLiteAuditor
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		auditor, err = audit.NewSQLiteAuditor(":memory:")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(auditor.Close()).To(Succeed())
	})

	Describe("schema creation", func() {
		It("creates all expected tables", func() {
			db := auditor.DB()
			tables := []string{"audit_log", "workload_details", "vm_details", "resource_details", "events"}
			for _, table := range tables {
				var name string
				err := db.QueryRow(
					`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
				).Scan(&name)
				Expect(err).NotTo(HaveOccurred(), "table %s should exist", table)
				Expect(name).To(Equal(table))
			}
		})

		It("creates expected indexes", func() {
			db := auditor.DB()
			indexes := []string{
				"idx_audit_log_started_at", "idx_audit_log_namespace", "idx_audit_log_status",
				"idx_audit_log_run_id",
				"idx_workload_details_audit_id", "idx_workload_details_type",
				"idx_vm_details_audit_id", "idx_vm_details_workload_id", "idx_vm_details_vm_name",
				"idx_resource_details_audit_id", "idx_resource_details_type",
				"idx_events_audit_id", "idx_events_event_type", "idx_events_occurred_at",
			}
			for _, idx := range indexes {
				var name string
				err := db.QueryRow(
					`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx,
				).Scan(&name)
				Expect(err).NotTo(HaveOccurred(), "index %s should exist", idx)
			}
		})
	})

	Describe("execution lifecycle", func() {
		var (
			cfg *config.Config
		)

		BeforeEach(func() {
			cfg = &config.Config{
				Namespace:           "test-ns",
				ContainerDiskImage:  "quay.io/test/image:latest",
				CPUCores:            4,
				Memory:              "4Gi",
				DataDiskSize:        "20Gi",
				KubeconfigPath:      "/tmp/kubeconfig",
				WaitForReady:        true,
				ReadyTimeoutSeconds: 300,
				Workloads: map[string]config.WorkloadConfig{
					"cpu":      {Enabled: true, VMCount: 2, CPUCores: 4, Memory: "4Gi"},
					"database": {Enabled: true, VMCount: 1, CPUCores: 2, Memory: "2Gi"},
				},
			}
		})

		It("records a full run lifecycle", func() {
			// Start execution
			execID, runID, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(execID).To(BeNumerically(">", 0))
			Expect(runID).NotTo(BeEmpty())

			// Verify audit_log row
			db := auditor.DB()
			var status, namespace, command string
			err = db.QueryRow(
				`SELECT status, namespace, command FROM audit_log WHERE id = ?`, execID,
			).Scan(&status, &namespace, &command)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("in_progress"))
			Expect(namespace).To(Equal("test-ns"))
			Expect(command).To(Equal("run"))

			// Record workload
			wlID, err := auditor.RecordWorkload(ctx, execID, audit.WorkloadRecord{
				WorkloadType:    "cpu",
				Enabled:         true,
				VMCount:         2,
				CPUCores:        4,
				Memory:          "4Gi",
				HasDataDisk:     false,
				RequiresService: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(wlID).To(BeNumerically(">", 0))

			// Record VM
			vmID, err := auditor.RecordVM(ctx, execID, wlID, audit.VMRecord{
				VMName:             "virtwork-cpu-0",
				Namespace:          "test-ns",
				Component:          "cpu",
				CPUCores:           4,
				Memory:             "4Gi",
				ContainerDiskImage: "quay.io/test/image:latest",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmID).To(BeNumerically(">", 0))

			// Update VM status to ready
			Expect(auditor.UpdateVMStatus(ctx, vmID, "Running", "ready")).To(Succeed())

			// Verify ready_at is set
			var readyAt sql.NullString
			err = db.QueryRow(`SELECT ready_at FROM vm_details WHERE id = ?`, vmID).Scan(&readyAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(readyAt.Valid).To(BeTrue())

			// Record resource
			resID, err := auditor.RecordResource(ctx, execID, audit.ResourceRecord{
				ResourceType: "Service",
				ResourceName: "virtwork-network",
				Namespace:    "test-ns",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resID).To(BeNumerically(">", 0))

			// Record event
			Expect(auditor.RecordEvent(ctx, execID, audit.EventRecord{
				EventType: "execution_started",
				Message:   "Execution started",
			})).To(Succeed())

			// Update workload status
			Expect(auditor.UpdateWorkloadStatus(ctx, wlID, "created")).To(Succeed())

			// Complete execution
			Expect(auditor.CompleteExecution(ctx, execID, "success", "")).To(Succeed())

			// Verify final status
			err = db.QueryRow(`SELECT status FROM audit_log WHERE id = ?`, execID).Scan(&status)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("success"))
		})

		It("records execution failure with error summary", func() {
			execID, _, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(auditor.CompleteExecution(ctx, execID, "failed", "cluster unreachable")).To(Succeed())

			db := auditor.DB()
			var status string
			var errSummary sql.NullString
			err = db.QueryRow(
				`SELECT status, error_summary FROM audit_log WHERE id = ?`, execID,
			).Scan(&status, &errSummary)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("failed"))
			Expect(errSummary.Valid).To(BeTrue())
			Expect(errSummary.String).To(Equal("cluster unreachable"))
		})
	})

	Describe("cleanup linking", func() {
		It("links cleanup to run via linked_run_ids JSON array", func() {
			cfg := &config.Config{Namespace: "test-ns"}

			// Simulate a run
			_, runID, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Simulate a cleanup
			cleanupID, _, err := auditor.StartExecution(ctx, "cleanup", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Link cleanup to the run
			Expect(auditor.LinkCleanupToRuns(ctx, cleanupID, []string{runID})).To(Succeed())

			// Verify linked_run_ids
			db := auditor.DB()
			var linkedJSON sql.NullString
			err = db.QueryRow(`SELECT linked_run_ids FROM audit_log WHERE id = ?`, cleanupID).Scan(&linkedJSON)
			Expect(err).NotTo(HaveOccurred())
			Expect(linkedJSON.Valid).To(BeTrue())

			var ids []string
			Expect(json.Unmarshal([]byte(linkedJSON.String), &ids)).To(Succeed())
			Expect(ids).To(HaveLen(1))
			Expect(ids[0]).To(Equal(runID))
		})

		It("links cleanup to multiple runs", func() {
			cfg := &config.Config{Namespace: "test-ns"}

			_, runID1, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())
			_, runID2, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			cleanupID, _, err := auditor.StartExecution(ctx, "cleanup", cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(auditor.LinkCleanupToRuns(ctx, cleanupID, []string{runID1, runID2})).To(Succeed())

			db := auditor.DB()
			var linkedJSON string
			err = db.QueryRow(`SELECT linked_run_ids FROM audit_log WHERE id = ?`, cleanupID).Scan(&linkedJSON)
			Expect(err).NotTo(HaveOccurred())

			var ids []string
			Expect(json.Unmarshal([]byte(linkedJSON), &ids)).To(Succeed())
			Expect(ids).To(ConsistOf(runID1, runID2))
		})

		It("records cleanup counts", func() {
			cfg := &config.Config{Namespace: "test-ns"}

			cleanupID, _, err := auditor.StartExecution(ctx, "cleanup", cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(auditor.RecordCleanupCounts(ctx, cleanupID, 5, 2, 10, true)).To(Succeed())

			db := auditor.DB()
			var vms, svcs, secrets int
			var nsDeleted int
			err = db.QueryRow(
				`SELECT vms_deleted, services_deleted, secrets_deleted, namespace_deleted FROM audit_log WHERE id = ?`,
				cleanupID,
			).Scan(&vms, &svcs, &secrets, &nsDeleted)
			Expect(err).NotTo(HaveOccurred())
			Expect(vms).To(Equal(5))
			Expect(svcs).To(Equal(2))
			Expect(secrets).To(Equal(10))
			Expect(nsDeleted).To(Equal(1))
		})
	})

	Describe("VM deletion tracking", func() {
		It("sets deleted_at on VM deletion", func() {
			cfg := &config.Config{Namespace: "test-ns"}
			execID, _, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			wlID, err := auditor.RecordWorkload(ctx, execID, audit.WorkloadRecord{
				WorkloadType: "cpu", Enabled: true, VMCount: 1, CPUCores: 2, Memory: "2Gi",
			})
			Expect(err).NotTo(HaveOccurred())

			vmID, err := auditor.RecordVM(ctx, execID, wlID, audit.VMRecord{
				VMName: "virtwork-cpu-0", Namespace: "test-ns", Component: "cpu",
				CPUCores: 2, Memory: "2Gi", ContainerDiskImage: "img",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(auditor.RecordVMDeletion(ctx, vmID)).To(Succeed())

			db := auditor.DB()
			var status string
			var deletedAt sql.NullString
			err = db.QueryRow(`SELECT status, deleted_at FROM vm_details WHERE id = ?`, vmID).Scan(&status, &deletedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("deleted"))
			Expect(deletedAt.Valid).To(BeTrue())
		})
	})

	Describe("resource deletion tracking", func() {
		It("sets deleted_at on resource deletion", func() {
			cfg := &config.Config{Namespace: "test-ns"}
			execID, _, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			resID, err := auditor.RecordResource(ctx, execID, audit.ResourceRecord{
				ResourceType: "Secret", ResourceName: "test-secret", Namespace: "test-ns",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(auditor.RecordResourceDeletion(ctx, resID)).To(Succeed())

			db := auditor.DB()
			var status string
			var deletedAt sql.NullString
			err = db.QueryRow(`SELECT status, deleted_at FROM resource_details WHERE id = ?`, resID).Scan(&status, &deletedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("deleted"))
			Expect(deletedAt.Valid).To(BeTrue())
		})
	})

	Describe("event recording", func() {
		It("records events with optional VM and workload IDs", func() {
			cfg := &config.Config{Namespace: "test-ns"}
			execID, _, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create real workload and VM so FK constraints are satisfied
			wlID, err := auditor.RecordWorkload(ctx, execID, audit.WorkloadRecord{
				WorkloadType: "cpu", Enabled: true, VMCount: 1, CPUCores: 2, Memory: "2Gi",
			})
			Expect(err).NotTo(HaveOccurred())

			vmID, err := auditor.RecordVM(ctx, execID, wlID, audit.VMRecord{
				VMName: "virtwork-cpu-0", Namespace: "test-ns", Component: "cpu",
				CPUCores: 2, Memory: "2Gi", ContainerDiskImage: "img",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(auditor.RecordEvent(ctx, execID, audit.EventRecord{
				VMID:        &vmID,
				WorkloadID:  &wlID,
				EventType:   "vm_created",
				Message:     "VM created successfully",
				ErrorDetail: "",
			})).To(Succeed())

			db := auditor.DB()
			var eventType, message string
			var evVMID, evWLID sql.NullInt64
			err = db.QueryRow(
				`SELECT event_type, message, vm_id, workload_id FROM events WHERE audit_id = ?`, execID,
			).Scan(&eventType, &message, &evVMID, &evWLID)
			Expect(err).NotTo(HaveOccurred())
			Expect(eventType).To(Equal("vm_created"))
			Expect(message).To(Equal("VM created successfully"))
			Expect(evVMID.Valid).To(BeTrue())
			Expect(evVMID.Int64).To(Equal(vmID))
		})
	})

	Describe("concurrent writes", func() {
		It("handles concurrent event inserts without errors", func() {
			cfg := &config.Config{Namespace: "test-ns"}
			execID, _, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			var wg sync.WaitGroup
			errs := make([]error, 20)
			for i := range 20 {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					errs[idx] = auditor.RecordEvent(ctx, execID, audit.EventRecord{
						EventType: "vm_creating",
						Message:   "concurrent test",
					})
				}(i)
			}
			wg.Wait()

			for _, e := range errs {
				Expect(e).NotTo(HaveOccurred())
			}

			db := auditor.DB()
			var count int
			err = db.QueryRow(`SELECT COUNT(*) FROM events WHERE audit_id = ?`, execID).Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(20))
		})
	})

	Describe("SSH auth tracking", func() {
		It("sets ssh_auth_configured=1 when password is set", func() {
			cfg := &config.Config{
				Namespace:   "test-ns",
				SSHPassword: "secret",
			}
			execID, _, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			db := auditor.DB()
			var sshAuth int
			err = db.QueryRow(`SELECT ssh_auth_configured FROM audit_log WHERE id = ?`, execID).Scan(&sshAuth)
			Expect(err).NotTo(HaveOccurred())
			Expect(sshAuth).To(Equal(1))
		})

		It("sets ssh_auth_configured=0 when no credentials", func() {
			cfg := &config.Config{Namespace: "test-ns"}
			execID, _, err := auditor.StartExecution(ctx, "run", cfg)
			Expect(err).NotTo(HaveOccurred())

			db := auditor.DB()
			var sshAuth int
			err = db.QueryRow(`SELECT ssh_auth_configured FROM audit_log WHERE id = ?`, execID).Scan(&sshAuth)
			Expect(err).NotTo(HaveOccurred())
			Expect(sshAuth).To(Equal(0))
		})
	})
})

var _ = Describe("NoOpAuditor", func() {
	var a audit.NoOpAuditor

	It("does nothing without error", func() {
		ctx := context.Background()
		cfg := &config.Config{Namespace: "test-ns"}

		id, runID, err := a.StartExecution(ctx, "run", cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal(int64(0)))
		Expect(runID).To(BeEmpty())

		Expect(a.CompleteExecution(ctx, 0, "success", "")).To(Succeed())
		Expect(a.LinkCleanupToRuns(ctx, 0, []string{"abc"})).To(Succeed())
		Expect(a.RecordCleanupCounts(ctx, 0, 1, 2, 3, true)).To(Succeed())

		wlID, err := a.RecordWorkload(ctx, 0, audit.WorkloadRecord{})
		Expect(err).NotTo(HaveOccurred())
		Expect(wlID).To(Equal(int64(0)))
		Expect(a.UpdateWorkloadStatus(ctx, 0, "done")).To(Succeed())

		vmID, err := a.RecordVM(ctx, 0, 0, audit.VMRecord{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vmID).To(Equal(int64(0)))
		Expect(a.UpdateVMStatus(ctx, 0, "Running", "ready")).To(Succeed())
		Expect(a.RecordVMDeletion(ctx, 0)).To(Succeed())

		resID, err := a.RecordResource(ctx, 0, audit.ResourceRecord{})
		Expect(err).NotTo(HaveOccurred())
		Expect(resID).To(Equal(int64(0)))
		Expect(a.RecordResourceDeletion(ctx, 0)).To(Succeed())

		Expect(a.RecordEvent(ctx, 0, audit.EventRecord{})).To(Succeed())
		Expect(a.Close()).To(Succeed())
	})
})
