// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

// newRootCmd builds a fresh command tree for testing. This mirrors the production
// command tree but allows us to capture output and inject dependencies.
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "virtwork",
		Short: "Create VMs on OpenShift with continuous workloads",
	}
	config.BindPersistentFlags(rootCmd)

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Create VMs and start workloads",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	config.BindRunFlags(runCmd, workloads.AllWorkloadNames())

	cleanupCmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete all managed resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	config.BindCleanupFlags(cleanupCmd)

	validateCmd := &cobra.Command{
		Use:   "validate [entry-names...]",
		Short: "Validate catalog entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	config.BindValidateFlags(validateCmd)

	rootCmd.AddCommand(runCmd, cleanupCmd, validateCmd)
	return rootCmd
}

var _ = Describe("Run command flags", func() {
	var rootCmd *cobra.Command

	BeforeEach(func() {
		rootCmd = newRootCmd()
	})

	It("should have default namespace", func() {
		rootCmd.SetArgs([]string{"run"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetString("namespace")
		Expect(err).NotTo(HaveOccurred())
		// Default is empty string since Viper provides defaults
		Expect(val).To(Equal(""))
	})

	It("should accept custom namespace", func() {
		rootCmd.SetArgs([]string{"run", "--namespace", "test-ns"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetString("namespace")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("test-ns"))
	})

	It("should accept workloads CSV", func() {
		rootCmd.SetArgs([]string{"run", "--workloads", "cpu,memory"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetStringSlice("workloads")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal([]string{"cpu", "memory"}))
	})

	It("should accept vm-count", func() {
		rootCmd.SetArgs([]string{"run", "--vm-count", "3"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetInt("vm-count")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal(3))
	})

	It("should accept cpu-cores", func() {
		rootCmd.SetArgs([]string{"run", "--cpu-cores", "4"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetInt("cpu-cores")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal(4))
	})

	It("should accept memory", func() {
		rootCmd.SetArgs([]string{"run", "--memory", "4Gi"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetString("memory")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("4Gi"))
	})

	It("should accept disk-size", func() {
		rootCmd.SetArgs([]string{"run", "--disk-size", "20Gi"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetString("disk-size")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("20Gi"))
	})

	It("should accept dry-run flag", func() {
		rootCmd.SetArgs([]string{"run", "--dry-run"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetBool("dry-run")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeTrue())
	})

	It("should accept no-wait flag", func() {
		rootCmd.SetArgs([]string{"run", "--no-wait"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetBool("no-wait")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeTrue())
	})

	It("should accept verbose flag", func() {
		rootCmd.SetArgs([]string{"run", "--verbose"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetBool("verbose")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeTrue())
	})

	It("should accept timeout", func() {
		rootCmd.SetArgs([]string{"run", "--timeout", "300"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetInt("timeout")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal(300))
	})

	It("should accept config file", func() {
		rootCmd.SetArgs([]string{"run", "--config", "/tmp/config.yaml"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetString("config")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("/tmp/config.yaml"))
	})

	It("should accept ssh-user flag", func() {
		rootCmd.SetArgs([]string{"run", "--ssh-user", "admin"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetString("ssh-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("admin"))
	})

	It("should accept ssh-password flag", func() {
		rootCmd.SetArgs([]string{"run", "--ssh-password", "secret"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetString("ssh-password")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("secret"))
	})

	It("should accept ssh-key flag (repeatable)", func() {
		rootCmd.SetArgs([]string{"run", "--ssh-key", "ssh-rsa AAAA key1", "--ssh-key", "ssh-ed25519 BBBB key2"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetStringSlice("ssh-key")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(HaveLen(2))
	})

	It("should accept ssh-key-file flag (repeatable)", func() {
		rootCmd.SetArgs([]string{"run", "--ssh-key-file", "/home/user/.ssh/id_rsa.pub"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetStringSlice("ssh-key-file")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(HaveLen(1))
		Expect(val[0]).To(Equal("/home/user/.ssh/id_rsa.pub"))
	})

	It("should accept vm-concurrency flag", func() {
		rootCmd.SetArgs([]string{"run", "--vm-concurrency", "5"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetInt("vm-concurrency")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal(5))
	})

	It("should default vm-concurrency to 10", func() {
		rootCmd.SetArgs([]string{"run"})
		Expect(rootCmd.Execute()).To(Succeed())

		runCmd, _, _ := rootCmd.Find([]string{"run"})
		val, err := runCmd.Flags().GetInt("vm-concurrency")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal(10))
	})
})

var _ = Describe("Cleanup command flags", func() {
	var rootCmd *cobra.Command

	BeforeEach(func() {
		rootCmd = newRootCmd()
	})

	It("should accept delete-namespace flag", func() {
		rootCmd.SetArgs([]string{"cleanup", "--delete-namespace"})
		Expect(rootCmd.Execute()).To(Succeed())

		cleanupCmd, _, _ := rootCmd.Find([]string{"cleanup"})
		val, err := cleanupCmd.Flags().GetBool("delete-namespace")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeTrue())
	})

	It("should default delete-namespace to false", func() {
		rootCmd.SetArgs([]string{"cleanup"})
		Expect(rootCmd.Execute()).To(Succeed())

		cleanupCmd, _, _ := rootCmd.Find([]string{"cleanup"})
		val, err := cleanupCmd.Flags().GetBool("delete-namespace")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeFalse())
	})

	It("should accept --yes flag", func() {
		rootCmd.SetArgs([]string{"cleanup", "--yes"})
		Expect(rootCmd.Execute()).To(Succeed())

		cleanupCmd, _, _ := rootCmd.Find([]string{"cleanup"})
		val, err := cleanupCmd.Flags().GetBool("yes")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeTrue())
	})

	It("should accept -y shorthand", func() {
		rootCmd.SetArgs([]string{"cleanup", "-y"})
		Expect(rootCmd.Execute()).To(Succeed())

		cleanupCmd, _, _ := rootCmd.Find([]string{"cleanup"})
		val, err := cleanupCmd.Flags().GetBool("yes")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeTrue())
	})

	It("should default --yes to false", func() {
		rootCmd.SetArgs([]string{"cleanup"})
		Expect(rootCmd.Execute()).To(Succeed())

		cleanupCmd, _, _ := rootCmd.Find([]string{"cleanup"})
		val, err := cleanupCmd.Flags().GetBool("yes")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeFalse())
	})

	It("should accept --run-id flag", func() {
		rootCmd.SetArgs([]string{"cleanup", "--run-id", "abc-123"})
		Expect(rootCmd.Execute()).To(Succeed())

		cleanupCmd, _, _ := rootCmd.Find([]string{"cleanup"})
		val, err := cleanupCmd.Flags().GetString("run-id")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("abc-123"))
	})

	It("should accept --dry-run flag on cleanup", func() {
		rootCmd.SetArgs([]string{"cleanup", "--dry-run"})
		Expect(rootCmd.Execute()).To(Succeed())

		cleanupCmd, _, _ := rootCmd.Find([]string{"cleanup"})
		val, err := cleanupCmd.Flags().GetBool("dry-run")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeTrue())
	})
})

var _ = Describe("Validate command flags", func() {
	var rootCmd *cobra.Command

	BeforeEach(func() {
		rootCmd = newRootCmd()
	})

	It("should accept --catalog-dir flag", func() {
		rootCmd.SetArgs([]string{"validate", "--catalog-dir", "/tmp/my-catalog"})
		Expect(rootCmd.Execute()).To(Succeed())

		validateCmd, _, _ := rootCmd.Find([]string{"validate"})
		val, err := validateCmd.Flags().GetString("catalog-dir")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(Equal("/tmp/my-catalog"))
	})

	It("should default catalog-dir to empty string", func() {
		rootCmd.SetArgs([]string{"validate"})
		Expect(rootCmd.Execute()).To(Succeed())

		validateCmd, _, _ := rootCmd.Find([]string{"validate"})
		val, err := validateCmd.Flags().GetString("catalog-dir")
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeEmpty())
	})
})
