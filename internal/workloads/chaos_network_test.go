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

	It("should include iproute-tc package", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("packages"))
		packages := parsed["packages"].([]interface{})
		Expect(packages).To(ContainElement("iproute-tc"))
	})

	It("should include start script, stop script, and systemd service in cloud-init", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("write_files"))
		files := parsed["write_files"].([]interface{})
		Expect(files).To(HaveLen(3))

		filesByPath := map[string]string{}
		for _, f := range files {
			fm := f.(map[string]interface{})
			filesByPath[fm["path"].(string)] = fm["content"].(string)
		}

		startScript := filesByPath["/usr/local/bin/virtwork-chaos-network-start.sh"]
		Expect(startScript).To(ContainSubstring("tc qdisc"))
		Expect(startScript).To(ContainSubstring("netem"))
		Expect(startScript).To(ContainSubstring("delay 100ms"), "should include default 100ms latency")
		Expect(startScript).To(ContainSubstring("loss 5.0%"), "should include default 5% packet loss")
		Expect(startScript).To(ContainSubstring("ip route show default"), "should auto-detect interface")
		Expect(startScript).To(ContainSubstring("modprobe sch_netem"), "should load netem kernel module")

		Expect(filesByPath).To(HaveKey("/usr/local/bin/virtwork-chaos-network-stop.sh"))
		Expect(filesByPath).To(HaveKey("/etc/systemd/system/virtwork-chaos-network.service"))
	})

	It("should install kernel-modules-extra for running kernel before enabling service", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		Expect(parsed).To(HaveKey("runcmd"))
		runcmds := parsed["runcmd"].([]interface{})

		foundKernelModules := false
		foundEnable := false
		kernelModulesIdx := -1
		enableIdx := -1
		for i, cmd := range runcmds {
			cmdSlice := cmd.([]interface{})
			if len(cmdSlice) >= 3 {
				if cmdSlice[0] == "bash" && cmdSlice[1] == "-c" {
					cmdStr := cmdSlice[2].(string)
					if cmdStr == "dnf install -y kernel-modules-extra-$(uname -r)" {
						foundKernelModules = true
						kernelModulesIdx = i
					}
				}
				if cmdSlice[0] == "systemctl" && cmdSlice[1] == "enable" {
					foundEnable = true
					enableIdx = i
					Expect(cmdSlice[2]).To(Equal("--now"))
				}
			}
		}
		Expect(foundKernelModules).To(BeTrue(), "should install kernel-modules-extra for running kernel")
		Expect(foundEnable).To(BeTrue(), "should have systemctl enable command")
		Expect(kernelModulesIdx).To(
			BeNumerically("<", enableIdx),
			"kernel modules install should come before service enable")
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
