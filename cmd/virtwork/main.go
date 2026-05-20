// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	kubevirtv1 "kubevirt.io/api/core/v1"
	sigyaml "sigs.k8s.io/yaml"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/cleanup"
	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
	"github.com/opdev/virtwork/internal/resources"
	"github.com/opdev/virtwork/internal/vm"
	"github.com/opdev/virtwork/internal/wait"
	"github.com/opdev/virtwork/internal/workloads"
)

var (
	// ErrReadinessCheck: readiness check failed
	ErrReadinessCheck = errors.New("readiness check failed")
	// Err
	ErrMultiVMWorkloadNotImplemented = errors.New("MultiVMWorkload not implemented")
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "virtwork",
		Short: "Create VMs on OpenShift with continuous workloads",
		Long: `Virtwork creates virtual machines on OpenShift clusters (with OpenShift
Virtualization installed) and runs continuous workloads inside them to produce
realistic CPU, memory, database, network, and disk I/O metrics.`,
		SilenceUsage: true,
	}

	pf := rootCmd.PersistentFlags()
	pf.String("namespace", "", "Kubernetes namespace for VMs")
	pf.String("kubeconfig", "", "Path to kubeconfig file")
	pf.String("config", "", "Path to YAML config file")
	pf.Bool("verbose", false, "Enable verbose output")
	pf.Bool("audit", true, "Enable audit logging to SQLite")
	pf.Bool("no-audit", false, "Disable audit logging")
	pf.String("audit-db", "", "Path to audit database file")

	rootCmd.AddCommand(newRunCmd(), newCleanupCmd())
	return rootCmd
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Create VMs and start workloads",
		Long: `Deploy virtual machines with the specified workloads. Each workload type
installs and configures its own software via cloud-init and runs continuously
via systemd.`,
		RunE: runE,
	}

	f := cmd.Flags()
	f.StringSlice("workloads", workloads.AllWorkloadNames(), "Workloads to deploy (comma-separated)")
	f.Int("vm-count", 1, "Number of VMs per workload")
	f.Int("cpu-cores", 0, "CPU cores per VM")
	f.String("memory", "", "Memory per VM (e.g., 2Gi)")
	f.String("disk-size", "", "Data disk size")
	f.String("container-disk-image", "", "Container disk image for VMs")
	f.Bool("dry-run", false, "Print specs without creating resources")
	f.Bool("no-wait", false, "Skip waiting for VM readiness")
	f.Int("timeout", 0, "Readiness timeout in seconds")
	f.String("ssh-user", "", "SSH user for VMs")
	f.String("ssh-password", "", "SSH password for VMs")
	f.StringSlice("ssh-key", nil, "SSH authorized key (repeatable)")
	f.StringSlice("ssh-key-file", nil, "SSH key file path (repeatable)")

	return cmd
}

func newCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete all managed resources",
		Long:  `Delete all VMs, services, secrets, and optionally the namespace created by virtwork.`,
		RunE:  cleanupE,
	}

	cmd.Flags().Bool("delete-namespace", false, "Also delete the namespace")
	cmd.Flags().String("run-id", "", "Only delete resources from this specific run (UUID)")
	cmd.Flags().Bool("dry-run", false, "Print intent without destroying resources")
	return cmd
}

// initAuditor creates the appropriate Auditor based on configuration flags.
func initAuditor(cmd *cobra.Command, cfg *config.Config) (audit.Auditor, error) {
	noAudit, _ := cmd.Flags().GetBool("no-audit")
	if noAudit || !cfg.AuditEnabled {
		return audit.NoOpAuditor{}, nil
	}

	dbPath := cfg.AuditDBPath
	if cmd.Flags().Changed("audit-db") {
		dbPath, _ = cmd.Flags().GetString("audit-db")
	}

	return audit.NewSQLiteAuditor(dbPath)
}

// vmPlan describes a single VM to be created during orchestration.
type vmPlan struct {
	workload  workloads.Workload
	vmSpec    *vm.VMSpecOpts
	vmName    string
	component string
	role      string
}

