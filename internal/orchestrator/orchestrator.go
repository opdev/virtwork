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
