// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("validateE", func() {
	var catalogDir string

	writeFile := func(dir, name, content string) {
		Expect(os.MkdirAll(dir, 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)).To(Succeed())
	}

	BeforeEach(func() {
		var err error
		catalogDir, err = os.MkdirTemp("", "virtwork-validate-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(catalogDir)).To(Succeed())
	})

	It("should succeed for a valid catalog entry", func() {
		entryDir := filepath.Join(catalogDir, "good")
		writeFile(entryDir, "workload.yaml", "description: test\n")
		writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/test\n")

		var out bytes.Buffer
		cmd := newValidateCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--catalog-dir", catalogDir})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("good"))
		Expect(out.String()).To(ContainSubstring("OK"))
	})

	It("should fail for a nonexistent catalog directory", func() {
		cmd := newValidateCmd()
		cmd.SetArgs([]string{"--catalog-dir", "/nonexistent/path"})
		Expect(cmd.Execute()).NotTo(Succeed())
	})

	It("should validate only named entries when positional args given", func() {
		entryDir := filepath.Join(catalogDir, "one")
		writeFile(entryDir, "workload.yaml", "description: test\n")
		writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/test\n")
		otherDir := filepath.Join(catalogDir, "two")
		writeFile(otherDir, "workload.yaml", "description: test2\n")
		writeFile(otherDir, "workload.service", "[Service]\nExecStart=/bin/test2\n")

		var out bytes.Buffer
		cmd := newValidateCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--catalog-dir", catalogDir, "one"})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("one"))
		Expect(out.String()).NotTo(ContainSubstring("two"))
	})

	It("should exit non-zero when an entry fails validation", func() {
		entryDir := filepath.Join(catalogDir, "bad")
		Expect(os.MkdirAll(entryDir, 0o750)).To(Succeed())

		cmd := newValidateCmd()
		cmd.SetArgs([]string{"--catalog-dir", catalogDir})
		Expect(cmd.Execute()).NotTo(Succeed())
	})

	It("should report placeholder warnings without failing", func() {
		entryDir := filepath.Join(catalogDir, "warn")
		writeFile(
			entryDir,
			"workload.yaml",
			"description: test\nparams:\n  - key: unused\n    type: string\n    default: x\n",
		)
		writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/test\n")

		var out bytes.Buffer
		cmd := newValidateCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--catalog-dir", catalogDir})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("unused"))
	})

	It("should fail when a placeholder has no matching param", func() {
		entryDir := filepath.Join(catalogDir, "typo")
		writeFile(entryDir, "workload.yaml", "description: test\n")
		writeFile(entryDir, "workload.service", "[Service]\nExecStart=/bin/test --x={{oops}}\n")

		cmd := newValidateCmd()
		cmd.SetArgs([]string{"--catalog-dir", catalogDir})
		Expect(cmd.Execute()).NotTo(Succeed())
	})
})
