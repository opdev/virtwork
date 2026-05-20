// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/logging"
)

func TestLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logging Suite")
}

var _ = Describe("NewLogger", func() {
	var buf *bytes.Buffer

	BeforeEach(func() {
		buf = &bytes.Buffer{}
	})

	Context("when verbose is false", func() {
		It("should create a logger with Info level", func() {
			logger := logging.NewLogger(buf, false)
			Expect(logger).NotTo(BeNil())

			// Log at debug level - should not appear
			logger.Debug("debug message")
			Expect(buf.String()).To(BeEmpty())

			// Log at info level - should appear
			buf.Reset()
			logger.Info("info message")
			Expect(buf.String()).To(ContainSubstring("info message"))
		})

		It("should output JSON format", func() {
			logger := logging.NewLogger(buf, false)
			logger.Info("test message", slog.String("key", "value"))

			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			Expect(err).NotTo(HaveOccurred())
			Expect(logEntry["msg"]).To(Equal("test message"))
			Expect(logEntry["key"]).To(Equal("value"))
		})
	})

	Context("when verbose is true", func() {
		It("should create a logger with Debug level", func() {
			logger := logging.NewLogger(buf, true)
			Expect(logger).NotTo(BeNil())

			// Log at debug level - should appear
			logger.Debug("debug message")
			Expect(buf.String()).To(ContainSubstring("debug message"))

			// Verify debug level in JSON
			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			Expect(err).NotTo(HaveOccurred())
			Expect(logEntry["level"]).To(Equal("DEBUG"))
		})
	})

	It("should support structured fields", func() {
		logger := logging.NewLogger(buf, false)
		logger.Info("vm created",
			slog.String("vm_name", "test-vm"),
			slog.String("namespace", "test-ns"),
			slog.Int("cpu_cores", 4),
		)

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		Expect(err).NotTo(HaveOccurred())
		Expect(logEntry["vm_name"]).To(Equal("test-vm"))
		Expect(logEntry["namespace"]).To(Equal("test-ns"))
		Expect(logEntry["cpu_cores"]).To(BeNumerically("==", 4))
	})
})
