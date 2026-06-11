// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/opdev/virtwork/internal/config"
)

// Auditor defines the contract for recording execution audit data.
type Auditor interface {
	// StartExecution creates an audit_log row and returns its ID and generated run UUID.
	StartExecution(
		ctx context.Context,
		cmd string,
		cfg *config.Config,
	) (executionID int64, runID string, err error)
	// CompleteExecution finalises the audit_log row with status and optional error summary.
	CompleteExecution(ctx context.Context, id int64, status string, errSummary string) error
	// LinkCleanupToRuns sets linked_run_ids on a cleanup audit_log row.
	LinkCleanupToRuns(ctx context.Context, cleanupID int64, runIDs []string) error
	// RecordCleanupCounts updates cleanup-specific counters on the audit_log row.
	RecordCleanupCounts(
		ctx context.Context,
		id int64,
		vmsDeleted, servicesDeleted, secretsDeleted, dvsDeleted, pvcsDeleted int,
		namespaceDeleted bool,
	) error

	// RecordWorkload inserts a workload_details row.
	RecordWorkload(
		ctx context.Context,
		executionID int64,
		w WorkloadRecord,
	) (workloadID int64, err error)
	// UpdateWorkloadStatus updates the status and completed_at of a workload_details row.
	UpdateWorkloadStatus(ctx context.Context, id int64, status string) error

	// RecordVM inserts a vm_details row.
	RecordVM(
		ctx context.Context,
		executionID int64,
		workloadID int64,
		v VMRecord,
	) (vmID int64, err error)
	// UpdateVMStatus updates phase and status on a vm_details row.
	UpdateVMStatus(ctx context.Context, id int64, phase string, status string) error
	// RecordVMDeletion sets deleted_at and status='deleted' on a vm_details row.
	RecordVMDeletion(ctx context.Context, id int64) error

	// RecordResource inserts a resource_details row.
	RecordResource(
		ctx context.Context,
		executionID int64,
		r ResourceRecord,
	) (resourceID int64, err error)
	// RecordResourceDeletion sets deleted_at and status='deleted' on a resource_details row.
	RecordResourceDeletion(ctx context.Context, id int64) error

	// MarkVMsDeletedByName marks vm_details rows as deleted by VM name and namespace,
	// scoped to audit records from the given run IDs.
	MarkVMsDeletedByName(
		ctx context.Context,
		vmNames []string,
		namespace string,
		runIDs []string,
	) error
	// MarkResourcesDeletedByName marks resource_details rows as deleted by type, name,
	// and namespace, scoped to audit records from the given run IDs.
	MarkResourcesDeletedByName(
		ctx context.Context,
		resourceType string,
		names []string,
		namespace string,
		runIDs []string,
	) error

	// RecordEvent inserts an events row.
	RecordEvent(ctx context.Context, executionID int64, e EventRecord) error

	// Close releases database resources.
	Close() error
}

// SQLiteAuditor implements Auditor backed by a SQLite database.
type SQLiteAuditor struct {
	db *sql.DB
}

