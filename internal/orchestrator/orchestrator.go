// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigyaml "sigs.k8s.io/yaml"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/resources"
	"github.com/opdev/virtwork/internal/vm"
	"github.com/opdev/virtwork/internal/wait"
	"github.com/opdev/virtwork/internal/workloads"
)

var ErrReadinessCheck = errors.New("readiness check failed")

// RunOrchestrator coordinates the "run" workflow: planning VMs, creating
// resources, and waiting for readiness. Dependencies are injected at
// construction for testability.
type RunOrchestrator struct {
	logger  *slog.Logger
	client  client.Client
	config  *config.Config
	auditor audit.Auditor
	writer  io.Writer
}

// NewRunOrchestrator creates a RunOrchestrator with the given dependencies.
func NewRunOrchestrator(
	logger *slog.Logger,
	c client.Client,
	cfg *config.Config,
	auditor audit.Auditor,
	writer io.Writer,
) *RunOrchestrator {
	return &RunOrchestrator{
		logger:  logger,
		client:  c,
		config:  cfg,
		auditor: auditor,
		writer:  writer,
	}
}

// Run executes the full orchestration flow: plan VMs, create resources,
// wait for readiness. The caller owns the audit execution lifecycle
// (StartExecution/CompleteExecution); Run records workloads, VMs, resources,
// and events internally.
func (ro *RunOrchestrator) Run(
	ctx context.Context,
	execID int64,
	runID string,
	workloadNames []string,
	vmCountFlag int,
) (*RunResult, error) {
	cfg := ro.config

	registry := workloads.DefaultRegistry()
	registryOpts := []workloads.Option{
		workloads.WithNamespace(cfg.Namespace),
		workloads.WithSSHCredentials(cfg.SSHUser, cfg.SSHPassword, cfg.SSHAuthorizedKeys),
		workloads.WithDataDiskSize(cfg.DataDiskSize),
	}

	plans, vmNames, workloadInstances, auditWorkloadIDs, err := ro.planVMs(
		ctx, execID, runID, workloadNames, vmCountFlag, registry, registryOpts,
	)
	if err != nil {
		return nil, err
	}

	if err := ro.auditor.RecordEvent(ctx, execID, audit.EventRecord{
		EventType: "execution_started",
		Message: fmt.Sprintf(
			"Planned %d VMs across %d workloads",
			len(plans),
			len(workloadNames),
		),
	}); err != nil {
		ro.logger.Warn(
			"audit record failed",
			slog.String("op", "RecordEvent"),
			slog.String("error", err.Error()),
		)
	}

	if cfg.DryRun {
		svcCount, secretCount, err := ro.printDryRun(plans, workloadInstances, runID)
		if err != nil {
			return nil, err
		}
		return &RunResult{
			RunID:        runID,
			VMCount:      len(plans),
			ServiceCount: svcCount,
			SecretCount:  secretCount,
		}, nil
	}

	svcCount, err := ro.createResources(ctx, execID, runID, plans, workloadInstances)
	if err != nil {
		return nil, err
	}

	secretCount, err := ro.createSecrets(ctx, execID, runID, plans)
	if err != nil {
		return nil, err
	}

	if err := ro.createVMs(ctx, execID, cfg, plans, auditWorkloadIDs); err != nil {
		for _, wlID := range auditWorkloadIDs {
			if auditErr := ro.auditor.UpdateWorkloadStatus(ctx, wlID, "failed"); auditErr != nil {
				ro.logger.Warn(
					"audit record failed",
					slog.String("op", "UpdateWorkloadStatus"),
					slog.String("error", auditErr.Error()),
				)
			}
		}
		return nil, fmt.Errorf("creating VMs: %w", err)
	}

	if err := ro.waitForReadiness(ctx, execID, vmNames, auditWorkloadIDs, plans); err != nil {
		return nil, err
	}

	for _, wlID := range auditWorkloadIDs {
		if err := ro.auditor.UpdateWorkloadStatus(ctx, wlID, "created"); err != nil {
			ro.logger.Warn(
				"audit record failed",
				slog.String("op", "UpdateWorkloadStatus"),
				slog.String("error", err.Error()),
			)
		}
	}

	return &RunResult{
		RunID:        runID,
		VMCount:      len(plans),
		ServiceCount: svcCount,
		SecretCount:  secretCount,
	}, nil
}

