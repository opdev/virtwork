// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"database/sql"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/audit"
)

var _ = Describe("Schema migration", func() {
	Describe("fresh database", func() {
		It("creates schema_version table with version 1", func() {
			auditor, err := audit.NewSQLiteAuditor(":memory:")
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(auditor.Close()).To(Succeed()) }()

			db := auditor.DB()
			var version int
			err = db.QueryRow(`SELECT version FROM schema_version`).Scan(&version)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal(1))
		})

		It("creates all 5 data tables", func() {
			auditor, err := audit.NewSQLiteAuditor(":memory:")
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(auditor.Close()).To(Succeed()) }()

			db := auditor.DB()
			tables := []string{
				"audit_log",
				"workload_details",
				"vm_details",
				"resource_details",
				"events",
			}
			for _, table := range tables {
				var name string
				err := db.QueryRow(
					`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
				).Scan(&name)
				Expect(err).NotTo(HaveOccurred(), "table %s should exist", table)
			}
		})
	})

	Describe("pre-migration database", func() {
		It("upgrades to version 1 without error", func() {
			db, err := sql.Open(
				"sqlite",
				"file::memory:?mode=memory&cache=shared&_pragma=foreign_keys(on)",
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(audit.SchemaSQL)
			Expect(err).NotTo(HaveOccurred())

			var count int
			err = db.QueryRow(
				`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`,
			).Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0), "schema_version should not exist before migration")

			Expect(db.Close()).To(Succeed())

			auditor, err := audit.NewSQLiteAuditor(":memory:")
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(auditor.Close()).To(Succeed()) }()

			adb := auditor.DB()
			var version int
			err = adb.QueryRow(`SELECT version FROM schema_version`).Scan(&version)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal(1))
		})
	})

	Describe("already-migrated database", func() {
		It("preserves version and data on reopen", func() {
			dir := GinkgoT().TempDir()
			dbPath := dir + "/test.db"

			auditor, err := audit.NewSQLiteAuditor(dbPath)
			Expect(err).NotTo(HaveOccurred())

			db := auditor.DB()
			var versionBefore int
			Expect(
				db.QueryRow(`SELECT version FROM schema_version`).Scan(&versionBefore),
			).To(Succeed())
			Expect(versionBefore).To(Equal(1))

			_, err = db.Exec(
				`INSERT INTO audit_log (run_id, command, status, namespace, started_at) VALUES ('test-run', 'run', 'success', 'ns', '2026-01-01T00:00:00Z')`,
			)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(
				`INSERT INTO events (audit_id, event_type, occurred_at) VALUES (1, 'test', '2026-01-01T00:00:00Z')`,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(auditor.Close()).To(Succeed())

			auditor2, err := audit.NewSQLiteAuditor(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(auditor2.Close()).To(Succeed()) }()

			db2 := auditor2.DB()
			var versionAfter int
			Expect(
				db2.QueryRow(`SELECT version FROM schema_version`).Scan(&versionAfter),
			).To(Succeed())
			Expect(versionAfter).To(Equal(1))

			var count int
			Expect(
				db2.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type = 'test'`).Scan(&count),
			).To(Succeed())
			Expect(count).To(Equal(1))
		})
	})

	Describe("version table structure", func() {
		It("has exactly one row", func() {
			auditor, err := audit.NewSQLiteAuditor(":memory:")
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(auditor.Close()).To(Succeed()) }()

			db := auditor.DB()
			var count int
			Expect(db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))
		})
	})
})
