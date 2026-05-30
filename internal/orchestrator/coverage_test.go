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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/logging"
	"github.com/opdev/virtwork/internal/orchestrator"
)

func fakeClientWithInterceptors(
	funcs interceptor.Funcs,
	objs ...client.Object,
) client.Client {
	scheme := cluster.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(funcs)
	if len(objs) > 0 {
		runtimeObjs := make([]runtime.Object, len(objs))
		for i, o := range objs {
			runtimeObjs[i] = o
		}
		builder = builder.WithRuntimeObjects(runtimeObjs...)
	}
	return builder.Build()
}

var _ = Describe("RunOrchestrator coverage", func() {
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

	Describe("waitForReadiness", func() {
		Context("when WaitForReady is true and all VMs are ready", func() {
			It("should succeed when VMIs are in Running phase", func() {
				cfg.WaitForReady = true
				cfg.ReadyTimeoutSeconds = 10

				vmi := &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
					},
				}
				c := newFakeClient(vmi)
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.VMCount).To(Equal(1))
			})
		})

		Context("when WaitForReady is true and VMs fail readiness", func() {
			It("should return ErrReadinessCheck when VMIs are not found", func() {
				cfg.WaitForReady = true
				cfg.ReadyTimeoutSeconds = 1

				c := newFakeClient()
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed"))
				Expect(err.Error()).To(ContainSubstring("1 of 1"))
			})
		})

		Context("when WaitForReady is true with partial readiness", func() {
			It("should report the correct failure count", func() {
				cfg.WaitForReady = true
				cfg.ReadyTimeoutSeconds = 1

				vmi0 := &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
					},
				}
				c := newFakeClient(vmi0)
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 2)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("1 of 2"))
			})
		})

		Context("when WaitForReady is true with audit errors", func() {
			It("should log warnings but still report readiness failure", func() {
				cfg.WaitForReady = true
				cfg.ReadyTimeoutSeconds = 1

				c := newFakeClient()
				logger := logging.NewLogger(buf, false)
				fa := failingAuditor{}
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, fa, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).To(HaveOccurred())

				output := buf.String()
				Expect(output).To(ContainSubstring("WARN"))
				Expect(output).To(ContainSubstring("audit"))
			})

			It("should log warnings for audit failures on ready VMs", func() {
				cfg.WaitForReady = true
				cfg.ReadyTimeoutSeconds = 10

				vmi := &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						Phase: kubevirtv1.Running,
					},
				}
				c := newFakeClient(vmi)
				logger := logging.NewLogger(buf, false)
				fa := failingAuditor{}
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, fa, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())

				output := buf.String()
				Expect(output).To(ContainSubstring("WARN"))
			})
		})
	})

	Describe("createVMs error paths", func() {
		Context("when VM creation fails", func() {
			It("should return error and update workload status to failed", func() {
				vmCreateErr := fmt.Errorf("quota exceeded")
				callCount := 0
				c := fakeClientWithInterceptors(
					interceptor.Funcs{
						Create: func(
							ctx context.Context,
							cl client.WithWatch,
							obj client.Object,
							opts ...client.CreateOption,
						) error {
							if _, ok := obj.(*kubevirtv1.VirtualMachine); ok {
								callCount++
								return vmCreateErr
							}
							return cl.Create(ctx, obj, opts...)
						},
					},
				)
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("creating VMs"))
				Expect(callCount).To(Equal(1))
			})

			It("should log audit warnings when failing auditor and VM creation fails", func() {
				c := fakeClientWithInterceptors(
					interceptor.Funcs{
						Create: func(
							ctx context.Context,
							cl client.WithWatch,
							obj client.Object,
							opts ...client.CreateOption,
						) error {
							if _, ok := obj.(*kubevirtv1.VirtualMachine); ok {
								return fmt.Errorf("vm create failed")
							}
							return cl.Create(ctx, obj, opts...)
						},
					},
				)
				logger := logging.NewLogger(buf, false)
				fa := failingAuditor{}
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, fa, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).To(HaveOccurred())

				output := buf.String()
				Expect(output).To(ContainSubstring("WARN"))
				Expect(output).To(ContainSubstring("UpdateWorkloadStatus"))
			})
		})

		Context("when VMConcurrency is set", func() {
			It("should create all VMs with concurrency limit", func() {
				cfg.VMConcurrency = 2
				c := newFakeClient()
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 4)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMCount).To(Equal(4))

				for i := range 4 {
					vmObj := &kubevirtv1.VirtualMachine{}
					err := c.Get(ctx, client.ObjectKey{
						Name:      fmt.Sprintf("virtwork-cpu-%d", i),
						Namespace: constants.DefaultNamespace,
					}, vmObj)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})
	})

	Describe("createResources error paths", func() {
		Context("when EnsureNamespace fails", func() {
			It("should return namespace error", func() {
				c := fakeClientWithInterceptors(
					interceptor.Funcs{
						Create: func(
							ctx context.Context,
							cl client.WithWatch,
							obj client.Object,
							opts ...client.CreateOption,
						) error {
							if _, ok := obj.(*corev1.Namespace); ok {
								return apierrors.NewForbidden(
									schema.GroupResource{Group: "", Resource: "namespaces"},
									constants.DefaultNamespace,
									fmt.Errorf("forbidden"),
								)
							}
							return cl.Create(ctx, obj, opts...)
						},
					},
				)
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("namespace"))
			})
		})

		Context("when CreateService fails", func() {
			It("should return service creation error", func() {
				c := fakeClientWithInterceptors(
					interceptor.Funcs{
						Create: func(
							ctx context.Context,
							cl client.WithWatch,
							obj client.Object,
							opts ...client.CreateOption,
						) error {
							if _, ok := obj.(*corev1.Service); ok {
								return fmt.Errorf("service quota exceeded")
							}
							return cl.Create(ctx, obj, opts...)
						},
					},
				)
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"network"}, 2)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("service"))
			})
		})

		Context("when secret creation fails", func() {
			It("should return secret creation error", func() {
				c := fakeClientWithInterceptors(
					interceptor.Funcs{
						Create: func(
							ctx context.Context,
							cl client.WithWatch,
							obj client.Object,
							opts ...client.CreateOption,
						) error {
							if secret, ok := obj.(*corev1.Secret); ok {
								if _, hasLabel := secret.Labels[constants.LabelComponent]; hasLabel {
									return fmt.Errorf("secret creation denied")
								}
							}
							return cl.Create(ctx, obj, opts...)
						},
					},
				)
				logger := logging.NewLogger(buf, false)
				ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

				_, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cloud-init secret"))
			})
		})
	})

	Describe("context cancellation", func() {
		It("should propagate context cancellation during VM creation", func() {
			cancelCtx, cancel := context.WithCancel(ctx)

			c := fakeClientWithInterceptors(
				interceptor.Funcs{
					Create: func(
						ctx context.Context,
						cl client.WithWatch,
						obj client.Object,
						opts ...client.CreateOption,
					) error {
						if _, ok := obj.(*kubevirtv1.VirtualMachine); ok {
							cancel()
							return ctx.Err()
						}
						return cl.Create(ctx, obj, opts...)
					},
				},
			)
			logger := logging.NewLogger(buf, false)
			ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, buf)

			_, err := ro.Run(cancelCtx, 0, "test-run-id", []string{"cpu"}, 1)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("workload config overrides", func() {
		It("should apply Params from per-workload config", func() {
			cfg.DryRun = true
			cfg.Workloads = map[string]config.WorkloadConfig{
				"cpu": {
					Params: map[string]string{"stress-ng-args": "--cpu 4"},
				},
			}
			logger := logging.NewLogger(buf, false)
			ro := orchestrator.NewRunOrchestrator(logger, nil, cfg, auditor, buf)

			result, err := ro.Run(ctx, 0, "test-run-id", []string{"cpu"}, 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.VMCount).To(Equal(1))
		})
	})
})

var _ = Describe("CleanupOrchestrator coverage", func() {
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

	Describe("Execute", func() {
		Context("with run-id filtering", func() {
			It("should only delete resources matching the run-id", func() {
				vm1 := &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
							constants.LabelRunID:     "run-keep",
						},
					},
				}
				vm2 := &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-1",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
							constants.LabelRunID:     "run-delete",
						},
					},
				}
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.DefaultNamespace,
					},
				}
				c := newFakeClient(ns, vm1, vm2)
				logger := logging.NewLogger(buf, false)
				co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

				result, err := co.Execute(ctx, 0, false, "run-delete")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMsDeleted).To(Equal(1))

				vmObj := &kubevirtv1.VirtualMachine{}
				err = c.Get(ctx, client.ObjectKey{
					Name:      "virtwork-cpu-0",
					Namespace: constants.DefaultNamespace,
				}, vmObj)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should record RunIDs and link cleanup to runs", func() {
				vm := &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
							constants.LabelRunID:     "tracked-run",
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
				co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

				result, err := co.Execute(ctx, 0, false, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMsDeleted).To(Equal(1))
				Expect(result.RunIDs).To(ContainElement("tracked-run"))
			})
		})

		Context("with mixed resource states", func() {
			It("should delete VMs, secrets, and services together", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.DefaultNamespace,
					},
				}
				vm := &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
						},
					},
				}
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0-cloudinit",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
						},
					},
				}
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-network-server",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
						},
					},
				}
				c := newFakeClient(ns, vm, secret, svc)
				logger := logging.NewLogger(buf, false)
				co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

				result, err := co.Execute(ctx, 0, false, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMsDeleted).To(Equal(1))
				Expect(result.SecretsDeleted).To(Equal(1))
				Expect(result.ServicesDeleted).To(Equal(1))
			})
		})

		Context("when audit errors occur during cleanup", func() {
			It("should log LinkCleanupToRuns audit failures", func() {
				vm := &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "virtwork-cpu-0",
						Namespace: constants.DefaultNamespace,
						Labels: map[string]string{
							constants.LabelManagedBy: constants.ManagedByValue,
							constants.LabelRunID:     "some-run",
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
				fa := failingAuditor{}
				co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, fa, buf)

				result, err := co.Execute(ctx, 0, false, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.VMsDeleted).To(Equal(1))

				output := buf.String()
				Expect(output).To(ContainSubstring("LinkCleanupToRuns"))
			})
		})
	})

	Describe("Preview", func() {
		Context("when preview encounters an error", func() {
			It("should return wrapped error", func() {
				c := fakeClientWithInterceptors(
					interceptor.Funcs{
						List: func(
							ctx context.Context,
							cl client.WithWatch,
							list client.ObjectList,
							opts ...client.ListOption,
						) error {
							if _, ok := list.(*kubevirtv1.VirtualMachineList); ok {
								return fmt.Errorf("api server unavailable")
							}
							return cl.List(ctx, list, opts...)
						},
					},
				)
				logger := logging.NewLogger(buf, false)
				co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, buf)

				_, err := co.Preview(ctx, 0, "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("previewing cleanup"))
			})
		})
	})
})