// NewSQLiteAuditor opens (or creates) the SQLite database at dbPath and ensures
// the schema is applied.
func NewSQLiteAuditor(dbPath string) (*SQLiteAuditor, error) {
	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("creating audit db directory: %w", err)
		}
	}

	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"
	if dbPath == ":memory:" {
		dsn = "file::memory:?mode=memory&cache=shared&_pragma=foreign_keys(on)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening audit db: %w", err)
	}

	if err := migrateDB(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating audit schema: %w", err)
	}

	return &SQLiteAuditor{db: db}, nil
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (a *SQLiteAuditor) StartExecution(
	ctx context.Context,
	cmd string,
	cfg *config.Config,
) (int64, string, error) {
	runID := uuid.New().String()

	workloadNames := make([]string, 0, len(cfg.Workloads))
	for name := range cfg.Workloads {
		workloadNames = append(workloadNames, name)
	}
	workloadsCSV := strings.Join(workloadNames, ",")

	sshConfigured := cfg.SSHPassword != "" || len(cfg.SSHAuthorizedKeys) > 0

	res, err := a.db.ExecContext(
		ctx, `
		INSERT INTO audit_log (
			run_id, command, status, kubeconfig_path, cluster_context, namespace,
			container_disk_image, default_cpu_cores, default_memory, data_disk_size,
			workloads_csv, dry_run, ssh_auth_configured, cleanup_mode,
			wait_for_ready, ready_timeout_seconds, started_at
		) VALUES (?, ?, 'in_progress', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, cmd, nullIfEmpty(cfg.KubeconfigPath), nullIfEmpty(cfg.ClusterContext), cfg.Namespace,
		cfg.ContainerDiskImage, cfg.CPUCores, cfg.Memory, cfg.DataDiskSize,
		workloadsCSV, boolToInt(cfg.DryRun), boolToInt(sshConfigured), nullIfEmpty(cfg.CleanupMode),
		boolToInt(cfg.WaitForReady), cfg.ReadyTimeoutSeconds, now(),
	)
	if err != nil {
		return 0, "", fmt.Errorf("inserting audit_log: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, "", fmt.Errorf("getting audit_log id: %w", err)
	}
	return id, runID, nil
}

func (a *SQLiteAuditor) CompleteExecution(
	ctx context.Context,
	id int64,
	status string,
	errSummary string,
) error {
	var errPtr *string
	if errSummary != "" {
		errPtr = &errSummary
	}
	_, err := a.db.ExecContext(ctx,
		`UPDATE audit_log SET status = ?, completed_at = ?, error_summary = ? WHERE id = ?`,
		status, now(), errPtr, id)
	return err
}

func (a *SQLiteAuditor) LinkCleanupToRuns(
	ctx context.Context,
	cleanupID int64,
	runIDs []string,
) error {
	data, err := json.Marshal(runIDs)
	if err != nil {
		return fmt.Errorf("marshaling run IDs: %w", err)
	}
	_, err = a.db.ExecContext(ctx,
		`UPDATE audit_log SET linked_run_ids = ? WHERE id = ?`,
		string(data), cleanupID)
	return err
}

func (a *SQLiteAuditor) RecordCleanupCounts(
	ctx context.Context,
	id int64,
	vmsDeleted, servicesDeleted, secretsDeleted, dvsDeleted, pvcsDeleted int,
	namespaceDeleted bool,
) error {
	_, err := a.db.ExecContext(
		ctx,
		`UPDATE audit_log SET vms_deleted = ?, services_deleted = ?, secrets_deleted = ?, dvs_deleted = ?, pvcs_deleted = ?, namespace_deleted = ? WHERE id = ?`,
		vmsDeleted,
		servicesDeleted,
		secretsDeleted,
		dvsDeleted,
		pvcsDeleted,
		boolToInt(namespaceDeleted),
		id,
	)
	return err
}

func (a *SQLiteAuditor) RecordWorkload(
	ctx context.Context,
	executionID int64,
	w WorkloadRecord,
) (int64, error) {
	res, err := a.db.ExecContext(
		ctx, `
		INSERT INTO workload_details (
			audit_id, workload_type, enabled, vm_count, cpu_cores, memory,
			has_data_disk, data_disk_size, requires_service, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		executionID, w.WorkloadType, boolToInt(w.Enabled), w.VMCount, w.CPUCores, w.Memory,
		boolToInt(w.HasDataDisk), nullIfEmpty(w.DataDiskSize), boolToInt(w.RequiresService),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting workload_details: %w", err)
	}
	return res.LastInsertId()
}

func (a *SQLiteAuditor) UpdateWorkloadStatus(ctx context.Context, id int64, status string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE workload_details SET status = ? WHERE id = ?`,
		status, id)
	return err
}

func (a *SQLiteAuditor) RecordVM(
	ctx context.Context,
	executionID int64,
	workloadID int64,
	v VMRecord,
) (int64, error) {
	res, err := a.db.ExecContext(
		ctx, `
		INSERT INTO vm_details (
			audit_id, workload_id, vm_name, namespace, component, role,
			cpu_cores, memory, container_disk_image, has_data_disk, data_disk_size,
			status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'created', ?)`,
		executionID, workloadID, v.VMName, v.Namespace, v.Component, nullIfEmpty(v.Role),
		v.CPUCores, v.Memory, v.ContainerDiskImage, boolToInt(v.HasDataDisk),
		nullIfEmpty(v.DataDiskSize), now(),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting vm_details: %w", err)
	}
	return res.LastInsertId()
}

func (a *SQLiteAuditor) UpdateVMStatus(
	ctx context.Context,
	id int64,
	phase string,
	status string,
) error {
	if status == "ready" {
		_, err := a.db.ExecContext(ctx,
			`UPDATE vm_details SET phase = ?, status = ?, ready_at = ? WHERE id = ?`,
			phase, status, now(), id)
		return err
	}
	_, err := a.db.ExecContext(ctx,
		`UPDATE vm_details SET phase = ?, status = ? WHERE id = ?`,
		phase, status, id)
	return err
}

func (a *SQLiteAuditor) RecordVMDeletion(ctx context.Context, id int64) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE vm_details SET status = 'deleted', deleted_at = ? WHERE id = ?`,
		now(), id)
	return err
}

func (a *SQLiteAuditor) RecordResource(
	ctx context.Context,
	executionID int64,
	r ResourceRecord,
) (int64, error) {
	res, err := a.db.ExecContext(
		ctx, `
		INSERT INTO resource_details (audit_id, resource_type, resource_name, namespace, status, created_at)
		VALUES (?, ?, ?, ?, 'created', ?)`,
		executionID, r.ResourceType, r.ResourceName, r.Namespace, now(),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting resource_details: %w", err)
	}
	return res.LastInsertId()
}

