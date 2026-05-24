// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestPromptForConfirmation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"yes confirms", "yes\n", true},
		{"no rejects", "no\n", false},
		{"empty rejects (default NO)", "\n", false},
		{"arbitrary text rejects", "maybe\n", false},
		{"YES case-insensitive", "YES\n", true},
		{"whitespace trimmed", "  yes  \n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			confirmed, err := PromptForConfirmation(reader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if confirmed != tt.expected {
				t.Errorf("got %v, want %v", confirmed, tt.expected)
			}
		})
	}
}
