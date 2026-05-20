// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package testutil_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/testutil"
)

var _ = Describe("BinaryPath", func() {
	var originalEnv string

	BeforeEach(func() {
		originalEnv = os.Getenv("VIRTWORK_BINARY")
	})

	AfterEach(func() {
		if originalEnv != "" {
			_ = os.Setenv("VIRTWORK_BINARY", originalEnv)
		} else {
			_ = os.Unsetenv("VIRTWORK_BINARY")
		}
	})

	Context("when VIRTWORK_BINARY environment variable is set", func() {
		It("should return the path from the environment variable", func() {
			expectedPath := "/custom/path/to/virtwork"
			_ = os.Setenv("VIRTWORK_BINARY", expectedPath)

			path, err := testutil.BinaryPath()
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal(expectedPath))
		})
	})

	Context("when VIRTWORK_BINARY environment variable is not set", func() {
		BeforeEach(func() {
			_ = os.Unsetenv("VIRTWORK_BINARY")
		})

		It("should build and return the binary path", func() {
			path, err := testutil.BinaryPath()
			Expect(err).NotTo(HaveOccurred())
			Expect(path).NotTo(BeEmpty())
			Expect(path).To(BeAnExistingFile())
		})

		It("should return a path in a temp directory", func() {
			path, err := testutil.BinaryPath()
			Expect(err).NotTo(HaveOccurred())
			Expect(filepath.Dir(path)).To(ContainSubstring("virtwork-e2e"))
		})

		It("should cache the built binary on subsequent calls", func() {
			path1, err := testutil.BinaryPath()
			Expect(err).NotTo(HaveOccurred())

			path2, err := testutil.BinaryPath()
			Expect(err).NotTo(HaveOccurred())

			Expect(path1).To(Equal(path2))
		})
	})
})

var _ = Describe("RunVirtwork", func() {
	Context("when running the virtwork binary with valid arguments", func() {
		It("should execute successfully with --help flag", func() {
			stdout, _, exitCode, err := testutil.RunVirtwork("--help")
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(0))
			Expect(stdout).To(ContainSubstring("virtwork"))
			Expect(stdout).To(ContainSubstring("Usage:"))
		})

		It("should execute successfully with help command", func() {
			stdout, _, exitCode, err := testutil.RunVirtwork("help")
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(0))
			Expect(stdout).To(ContainSubstring("virtwork"))
		})

		It("should show available commands in help output", func() {
			stdout, _, exitCode, err := testutil.RunVirtwork("--help")
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(0))
			Expect(stdout).To(ContainSubstring("Available Commands"))
			Expect(stdout).To(ContainSubstring("run"))
			Expect(stdout).To(ContainSubstring("cleanup"))
		})
	})

	Context("when running with invalid arguments", func() {
		It("should return non-zero exit code for invalid flags", func() {
			_, _, exitCode, err := testutil.RunVirtwork("--invalid-flag-that-does-not-exist")
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).NotTo(Equal(0))
		})

		It("should return non-zero exit code for invalid commands", func() {
			_, _, exitCode, err := testutil.RunVirtwork("invalid-command")
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).NotTo(Equal(0))
		})
	})

	Context("when running without arguments", func() {
		It("should show help message", func() {
			stdout, _, exitCode, err := testutil.RunVirtwork()
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(0))
			Expect(stdout).To(ContainSubstring("virtwork"))
			Expect(stdout).To(ContainSubstring("Usage:"))
		})
	})
})
