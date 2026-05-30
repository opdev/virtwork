// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package orchestrator_test

import (
	"bytes"
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/logging"
	"github.com/opdev/virtwork/internal/orchestrator"
)

var errAuditWrite = errors.New("audit: disk full")

// failingAuditor implements audit.Auditor and returns errors from every method
// except StartExecution (which must succeed so the caller gets a valid execID).
type failingAuditor struct {
	audit.NoOpAuditor
}

func (failingAuditor) CompleteExecution(_ context.Context, _ int64, _ string, _ string) error {
	return errAuditWrite
}

func (failingAuditor) LinkCleanupToRuns(_ context.Context, _ int64, _ []string) error {
	return errAuditWrite
}

func (failingAuditor) RecordCleanupCounts(_ context.Context, _ int64, _, _, _, _, _ int, _ bool) error {
	return errAuditWrite
}

func (failingAuditor) RecordWorkload(_ context.Context, _ int64, _ audit.WorkloadRecord) (int64, error) {
	return 0, errAuditWrite
}

func (failingAuditor) UpdateWorkloadStatus(_ context.Context, _ int64, _ string) error {
	return errAuditWrite
}

func (failingAuditor) RecordVM(_ context.Context, _ int64, _ int64, _ audit.VMRecord) (int64, error) {
	return 0, errAuditWrite
}

func (failingAuditor) RecordResource(_ context.Context, _ int64, _ audit.ResourceRecord) (int64, error) {
	return 0, errAuditWrite
}

func (failingAuditor) RecordEvent(_ context.Context, _ int64, _ audit.EventRecord) error {
	return errAuditWrite
}

func (failingAuditor) Close() error {
	return errAuditWrite
}

var _ = Describe("Audit error logging", func() {
	var (
		ctx context.Context
		buf *bytes.Buffer
		cfg *config.Config
		fa  failingAuditor
	)

	BeforeEach(func() {
		ctx = context.Background()
		buf = &bytes.Buffer{}
		cfg = defaultConfig()
		fa = failingAuditor{}
	})

	Describe("RunOrchestrator", func() {
		Context("when auditor returns errors", func() {
			It("should still succeed in dry-run mode", func() {
				cfg.DryRun = true
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, fa, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.VMCount).To(Equal(1))
			})

			It("should log warnings for failed audit calls in dry-run", func() {
				cfg.DryRun = true
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, fa, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())

				output := buf.String()
				Expect(output).To(ContainSubstring("WARN"))
				Expect(output).To(ContainSubstring("audit"))
				Expect(output).To(ContainSubstring("disk full"))
			})

			It("should still succeed in normal mode with fake client", func() {
				logger := logging.NewLogger(buf, false)
				c := newFakeClient()
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, fa, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.VMCount).To(Equal(1))
			})

			It("should log warnings for RecordResource and RecordVM failures", func() {
				logger := logging.NewLogger(buf, false)
				c := newFakeClient()
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, fa, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())

				output := buf.String()
				Expect(output).To(ContainSubstring("WARN"))
				Expect(output).To(ContainSubstring("disk full"))
			})
		})
	})

	Describe("CleanupOrchestrator", func() {
		Context("when auditor returns errors", func() {
			It("should still succeed when deleting resources", func() {
				vm := &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
						},
					},
				}
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.DefaultNamespace,
					},
				}
				c := newFakeClient(ns, vm)
				logger := logging.NewLogger(buf, false)
				co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, fa, buf)

				result, err := co.Execute(ctx, 0, false, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMsDeleted).To(Equal(1))
			})

			It("should log warnings for failed audit calls during cleanup", func() {
				vm := &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
						},
					},
				}
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.DefaultNamespace,
					},
				}
				c := newFakeClient(ns, vm)
				logger := logging.NewLogger(buf, false)
				co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, fa, buf)

				_, err := co.Execute(ctx, 0, false, "")
				Expect(err).NotTo(HaveOccurred())

				output := buf.String()
				Expect(output).To(ContainSubstring("WARN"))
				Expect(output).To(ContainSubstring("audit"))
				Expect(output).To(ContainSubstring("disk full"))
			})
		})
	})
})
