// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"testing"
)

func TestAuditStatus_cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, msg := auditStatus(ctx, fmt.Errorf("context canceled"))
	if status != "cancelled" {
		t.Errorf("expected status %q, got %q", "cancelled", status)
	}
	if msg != "interrupted by signal" {
		t.Errorf("expected message %q, got %q", "interrupted by signal", msg)
	}
}

func TestAuditStatus_failed(t *testing.T) {
	ctx := context.Background()

	status, msg := auditStatus(ctx, fmt.Errorf("connection refused"))
	if status != "failed" {
		t.Errorf("expected status %q, got %q", "failed", status)
	}
	if msg != "connection refused" {
		t.Errorf("expected message %q, got %q", "connection refused", msg)
	}
}