func (a *SQLiteAuditor) RecordResourceDeletion(ctx context.Context, id int64) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE resource_details SET status = 'deleted', deleted_at = ? WHERE id = ?`,
		now(), id)
	return err
}

func (a *SQLiteAuditor) MarkVMsDeletedByName(
	ctx context.Context,
	vmNames []string,
	namespace string,
	runIDs []string,
) error {
	if len(vmNames) == 0 || len(runIDs) == 0 {
		return nil
	}
	query, args := buildMarkDeletedQuery(
		"vm_details", "vm_name", "",
		vmNames, namespace, runIDs,
	)
	_, err := a.db.ExecContext(ctx, query, args...)
	return err
}

func (a *SQLiteAuditor) MarkResourcesDeletedByName(
	ctx context.Context,
	resourceType string,
	names []string,
	namespace string,
	runIDs []string,
) error {
	if len(names) == 0 || len(runIDs) == 0 {
		return nil
	}
	query, args := buildMarkDeletedQuery(
		"resource_details", "resource_name", resourceType,
		names, namespace, runIDs,
	)
	_, err := a.db.ExecContext(ctx, query, args...)
	return err
}

func buildMarkDeletedQuery(
	table, nameCol, resourceType string,
	names []string,
	namespace string,
	runIDs []string,
) (string, []interface{}) {
	args := []interface{}{now()}

	namePlaceholders := make([]string, len(names))
	for i, n := range names {
		namePlaceholders[i] = "?"
		args = append(args, n)
	}
	args = append(args, namespace)

	runPlaceholders := make([]string, len(runIDs))
	for i, r := range runIDs {
		runPlaceholders[i] = "?"
		args = append(args, r)
	}

	query := fmt.Sprintf(
		`UPDATE %s SET status = 'deleted', deleted_at = ?
		 WHERE %s IN (%s) AND namespace = ?
		   AND audit_id IN (SELECT id FROM audit_log WHERE run_id IN (%s))`,
		table, nameCol,
		strings.Join(namePlaceholders, ","),
		strings.Join(runPlaceholders, ","),
	)

	if resourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, resourceType)
	}

	return query, args
}

func (a *SQLiteAuditor) RecordEvent(ctx context.Context, executionID int64, e EventRecord) error {
	_, err := a.db.ExecContext(
		ctx, `
		INSERT INTO events (audit_id, vm_id, workload_id, event_type, message, error_detail, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		executionID, e.VMID, e.WorkloadID, e.EventType,
		nullIfEmpty(e.Message), nullIfEmpty(e.ErrorDetail), now(),
	)
	return err
}

// DB returns the underlying sql.DB for testing purposes.
func (a *SQLiteAuditor) DB() *sql.DB {
	return a.db
}

func (a *SQLiteAuditor) Close() error {
	return a.db.Close()
}

// NoOpAuditor is an Auditor that does nothing, used when auditing is disabled.
type NoOpAuditor struct{}

func (NoOpAuditor) StartExecution(
	_ context.Context,
	_ string,
	_ *config.Config,
) (int64, string, error) {
	return 0, "", nil
}

func (NoOpAuditor) CompleteExecution(_ context.Context, _ int64, _ string, _ string) error {
	return nil
}

func (NoOpAuditor) LinkCleanupToRuns(_ context.Context, _ int64, _ []string) error { return nil }

func (NoOpAuditor) RecordCleanupCounts(
	_ context.Context,
	_ int64,
	_, _, _, _, _ int,
	_ bool,
) error {
	return nil
}

func (NoOpAuditor) RecordWorkload(_ context.Context, _ int64, _ WorkloadRecord) (int64, error) {
	return 0, nil
}
func (NoOpAuditor) UpdateWorkloadStatus(_ context.Context, _ int64, _ string) error { return nil }
func (NoOpAuditor) RecordVM(_ context.Context, _ int64, _ int64, _ VMRecord) (int64, error) {
	return 0, nil
}

func (NoOpAuditor) UpdateVMStatus(
	_ context.Context,
	_ int64,
	_ string,
	_ string,
) error {
	return nil
}

func (NoOpAuditor) RecordVMDeletion(_ context.Context, _ int64) error { return nil }

func (NoOpAuditor) RecordResource(_ context.Context, _ int64, _ ResourceRecord) (int64, error) {
	return 0, nil
}
func (NoOpAuditor) RecordResourceDeletion(_ context.Context, _ int64) error { return nil }
func (NoOpAuditor) MarkVMsDeletedByName(_ context.Context, _ []string, _ string, _ []string) error {
	return nil
}

func (NoOpAuditor) MarkResourcesDeletedByName(
	_ context.Context,
	_ string,
	_ []string,
	_ string,
	_ []string,
) error {
	return nil
}

func (NoOpAuditor) RecordEvent(_ context.Context, _ int64, _ EventRecord) error {
	return nil
}
func (NoOpAuditor) Close() error { return nil }

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
