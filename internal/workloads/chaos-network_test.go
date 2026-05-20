// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("ChaosNetworkWorkload", func() {
	var w *workloads.ChaosNetworkWorkload

	BeforeEach(func() {
		w = workloads.NewChaosNetworkWorkload(config.WorkloadConfig{
			Enabled:  true,
			VMCount:  1,
			CPUCores: 1,
			Memory:   "1Gi",
		}, "virtwork", "", nil)
	})

	It("should return 'chaos-network' for Name", func() {
		Expect(w.Name()).To(Equal("chaos-network"))
	})

	It("should not include packages (assumes golden image)", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		_, hasPackages := parsed["packages"]
		Expect(hasPackages).To(BeFalse(), "chaos-network should not install packages (assumes golden image)")
	})

	It("should include systemd service in cloud-init", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("write_files"))
		files := parsed["write_files"].([]interface{})
		Expect(files).To(HaveLen(1))

		file := files[0].(map[string]interface{})
		Expect(file["path"]).To(Equal("/etc/systemd/system/virtwork-chaos-network.service"))

		content := file["content"].(string)
		Expect(content).To(ContainSubstring("tc qdisc"))
		Expect(content).To(ContainSubstring("netem"))
		Expect(content).To(ContainSubstring("delay 100ms"), "should include default 100ms latency")
		Expect(content).To(ContainSubstring("loss 5.0%"), "should include default 5% packet loss")
	})

	It("should include systemd enable commands", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("runcmd"))
		runcmds := parsed["runcmd"].([]interface{})

		// Should have daemon-reload and enable commands
		found := false
		for _, cmd := range runcmds {
			cmdSlice := cmd.([]interface{})
			if len(cmdSlice) >= 3 {
				if cmdSlice[0] == "systemctl" && cmdSlice[1] == "enable" {
					found = true
					Expect(cmdSlice[2]).To(Equal("--now"))
				}
			}
		}
		Expect(found).To(BeTrue(), "should have systemctl enable command")
	})

	It("should produce valid YAML", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HavePrefix("#cloud-config\n"))

		parsed := parseYAML(result)
		Expect(parsed).NotTo(BeNil())
	})

	It("should have no extra disks", func() {
		Expect(w.ExtraDisks()).To(BeNil())
	})

	It("should have no data volumes", func() {
		Expect(w.DataVolumeTemplates()).To(BeNil())
	})

	It("should have no service", func() {
		Expect(w.RequiresService()).To(BeFalse())
		Expect(w.ServiceSpec()).To(BeNil())
	})

	It("should reflect config in VMResources", func() {
		res := w.VMResources()
		Expect(res.CPUCores).To(Equal(1))
		Expect(res.Memory).To(Equal("1Gi"))
	})

	It("should have VMCount of 1", func() {
		Expect(w.VMCount()).To(Equal(1))
	})
})
