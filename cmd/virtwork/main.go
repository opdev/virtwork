// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/cleanup"
	"github.com/opdev/virtwork/internal/cluster"
	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/logging"
	"github.com/opdev/virtwork/internal/orchestrator"
	"github.com/opdev/virtwork/internal/workloads"
)

var (
	version = ""
	commit  = ""
	date    = ""
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

	config.BindPersistentFlags(rootCmd)

	rootCmd.AddCommand(newRunCmd(), newCleanupCmd(), newVersionCmd())
	return rootCmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			v := version
			if v == "" {
				v = "(dev)"
			}
			c := commit
			if c == "" {
				c = "(unknown)"
			}
			d := date
			if d == "" {
				d = "(unknown)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "virtwork version %s\n  commit: %s\n  built:  %s\n", v, c, d)
			return nil
		},
	}
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

	config.BindRunFlags(cmd, workloads.AllWorkloadNames())

	return cmd
}

func newCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete all managed resources",
		Long:  `Delete all VMs, services, secrets, and optionally the namespace created by virtwork.`,
		RunE:  cleanupE,
	}

	config.BindCleanupFlags(cmd)
	return cmd
}

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

func runE(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	logger := logging.NewLogger(cmd.OutOrStdout(), verbose)

	auditor, err := initAuditor(cmd, cfg)
	if err != nil {
		return fmt.Errorf("initializing auditor: %w", err)
	}
	defer func() {
		_ = auditor.Close()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var c client.Client
	if !cfg.DryRun {
		var contextName string
		c, contextName, err = cluster.Connect(cluster.ResolveKubeconfigPath(cfg.KubeconfigPath))
		if err != nil {
			return fmt.Errorf("connecting to cluster: %w", err)
		}
		cfg.ClusterContext = contextName
	}

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
			status, msg := auditStatus(ctx, err)
			_ = auditor.CompleteExecution(ctx, execID, status, msg)
		}
	}()

	_ = auditor.RecordEvent(ctx, execID, audit.EventRecord{
		EventType: "execution_started",
		Message:   fmt.Sprintf("Starting %s with run-id %s", cmdName, runID),
	})

	workloadNames, _ := cmd.Flags().GetStringSlice("workloads")
	vmCountFlag, _ := cmd.Flags().GetInt("vm-count")

	ro := orchestrator.NewRunOrchestrator(logger, c, cfg, auditor, cmd.OutOrStdout())
	result, err := ro.Run(ctx, execID, runID, workloadNames, vmCountFlag)
	if err != nil {
		return err
	}

	_ = auditor.CompleteExecution(ctx, execID, "success", "")
	err = nil

	printSummary(logger, result, cfg)
	return nil
}

func cleanupE(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(cmd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	logger := logging.NewLogger(cmd.OutOrStdout(), verbose)

	auditor, err := initAuditor(cmd, cfg)
	if err != nil {
		return fmt.Errorf("initializing auditor: %w", err)
	}
	defer func() {
		_ = auditor.Close()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deleteNS, _ := cmd.Flags().GetBool("delete-namespace")
	targetRunID, _ := cmd.Flags().GetString("run-id")

	if cfg.DryRun {
		cfg.CleanupMode = "dry-run"
	} else if targetRunID != "" {
		cfg.CleanupMode = "run-id"
	} else {
		cfg.CleanupMode = "all"
	}

	c, contextName, err := cluster.Connect(cluster.ResolveKubeconfigPath(cfg.KubeconfigPath))
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}
	cfg.ClusterContext = contextName

	cmdName := "cleanup"
	if cfg.DryRun {
		cmdName = "cleanup --dry-run"
	}

	execID, _, err := auditor.StartExecution(ctx, cmdName, cfg)
	if err != nil {
		return fmt.Errorf("starting audit execution: %w", err)
	}
	defer func() {
		if err != nil {
			status, msg := auditStatus(ctx, err)
			_ = auditor.CompleteExecution(ctx, execID, status, msg)
		}
	}()

	co := orchestrator.NewCleanupOrchestrator(logger, c, cfg, auditor, cmd.OutOrStdout())

	preview, err := co.Preview(ctx, execID, targetRunID)
	if err != nil {
		return fmt.Errorf("previewing cleanup: %w", err)
	}

	printCleanupPreview(logger, preview, cfg.Namespace, targetRunID)

	if preview.TotalCount == 0 {
		logger.Info("nothing to clean up")
		_ = auditor.CompleteExecution(ctx, execID, "success", "")
		err = nil
		return nil
	}

	if cfg.DryRun {
		logger.Info("dry-run mode — no resources were deleted")
		_ = auditor.CompleteExecution(ctx, execID, "success", "")
		err = nil
		return nil
	}

	skipPrompt, _ := cmd.Flags().GetBool("yes")
	if !skipPrompt && targetRunID == "" {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "Proceed with deletion? (yes/NO): ")
		confirmed, promptErr := PromptForConfirmation(cmd.InOrStdin())
		if promptErr != nil {
			return fmt.Errorf("reading confirmation: %w", promptErr)
		}
		if !confirmed {
			logger.Info("cleanup aborted by user")
			_ = auditor.CompleteExecution(ctx, execID, "aborted", "user declined confirmation")
			err = nil
			return nil
		}
	}

	result, err := co.Execute(ctx, execID, deleteNS, targetRunID)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	_ = auditor.CompleteExecution(ctx, execID, "success", "")
	err = nil

	logger.Info("cleanup complete",
		slog.Bool("dry_run", cfg.DryRun),
		slog.Int("vms_deleted", result.VMsDeleted),
		slog.Int("services_deleted", result.ServicesDeleted),
		slog.Int("secrets_deleted", result.SecretsDeleted),
		slog.Int("dvs_deleted", result.DVsDeleted),
		slog.Int("pvcs_deleted", result.PVCsDeleted),
		slog.Bool("namespace_deleted", result.NamespaceDeleted))

	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			logger.Warn("cleanup warning", slog.String("error", e.Error()))
		}
	}

	return nil
}

func printSummary(logger *slog.Logger, result *orchestrator.RunResult, cfg *config.Config) {
	logger.Info("deployment summary",
		slog.String("run_id", result.RunID),
		slog.String("namespace", cfg.Namespace),
		slog.Int("vms_created", result.VMCount),
		slog.Int("services_created", result.ServiceCount),
		slog.Int("secrets_created", result.SecretCount),
		slog.String("container_image", cfg.ContainerDiskImage))
}

func printCleanupPreview(logger *slog.Logger, preview *cleanup.CleanupPreview, namespace, runID string) {
	attrs := []slog.Attr{
		slog.String("namespace", namespace),
		slog.Int("vms", preview.VMCount),
		slog.Int("services", preview.ServiceCount),
		slog.Int("secrets", preview.SecretCount),
		slog.Int("dvs", preview.DVCount),
		slog.Int("pvcs", preview.PVCCount),
		slog.Int("total", preview.TotalCount),
	}
	if runID != "" {
		attrs = append(attrs, slog.String("run_id_filter", runID))
	}
	if len(preview.RunIDs) > 0 {
		attrs = append(attrs, slog.String("run_ids", strings.Join(preview.RunIDs, ", ")))
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	logger.Info("resources to be deleted", args...)
}

func auditStatus(ctx context.Context, err error) (status, message string) {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "cancelled", "interrupted by signal"
	}
	return "failed", err.Error()
}

func PromptForConfirmation(r io.Reader) (bool, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("reading confirmation: %w", err)
		}
		return false, nil
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "yes", nil
}
