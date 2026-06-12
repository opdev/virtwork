// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrParamEmpty          = errors.New("param must not be empty")
	ErrParamNotInt         = errors.New("param must be an integer")
	ErrParamNotBool        = errors.New("param must be \"true\" or \"false\"")
	ErrParamInvalidElement = errors.New("param has invalid element")
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
			return fmt.Errorf("param %q: %w", d.Key, ErrParamEmpty)
		}
	case ParamInt:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("param %q value %q: %w", d.Key, value, ErrParamNotInt)
		}
	case ParamBool:
		lower := strings.ToLower(value)
		if lower != "true" && lower != "false" {
			return fmt.Errorf("param %q value %q: %w", d.Key, value, ErrParamNotBool)
		}
	case ParamList:
		parts := strings.Split(value, ";")
		for i, p := range parts {
			if p == "" {
				return fmt.Errorf(
					"param %q position %d empty element: %w",
					d.Key,
					i,
					ErrParamInvalidElement,
				)
			}
		}
	case ParamDict:
		parts := strings.Split(value, ";")
		for i, p := range parts {
			if p == "" {
				return fmt.Errorf(
					"param %q position %d empty element: %w",
					d.Key,
					i,
					ErrParamInvalidElement,
				)
			}
			if !strings.Contains(p, "=") {
				return fmt.Errorf(
					"param %q entry %q must contain \"=\": %w",
					d.Key,
					p,
					ErrParamInvalidElement,
				)
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
