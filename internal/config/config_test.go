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

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
)

func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	config.BindFlags(cmd)
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
