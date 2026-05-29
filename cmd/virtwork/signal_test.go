// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"
)

var (
	ErrContextCanceled   = errors.New("context canceled")
	ErrConnectionRefused = errors.New("connection refused")
)

func TestAuditStatus_cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, msg := auditStatus(ctx, ErrContextCanceled)
	if status != "cancelled" {
		t.Errorf("expected status %q, got %q", "cancelled", status)
	}
	if msg != "interrupted by signal" {
		t.Errorf("expected message %q, got %q", "interrupted by signal", msg)
	}
}

func TestAuditStatus_failed(t *testing.T) {
	ctx := context.Background()

	status, msg := auditStatus(ctx, ErrConnectionRefused)
	if status != "failed" {
		t.Errorf("expected status %q, got %q", "failed", status)
	}
	if msg != "connection refused" {
		t.Errorf("expected message %q, got %q", "connection refused", msg)
	}
}