func (ro *RunOrchestrator) planVMs(
	ctx context.Context,
	execID int64,
	runID string,
	workloadNames []string,
	vmCountFlag int,
	registry workloads.Registry,
	registryOpts []workloads.Option,
) ([]VMPlan, []string, map[string]workloads.Workload, map[string]int64, error) {
	cfg := ro.config

	var plans []VMPlan
	var vmNames []string
	auditWorkloadIDs := make(map[string]int64)
	workloadInstances := make(map[string]workloads.Workload)

	for _, name := range workloadNames {
		if fileCfg, ok := cfg.Workloads[name]; ok {
			if fileCfg.Enabled != nil && !*fileCfg.Enabled {
				if err := ro.auditor.RecordEvent(ctx, execID, audit.EventRecord{
					EventType: "workload_skipped",
					Message: fmt.Sprintf(
						"Workload %q disabled via config (enabled: false)",
						name,
					),
				}); err != nil {
					ro.logger.Warn(
						"audit record failed",
						slog.String("op", "RecordEvent"),
						slog.String("error", err.Error()),
					)
				}
				ro.logger.Info("workload skipped",
					slog.String("workload", name),
					slog.String("reason", "disabled in config"))
				continue
			}
		}

		wlCfg := config.WorkloadConfig{
			Enabled:  new(true),
			VMCount:  vmCountFlag,
			CPUCores: cfg.CPUCores,
			Memory:   cfg.Memory,
		}
		if fileCfg, ok := cfg.Workloads[name]; ok {
			if fileCfg.CPUCores > 0 {
				wlCfg.CPUCores = fileCfg.CPUCores
			}
			if fileCfg.Memory != "" {
				wlCfg.Memory = fileCfg.Memory
			}
			if fileCfg.VMCount > 0 {
				wlCfg.VMCount = fileCfg.VMCount
			}
			if len(fileCfg.Params) > 0 {
				wlCfg.Params = fileCfg.Params
			}
		}

		if len(wlCfg.Params) > 0 {
			if err := registry.ValidateParams(name, wlCfg.Params); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("invalid params for workload %q: %w", name, err)
			}
		}

		w, err := registry.Get(name, wlCfg, registryOpts...)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("creating workload %q: %w", name, err)
		}
		workloadInstances[name] = w

		vmCount := w.VMCount()
		res := w.VMResources()

		dvTemplatesForAudit, err := w.DataVolumeTemplates()
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf(
				"building data volume templates for %q: %w",
				name,
				err,
			)
		}
		wlID, auditErr := ro.auditor.RecordWorkload(ctx, execID, audit.WorkloadRecord{
			WorkloadType:    name,
			Enabled:         true,
			VMCount:         vmCount,
			CPUCores:        res.CPUCores,
			Memory:          res.Memory,
			HasDataDisk:     len(dvTemplatesForAudit) > 0,
			DataDiskSize:    cfg.DataDiskSize,
			RequiresService: w.RequiresService(),
		})
		if auditErr != nil {
			ro.logger.Warn(
				"audit record failed",
				slog.String("op", "RecordWorkload"),
				slog.String("error", auditErr.Error()),
			)
		}
		auditWorkloadIDs[name] = wlID

		if multiVM, isMulti := w.(workloads.MultiVMWorkload); !isMulti {
			userdata, err := w.CloudInitUserdata()
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("generating cloud-init for %q: %w", name, err)
			}

			for i := range vmCount {
				vmName := fmt.Sprintf("virtwork-%s-%d", name, i)

				dvts, err := w.DataVolumeTemplates()
				if err != nil {
					return nil, nil, nil, nil, fmt.Errorf(
						"building data volume templates for %q vm %s: %w",
						name,
						vmName,
						err,
					)
				}
				dvTemplates, extraVols := NamespaceDataVolumes(dvts, w.ExtraVolumes(), vmName)

				plans = append(plans, VMPlan{
					WorkloadName: name,
					Component:    name,
					VMName:       vmName,
					VMSpec: &VMSpecInput{
						Name:               vmName,
						Namespace:          cfg.Namespace,
						ContainerDiskImage: cfg.ContainerDiskImage,
						CloudInitUserdata:  userdata,
						CPUCores:           res.CPUCores,
						Memory:             res.Memory,
						Labels: map[string]string{
							constants.LabelAppName:   fmt.Sprintf("virtwork-%s", name),
							constants.LabelManagedBy: constants.ManagedByValue,
							constants.LabelComponent: name,
							constants.LabelRunID:     runID,
						},
						ExtraDisks:          w.ExtraDisks(),
						ExtraVolumes:        extraVols,
						DataVolumeTemplates: dvTemplates,
					},
				})
				vmNames = append(vmNames, vmName)
			}
		} else {
			for _, rs := range multiVM.RoleDistribution() {
				userdata, err := multiVM.UserdataForRole(rs.Role, cfg.Namespace)
				if err != nil {
					return nil, nil, nil, nil, fmt.Errorf(
						"generating cloud-init for %q role %q: %w",
						name,
						rs.Role,
						err,
					)
				}

				for i := range rs.VMCount {
					vmName := fmt.Sprintf("virtwork-%s-%s-%d", name, rs.Role, i)
					labels := map[string]string{
						constants.LabelAppName:   fmt.Sprintf("virtwork-%s", name),
						constants.LabelManagedBy: constants.ManagedByValue,
						constants.LabelComponent: name,
						constants.LabelRunID:     runID,
						"virtwork/role":          rs.Role,
					}

					dvts, err := w.DataVolumeTemplates()
					if err != nil {
						return nil, nil, nil, nil, fmt.Errorf(
							"building data volume templates for %q role %q vm %s: %w",
							name,
							rs.Role,
							vmName,
							err,
						)
					}
					dvTemplates, extraVols := NamespaceDataVolumes(
						dvts,
						w.ExtraVolumes(),
						vmName,
					)

					plans = append(plans, VMPlan{
						WorkloadName: name,
						Component:    name,
						VMName:       vmName,
						Role:         rs.Role,
						VMSpec: &VMSpecInput{
							Name:                vmName,
							Namespace:           cfg.Namespace,
							ContainerDiskImage:  cfg.ContainerDiskImage,
							CloudInitUserdata:   userdata,
							CPUCores:            res.CPUCores,
							Memory:              res.Memory,
							Labels:              labels,
							ExtraDisks:          w.ExtraDisks(),
							ExtraVolumes:        extraVols,
							DataVolumeTemplates: dvTemplates,
						},
					})
					vmNames = append(vmNames, vmName)
				}
			}
		}
	}

	return plans, vmNames, workloadInstances, auditWorkloadIDs, nil
}

