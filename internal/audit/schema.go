// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package audit

// schemaSQL contains the DDL for the audit database.
// All timestamps are stored as ISO 8601 TEXT for SQLite compatibility
// while remaining PostgreSQL-compatible (TEXT maps to TEXT/TIMESTAMP,
// linked_run_ids maps to JSONB).
const schemaSQL = `
CREATE TABLE IF NOT EXISTS audit_log (
	id                    INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id                TEXT    NOT NULL UNIQUE,
	linked_run_ids        TEXT,
	command               TEXT    NOT NULL,
	status                TEXT    NOT NULL DEFAULT 'in_progress',
	kubeconfig_path       TEXT,
	cluster_context       TEXT,
	namespace             TEXT    NOT NULL,
	container_disk_image  TEXT,
	default_cpu_cores     INTEGER,
	default_memory        TEXT,
	data_disk_size        TEXT,
	workloads_csv         TEXT,
	total_vm_count        INTEGER,
	total_workload_count  INTEGER,
	dry_run               INTEGER NOT NULL DEFAULT 0,
	ssh_auth_configured   INTEGER NOT NULL DEFAULT 0,
	cleanup_mode          TEXT,
	wait_for_ready        INTEGER NOT NULL DEFAULT 1,
	ready_timeout_seconds INTEGER,
	vms_deleted           INTEGER,
	services_deleted      INTEGER,
	secrets_deleted       INTEGER,
	dvs_deleted           INTEGER,
	pvcs_deleted          INTEGER,
	namespace_deleted     INTEGER,
	started_at            TEXT    NOT NULL,
	completed_at          TEXT,
	error_summary         TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_log_started_at ON audit_log(started_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_namespace  ON audit_log(namespace);
CREATE INDEX IF NOT EXISTS idx_audit_log_status     ON audit_log(status);
CREATE INDEX IF NOT EXISTS idx_audit_log_run_id     ON audit_log(run_id);

CREATE TABLE IF NOT EXISTS workload_details (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	audit_id         INTEGER NOT NULL REFERENCES audit_log(id),
	workload_type    TEXT    NOT NULL,
	enabled          INTEGER NOT NULL DEFAULT 1,
	vm_count         INTEGER NOT NULL,
	cpu_cores        INTEGER NOT NULL,
	memory           TEXT    NOT NULL,
	has_data_disk    INTEGER NOT NULL DEFAULT 0,
	data_disk_size   TEXT,
	requires_service INTEGER NOT NULL DEFAULT 0,
	status           TEXT    NOT NULL DEFAULT 'pending'
);

CREATE INDEX IF NOT EXISTS idx_workload_details_audit_id ON workload_details(audit_id);
CREATE INDEX IF NOT EXISTS idx_workload_details_type     ON workload_details(workload_type);

CREATE TABLE IF NOT EXISTS vm_details (
	id                   INTEGER PRIMARY KEY AUTOINCREMENT,
	audit_id             INTEGER NOT NULL REFERENCES audit_log(id),
	workload_id          INTEGER REFERENCES workload_details(id),
	vm_name              TEXT    NOT NULL,
	namespace            TEXT    NOT NULL,
	component            TEXT    NOT NULL,
	role                 TEXT,
	cpu_cores            INTEGER NOT NULL,
	memory               TEXT    NOT NULL,
	container_disk_image TEXT    NOT NULL,
	has_data_disk        INTEGER NOT NULL DEFAULT 0,
	data_disk_size       TEXT,
	phase                TEXT,
	status               TEXT    NOT NULL DEFAULT 'planned',
	created_at           TEXT,
	ready_at             TEXT,
	deleted_at           TEXT
);

CREATE INDEX IF NOT EXISTS idx_vm_details_audit_id    ON vm_details(audit_id);
CREATE INDEX IF NOT EXISTS idx_vm_details_workload_id ON vm_details(workload_id);
CREATE INDEX IF NOT EXISTS idx_vm_details_vm_name     ON vm_details(vm_name);

CREATE TABLE IF NOT EXISTS resource_details (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	audit_id      INTEGER NOT NULL REFERENCES audit_log(id),
	resource_type TEXT    NOT NULL,
	resource_name TEXT    NOT NULL,
	namespace     TEXT    NOT NULL,
	status        TEXT    NOT NULL DEFAULT 'created',
	created_at    TEXT,
	deleted_at    TEXT
);

CREATE INDEX IF NOT EXISTS idx_resource_details_audit_id ON resource_details(audit_id);
CREATE INDEX IF NOT EXISTS idx_resource_details_type     ON resource_details(resource_type);

CREATE TABLE IF NOT EXISTS events (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	audit_id     INTEGER NOT NULL REFERENCES audit_log(id),
	vm_id        INTEGER REFERENCES vm_details(id),
	workload_id  INTEGER REFERENCES workload_details(id),
	event_type   TEXT    NOT NULL,
	message      TEXT,
	error_detail TEXT,
	occurred_at  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_audit_id   ON events(audit_id);
CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_occurred_at ON events(occurred_at);
`