// namespaceDataVolumes appends the VM name to DataVolume template names and
// updates corresponding volume references to prevent name collisions when
// deploying multiple VMs of the same workload type.
func namespaceDataVolumes(
	baseTemplates []kubevirtv1.DataVolumeTemplateSpec,
	baseVolumes []kubevirtv1.Volume,
	vmName string,
) ([]kubevirtv1.DataVolumeTemplateSpec, []kubevirtv1.Volume) {
	if len(baseTemplates) == 0 {
		return baseTemplates, baseVolumes
	}

	// Build a map of original DataVolume names to namespaced names
	nameMap := make(map[string]string, len(baseTemplates))
	templates := make([]kubevirtv1.DataVolumeTemplateSpec, len(baseTemplates))
	for i, tmpl := range baseTemplates {
		oldName := tmpl.Name
		newName := fmt.Sprintf("%s-%s", oldName, vmName)
		nameMap[oldName] = newName

		templates[i] = tmpl
		templates[i].Name = newName
	}

	// Update volume references to use the namespaced names
	volumes := make([]kubevirtv1.Volume, len(baseVolumes))
	for i, vol := range baseVolumes {
		volumes[i] = vol
		if vol.DataVolume != nil {
			if newName, ok := nameMap[vol.DataVolume.Name]; ok {
				volumes[i].DataVolume = &kubevirtv1.DataVolumeSource{
					Name: newName,
				}
			}
		}
	}

	return templates, volumes
}