func (ro *RunOrchestrator) createResources(
	ctx context.Context,
	execID int64,
	runID string,
	plans []VMPlan,
	workloadInstances map[string]workloads.Workload,
) (int, error) {
	cfg := ro.config

	if err := resources.EnsureNamespace(ctx, ro.client, cfg.Namespace, map[string]string{
		constants.LabelManagedBy: constants.ManagedByValue,
	}); err != nil {
		return 0, fmt.Errorf("ensuring namespace %q: %w", cfg.Namespace, err)
	}
	ro.logger.Info("namespace ensured", slog.String("namespace", cfg.Namespace))

	servicesCreated := 0
	for name, w := range workloadInstances {
		if w.RequiresService() {
			svc := w.ServiceSpec()
			if svc != nil {
				if svc.Labels == nil {
					svc.Labels = make(map[string]string)
				}
				svc.Labels[constants.LabelRunID] = runID

				if err := resources.CreateService(ctx, ro.client, svc); err != nil {
					return 0, fmt.Errorf("creating service for %q: %w", name, err)
				}
				servicesCreated++
				ro.logger.Info("service created",
					slog.String("service_name", svc.Name),
					slog.String("namespace", svc.Namespace))

				if _, auditErr := ro.auditor.RecordResource(ctx, execID, audit.ResourceRecord{
					ResourceType: "Service",
					ResourceName: svc.Name,
					Namespace:    svc.Namespace,
				}); auditErr != nil {
					ro.logger.Warn(
						"audit record failed",
						slog.String("op", "RecordResource"),
						slog.String("error", auditErr.Error()),
					)
				}
				if auditErr := ro.auditor.RecordEvent(ctx, execID, audit.EventRecord{
					EventType: "service_created",
					Message:   fmt.Sprintf("Service %s created", svc.Name),
				}); auditErr != nil {
					ro.logger.Warn(
						"audit record failed",
						slog.String("op", "RecordEvent"),
						slog.String("error", auditErr.Error()),
					)
				}
			}
		}
	}

	return servicesCreated, nil
}

