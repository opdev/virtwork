// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("newRootCmd", func() {
	It("creates a command named virtwork", func() {
		cmd := newRootCmd()
		Expect(cmd.Use).To(Equal("virtwork"))
	})

	It("has run, cleanup, and version subcommands", func() {
		cmd := newRootCmd()
		names := make([]string, 0)
		for _, sub := range cmd.Commands() {
			names = append(names, sub.Name())
		}
		Expect(names).To(ContainElements("run", "cleanup", "version"))
	})

	It("has persistent flags for namespace, kubeconfig, config, verbose, audit", func() {
		cmd := newRootCmd()
		Expect(cmd.PersistentFlags().Lookup("namespace")).NotTo(BeNil())
		Expect(cmd.PersistentFlags().Lookup("kubeconfig")).NotTo(BeNil())
		Expect(cmd.PersistentFlags().Lookup("config")).NotTo(BeNil())
		Expect(cmd.PersistentFlags().Lookup("verbose")).NotTo(BeNil())
		Expect(cmd.PersistentFlags().Lookup("audit")).NotTo(BeNil())
		Expect(cmd.PersistentFlags().Lookup("no-audit")).NotTo(BeNil())
		Expect(cmd.PersistentFlags().Lookup("audit-db")).NotTo(BeNil())
	})

	It("silences usage on error", func() {
		cmd := newRootCmd()
		Expect(cmd.SilenceUsage).To(BeTrue())
	})
})

var _ = Describe("newRunCmd", func() {
	It("creates a command named run", func() {
		cmd := newRunCmd()
		Expect(cmd.Use).To(Equal("run"))
	})

	It("has expected flags", func() {
		cmd := newRunCmd()
		Expect(cmd.Flags().Lookup("workloads")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("vm-count")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("cpu-cores")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("memory")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("dry-run")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("no-wait")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("timeout")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("ssh-user")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("ssh-password")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("ssh-key")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("ssh-key-file")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("vm-concurrency")).NotTo(BeNil())
	})

	It("has RunE set to production runE function", func() {
		cmd := newRunCmd()
		Expect(cmd.RunE).NotTo(BeNil())
	})
})

var _ = Describe("newCleanupCmd", func() {
	It("creates a command named cleanup", func() {
		cmd := newCleanupCmd()
		Expect(cmd.Use).To(Equal("cleanup"))
	})

	It("has expected flags", func() {
		cmd := newCleanupCmd()
		Expect(cmd.Flags().Lookup("delete-namespace")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("run-id")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("dry-run")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("yes")).NotTo(BeNil())
	})

	It("has -y shorthand for --yes", func() {
		cmd := newCleanupCmd()
		flag := cmd.Flags().Lookup("yes")
		Expect(flag).NotTo(BeNil())
		Expect(flag.Shorthand).To(Equal("y"))
	})

	It("has RunE set to production cleanupE function", func() {
		cmd := newCleanupCmd()
		Expect(cmd.RunE).NotTo(BeNil())
	})
})
