// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package orchestrator_test

import (
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/logging"
	"github.com/opdev/virtwork/internal/orchestrator"
)

var _ = Describe("CleanupOrchestrator", func() {
	var (
		ctx     context.Context
		buf     *bytes.Buffer
		cfg     *config.Config
		auditor audit.Auditor
	)

	BeforeEach(func() {
		ctx = context.Background()
		buf = &bytes.Buffer{}
		cfg = defaultConfig()
		auditor = audit.NoOpAuditor{}
	})

	Describe("Preview", func() {
		It("should return zero counts when no resources exist", func() {
			c := newFakeClient()
			logger := logging.NewLogger(buf, false)
			co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

			preview, err := co.Preview(ctx, 0, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(preview.TotalCount).To(Equal(0))
		})

		It("should count managed VMs", func() {
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "virtwork-cpu-0",
					Namespace: constants.DefaultNamespace,
					Labels: map[string]string{
						constants.LabelManagedBy: constants.ManagedByValue,
					},
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.DefaultNamespace,
				},
			}
			c := newFakeClient(ns, vm)
			logger := logging.NewLogger(buf, false)
			co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

			preview, err := co.Preview(ctx, 0, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(preview.VMCount).To(Equal(1))
			Expect(preview.TotalCount).To(BeNumerically(">=", 1))
		})

		It("should filter by run-id when provided", func() {
			vm1 := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "virtwork-cpu-0",
					Namespace: constants.DefaultNamespace,
					Labels: map[string]string{
						constants.LabelManagedBy: constants.ManagedByValue,
						constants.LabelRunID:     "run-1",
					},
				},
			}
			vm2 := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "virtwork-cpu-1",
					Namespace: constants.DefaultNamespace,
					Labels: map[string]string{
						constants.LabelManagedBy: constants.ManagedByValue,
						constants.LabelRunID:     "run-2",
					},
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.DefaultNamespace,
				},
			}
			c := newFakeClient(ns, vm1, vm2)
			logger := logging.NewLogger(buf, false)
			co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

			preview, err := co.Preview(ctx, 0, "run-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(preview.VMCount).To(Equal(1))
		})
	})

	Describe("Execute", func() {
		It("should delete managed VMs", func() {
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "virtwork-cpu-0",
					Namespace: constants.DefaultNamespace,
					Labels: map[string]string{
						constants.LabelManagedBy: constants.ManagedByValue,
					},
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.DefaultNamespace,
				},
			}
			c := newFakeClient(ns, vm)
			logger := logging.NewLogger(buf, false)
			co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

			result, err := co.Execute(ctx, 0, false, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.VMsDeleted).To(Equal(1))

			// Verify VM is gone
			vmObj := &kubevirtv1.VirtualMachine{}
			err = c.Get(ctx, client.ObjectKey{
				Name:      "virtwork-cpu-0",
				Namespace: constants.DefaultNamespace,
			}, vmObj)
			Expect(err).To(HaveOccurred())
		})

		It("should delete managed secrets", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "virtwork-cpu-0-cloudinit",
					Namespace: constants.DefaultNamespace,
					Labels: map[string]string{
						constants.LabelManagedBy: constants.ManagedByValue,
					},
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.DefaultNamespace,
				},
			}
			c := newFakeClient(ns, secret)
			logger := logging.NewLogger(buf, false)
			co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

			result, err := co.Execute(ctx, 0, false, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SecretsDeleted).To(Equal(1))
		})

		It("should record cleanup counts in audit", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.DefaultNamespace,
				},
			}
			c := newFakeClient(ns)
			logger := logging.NewLogger(buf, false)
			co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

			result, err := co.Execute(ctx, 0, false, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("should mark VMs and resources as deleted in audit", func() {
			sqlAuditor, err := audit.NewSQLiteAuditor(":memory:")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sqlAuditor.Close() }()

			runCfg := &config.Config{Namespace: constants.DefaultNamespace}
			runExecID, runID, err := sqlAuditor.StartExecution(ctx, "run", runCfg)
			Expect(err).NotTo(HaveOccurred())

			wlID, err := sqlAuditor.RecordWorkload(ctx, runExecID, audit.WorkloadRecord{
				WorkloadType: "cpu", Enabled: true, VMCount: 1, CPUCores: 2, Memory: "2Gi",
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = sqlAuditor.RecordVM(ctx, runExecID, wlID, audit.VMRecord{
				VMName: "virtwork-cpu-0", Namespace: constants.DefaultNamespace, Component: "cpu",
				CPUCores: 2, Memory: "2Gi", ContainerDiskImage: "img",
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = sqlAuditor.RecordResource(ctx, runExecID, audit.ResourceRecord{
				ResourceType: "Secret", ResourceName: "virtwork-cpu-0-cloudinit",
				Namespace: constants.DefaultNamespace,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(sqlAuditor.CompleteExecution(ctx, runExecID, "success", "")).To(Succeed())

			// Create matching K8s resources with the run-id label
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "virtwork-cpu-0",
					Namespace: constants.DefaultNamespace,
					Labels: map[string]string{
						constants.LabelManagedBy: constants.ManagedByValue,
						constants.LabelRunID:     runID,
					},
				},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "virtwork-cpu-0-cloudinit",
					Namespace: constants.DefaultNamespace,
					Labels: map[string]string{
						constants.LabelManagedBy: constants.ManagedByValue,
						constants.LabelRunID:     runID,
					},
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: constants.DefaultNamespace},
			}
			c := newFakeClient(ns, vm, secret)
			logger := logging.NewLogger(buf, false)
			co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, sqlAuditor, buf)

			cleanupExecID, _, err := sqlAuditor.StartExecution(ctx, "cleanup", runCfg)
			Expect(err).NotTo(HaveOccurred())

			result, err := co.Execute(ctx, cleanupExecID, false, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.VMsDeleted).To(Equal(1))
			Expect(result.SecretsDeleted).To(Equal(1))

			db := sqlAuditor.DB()
			var vmStatus string
			err = db.QueryRow(
				`SELECT status FROM vm_details WHERE vm_name = ? AND namespace = ?`,
				"virtwork-cpu-0", constants.DefaultNamespace,
			).Scan(&vmStatus)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmStatus).To(Equal("deleted"))

			var resStatus string
			err = db.QueryRow(
				`SELECT status FROM resource_details WHERE resource_name = ? AND namespace = ?`,
				"virtwork-cpu-0-cloudinit", constants.DefaultNamespace,
			).Scan(&resStatus)
			Expect(err).NotTo(HaveOccurred())
			Expect(resStatus).To(Equal("deleted"))
		})
	})
})
