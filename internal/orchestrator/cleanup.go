// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package orchestrator

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/audit"
	"github.com/opdev/virtwork/internal/cleanup"
	"github.com/opdev/virtwork/internal/config"
)

// CleanupOrchestrator coordinates the "cleanup" workflow: previewing and
// deleting managed resources. Dependencies are injected at construction
// for testability.
type CleanupOrchestrator struct {
	logger  *slog.Logger
	client  client.Client
	config  *config.Config
	auditor audit.Auditor
	writer  io.Writer
}

// NewCleanupOrchestrator creates a CleanupOrchestrator with the given dependencies.
func NewCleanupOrchestrator(
	logger *slog.Logger,
	c client.Client,
	cfg *config.Config,
	auditor audit.Auditor,
	writer io.Writer,
) *CleanupOrchestrator {
	return &CleanupOrchestrator{
		logger:  logger,
		client:  c,
		config:  cfg,
		auditor: auditor,
		writer:  writer,
	}
}

// Preview returns what would be deleted without modifying any resources.
func (co *CleanupOrchestrator) Preview(
	ctx context.Context,
	execID int64,
	runID string,
) (*cleanup.CleanupPreview, error) {
	preview, err := cleanup.PreviewCleanup(ctx, co.client, co.config, runID)
	if err != nil {
		return nil, fmt.Errorf("previewing cleanup: %w", err)
	}
	return preview, nil
}

// Execute performs the cleanup, deleting managed resources and recording
// audit data.
func (co *CleanupOrchestrator) Execute(
	ctx context.Context,
	execID int64,
	deleteNamespace bool,
	runID string,
) (*cleanup.CleanupResult, error) {
	if err := co.auditor.RecordEvent(ctx, execID, audit.EventRecord{
		EventType: "cleanup_started",
		Message:   fmt.Sprintf("Starting cleanup (namespace: %s, run-id filter: %q)", co.config.Namespace, runID),
	}); err != nil {
		co.logger.Warn("audit record failed", slog.String("op", "RecordEvent"), slog.String("error", err.Error()))
	}

	result, err := cleanup.CleanupAll(ctx, co.client, co.config, deleteNamespace, runID)
	if err != nil {
		return nil, fmt.Errorf("cleanup failed: %w", err)
	}

	if len(result.RunIDs) > 0 {
		if auditErr := co.auditor.LinkCleanupToRuns(ctx, execID, result.RunIDs); auditErr != nil {
			co.logger.Warn(
				"audit record failed",
				slog.String("op", "LinkCleanupToRuns"),
				slog.String("error", auditErr.Error()),
			)
		}
	}

	if auditErr := co.auditor.RecordCleanupCounts(ctx, execID,
		result.VMsDeleted, result.ServicesDeleted, result.SecretsDeleted,
		result.DVsDeleted, result.PVCsDeleted, result.NamespaceDeleted); auditErr != nil {
		co.logger.Warn(
			"audit record failed",
			slog.String("op", "RecordCleanupCounts"),
			slog.String("error", auditErr.Error()),
		)
	}

	if auditErr := co.auditor.RecordEvent(ctx, execID, audit.EventRecord{
		EventType: "cleanup_completed",
		Message: fmt.Sprintf("Deleted %d VMs, %d services, %d secrets, %d DVs, %d PVCs",
			result.VMsDeleted, result.ServicesDeleted, result.SecretsDeleted,
			result.DVsDeleted, result.PVCsDeleted),
	}); auditErr != nil {
		co.logger.Warn("audit record failed", slog.String("op", "RecordEvent"), slog.String("error", auditErr.Error()))
	}

	return result, nil
}
