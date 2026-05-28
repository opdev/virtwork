// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
)

func newTestCommand() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	config.BindPersistentFlags(root)

	cmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	config.BindRunFlags(cmd, nil)
	root.AddCommand(cmd)

	// Merge root's persistent flags into cmd (Cobra does this during Execute;
	// tests call LoadConfig directly, so we replicate the merge here).
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		cmd.Flags().AddFlag(f)
	})

	return cmd
}

func writeConfigFile(dir, content string) string {
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(content), 0o600)
	Expect(err).NotTo(HaveOccurred())
	return path
}

func writeKeyFile(dir, name, content string) string {
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0o600)
	Expect(err).NotTo(HaveOccurred())
	return path
}

var _ = Describe("Config", func() {
	var cmd *cobra.Command

	BeforeEach(func() {
		// Clear all VIRTWORK_ env vars before each test
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "VIRTWORK_") {
				virtworkEnv := strings.Split(env, "=")[0]
				_ = os.Unsetenv(virtworkEnv)
			}
		}
		cmd = newTestCommand()
	})

	Context("with defaults", func() {
		It("should have correct default namespace", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Namespace).To(Equal(constants.DefaultNamespace))
		})

		It("should have correct default CPU cores", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CPUCores).To(Equal(constants.DefaultCPUCores))
		})

		It("should have correct default memory", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Memory).To(Equal(constants.DefaultMemory))
		})

		It("should have correct default container disk image", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.ContainerDiskImage).To(Equal(constants.DefaultContainerDiskImage))
		})

		It("should have correct default disk size", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DataDiskSize).To(Equal(constants.DefaultDiskSize))
		})

		It("should default DryRun to false", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DryRun).To(BeFalse())
		})

		It("should default Verbose to false", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Verbose).To(BeFalse())
		})

		It("should default WaitForReady to true", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.WaitForReady).To(BeTrue())
		})

		It("should have correct default ready timeout", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.ReadyTimeoutSeconds).To(Equal(600))
		})
	})

	Context("with YAML config file", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "virtwork-config-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpDir)
		})

		It("should load namespace from file", func() {
			path := writeConfigFile(tmpDir, `namespace: custom-ns`)
			err1 := cmd.Flags().Set("config", path)
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Namespace).To(Equal("custom-ns"))
		})

		It("should load multiple values from file", func() {
			path := writeConfigFile(tmpDir, `
namespace: from-file
cpu-cores: 4
memory: 4Gi
container-disk-image: quay.io/test/image:latest
`)
			err1 := cmd.Flags().Set("config", path)
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Namespace).To(Equal("from-file"))
			Expect(cfg.CPUCores).To(Equal(4))
			Expect(cfg.Memory).To(Equal("4Gi"))
			Expect(cfg.ContainerDiskImage).To(Equal("quay.io/test/image:latest"))
		})

		It("should return error for missing file", func() {
			err1 := cmd.Flags().Set("config", "/nonexistent/path/config.yaml")
			Expect(err1).To(BeNil())

			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("with environment variables", func() {
		It("should override defaults with VIRTWORK_ env vars", func() {
			_ = os.Setenv("VIRTWORK_NAMESPACE", "env-ns")
			defer func() {
				_ = os.Unsetenv("VIRTWORK_NAMESPACE")
			}()

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Namespace).To(Equal("env-ns"))
		})

		It("should override CPU cores from env", func() {
			_ = os.Setenv("VIRTWORK_CPU_CORES", "8")
			defer func() {
				_ = os.Unsetenv("VIRTWORK_CPU_CORES")
			}()

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CPUCores).To(Equal(8))
		})

		It("should override memory from env", func() {
			_ = os.Setenv("VIRTWORK_MEMORY", "8Gi")
			defer func() {
				_ = os.Unsetenv("VIRTWORK_MEMORY")
			}()

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Memory).To(Equal("8Gi"))
		})
	})

	Context("priority chain", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "virtwork-config-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				fmt.Printf("failed deleting %s", tmpDir)
			}
		})

		It("should prefer flags over env vars", func() {
			_ = os.Setenv("VIRTWORK_NAMESPACE", "env-ns")
			defer func() {
				_ = os.Unsetenv("VIRTWORK_NAMESPACE")
			}()

			err1 := cmd.Flags().Set("namespace", "flag-ns")
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Namespace).To(Equal("flag-ns"))
		})

		It("should prefer env vars over config file", func() {
			path := writeConfigFile(tmpDir, `namespace: file-ns`)
			err1 := cmd.Flags().Set("config", path)
			Expect(err1).NotTo(HaveOccurred())

			_ = os.Setenv("VIRTWORK_NAMESPACE", "env-ns")
			defer func() {
				_ = os.Unsetenv("VIRTWORK_NAMESPACE")
			}()

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Namespace).To(Equal("env-ns"))
		})

		It("should prefer config file over defaults", func() {
			path := writeConfigFile(tmpDir, `namespace: file-ns`)
			err1 := cmd.Flags().Set("config", path)
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Namespace).To(Equal("file-ns"))
		})

		It("should apply --disk-size flag to DataDiskSize", func() {
			err1 := cmd.Flags().Set("disk-size", "50Gi")
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DataDiskSize).To(Equal("50Gi"))
		})

		It("should prefer --disk-size flag over env var", func() {
			_ = os.Setenv("VIRTWORK_DISK_SIZE", "100Gi")
			defer func() {
				_ = os.Unsetenv("VIRTWORK_DISK_SIZE")
			}()

			err1 := cmd.Flags().Set("disk-size", "50Gi")
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DataDiskSize).To(Equal("50Gi"))
		})

		It("should prefer VIRTWORK_DISK_SIZE env over config file", func() {
			path := writeConfigFile(tmpDir, `disk-size: 30Gi`)
			err1 := cmd.Flags().Set("config", path)
			Expect(err1).NotTo(HaveOccurred())

			_ = os.Setenv("VIRTWORK_DISK_SIZE", "60Gi")
			defer func() {
				_ = os.Unsetenv("VIRTWORK_DISK_SIZE")
			}()

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DataDiskSize).To(Equal("60Gi"))
		})
	})

	Context("SSH config fields", func() {
		It("should default SSHUser to virtwork", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHUser).To(Equal(constants.DefaultSSHUser))
		})

		It("should default SSHPassword to empty", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHPassword).To(BeEmpty())
		})

		It("should default SSHAuthorizedKeys to empty", func() {
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHAuthorizedKeys).To(BeEmpty())
		})

		It("should accept ssh-user flag", func() {
			err1 := cmd.Flags().Set("ssh-user", "testuser")
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHUser).To(Equal("testuser"))
		})

		It("should accept ssh-password flag", func() {
			err1 := cmd.Flags().Set("ssh-password", "secret123")
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHPassword).To(Equal("secret123"))
		})

		It("should accept ssh-key flag", func() {
			err1 := cmd.Flags().Set("ssh-key", "ssh-rsa AAAA...")
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-rsa AAAA..."))
		})

		It("should split comma-separated VIRTWORK_SSH_AUTHORIZED_KEYS", func() {
			_ = os.Setenv("VIRTWORK_SSH_AUTHORIZED_KEYS", "ssh-rsa KEY1,ssh-ed25519 KEY2")
			defer func() {
				_ = os.Unsetenv("VIRTWORK_NAMESPACE")
			}()

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHAuthorizedKeys).To(HaveLen(2))
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-rsa KEY1"))
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-ed25519 KEY2"))
		})

		It("should load ssh-authorized-keys from YAML as list", func() {
			tmpDir, err := os.MkdirTemp("", "virtwork-config-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = os.RemoveAll(tmpDir)
			}()

			path := writeConfigFile(tmpDir, `
ssh-user: yamluser
ssh-authorized-keys:
  - ssh-rsa YAMLKEY1
  - ssh-ed25519 YAMLKEY2
`)
			err1 := cmd.Flags().Set("config", path)
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHUser).To(Equal("yamluser"))
			Expect(cfg.SSHAuthorizedKeys).To(HaveLen(2))
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-rsa YAMLKEY1"))
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-ed25519 YAMLKEY2"))
		})

		It("should read public key from --ssh-key-file", func() {
			tmpDir, err := os.MkdirTemp("", "virtwork-config-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = os.RemoveAll(tmpDir)
			}()

			keyPath := writeKeyFile(tmpDir, "id_ed25519.pub", "ssh-ed25519 AAAAC3filekey user@host\n")
			err1 := cmd.Flags().Set("ssh-key-file", keyPath)
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-ed25519 AAAAC3filekey user@host"))
		})

		It("should read multiple --ssh-key-file flags", func() {
			tmpDir, err := os.MkdirTemp("", "virtwork-config-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = os.RemoveAll(tmpDir)
			}()

			keyPath1 := writeKeyFile(tmpDir, "key1.pub", "ssh-rsa AAAA1 user1@host\n")
			keyPath2 := writeKeyFile(tmpDir, "key2.pub", "ssh-ed25519 AAAA2 user2@host\n")
			err1 := cmd.Flags().Set("ssh-key-file", keyPath1)
			Expect(err1).NotTo(HaveOccurred())
			err2 := cmd.Flags().Set("ssh-key-file", keyPath2)
			Expect(err2).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHAuthorizedKeys).To(HaveLen(2))
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-rsa AAAA1 user1@host"))
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-ed25519 AAAA2 user2@host"))
		})

		It("should merge --ssh-key and --ssh-key-file", func() {
			tmpDir, err := os.MkdirTemp("", "virtwork-config-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = os.RemoveAll(tmpDir)
			}()

			keyPath := writeKeyFile(tmpDir, "id.pub", "ssh-ed25519 FILEkey user@host\n")
			err1 := cmd.Flags().Set("ssh-key", "ssh-rsa INLINEkey")
			Expect(err1).NotTo(HaveOccurred())
			err2 := cmd.Flags().Set("ssh-key-file", keyPath)
			Expect(err2).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SSHAuthorizedKeys).To(HaveLen(2))
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-rsa INLINEkey"))
			Expect(cfg.SSHAuthorizedKeys).To(ContainElement("ssh-ed25519 FILEkey user@host"))
		})

		It("should return error for nonexistent --ssh-key-file", func() {
			err1 := cmd.Flags().Set("ssh-key-file", "/nonexistent/key.pub")
			Expect(err1).NotTo(HaveOccurred())

			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("/nonexistent/key.pub"))
		})
	})

	Context("workload config", func() {
		It("should load workloads from YAML", func() {
			tmpDir, err := os.MkdirTemp("", "virtwork-config-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					log.Println("cleaning temporary directory failed")
				}
			}()

			path := writeConfigFile(tmpDir, `
workloads:
  cpu:
    enabled: true
    vm-count: 2
    cpu-cores: 4
    memory: 4Gi
  disk:
    enabled: false
`)
			err1 := cmd.Flags().Set("config", path)
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Workloads).To(HaveKey("cpu"))
			Expect(cfg.Workloads["cpu"].Enabled).NotTo(BeNil())
			Expect(*cfg.Workloads["cpu"].Enabled).To(BeTrue())
			Expect(cfg.Workloads["cpu"].VMCount).To(Equal(2))
			Expect(cfg.Workloads["cpu"].CPUCores).To(Equal(4))
			Expect(cfg.Workloads["cpu"].Memory).To(Equal("4Gi"))
			Expect(cfg.Workloads["disk"].Enabled).NotTo(BeNil())
			Expect(*cfg.Workloads["disk"].Enabled).To(BeFalse())
		})

		It("should load workload params from YAML", func() {
			tmpDir, err := os.MkdirTemp("", "virtwork-config-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					log.Println("cleaning temporary directory failed")
				}
			}()

			path := writeConfigFile(tmpDir, `
workloads:
  tps:
    enabled: true
    vm-count: 2
    params:
      file-size: 200M
      mode: file-transfer
`)
			err1 := cmd.Flags().Set("config", path)
			Expect(err1).NotTo(HaveOccurred())

			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Workloads).To(HaveKey("tps"))
			Expect(cfg.Workloads["tps"].Params).To(HaveKeyWithValue("file-size", "200M"))
			Expect(cfg.Workloads["tps"].Params).To(HaveKeyWithValue("mode", "file-transfer"))
		})
	})
})