func (ro *RunOrchestrator) createSecrets(
	ctx context.Context,
	execID int64,
	runID string,
	plans []VMPlan,
) (int, error) {
	cfg := ro.config

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	var secretsCreated atomic.Int32
	for i := range plans {
		g.Go(func() error {
			secretName := plans[i].VMName + "-cloudinit"
			secretLabels := map[string]string{
				constants.LabelAppName:   plans[i].VMSpec.Labels[constants.LabelAppName],
				constants.LabelManagedBy: constants.ManagedByValue,
				constants.LabelComponent: plans[i].Component,
				constants.LabelRunID:     runID,
			}
			if err := resources.CreateCloudInitSecret(gctx, ro.client, secretName,
				cfg.Namespace, plans[i].VMSpec.CloudInitUserdata, secretLabels); err != nil {
				return fmt.Errorf("creating cloud-init secret for %q: %w", plans[i].VMName, err)
			}
			plans[i].VMSpec.CloudInitSecretName = secretName
			secretsCreated.Add(1)
			ro.logger.Info("secret created",
				slog.String("secret_name", secretName),
				slog.String("namespace", cfg.Namespace))

			if _, auditErr := ro.auditor.RecordResource(ctx, execID, audit.ResourceRecord{
				ResourceType: "Secret",
				ResourceName: secretName,
				Namespace:    cfg.Namespace,
			}); auditErr != nil {
				ro.logger.Warn(
					"audit record failed",
					slog.String("op", "RecordResource"),
					slog.String("error", auditErr.Error()),
				)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return 0, err
	}
	return int(secretsCreated.Load()), nil
}

func (ro *RunOrchestrator) createVMs(
	ctx context.Context,
	execID int64,
	cfg *config.Config,
	plans []VMPlan,
	auditWorkloadIDs map[string]int64,
) error {
	g, gctx := errgroup.WithContext(ctx)
	if cfg.VMConcurrency > 0 {
		g.SetLimit(cfg.VMConcurrency)
	}
	for _, plan := range plans {
		p := plan
		g.Go(func() error {
			vmObj, err := vm.BuildVMSpec(vm.VMSpecOpts{
				Name:                p.VMSpec.Name,
				Namespace:           p.VMSpec.Namespace,
				ContainerDiskImage:  p.VMSpec.ContainerDiskImage,
				CloudInitUserdata:   p.VMSpec.CloudInitUserdata,
				CloudInitSecretName: p.VMSpec.CloudInitSecretName,
				CPUCores:            p.VMSpec.CPUCores,
				Memory:              p.VMSpec.Memory,
				Labels:              p.VMSpec.Labels,
				ExtraDisks:          p.VMSpec.ExtraDisks,
				ExtraVolumes:        p.VMSpec.ExtraVolumes,
				DataVolumeTemplates: p.VMSpec.DataVolumeTemplates,
			})
			if err != nil {
				return fmt.Errorf("building VM spec for %q: %w", p.VMName, err)
			}
			if err := vm.CreateVM(gctx, ro.client, vmObj); err != nil {
				if auditErr := ro.auditor.RecordEvent(ctx, execID, audit.EventRecord{
					EventType:   "vm_failed",
					Message:     fmt.Sprintf("Failed to create VM %s", p.VMName),
					ErrorDetail: err.Error(),
				}); auditErr != nil {
					ro.logger.Warn(
						"audit record failed",
						slog.String("op", "RecordEvent"),
						slog.String("error", auditErr.Error()),
					)
				}
				return fmt.Errorf("creating VM %q: %w", p.VMName, err)
			}
			ro.logger.Info("vm created",
				slog.String("vm_name", p.VMName),
				slog.String("namespace", cfg.Namespace),
				slog.String("workload", p.Component))

			wlID := auditWorkloadIDs[p.Component]
			if _, auditErr := ro.auditor.RecordVM(ctx, execID, wlID, audit.VMRecord{
				VMName:             p.VMName,
				Namespace:          cfg.Namespace,
				Component:          p.Component,
				Role:               p.Role,
				CPUCores:           p.VMSpec.CPUCores,
				Memory:             p.VMSpec.Memory,
				ContainerDiskImage: p.VMSpec.ContainerDiskImage,
				HasDataDisk:        len(p.VMSpec.DataVolumeTemplates) > 0,
				DataDiskSize:       cfg.DataDiskSize,
			}); auditErr != nil {
				ro.logger.Warn(
					"audit record failed",
					slog.String("op", "RecordVM"),
					slog.String("error", auditErr.Error()),
				)
			}
			if auditErr := ro.auditor.RecordEvent(ctx, execID, audit.EventRecord{
				EventType: "vm_created",
				Message:   fmt.Sprintf("VM %s created", p.VMName),
			}); auditErr != nil {
				ro.logger.Warn(
					"audit record failed",
					slog.String("op", "RecordEvent"),
					slog.String("error", auditErr.Error()),
				)
			}
			return nil
		})
	}
	return g.Wait()
}

func (ro *RunOrchestrator) waitForReadiness(
	ctx context.Context,
	execID int64,
	vmNames []string,
	auditWorkloadIDs map[string]int64,
	plans []VMPlan,
) error {
	if !ro.config.WaitForReady {
		return nil
	}

	vmToComponent := make(map[string]string, len(plans))
	for _, p := range plans {
		vmToComponent[p.VMName] = p.Component
	}

	timeout := time.Duration(ro.config.ReadyTimeoutSeconds) * time.Second
	ro.logger.Info("waiting for VMs to become ready",
		slog.Int("vm_count", len(vmNames)),
		slog.Duration("timeout", timeout))
	results := wait.WaitForAllVMsReady(ctx, ro.client, ro.logger, vmNames, ro.config.Namespace,
		timeout, constants.DefaultPollInterval)

	failures := 0
	failedWorkloads := make(map[string]bool)
	for name, err := range results {
		if err != nil {
			ro.logger.Error("vm readiness check failed",
				slog.String("vm_name", name),
				slog.String("error", err.Error()))
			failures++
			if auditErr := ro.auditor.RecordEvent(ctx, execID, audit.EventRecord{
				EventType:   "vm_timeout",
				Message:     fmt.Sprintf("VM %s failed readiness check", name),
				ErrorDetail: err.Error(),
			}); auditErr != nil {
				ro.logger.Warn(
					"audit record failed",
					slog.String("op", "RecordEvent"),
					slog.String("error", auditErr.Error()),
				)
			}
			if comp, ok := vmToComponent[name]; ok {
				failedWorkloads[comp] = true
			}
		} else {
			if auditErr := ro.auditor.RecordEvent(ctx, execID, audit.EventRecord{
				EventType: "vm_ready",
				Message:   fmt.Sprintf("VM %s is ready", name),
			}); auditErr != nil {
				ro.logger.Warn(
					"audit record failed",
					slog.String("op", "RecordEvent"),
					slog.String("error", auditErr.Error()),
				)
			}
		}
	}
	if failures > 0 {
		for comp, wlID := range auditWorkloadIDs {
			if failedWorkloads[comp] {
				if auditErr := ro.auditor.UpdateWorkloadStatus(
					ctx,
					wlID,
					"failed",
				); auditErr != nil {
					ro.logger.Warn(
						"audit record failed",
						slog.String("op", "UpdateWorkloadStatus"),
						slog.String("error", auditErr.Error()),
					)
				}
			} else {
				if auditErr := ro.auditor.UpdateWorkloadStatus(
					ctx,
					wlID,
					"created",
				); auditErr != nil {
					ro.logger.Warn(
						"audit record failed",
						slog.String("op", "UpdateWorkloadStatus"),
						slog.String("error", auditErr.Error()),
					)
				}
			}
		}
		return fmt.Errorf("%d of %d VMs failed; %w", failures, len(vmNames), ErrReadinessCheck)
	}
	ro.logger.Info("all VMs ready", slog.Int("vm_count", len(vmNames)))
	return nil
}

func (ro *RunOrchestrator) printDryRun(
	plans []VMPlan,
	workloadInstances map[string]workloads.Workload,
	runID string,
) (int, int, error) {
	cfg := ro.config
	ro.logger.Info("dry run mode", slog.Int("total_vms", len(plans)))

	svcCount := 0
	for name, w := range workloadInstances {
		if w.RequiresService() {
			svc := w.ServiceSpec()
			if svc != nil {
				svc.TypeMeta = metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				}
				if svc.Labels == nil {
					svc.Labels = make(map[string]string)
				}
				svc.Labels[constants.LabelRunID] = runID

				data, err := sigyaml.Marshal(svc)
				if err != nil {
					return 0, 0, fmt.Errorf("marshaling Service for %q: %w", name, err)
				}
				_, _ = fmt.Fprintf(
					ro.writer,
					"# Service: %s (workload: %s)\n%s\n%s\n",
					svc.Name,
					name,
					string(data),
					"---",
				)
				svcCount++
			}
		}
	}

	for i := range plans {
		secretName := plans[i].VMName + "-cloudinit"
		plans[i].VMSpec.CloudInitSecretName = secretName

		secret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: cfg.Namespace,
				Labels: map[string]string{
					constants.LabelAppName:   plans[i].VMSpec.Labels[constants.LabelAppName],
					constants.LabelManagedBy: constants.ManagedByValue,
					constants.LabelComponent: plans[i].Component,
					constants.LabelRunID:     runID,
				},
			},
			StringData: map[string]string{
				constants.SecretKeyUserdata: plans[i].VMSpec.CloudInitUserdata,
			},
		}
		data, err := sigyaml.Marshal(secret)
		if err != nil {
			return 0, 0, fmt.Errorf("marshaling Secret for %q: %w", plans[i].VMName, err)
		}
		_, _ = fmt.Fprintf(
			ro.writer,
			"# Secret: %s (workload: %s)\n%s\n%s\n",
			secretName,
			plans[i].Component,
			string(data),
			"---",
		)
	}

	for _, p := range plans {
		vmObj, err := vm.BuildVMSpec(vm.VMSpecOpts{
			Name:                p.VMSpec.Name,
			Namespace:           p.VMSpec.Namespace,
			ContainerDiskImage:  p.VMSpec.ContainerDiskImage,
			CloudInitUserdata:   p.VMSpec.CloudInitUserdata,
			CloudInitSecretName: p.VMSpec.CloudInitSecretName,
			CPUCores:            p.VMSpec.CPUCores,
			Memory:              p.VMSpec.Memory,
			Labels:              p.VMSpec.Labels,
			ExtraDisks:          p.VMSpec.ExtraDisks,
			ExtraVolumes:        p.VMSpec.ExtraVolumes,
			DataVolumeTemplates: p.VMSpec.DataVolumeTemplates,
		})
		if err != nil {
			return 0, 0, fmt.Errorf("building VM spec for %q: %w", p.VMName, err)
		}
		data, err := sigyaml.Marshal(vmObj)
		if err != nil {
			return 0, 0, fmt.Errorf("marshaling VM spec for %q: %w", p.VMName, err)
		}
		_, _ = fmt.Fprintf(
			ro.writer,
			"# VM: %s (workload: %s)\n%s\n%s\n",
			p.VMName,
			p.Component,
			string(data),
			"---",
		)
	}
	return svcCount, len(plans), nil
}
