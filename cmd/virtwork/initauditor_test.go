// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/config"
)

var _ = Describe("initAuditor", func() {
	var cmd *cobra.Command

	BeforeEach(func() {
		cmd = &cobra.Command{}
		cmd.Flags().Bool("no-audit", false, "")
		cmd.Flags().String("audit-db", "", "")
	})

	It("returns NoOpAuditor when --no-audit is set", func() {
		Expect(cmd.Flags().Set("no-audit", "true")).To(Succeed())

		cfg := &config.Config{AuditEnabled: true}
		auditor, err := initAuditor(cmd, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(auditor).To(BeAssignableToTypeOf(audit.NoOpAuditor{}))
	})

	It("returns NoOpAuditor when cfg.AuditEnabled is false", func() {
		cfg := &config.Config{AuditEnabled: false}
		auditor, err := initAuditor(cmd, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(auditor).To(BeAssignableToTypeOf(audit.NoOpAuditor{}))
	})

	It("returns SQLiteAuditor with default db path from config", func() {
		tmpDir := GinkgoT().TempDir()
		dbPath := filepath.Join(tmpDir, "audit.db")

		cfg := &config.Config{AuditEnabled: true, AuditDBPath: dbPath}
		auditor, err := initAuditor(cmd, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(auditor).NotTo(BeNil())
		Expect(auditor.Close()).To(Succeed())

		_, statErr := os.Stat(dbPath)
		Expect(statErr).NotTo(HaveOccurred())
	})

	It("uses --audit-db flag path when set", func() {
		tmpDir := GinkgoT().TempDir()
		flagPath := filepath.Join(tmpDir, "override.db")
		Expect(cmd.Flags().Set("audit-db", flagPath)).To(Succeed())

		cfg := &config.Config{AuditEnabled: true, AuditDBPath: "/should/not/use/this"}
		auditor, err := initAuditor(cmd, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(auditor).NotTo(BeNil())
		Expect(auditor.Close()).To(Succeed())

		_, statErr := os.Stat(flagPath)
		Expect(statErr).NotTo(HaveOccurred())
	})

	It("returns error for invalid db path", func() {
		cfg := &config.Config{
			AuditEnabled: true,
			AuditDBPath:  "/nonexistent/deeply/nested/path/audit.db",
		}
		_, err := initAuditor(cmd, cfg)
		Expect(err).To(HaveOccurred())
	})
})