// runE is the main orchestration flow for the "run" subcommand.
func runE(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Initialize auditor
	auditor, err := initAuditor(cmd, cfg)
	if err != nil {
		return fmt.Errorf("initializing auditor: %w", err)
	}
	defer func() {
		_ = auditor.Close()
	}()

	ctx := context.Background()

	// Start audit execution
	cmdName := "run"
	if cfg.DryRun {
		cmdName = "dry-run"
	}
	execID, runID, err := auditor.StartExecution(ctx, cmdName, cfg)
	if err != nil {
		return fmt.Errorf("starting audit execution: %w", err)
	}
	defer func() {
		if err != nil {
			_ = auditor.CompleteExecution(ctx, execID, "failed", err.Error())
		}
	}()

	_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
		EventType: "execution_started",
		Message:   fmt.Sprintf("Starting %s with run-id %s", cmdName, runID),
	})

	// Determine which workloads to deploy
	workloadNames, _ := cmd.Flags().GetStringSlice("workloads")
	vmCountFlag, _ := cmd.Flags().GetInt("vm-count")

	registry := workloads.DefaultRegistry()
	registryOpts := []workloads.Option{
		workloads.WithNamespace(cfg.Namespace),
		workloads.WithSSHCredentials(cfg.SSHUser, cfg.SSHPassword, cfg.SSHAuthorizedKeys),
		workloads.WithDataDiskSize(cfg.DataDiskSize),
	}

	// Build workload instances
	var plans []vmPlan
	var vmNames []string
	auditWorkloadIDs := make(map[string]int64) // workload name -> audit workload ID

	for _, name := range workloadNames {
		wlCfg := config.WorkloadConfig{
			Enabled:  true,
			VMCount:  vmCountFlag,
			CPUCores: cfg.CPUCores,
			Memory:   cfg.Memory,
		}
		// Override with per-workload config from YAML if present
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

		w, err := registry.Get(name, wlCfg, registryOpts...)
		if err != nil {
			return fmt.Errorf("creating workload %q: %w", name, err)
		}

		vmCount := w.VMCount()
		res := w.VMResources()

		// Record workload in audit
		wlID, _ := auditor.RecordWorkload(ctx, execID, audit.WorkloadRecord{
			WorkloadType:    name,
			Enabled:         true,
			VMCount:         vmCount,
			CPUCores:        res.CPUCores,
			Memory:          res.Memory,
			HasDataDisk:     len(w.DataVolumeTemplates()) > 0,
			DataDiskSize:    cfg.DataDiskSize,
			RequiresService: w.RequiresService(),
		})
		auditWorkloadIDs[name] = wlID

		if _, isMulti := w.(workloads.MultiVMWorkload); !isMulti {
			userdata, err := w.CloudInitUserdata()
			if err != nil {
				return fmt.Errorf("generating cloud-init for %q: %w", name, err)
			}

			for i := range vmCount {
				vmName := fmt.Sprintf("virtwork-%s-%d", name, i)

				// Namespace DataVolume names to avoid collisions across VMs
				dvTemplates, extraVols := namespaceDataVolumes(
					w.DataVolumeTemplates(),
					w.ExtraVolumes(),
					vmName,
				)

				plans = append(plans, vmPlan{
					workload:  w,
					component: name,
					vmName:    vmName,
					vmSpec: &vm.VMSpecOpts{
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
			// Multi-VM workload — use UserdataForRole
			multiVM, ok := w.(workloads.MultiVMWorkload)
			if !ok {
				return fmt.Errorf(
					"workload %q reports VMCount=%d; %w",
					name,
					vmCount,
					ErrMultiVMWorkloadNotImplemented,
				)
			}

			roles := multiVM.Roles()
			perRole := vmCount / len(roles)
			for _, role := range roles {
				userdata, err := multiVM.UserdataForRole(role, cfg.Namespace)
				if err != nil {
					return fmt.Errorf("generating cloud-init for %q role %q: %w", name, role, err)
				}

				for i := range perRole {
					vmName := fmt.Sprintf("virtwork-%s-%s-%d", name, role, i)
					labels := map[string]string{
						constants.LabelAppName:   fmt.Sprintf("virtwork-%s", name),
						constants.LabelManagedBy: constants.ManagedByValue,
						constants.LabelComponent: name,
						constants.LabelRunID:     runID,
						"virtwork/role":          role,
					}

					// Namespace DataVolume names to avoid collisions across VMs
					dvTemplates, extraVols := namespaceDataVolumes(
						w.DataVolumeTemplates(),
						w.ExtraVolumes(),
						vmName,
					)

					plans = append(plans, vmPlan{
						workload:  w,
						component: name,
						vmName:    vmName,
						role:      role,
						vmSpec: &vm.VMSpecOpts{
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

	// Update audit with total counts
	_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
		EventType: "execution_started",
		Message:   fmt.Sprintf("Planned %d VMs across %d workloads", len(plans), len(workloadNames)),
	})

	// Dry-run: print specs and return
	if cfg.DryRun {
		if err := printDryRun(plans); err != nil {
			return err
		}
		_ = auditor.CompleteExecution(ctx, execID, "success", "")
		err = nil // clear for defer
		return nil
	}

	// Connect to cluster
	c, err := cluster.Connect(cfg.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	// Ensure namespace exists
	if err := resources.EnsureNamespace(ctx, c, cfg.Namespace, map[string]string{
		constants.LabelManagedBy: constants.ManagedByValue,
	}); err != nil {
		return fmt.Errorf("ensuring namespace %q: %w", cfg.Namespace, err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Namespace %s ensured\n", cfg.Namespace)

	// Create services before VMs (DNS must resolve for client VMs)
	servicesCreated := 0
	for _, name := range workloadNames {
		// Re-fetch workload to check service requirement
		wlCfg := config.WorkloadConfig{
			Enabled:  true,
			VMCount:  vmCountFlag,
			CPUCores: cfg.CPUCores,
			Memory:   cfg.Memory,
		}
		w, err := registry.Get(name, wlCfg, registryOpts...)
		if err != nil {
			continue
		}
		if w.RequiresService() {
			svc := w.ServiceSpec()
			if svc != nil {
				// Add run-id label to service
				if svc.Labels == nil {
					svc.Labels = make(map[string]string)
				}
				svc.Labels[constants.LabelRunID] = runID

				if err := resources.CreateService(ctx, c, svc); err != nil {
					return fmt.Errorf("creating service for %q: %w", name, err)
				}
				servicesCreated++
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Service %s created\n", svc.Name)

				_, _ = auditor.RecordResource(ctx, execID, audit.ResourceRecord{
					ResourceType: "Service",
					ResourceName: svc.Name,
					Namespace:    svc.Namespace,
				})
				_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
					EventType: "service_created",
					Message:   fmt.Sprintf("Service %s created", svc.Name),
				})
			}
		}
	}

	// Create cloud-init secrets before VMs
	secretsCreated := 0
	for i := range plans {
		secretName := plans[i].vmName + "-cloudinit"
		secretLabels := map[string]string{
			constants.LabelAppName:   plans[i].vmSpec.Labels[constants.LabelAppName],
			constants.LabelManagedBy: constants.ManagedByValue,
			constants.LabelComponent: plans[i].component,
			constants.LabelRunID:     runID,
		}
		if err := resources.CreateCloudInitSecret(ctx, c, secretName,
			cfg.Namespace, plans[i].vmSpec.CloudInitUserdata, secretLabels); err != nil {
			return fmt.Errorf("creating cloud-init secret for %q: %w", plans[i].vmName, err)
		}
		plans[i].vmSpec.CloudInitSecretName = secretName
		secretsCreated++
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Secret %s created\n", secretName)

		_, _ = auditor.RecordResource(ctx, execID, audit.ResourceRecord{
			ResourceType: "Secret",
			ResourceName: secretName,
			Namespace:    cfg.Namespace,
		})
	}

	// Create VMs concurrently via errgroup
	g, gctx := errgroup.WithContext(ctx)
	for _, plan := range plans {
		p := plan // capture loop variable
		g.Go(func() error {
			vmObj := vm.BuildVMSpec(*p.vmSpec)
			if err := vm.CreateVM(gctx, c, vmObj); err != nil {
				_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
					EventType:   "vm_failed",
					Message:     fmt.Sprintf("Failed to create VM %s", p.vmName),
					ErrorDetail: err.Error(),
				})
				return fmt.Errorf("creating VM %q: %w", p.vmName, err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "VM %s created\n", p.vmName)

			wlID := auditWorkloadIDs[p.component]
			_, _ = auditor.RecordVM(ctx, execID, wlID, audit.VMRecord{
				VMName:             p.vmName,
				Namespace:          cfg.Namespace,
				Component:          p.component,
				Role:               p.role,
				CPUCores:           p.vmSpec.CPUCores,
				Memory:             p.vmSpec.Memory,
				ContainerDiskImage: p.vmSpec.ContainerDiskImage,
				HasDataDisk:        len(p.vmSpec.DataVolumeTemplates) > 0,
				DataDiskSize:       cfg.DataDiskSize,
			})
			_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
				EventType: "vm_created",
				Message:   fmt.Sprintf("VM %s created", p.vmName),
			})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("creating VMs: %w", err)
	}

	// Wait for readiness
	if cfg.WaitForReady {
		timeout := time.Duration(cfg.ReadyTimeoutSeconds) * time.Second
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Waiting for %d VMs to become ready (timeout: %s)...\n",
			len(vmNames), timeout)
		results := wait.WaitForAllVMsReady(ctx, c, vmNames, cfg.Namespace,
			timeout, constants.DefaultPollInterval)

		failures := 0
		for name, err := range results {
			if err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "VM %s: %v\n", name, err)
				failures++
				_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
					EventType:   "vm_timeout",
					Message:     fmt.Sprintf("VM %s failed readiness check", name),
					ErrorDetail: err.Error(),
				})
			} else {
				_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
					EventType: "vm_ready",
					Message:   fmt.Sprintf("VM %s is ready", name),
				})
			}
		}
		if failures > 0 {
			return fmt.Errorf("%d of %d VMs failed; %w", failures, len(vmNames), ErrReadinessCheck)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "All %d VMs ready\n", len(vmNames))
	}

	// Mark all workloads as created
	for _, wlID := range auditWorkloadIDs {
		_ = auditor.UpdateWorkloadStatus(ctx, wlID, "created")
	}

	// Complete audit
	_ = auditor.CompleteExecution(ctx, execID, "success", "")
	err = nil // clear for defer

	// Print summary
	printSummary(cmd, len(plans), servicesCreated, secretsCreated, cfg, runID)
	return nil
}

// cleanupE is the cleanup flow for the "cleanup" subcommand.
func cleanupE(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Initialize auditor
	auditor, err := initAuditor(cmd, cfg)
	if err != nil {
		return fmt.Errorf("initializing auditor: %w", err)
	}
	defer func() {
		_ = auditor.Close()
	}()

	ctx := context.Background()

	// Start audit execution
	dryRunBanner := ""
	cmdName := "cleanup"
	if cfg.DryRun {
		// Log the command being executed
		cmdName = "cleanup --dry-run"
		// Add banner if '--dry-run' option is passed
		dryRunBanner = "--- Dry Run ---\n"
	}

	execID, _, err := auditor.StartExecution(ctx, cmdName, cfg)
	if err != nil {
		return fmt.Errorf("starting audit execution: %w", err)
	}
	defer func() {
		if err != nil {
			_ = auditor.CompleteExecution(ctx, execID, "failed", err.Error())
		}
	}()

	deleteNS, _ := cmd.Flags().GetBool("delete-namespace")
	targetRunID, _ := cmd.Flags().GetString("run-id")

	_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
		EventType: "cleanup_started",
		Message:   fmt.Sprintf("Started %s: (namespace: %s, run-id filter: %q)", cmdName, cfg.Namespace, targetRunID),
	})

	c, err := cluster.Connect(cfg.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	result, err := cleanup.CleanupAll(ctx, c, cfg, deleteNS, targetRunID)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	// Link cleanup to discovered run IDs
	if len(result.RunIDs) > 0 {
		_ = auditor.LinkCleanupToRuns(ctx, execID, result.RunIDs)
	}

	// Record cleanup counts
	_ = auditor.RecordCleanupCounts(ctx, execID,
		result.VMsDeleted, result.ServicesDeleted, result.SecretsDeleted, result.NamespaceDeleted)

	_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
		EventType: "cleanup_completed",
		Message: fmt.Sprintf("Deleted %d VMs, %d services, %d secrets",
			result.VMsDeleted, result.ServicesDeleted, result.SecretsDeleted),
	})

	// Complete audit
	_ = auditor.CompleteExecution(ctx, execID, "success", "")
	err = nil // clear for defer

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sCleanup complete: %d VMs deleted, %d services deleted, %d secrets deleted",
		dryRunBanner, result.VMsDeleted, result.ServicesDeleted, result.SecretsDeleted)
	if result.NamespaceDeleted {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), ", namespace deleted")
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	if len(result.Errors) > 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warnings (%d):\n", len(result.Errors))
		for _, e := range result.Errors {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  - %v\n", e)
		}
	}

	return nil
}

