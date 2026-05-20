// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package cloudinit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/opdev/virtwork/internal/cloudinit"
)

var _ = Describe("BuildCloudConfig", func() {
	It("should return valid #cloud-config header for empty opts", func() {
		result, err := cloudinit.BuildCloudConfig(cloudinit.CloudConfigOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HavePrefix("#cloud-config\n"))
	})

	It("should include packages list", func() {
		opts := cloudinit.CloudConfigOpts{
			Packages: []string{"vim", "curl"},
		}
		result, err := cloudinit.BuildCloudConfig(opts)
		Expect(err).NotTo(HaveOccurred())

		var parsed map[string]interface{}
		Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ConsistOf("vim", "curl"))
	})

	It("should include write_files with correct structure", func() {
		opts := cloudinit.CloudConfigOpts{
			WriteFiles: []cloudinit.WriteFile{
				{
					Path:        "/etc/myconfig",
					Content:     "key=value",
					Permissions: "0644",
				},
			},
		}
		result, err := cloudinit.BuildCloudConfig(opts)
		Expect(err).NotTo(HaveOccurred())

		var parsed map[string]interface{}
		Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
		files, ok := parsed["write_files"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(files).To(HaveLen(1))

		file := files[0].(map[string]interface{})
		Expect(file["path"]).To(Equal("/etc/myconfig"))
		Expect(file["content"]).To(Equal("key=value"))
		Expect(file["permissions"]).To(Equal("0644"))
	})

	It("should include runcmd entries", func() {
		opts := cloudinit.CloudConfigOpts{
			RunCmd: [][]string{
				{"systemctl", "start", "myservice"},
				{"echo", "done"},
			},
		}
		result, err := cloudinit.BuildCloudConfig(opts)
		Expect(err).NotTo(HaveOccurred())

		var parsed map[string]interface{}
		Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
		cmds, ok := parsed["runcmd"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(cmds).To(HaveLen(2))

		// Each entry is a list of strings
		cmd0 := cmds[0].([]interface{})
		Expect(cmd0).To(ConsistOf("systemctl", "start", "myservice"))
	})

	It("should merge extra keys at top level", func() {
		opts := cloudinit.CloudConfigOpts{
			Extra: map[string]interface{}{
				"timezone": "UTC",
				"locale":   "en_US.UTF-8",
			},
		}
		result, err := cloudinit.BuildCloudConfig(opts)
		Expect(err).NotTo(HaveOccurred())

		var parsed map[string]interface{}
		Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
		Expect(parsed["timezone"]).To(Equal("UTC"))
		Expect(parsed["locale"]).To(Equal("en_US.UTF-8"))
	})

	It("should omit nil/empty values", func() {
		opts := cloudinit.CloudConfigOpts{}
		result, err := cloudinit.BuildCloudConfig(opts)
		Expect(err).NotTo(HaveOccurred())

		var parsed map[string]interface{}
		Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
		Expect(parsed).NotTo(HaveKey("packages"))
		Expect(parsed).NotTo(HaveKey("write_files"))
		Expect(parsed).NotTo(HaveKey("runcmd"))
		Expect(parsed).NotTo(HaveKey("users"))
		Expect(parsed).NotTo(HaveKey("ssh_pwauth"))
	})

	It("should produce output parseable by yaml.Unmarshal", func() {
		opts := cloudinit.CloudConfigOpts{
			Packages: []string{"stress-ng"},
			WriteFiles: []cloudinit.WriteFile{
				{Path: "/etc/test", Content: "data", Permissions: "0644"},
			},
			RunCmd: [][]string{{"echo", "hello"}},
			Extra:  map[string]interface{}{"hostname": "test-vm"},
		}
		result, err := cloudinit.BuildCloudConfig(opts)
		Expect(err).NotTo(HaveOccurred())

		var parsed map[string]interface{}
		Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
		Expect(parsed).To(HaveKey("packages"))
		Expect(parsed).To(HaveKey("write_files"))
		Expect(parsed).To(HaveKey("runcmd"))
		Expect(parsed).To(HaveKey("hostname"))
	})

	It("should have exact #cloud-config header", func() {
		result, err := cloudinit.BuildCloudConfig(cloudinit.CloudConfigOpts{})
		Expect(err).NotTo(HaveOccurred())
		// Must start with exactly "#cloud-config\n" — cloud-init requires this
		Expect(result).To(HavePrefix("#cloud-config\n"))
		// For empty opts, the entire output should be just the header
		Expect(result).To(Equal("#cloud-config\n"))
	})

	Context("SSH user support", func() {
		It("should create users block when SSHUser is set", func() {
			opts := cloudinit.CloudConfigOpts{
				SSHUser: "testuser",
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("users"))

			users := parsed["users"].([]interface{})
			Expect(users).To(HaveLen(1))

			user := users[0].(map[string]interface{})
			Expect(user["name"]).To(Equal("testuser"))
			Expect(user["sudo"]).To(Equal("ALL=(ALL) NOPASSWD:ALL"))
			Expect(user["shell"]).To(Equal("/bin/bash"))
		})

		It("should set lock_passwd true when no password", func() {
			opts := cloudinit.CloudConfigOpts{
				SSHUser: "testuser",
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())

			users := parsed["users"].([]interface{})
			user := users[0].(map[string]interface{})
			Expect(user["lock_passwd"]).To(BeTrue())
		})

		It("should set lock_passwd false and plain_text_passwd when password set", func() {
			opts := cloudinit.CloudConfigOpts{
				SSHUser:     "testuser",
				SSHPassword: "secret123",
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())

			users := parsed["users"].([]interface{})
			user := users[0].(map[string]interface{})
			Expect(user["lock_passwd"]).To(BeFalse())
			Expect(user["plain_text_passwd"]).To(Equal("secret123"))
		})

		It("should set ssh_pwauth true only when password is set", func() {
			// With password
			opts := cloudinit.CloudConfigOpts{
				SSHUser:     "testuser",
				SSHPassword: "secret123",
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed["ssh_pwauth"]).To(BeTrue())

			// Without password — ssh_pwauth should not be present
			optsNoPass := cloudinit.CloudConfigOpts{
				SSHUser: "testuser",
			}
			resultNoPass, err := cloudinit.BuildCloudConfig(optsNoPass)
			Expect(err).NotTo(HaveOccurred())

			var parsedNoPass map[string]interface{}
			Expect(yaml.Unmarshal([]byte(resultNoPass), &parsedNoPass)).To(Succeed())
			Expect(parsedNoPass).NotTo(HaveKey("ssh_pwauth"))
		})

		It("should include ssh_authorized_keys when keys provided", func() {
			opts := cloudinit.CloudConfigOpts{
				SSHUser:           "testuser",
				SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA... user@host"},
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())

			users := parsed["users"].([]interface{})
			user := users[0].(map[string]interface{})
			keys := user["ssh_authorized_keys"].([]interface{})
			Expect(keys).To(ConsistOf("ssh-ed25519 AAAA... user@host"))
		})

		It("should handle multiple authorized keys", func() {
			opts := cloudinit.CloudConfigOpts{
				SSHUser: "testuser",
				SSHAuthorizedKeys: []string{
					"ssh-ed25519 AAAA... user1@host",
					"ssh-rsa BBBB... user2@host",
				},
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())

			users := parsed["users"].([]interface{})
			user := users[0].(map[string]interface{})
			keys := user["ssh_authorized_keys"].([]interface{})
			Expect(keys).To(HaveLen(2))
			Expect(keys).To(ConsistOf(
				"ssh-ed25519 AAAA... user1@host",
				"ssh-rsa BBBB... user2@host",
			))
		})

		It("should handle combined password and keys", func() {
			opts := cloudinit.CloudConfigOpts{
				SSHUser:           "testuser",
				SSHPassword:       "mypassword",
				SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA... user@host"},
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())

			// Top level ssh_pwauth should be true
			Expect(parsed["ssh_pwauth"]).To(BeTrue())

			users := parsed["users"].([]interface{})
			user := users[0].(map[string]interface{})
			// Password auth
			Expect(user["lock_passwd"]).To(BeFalse())
			Expect(user["plain_text_passwd"]).To(Equal("mypassword"))
			// Key auth
			keys := user["ssh_authorized_keys"].([]interface{})
			Expect(keys).To(ConsistOf("ssh-ed25519 AAAA... user@host"))
		})

		It("should not create users block when SSHUser is empty", func() {
			opts := cloudinit.CloudConfigOpts{
				SSHUser: "",
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("users"))
		})

		It("should not include ssh_authorized_keys when keys list is empty", func() {
			opts := cloudinit.CloudConfigOpts{
				SSHUser:           "testuser",
				SSHAuthorizedKeys: []string{},
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())

			users := parsed["users"].([]interface{})
			user := users[0].(map[string]interface{})
			Expect(user).NotTo(HaveKey("ssh_authorized_keys"))
		})

		It("should coexist with workload params (packages, runcmd)", func() {
			opts := cloudinit.CloudConfigOpts{
				Packages:          []string{"stress-ng"},
				RunCmd:            [][]string{{"systemctl", "enable", "stress"}},
				SSHUser:           "testuser",
				SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA... user@host"},
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())

			// Workload params present
			Expect(parsed).To(HaveKey("packages"))
			Expect(parsed).To(HaveKey("runcmd"))

			// SSH user present
			Expect(parsed).To(HaveKey("users"))
			users := parsed["users"].([]interface{})
			user := users[0].(map[string]interface{})
			Expect(user["name"]).To(Equal("testuser"))
		})
	})

	Context("Extra key collision validation", func() {
		It("should reject Extra with 'packages' key", func() {
			opts := cloudinit.CloudConfigOpts{
				Packages: []string{"vim"},
				Extra: map[string]interface{}{
					"packages": []string{"curl"},
				},
			}
			_, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("packages")))
			Expect(err).To(MatchError(ContainSubstring("reserved")))
		})

		It("should reject Extra with 'write_files' key", func() {
			opts := cloudinit.CloudConfigOpts{
				Extra: map[string]interface{}{
					"write_files": []interface{}{},
				},
			}
			_, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("write_files")))
		})

		It("should reject Extra with 'runcmd' key", func() {
			opts := cloudinit.CloudConfigOpts{
				Extra: map[string]interface{}{
					"runcmd": []interface{}{},
				},
			}
			_, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("runcmd")))
		})

		It("should reject Extra with 'users' key", func() {
			opts := cloudinit.CloudConfigOpts{
				Extra: map[string]interface{}{
					"users": []interface{}{},
				},
			}
			_, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("users")))
		})

		It("should reject Extra with 'ssh_pwauth' key", func() {
			opts := cloudinit.CloudConfigOpts{
				Extra: map[string]interface{}{
					"ssh_pwauth": true,
				},
			}
			_, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("ssh_pwauth")))
		})

		It("should allow Extra with non-reserved keys", func() {
			opts := cloudinit.CloudConfigOpts{
				Extra: map[string]interface{}{
					"timezone": "UTC",
					"hostname": "test-vm",
					"locale":   "en_US.UTF-8",
				},
			}
			result, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(yaml.Unmarshal([]byte(result), &parsed)).To(Succeed())
			Expect(parsed["timezone"]).To(Equal("UTC"))
			Expect(parsed["hostname"]).To(Equal("test-vm"))
			Expect(parsed["locale"]).To(Equal("en_US.UTF-8"))
		})

		It("should allow empty Extra map", func() {
			opts := cloudinit.CloudConfigOpts{
				Extra: map[string]interface{}{},
			}
			_, err := cloudinit.BuildCloudConfig(opts)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
