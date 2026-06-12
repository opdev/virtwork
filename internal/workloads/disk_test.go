// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("DiskWorkload", func() {
	var w *workloads.DiskWorkload

	BeforeEach(func() {
		w = workloads.NewDiskWorkload(config.WorkloadConfig{
			Enabled:  new(true),
			VMCount:  1,
			CPUCores: 2,
			Memory:   "2Gi",
		}, "10Gi", "virtwork", "", nil)
	})

	It("should return 'disk' for Name", func() {
		Expect(w.Name()).To(Equal("disk"))
	})

	It("should include fio in packages", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		pkgs, ok := parsed["packages"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(pkgs).To(ContainElement("fio"))
	})

	It("should include fio profiles and disk-setup script in write_files", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		// Should have: disk-setup.sh, mixed-rw.fio, seq-write.fio, systemd unit = 4 files
		Expect(files).To(HaveLen(4))

		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.(map[string]interface{})["path"].(string)
		}
		Expect(paths).To(ContainElement("/usr/local/bin/virtwork-disk-setup.sh"))
		Expect(paths).To(ContainElement("/etc/fio/mixed-rw.fio"))
		Expect(paths).To(ContainElement("/etc/fio/seq-write.fio"))
		Expect(paths).To(ContainElement("/etc/systemd/system/virtwork-disk.service"))
	})

	It("should have data volume template", func() {
		dvts, err := w.DataVolumeTemplates()
		Expect(err).NotTo(HaveOccurred())
		Expect(dvts).To(HaveLen(1))
		Expect(dvts[0].Name).To(Equal("virtwork-disk-data"))
	})

	It("should have extra disk for data volume with serial", func() {
		disks := w.ExtraDisks()
		Expect(disks).To(HaveLen(1))
		Expect(disks[0].Name).To(Equal("datadisk"))
		Expect(disks[0].Serial).To(Equal("virtwork-disk"))

		volumes := w.ExtraVolumes()
		Expect(volumes).To(HaveLen(1))
		Expect(volumes[0].Name).To(Equal("datadisk"))
	})

	It("should include disk-setup script with serial discovery", func() {
		result, err := w.CloudInitUserdata()
		Expect(err).NotTo(HaveOccurred())

		parsed := parseYAML(result)
		files := parsed["write_files"].([]interface{})

		var setupContent string
		for _, f := range files {
			fm := f.(map[string]interface{})
			if fm["path"] == "/usr/local/bin/virtwork-disk-setup.sh" {
				setupContent = fm["content"].(string)
				break
			}
		}
		Expect(setupContent).NotTo(BeEmpty())
		Expect(setupContent).To(ContainSubstring("virtio-virtwork-disk"))
		Expect(setupContent).To(ContainSubstring("/mnt/data"))
		Expect(setupContent).To(ContainSubstring("mkfs.xfs"))
	})

	It("should not require service", func() {
		Expect(w.RequiresService()).To(BeFalse())
		Expect(w.ServiceSpec()).To(BeNil())
	})

	Context("param wiring", func() {
		It("should use default param values when Params is nil", func() {
			result, err := w.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			filesByPath := map[string]string{}
			for _, f := range files {
				fm := f.(map[string]interface{})
				filesByPath[fm["path"].(string)] = fm["content"].(string)
			}

			mixedRW := filesByPath["/etc/fio/mixed-rw.fio"]
			Expect(mixedRW).To(ContainSubstring("bs=4k"))
			Expect(mixedRW).To(ContainSubstring("rwmixread=70"))
			Expect(mixedRW).To(ContainSubstring("numjobs=4"))
			Expect(mixedRW).To(ContainSubstring("runtime=300"))

			seqWrite := filesByPath["/etc/fio/seq-write.fio"]
			Expect(seqWrite).To(ContainSubstring("bs=128k"))
			Expect(seqWrite).To(ContainSubstring("runtime=300"))
		})

		It("should wire custom params from WorkloadConfig.Params", func() {
			custom := workloads.NewDiskWorkload(config.WorkloadConfig{
				Enabled:  new(true),
				VMCount:  1,
				CPUCores: 2,
				Memory:   "2Gi",
				Params: map[string]string{
					"block-size-rw":  "8k",
					"block-size-seq": "256k",
					"rwmixread":      "50",
					"numjobs":        "8",
					"runtime":        "600",
				},
			}, "10Gi", "virtwork", "", nil)

			result, err := custom.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			filesByPath := map[string]string{}
			for _, f := range files {
				fm := f.(map[string]interface{})
				filesByPath[fm["path"].(string)] = fm["content"].(string)
			}

			mixedRW := filesByPath["/etc/fio/mixed-rw.fio"]
			Expect(mixedRW).To(ContainSubstring("bs=8k"))
			Expect(mixedRW).To(ContainSubstring("rwmixread=50"))
			Expect(mixedRW).To(ContainSubstring("numjobs=8"))
			Expect(mixedRW).To(ContainSubstring("runtime=600"))

			seqWrite := filesByPath["/etc/fio/seq-write.fio"]
			Expect(seqWrite).To(ContainSubstring("bs=256k"))
			Expect(seqWrite).To(ContainSubstring("runtime=600"))
		})

		It("should use defaults for missing individual params", func() {
			partial := workloads.NewDiskWorkload(config.WorkloadConfig{
				Enabled:  new(true),
				VMCount:  1,
				CPUCores: 2,
				Memory:   "2Gi",
				Params: map[string]string{
					"block-size-rw": "16k",
					"runtime":       "120",
				},
			}, "10Gi", "virtwork", "", nil)

			result, err := partial.CloudInitUserdata()
			Expect(err).NotTo(HaveOccurred())

			parsed := parseYAML(result)
			files := parsed["write_files"].([]interface{})

			filesByPath := map[string]string{}
			for _, f := range files {
				fm := f.(map[string]interface{})
				filesByPath[fm["path"].(string)] = fm["content"].(string)
			}

			mixedRW := filesByPath["/etc/fio/mixed-rw.fio"]
			Expect(mixedRW).To(ContainSubstring("bs=16k"))
			Expect(mixedRW).To(ContainSubstring("rwmixread=70"))
			Expect(mixedRW).To(ContainSubstring("numjobs=4"))
			Expect(mixedRW).To(ContainSubstring("runtime=120"))

			seqWrite := filesByPath["/etc/fio/seq-write.fio"]
			Expect(seqWrite).To(ContainSubstring("bs=128k"))
			Expect(seqWrite).To(ContainSubstring("runtime=120"))
		})
	})
})