// printDryRun outputs VM specs in YAML without connecting to a cluster.
func printDryRun(plans []vmPlan) error {
	_, _ = fmt.Printf("--- Dry Run ---\nTotal VMs to create: %d\n\n", len(plans))

	for _, p := range plans {
		vmObj := vm.BuildVMSpec(*p.vmSpec)
		data, err := sigyaml.Marshal(vmObj)
		if err != nil {
			return fmt.Errorf("marshaling VM spec for %q: %w", p.vmName, err)
		}
		_, _ = fmt.Printf("# VM: %s (workload: %s)\n%s\n%s\n", p.vmName, p.component, string(data), "---")
	}
	return nil
}

// printSummary outputs a deployment summary table.
func printSummary(cmd *cobra.Command, vmCount, svcCount, secCount int, cfg *config.Config, runID string) {
	summaryTemplate := `%s
Deployment Summary
%s
Run ID:       %s
Namespace:    %s
VMs created:  %d
Services:     %d
Secrets:      %d
Image:        %s
%s`
	_, _ = fmt.Fprintf(
		cmd.OutOrStdout(),
		summaryTemplate,
		strings.Repeat("=", 50),
		strings.Repeat("=", 50),
		runID,
		cfg.Namespace,
		vmCount,
		svcCount,
		secCount,
		cfg.ContainerDiskImage,
		strings.Repeat("=", 50),
	)
}