var _ = Describe("Config Validation", func() {
	var cmd *cobra.Command

	BeforeEach(func() {
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "VIRTWORK_") {
				_ = os.Unsetenv(strings.Split(env, "=")[0])
			}
		}
		cmd = newTestCommand()
	})

	Context("namespace", func() {
		It("should reject an empty namespace", func() {
			Expect(cmd.Flags().Set("namespace", "")).To(Succeed())
			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespace"))
		})

		It("should accept a non-empty namespace", func() {
			Expect(cmd.Flags().Set("namespace", "valid-ns")).To(Succeed())
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Namespace).To(Equal("valid-ns"))
		})
	})

	//nolint:dupl
	Context("cpu-cores", func() {
		It("should reject zero CPU cores", func() {
			Expect(cmd.Flags().Set("cpu-cores", "0")).To(Succeed())
			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cpu-cores"))
		})

		It("should reject negative CPU cores", func() {
			Expect(cmd.Flags().Set("cpu-cores", "-1")).To(Succeed())
			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cpu-cores"))
		})

		It("should accept positive CPU cores", func() {
			Expect(cmd.Flags().Set("cpu-cores", "4")).To(Succeed())
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CPUCores).To(Equal(4))
		})
	})

	//nolint:dupl
	Context("memory", func() {
		It("should reject an empty memory value", func() {
			Expect(cmd.Flags().Set("memory", "")).To(Succeed())
			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("memory"))
		})

		It("should reject an invalid memory quantity", func() {
			Expect(cmd.Flags().Set("memory", "not-a-quantity")).To(Succeed())
			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("memory"))
		})

		It("should accept a valid memory quantity", func() {
			Expect(cmd.Flags().Set("memory", "4Gi")).To(Succeed())
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Memory).To(Equal("4Gi"))
		})
	})

	Context("disk-size", func() {
		It("should reject an invalid disk-size quantity", func() {
			Expect(cmd.Flags().Set("disk-size", "bad-size")).To(Succeed())
			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("disk-size"))
		})

		It("should accept a valid disk-size quantity", func() {
			Expect(cmd.Flags().Set("disk-size", "20Gi")).To(Succeed())
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DataDiskSize).To(Equal("20Gi"))
		})
	})

	Context("timeout", func() {
		It("should reject zero timeout when wait-for-ready is enabled", func() {
			Expect(cmd.Flags().Set("timeout", "0")).To(Succeed())
			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout"))
		})

		It("should reject negative timeout when wait-for-ready is enabled", func() {
			Expect(cmd.Flags().Set("timeout", "-5")).To(Succeed())
			_, err := config.LoadConfig(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout"))
		})

		It("should accept positive timeout", func() {
			Expect(cmd.Flags().Set("timeout", "300")).To(Succeed())
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.ReadyTimeoutSeconds).To(Equal(300))
		})

		It("should skip timeout validation when wait-for-ready is disabled", func() {
			Expect(cmd.Flags().Set("no-wait", "true")).To(Succeed())
			Expect(cmd.Flags().Set("timeout", "0")).To(Succeed())
			cfg, err := config.LoadConfig(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.WaitForReady).To(BeFalse())
		})
	})
})
