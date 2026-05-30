// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("NetworkWorkload", func() {
	var w *workloads.NetworkWorkload

	BeforeEach(func() {
		w = workloads.NewNetworkWorkload(config.WorkloadConfig{
			Enabled:  new(true),
			VMCount:  2,
			CPUCores: 2,
			Memory:   "2Gi",
		}, "virtwork", "virtwork", "", nil)
	})

	It("should return 'network' for Name", func() {
		Expect(w.Name()).To(Equal("network"))
	})

	It("should return 2x VMCount for server/client pairs", func() {
		Expect(w.VMCount()).To(Equal(4)) // VMCount=2 config → 2 servers + 2 clients
	})

	It("should require service", func() {
		Expect(w.RequiresService()).To(BeTrue())
	})

	It("should return per-role VM counts via RoleDistribution", func() {
		dist := w.RoleDistribution()
		Expect(dist).To(HaveLen(2))
		Expect(dist[0]).To(Equal(workloads.RoleSpec{Role: "server", VMCount: 2}))
		Expect(dist[1]).To(Equal(workloads.RoleSpec{Role: "client", VMCount: 2}))
	})

	It("should have VMCount equal to sum of RoleDistribution counts", func() {
		dist := w.RoleDistribution()
		total := 0
		for _, rs := range dist {
			total += rs.VMCount
		}
		Expect(w.VMCount()).To(Equal(total))
	})

	It("should produce server userdata with iperf3 -s", func() {
		result, err := w.UserdataForRole("server", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var serviceContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/etc/systemd/system/virtwork-network.service" {
				serviceContent = file["content"].(string)
				break
			}
		}
		Expect(serviceContent).NotTo(BeEmpty())
		Expect(serviceContent).To(ContainSubstring("iperf3 -s"))
	})

	It("should produce client userdata with DNS name", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var serviceContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/etc/systemd/system/virtwork-network.service" {
				serviceContent = file["content"].(string)
				break
			}
		}
		Expect(serviceContent).NotTo(BeEmpty())
		Expect(serviceContent).To(ContainSubstring("iperf3 -c"))
		Expect(serviceContent).To(ContainSubstring("virtwork-iperf3-server.virtwork.svc.cluster.local"))
		Expect(serviceContent).To(ContainSubstring("--bidir"))
	})

	It("should produce client userdata with custom namespace in DNS", func() {
		result, err := w.UserdataForRole("client", "custom-ns")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var serviceContent string
		for _, f := range files {
			file := f.(map[string]interface{})
			if file["path"].(string) == "/etc/systemd/system/virtwork-network.service" {
				serviceContent = file["content"].(string)
				break
			}
		}
		Expect(serviceContent).To(ContainSubstring("virtwork-iperf3-server.custom-ns.svc.cluster.local"))
	})

	It("should return error for unknown role", func() {
		_, err := w.UserdataForRole("unknown", "virtwork")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(workloads.ErrUnknownNetworkRole))
	})

	It("should include iperf3 in packages for server", func() {
		result, err := w.UserdataForRole("server", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("iperf3"))
	})

	It("should include iperf3 in packages for client", func() {
		result, err := w.UserdataForRole("client", "virtwork")
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("iperf3"))
	})

	It("should have service spec with correct port", func() {
		svc := w.ServiceSpec()
		Expect(svc).NotTo(BeNil())
		Expect(svc.Spec.Ports).To(HaveLen(1))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(5201)))
	})

	It("should have service spec with correct selector", func() {
		svc := w.ServiceSpec()
		Expect(svc).NotTo(BeNil())
		Expect(svc.Spec.Selector).To(HaveKeyWithValue("virtwork/role", "server"))
	})

	It("should have service spec with correct name", func() {
		svc := w.ServiceSpec()
		Expect(svc).NotTo(BeNil())
		Expect(svc.Name).To(Equal("virtwork-iperf3-server"))
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
})
