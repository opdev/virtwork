// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"fmt"
	"strconv"
	"strings"
)

// ParamType identifies the expected value category for a workload param.
type ParamType int

const (
	ParamString ParamType = iota
	ParamInt
	ParamBool
	ParamList
	ParamDict
)

// ParamDef declares a single configurable param for a workload.
type ParamDef struct {
	Key     string
	Type    ParamType
	Default string
	Desc    string
}

// Validate checks that value conforms to the param's declared type.
func (d *ParamDef) Validate(value string) error {
	switch d.Type {
	case ParamString:
		if value == "" {
			return fmt.Errorf("param %q must not be empty", d.Key)
		}
	case ParamInt:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("param %q must be an integer, got %q", d.Key, value)
		}
	case ParamBool:
		lower := strings.ToLower(value)
		if lower != "true" && lower != "false" {
			return fmt.Errorf("param %q must be \"true\" or \"false\", got %q", d.Key, value)
		}
	case ParamList:
		parts := strings.Split(value, ";")
		for i, p := range parts {
			if p == "" {
				return fmt.Errorf("param %q: empty element at position %d", d.Key, i)
			}
		}
	case ParamDict:
		parts := strings.Split(value, ";")
		for i, p := range parts {
			if p == "" {
				return fmt.Errorf("param %q: empty element at position %d", d.Key, i)
			}
			if !strings.Contains(p, "=") {
				return fmt.Errorf("param %q: entry %q must contain \"=\"", d.Key, p)
			}
		}
	}
	return nil
}

// ParamSchema is the ordered list of param declarations for a workload.
type ParamSchema []ParamDef

// Find returns the ParamDef for the given key, or nil if not declared.
func (s ParamSchema) Find(key string) *ParamDef {
	for i := range s {
		if s[i].Key == key {
			return &s[i]
		}
	}
	return nil
}
