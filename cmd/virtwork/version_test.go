// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Version command", func() {
	It("should execute without error", func() {
		cmd := newVersionCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		Expect(cmd.Execute()).To(Succeed())
	})

	It("should print virtwork in the output", func() {
		cmd := newVersionCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		Expect(cmd.Execute()).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("virtwork"))
	})

	It("should show dev when version is not set via ldflags", func() {
		cmd := newVersionCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		Expect(cmd.Execute()).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("(dev)"))
	})

	It("should show unknown when commit is not set via ldflags", func() {
		cmd := newVersionCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		Expect(cmd.Execute()).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("(unknown)"))
	})
})
