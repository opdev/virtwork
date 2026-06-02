// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package orchestrator_test

import (
	"bytes"
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/logging"
	"github.com/opdev/virtwork/internal/orchestrator"
)

func newFakeClient(objs ...client.Object) client.Client {
	scheme := cluster.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		runtimeObjs := make([]runtime.Object, len(objs))
		for i, o := range objs {
			runtimeObjs[i] = o
		}
		builder = builder.WithRuntimeObjects(runtimeObjs...)
	}
	return builder.Build()
}

func defaultConfig() *config.Config {
	return &config.Config{
		Namespace:           constants.DefaultNamespace,
		ContainerDiskImage:  constants.DefaultContainerDiskImage,
		DataDiskSize:        constants.DefaultDiskSize,
		CPUCores:            constants.DefaultCPUCores,
		Memory:              constants.DefaultMemory,
		SSHUser:             constants.DefaultSSHUser,
		WaitForReady:        false,
		ReadyTimeoutSeconds: 600,
		AuditEnabled:        false,
	}
}

var _ = Describe("RunOrchestrator", func() {
	var (
		ctx     context.Context
		buf     *bytes.Buffer
		cfg     *config.Config
		auditor audit.Auditor
	)

	BeforeEach(func() {
		ctx = context.Background()
		buf = &bytes.Buffer{}
		cfg = defaultConfig()
		auditor = audit.NoOpAuditor{}
	})

	Describe("Run", func() {
		Context("dry-run mode", func() {
			BeforeEach(func() {
				cfg.DryRun = true
			})

			It("should build plans and print YAML without requiring a client", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.VMCount).To(Equal(1))

				output := buf.String()
				Expect(output).To(ContainSubstring("virtwork-cpu-0"))

				By("rendering VMs with secretRef instead of inline userData")
				Expect(output).To(ContainSubstring("secretRef:"))
				Expect(output).To(ContainSubstring("name: virtwork-cpu-0-cloudinit"))

				By("rendering cloud-init Secret YAML")
				Expect(output).To(ContainSubstring("kind: Secret"))
				Expect(output).To(ContainSubstring("virtwork-cpu-0-cloudinit"))
			})

			It("should handle multiple workloads in dry-run", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu", "memory"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(2))
			})

			It("should respect vm-count flag in dry-run", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(3))
			})

			It("should handle multi-VM workloads in dry-run", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"network"}, 2)
				Expect(err).NotTo(HaveOccurred())
				// network workload with VMCount=2 creates 4 VMs (2 server + 2 client)
				Expect(result.VMCount).To(Equal(4))

				output := buf.String()

				By("rendering Service YAML for workloads that require it")
				Expect(output).To(ContainSubstring("kind: Service"))

				By("rendering Secrets for each VM")
				Expect(output).To(ContainSubstring("virtwork-network-server-0-cloudinit"))
				Expect(output).To(ContainSubstring("virtwork-network-client-0-cloudinit"))
			})
		})

		Context("workload filtering", func() {
			It("should skip disabled workloads", func() {
				cfg.DryRun = true
				enabled := false
				cfg.Workloads = map[string]config.WorkloadConfig{
					"cpu": {Enabled: &enabled},
				}

				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(0))
			})

			It("should include explicitly enabled workloads", func() {
				cfg.DryRun = true
				enabled := true
				cfg.Workloads = map[string]config.WorkloadConfig{
					"cpu": {Enabled: &enabled, VMCount: 2},
				}

				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(2))
			})

			It("should return error for unknown workload", func() {
				cfg.DryRun = true
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"nonexistent"}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("nonexistent"))
			})
		})

		Context("normal mode with fake client", func() {
			var c client.Client

			BeforeEach(func() {
				c = newFakeClient()
			})

			It("should create namespace, services, secrets, and VMs", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(1))
				Expect(result.SecretCount).To(Equal(1))

				// Verify namespace was created
				ns := &corev1.Namespace{}
				err = c.Get(ctx, client.ObjectKey{Name: constants.DefaultNamespace}, ns)
				Expect(err).NotTo(HaveOccurred())
				Expect(ns.Labels[constants.LabelManagedBy]).To(Equal(constants.ManagedByValue))

				// Verify VM was created
				vmObj := &kubevirtv1.VirtualMachine{}
				err = c.Get(ctx, client.ObjectKey{
					Name:      "virtwork-cpu-0",
					Namespace: constants.DefaultNamespace,
				}, vmObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmObj.Labels[constants.LabelRunID]).To(Equal("test-run-id"))

				// Verify secret was created
				secret := &corev1.Secret{}
				err = c.Get(ctx, client.ObjectKey{
					Name:      "virtwork-cpu-0-cloudinit",
					Namespace: constants.DefaultNamespace,
				}, secret)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should create service for workloads that require it", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"network"}, 2)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.ServiceCount).To(BeNumerically(">", 0))
			})

			It("should set run-id labels on all resources", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				_, err := ro.Run(ctx, 0, "my-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())

				vmObj := &kubevirtv1.VirtualMachine{}
				err = c.Get(ctx, client.ObjectKey{
					Name:      "virtwork-cpu-0",
					Namespace: constants.DefaultNamespace,
				}, vmObj)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmObj.Labels[constants.LabelRunID]).To(Equal("my-run-id"))
			})

			It("should handle per-workload config overrides from YAML", func() {
				cfg.Workloads = map[string]config.WorkloadConfig{
					"cpu": {CPUCores: 8, Memory: "16Gi"},
				}
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(1))
			})

			It("should create multiple VMs concurrently", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 3)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(3))

				for i := range 3 {
					vmObj := &kubevirtv1.VirtualMachine{}
					err := c.Get(ctx, client.ObjectKey{
						Name:      fmt.Sprintf("virtwork-cpu-%d", i),
						Namespace: constants.DefaultNamespace,
					}, vmObj)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("should create multiple secrets concurrently", func() {
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 5)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.SecretCount).To(Equal(5))

				for i := range 5 {
					secret := &corev1.Secret{}
					err := c.Get(ctx, client.ObjectKey{
						Name:      fmt.Sprintf("virtwork-cpu-%d-cloudinit", i),
						Namespace: constants.DefaultNamespace,
					}, secret)
					Expect(err).NotTo(HaveOccurred())
					Expect(secret.Labels[constants.LabelRunID]).To(Equal("test-run-id"))
				}
			})
		})

		Context("workloads with DataVolumes", func() {
			It("should namespace DataVolume names across VMs", func() {
				cfg.DryRun = true
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"disk"}, 2)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(2))
			})
		})

		Context("RunResult", func() {
			It("should set RunID from provided value", func() {
				cfg.DryRun = true
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "expected-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RunID).To(Equal("expected-run-id"))
			})
		})
	})
})
