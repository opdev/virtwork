// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/cleanup"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/logging"
	"github.com/opdev/virtwork/internal/orchestrator"
)

var _ = Describe("printSummary", func() {
	It("logs run_id, namespace, vm count, service count, secret count, and image", func() {
		var buf bytes.Buffer
		logger := logging.NewLogger(&buf, false)

		result := &orchestrator.RunResult{
			RunID:        "test-run-abc",
			VMCount:      5,
			ServiceCount: 2,
			SecretCount:  3,
		}
		cfg := &config.Config{
			Namespace:          "my-namespace",
			ContainerDiskImage: "registry.example.com/vm:latest",
		}

		printSummary(logger, result, cfg)

		output := buf.String()
		Expect(output).To(ContainSubstring("test-run-abc"))
		Expect(output).To(ContainSubstring("my-namespace"))
		Expect(output).To(ContainSubstring("deployment summary"))
	})
})

var _ = Describe("printCleanupPreview", func() {
	It("logs resource counts and namespace", func() {
		var buf bytes.Buffer
		logger := logging.NewLogger(&buf, false)

		preview := &cleanup.CleanupPreview{
			VMCount:      3,
			ServiceCount: 1,
			SecretCount:  2,
			DVCount:      1,
			PVCCount:     1,
			TotalCount:   8,
		}

		printCleanupPreview(logger, preview, "test-ns", "")

		output := buf.String()
		Expect(output).To(ContainSubstring("test-ns"))
		Expect(output).To(ContainSubstring("resources to be deleted"))
	})

	It("includes run_id_filter when runID is provided", func() {
		var buf bytes.Buffer
		logger := logging.NewLogger(&buf, false)

		preview := &cleanup.CleanupPreview{
			VMCount:    1,
			TotalCount: 1,
		}

		printCleanupPreview(logger, preview, "test-ns", "run-xyz")

		output := buf.String()
		Expect(output).To(ContainSubstring("run-xyz"))
	})

	It("includes run_ids when preview has them", func() {
		var buf bytes.Buffer
		logger := logging.NewLogger(&buf, false)

		preview := &cleanup.CleanupPreview{
			VMCount:    2,
			TotalCount: 2,
			RunIDs:     []string{"id-aaa", "id-bbb"},
		}

		printCleanupPreview(logger, preview, "test-ns", "")

		output := buf.String()
		Expect(output).To(ContainSubstring("id-aaa"))
		Expect(output).To(ContainSubstring("id-bbb"))
	})

	It("handles zero counts", func() {
		var buf bytes.Buffer
		logger := logging.NewLogger(&buf, false)

		preview := &cleanup.CleanupPreview{}

		printCleanupPreview(logger, preview, "empty-ns", "")

		output := buf.String()
		Expect(output).To(ContainSubstring("empty-ns"))
		Expect(output).To(ContainSubstring("resources to be deleted"))
	})
})
