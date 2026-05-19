// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("TPSWorkload", func() {
	var w *workloads.TPSWorkload

	BeforeEach(func() {
		w = workloads.NewTPSWorkload(config.WorkloadConfig{
			Enabled:  true,
			VMCount:  2,
			CPUCores: 2,
			Memory:   "2Gi",
		}, "virtwork", "virtwork", "", nil)
	})

	It("should return 'tps' for Name", func() {
		Expect(w.Name()).To(Equal("tps"))
	})

	It("should return 2x VMCount for server/client pairs", func() {
		Expect(w.VMCount()).To(Equal(4))
	})

	It("should require service", func() {
		Expect(w.RequiresService()).To(BeTrue())
	})

	It("should produce server userdata with netserver and HTTP server", func() {
		result, err := w.UserdataForRole("server", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var scriptContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/usr/local/bin/virtwork-tps-server.sh" {
				scriptContent = file["content"].(string)
				break
			}
		}
		Expect(scriptContent).NotTo(BeEmpty())
		Expect(scriptContent).To(ContainSubstring("netserver"))
		Expect(scriptContent).To(ContainSubstring("12865"))
		Expect(scriptContent).To(ContainSubstring("http.server 8080"))
		Expect(scriptContent).To(ContainSubstring("dd if=/dev/urandom"))
		Expect(scriptContent).To(ContainSubstring("/srv/virtwork/testfile"))
	})

	It("should produce client userdata with TCP_RR and HTTP tests", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var scriptContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/usr/local/bin/virtwork-tps-client.sh" {
				scriptContent = file["content"].(string)
				break
			}
		}
		Expect(scriptContent).NotTo(BeEmpty())
		Expect(scriptContent).To(ContainSubstring("netperf"))
		Expect(scriptContent).To(ContainSubstring("TCP_RR"))
		Expect(scriptContent).To(ContainSubstring("curl"))
		Expect(scriptContent).To(ContainSubstring("virtwork-tps-server.virtwork.svc.cluster.local"))
	})

	It("should produce client userdata with custom namespace in DNS", func() {
		result, err := w.UserdataForRole("client", "custom-ns")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var scriptContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/usr/local/bin/virtwork-tps-client.sh" {
				scriptContent = file["content"].(string)
				break
			}
		}
		Expect(scriptContent).To(ContainSubstring("virtwork-tps-server.custom-ns.svc.cluster.local"))
	})

	It("should pin the data port to 12866", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var scriptContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/usr/local/bin/virtwork-tps-client.sh" {
				scriptContent = file["content"].(string)
				break
			}
		}
		Expect(scriptContent).To(ContainSubstring("12866"))
	})

	It("should include iteration count in client script", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var scriptContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/usr/local/bin/virtwork-tps-client.sh" {
				scriptContent = file["content"].(string)
				break
			}
		}
		Expect(scriptContent).To(ContainSubstring("ITERATIONS=30"))
	})

	It("should not include UDP_RR in client script", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var scriptContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/usr/local/bin/virtwork-tps-client.sh" {
				scriptContent = file["content"].(string)
				break
			}
		}
		Expect(scriptContent).NotTo(ContainSubstring("UDP_RR"))
	})

	It("should wait for server readiness before starting tests", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var scriptContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/usr/local/bin/virtwork-tps-client.sh" {
				scriptContent = file["content"].(string)
				break
			}
		}
		Expect(scriptContent).To(ContainSubstring("curl"))
		Expect(scriptContent).To(ContainSubstring("8080"))
		Expect(scriptContent).To(ContainSubstring("Waiting for server"))
	})

	It("should return error for unknown role", func() {
		_, err := w.UserdataForRole("unknown", "virtwork")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(workloads.ErrUnknownTPSRole))
	})

	It("should include netperf in packages for server", func() {
		result, err := w.UserdataForRole("server", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("netperf"))
	})

	It("should include netperf in packages for client", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("netperf"))
	})

	It("should have service spec with three ports", func() {
		svc := w.ServiceSpec()
		Expect(svc).NotTo(BeNil())
		Expect(svc.Spec.Ports).To(HaveLen(3))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(12865)))
		Expect(svc.Spec.Ports[0].Name).To(Equal("netperf-ctrl"))
		Expect(svc.Spec.Ports[1].Port).To(Equal(int32(12866)))
		Expect(svc.Spec.Ports[1].Name).To(Equal("netperf-data"))
		Expect(svc.Spec.Ports[2].Port).To(Equal(int32(8080)))
		Expect(svc.Spec.Ports[2].Name).To(Equal("http-data"))
	})

	It("should have service spec with correct selector", func() {
		svc := w.ServiceSpec()
		Expect(svc).NotTo(BeNil())
		Expect(svc.Spec.Selector).To(HaveKeyWithValue("virtwork/role", "server"))
		Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/component", "tps"))
	})

	It("should have service spec with correct name", func() {
		svc := w.ServiceSpec()
		Expect(svc).NotTo(BeNil())
		Expect(svc.Name).To(Equal("virtwork-tps-server"))
		Expect(svc.Namespace).To(Equal("virtwork"))
	})

	It("should produce valid YAML for server role", func() {
		result, err := w.UserdataForRole("server", "virtwork")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HavePrefix("#cloud-config\n"))

		parsed := parseYAML(result)
		Expect(parsed).NotTo(BeNil())
	})

	It("should produce valid YAML for client role", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HavePrefix("#cloud-config\n"))

		parsed := parseYAML(result)
		Expect(parsed).NotTo(BeNil())
	})

	It("should return server userdata from CloudInitUserdata", func() {
		defaultResult, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		serverResult, err := w.UserdataForRole("server", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		Expect(defaultResult).To(Equal(serverResult))
	})

	It("should have no extra disks", func() {
		Expect(w.ExtraDisks()).To(BeNil())
	})

	It("should have no extra volumes", func() {
		Expect(w.ExtraVolumes()).To(BeNil())
	})

	It("should have no data volume templates", func() {
		Expect(w.DataVolumeTemplates()).To(BeNil())
	})

	It("should reflect config in VMResources", func() {
		res := w.VMResources()
		Expect(res.CPUCores).To(Equal(2))
		Expect(res.Memory).To(Equal("2Gi"))
	})

	It("should implement MultiVMWorkload interface", func() {
		var _ workloads.MultiVMWorkload = w
	})

	Context("combined single-service architecture", func() {
		It("should use a single systemd unit for server", func() {
			result, err := w.UserdataForRole("server", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			serviceCount := 0
			for _, f := range files {
				file := f.(map[string]interface{})
				if strings.HasPrefix(file["path"].(string), "/etc/systemd/system/virtwork-tps") {
					serviceCount++
				}
			}
			Expect(serviceCount).To(Equal(1))
		})

		It("should use a single systemd unit for client", func() {
			result, err := w.UserdataForRole("client", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			serviceCount := 0
			for _, f := range files {
				file := f.(map[string]interface{})
				if strings.HasPrefix(file["path"].(string), "/etc/systemd/system/virtwork-tps") {
					serviceCount++
				}
			}
			Expect(serviceCount).To(Equal(1))
		})

		It("should enable only virtwork-tps.service in server runcmd", func() {
			result, err := w.UserdataForRole("server", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			runcmd := parsed["runcmd"].([]interface{})

			var cmds []string
			for _, cmd := range runcmd {
				parts := cmd.([]interface{})
				for _, p := range parts {
					cmds = append(cmds, p.(string))
				}
			}
			joined := strings.Join(cmds, " ")
			Expect(joined).To(ContainSubstring("virtwork-tps.service"))
			Expect(strings.Count(joined, "virtwork-tps")).To(Equal(1))
		})

		It("should enable only virtwork-tps.service in client runcmd", func() {
			result, err := w.UserdataForRole("client", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			runcmd := parsed["runcmd"].([]interface{})

			var cmds []string
			for _, cmd := range runcmd {
				parts := cmd.([]interface{})
				for _, p := range parts {
					cmds = append(cmds, p.(string))
				}
			}
			joined := strings.Join(cmds, " ")
			Expect(joined).To(ContainSubstring("virtwork-tps.service"))
			Expect(strings.Count(joined, "virtwork-tps")).To(Equal(1))
		})

		It("should include python3 in server packages", func() {
			result, err := w.UserdataForRole("server", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			pkgs := parsed["packages"].([]interface{})
			Expect(pkgs).To(ContainElement("python3"))
		})

		It("should include curl in client packages", func() {
			result, err := w.UserdataForRole("client", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			pkgs := parsed["packages"].([]interface{})
			Expect(pkgs).To(ContainElement("curl"))
		})
	})

	Context("configurable params", func() {
		It("should default file size to 10M", func() {
			result, err := w.UserdataForRole("server", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			var scriptContent string
			for _, f := range files {
				file := f.(map[string]interface{})
				if file["path"].(string) == "/usr/local/bin/virtwork-tps-server.sh" {
					scriptContent = file["content"].(string)
					break
				}
			}
			Expect(scriptContent).To(ContainSubstring("count=10"))
		})

		It("should use custom file size from params", func() {
			w2 := workloads.NewTPSWorkload(config.WorkloadConfig{
				Enabled:  true,
				VMCount:  2,
				CPUCores: 2,
				Memory:   "2Gi",
				Params:   map[string]string{"file-size": "200M"},
			}, "virtwork", "virtwork", "", nil)

			result, err := w2.UserdataForRole("server", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			var scriptContent string
			for _, f := range files {
				file := f.(map[string]interface{})
				if file["path"].(string) == "/usr/local/bin/virtwork-tps-server.sh" {
					scriptContent = file["content"].(string)
					break
				}
			}
			Expect(scriptContent).To(ContainSubstring("count=200"))
		})

		It("should use custom iterations from params", func() {
			w2 := workloads.NewTPSWorkload(config.WorkloadConfig{
				Enabled:  true,
				VMCount:  2,
				CPUCores: 2,
				Memory:   "2Gi",
				Params:   map[string]string{"iterations": "5"},
			}, "virtwork", "virtwork", "", nil)

			result, err := w2.UserdataForRole("client", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			var scriptContent string
			for _, f := range files {
				file := f.(map[string]interface{})
				if file["path"].(string) == "/usr/local/bin/virtwork-tps-client.sh" {
					scriptContent = file["content"].(string)
					break
				}
			}
			Expect(scriptContent).To(ContainSubstring("ITERATIONS=5"))
		})

		It("should use custom duration from params", func() {
			w2 := workloads.NewTPSWorkload(config.WorkloadConfig{
				Enabled:  true,
				VMCount:  2,
				CPUCores: 2,
				Memory:   "2Gi",
				Params:   map[string]string{"duration": "10"},
			}, "virtwork", "virtwork", "", nil)

			result, err := w2.UserdataForRole("client", "virtwork")
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			var scriptContent string
			for _, f := range files {
				file := f.(map[string]interface{})
				if file["path"].(string) == "/usr/local/bin/virtwork-tps-client.sh" {
					scriptContent = file["content"].(string)
					break
				}
			}
			Expect(scriptContent).To(ContainSubstring("DURATION=10"))
		})
	})
})
