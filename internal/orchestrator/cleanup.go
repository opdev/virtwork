// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package orchestrator

import (
	"io"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opdev/virtwork/internal/audit"
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
