// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/opdev/virtwork/internal/config"
)

var (
	ErrCatalogEntryNotFound      = errors.New("catalog entry not found")
	ErrCatalogNoServices         = errors.New("catalog entry has no .service files")
	ErrCatalogManifestRequired   = errors.New("workload.yaml is required for multi-role catalog entries")
	ErrCatalogMissingRoleService = errors.New("missing .service file for declared role")
)

// RoleDefinition declares a role and its default VM count in a catalog manifest.
type RoleDefinition struct {
	Name    string `yaml:"name"`
	VMCount int    `yaml:"vm-count"`
}

// catalogParamDef mirrors ParamDef for YAML unmarshaling with string type names.
type catalogParamDef struct {
	Key     string `yaml:"key"`
	Type    string `yaml:"type"`
	Default string `yaml:"default"`
	Desc    string `yaml:"desc"`
}

// CatalogManifest holds the parsed workload.yaml from a catalog entry.
type CatalogManifest struct {
	Description string            `yaml:"description"`
	Packages    []string          `yaml:"packages"`
	Params      []catalogParamDef `yaml:"params"`
	Roles       []RoleDefinition  `yaml:"roles"`
}

// CatalogEntry represents a loaded catalog workload entry.
type CatalogEntry struct {
	Name         string
	Dir          string
	Manifest     CatalogManifest
	ServiceFiles map[string]string
}

// IsMultiRole returns true if the entry declares multiple roles.
func (e *CatalogEntry) IsMultiRole() bool {
	return len(e.Manifest.Roles) > 1
}

// Schema returns the ParamSchema derived from the manifest's param declarations.
func (e *CatalogEntry) Schema() ParamSchema {
	if len(e.Manifest.Params) == 0 {
		return nil
	}
	schema := make(ParamSchema, len(e.Manifest.Params))
	for i, p := range e.Manifest.Params {
		schema[i] = ParamDef{
			Key:     p.Key,
			Type:    parseParamType(p.Type),
			Default: p.Default,
			Desc:    p.Desc,
		}
	}
	return schema
}

func parseParamType(s string) ParamType {
	switch strings.ToLower(s) {
	case "int":
		return ParamInt
	case "bool":
		return ParamBool
	case "list":
		return ParamList
	case "dict":
		return ParamDict
	default:
		return ParamString
	}
}

// Factory returns a WorkloadFactory for this catalog entry. Single-role entries
// produce GenericWorkload instances; multi-role entries produce GenericMultiWorkload.
func (e *CatalogEntry) Factory() WorkloadFactory {
	entry := e
	if entry.IsMultiRole() {
		return func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
			return NewGenericMultiWorkload(cfg, entry, opts.Namespace,
				opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
		}
	}
	return func(cfg config.WorkloadConfig, opts *RegistryOpts) Workload {
		return NewGenericWorkload(cfg, entry,
			opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
	}
}

// LoadCatalogEntry reads a catalog entry from catalogDir/entryName. It parses
// workload.yaml (if present) and discovers .service files. For multi-role entries,
// service files are mapped to roles by filename (e.g., server.service → "server").
func LoadCatalogEntry(catalogDir, entryName string) (*CatalogEntry, error) {
	entryDir := filepath.Join(catalogDir, entryName)
	info, err := os.Stat(entryDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("catalog entry %q: %w", entryName, ErrCatalogEntryNotFound)
	}

	entry := &CatalogEntry{
		Name:         entryName,
		Dir:          entryDir,
		ServiceFiles: make(map[string]string),
	}

	manifestPath := filepath.Join(entryDir, "workload.yaml")
	//nolint:gosec // catalog path is user-supplied intentionally
	if data, err := os.ReadFile(manifestPath); err == nil {
		if err := yaml.Unmarshal(data, &entry.Manifest); err != nil {
			return nil, fmt.Errorf("parsing workload.yaml for %q: %w", entryName, err)
		}
	}

	serviceFiles, err := filepath.Glob(filepath.Join(entryDir, "*.service"))
	if err != nil {
		return nil, fmt.Errorf("discovering service files for %q: %w", entryName, err)
	}
	if len(serviceFiles) == 0 {
		return nil, fmt.Errorf("catalog entry %q: %w", entryName, ErrCatalogNoServices)
	}

	sort.Strings(serviceFiles)

	if entry.IsMultiRole() {
		return loadMultiRoleServices(entry, serviceFiles)
	}
	return loadSingleRoleServices(entry, serviceFiles)
}

func loadSingleRoleServices(entry *CatalogEntry, serviceFiles []string) (*CatalogEntry, error) {
	for _, path := range serviceFiles {
		data, err := os.ReadFile(path) //nolint:gosec // catalog path is user-supplied intentionally
		if err != nil {
			return nil, fmt.Errorf("reading service file %q: %w", path, err)
		}
		entry.ServiceFiles[filepath.Base(path)] = string(data)
	}
	return entry, nil
}

func loadMultiRoleServices(entry *CatalogEntry, serviceFiles []string) (*CatalogEntry, error) {
	svcByRole := make(map[string]string)
	for _, path := range serviceFiles {
		base := filepath.Base(path)
		role := strings.TrimSuffix(base, ".service")
		data, err := os.ReadFile(path) //nolint:gosec // catalog path is user-supplied intentionally
		if err != nil {
			return nil, fmt.Errorf("reading service file %q: %w", path, err)
		}
		svcByRole[role] = string(data)
	}

	for _, rd := range entry.Manifest.Roles {
		if _, ok := svcByRole[rd.Name]; !ok {
			return nil, fmt.Errorf(
				"role %q in catalog entry %q: %w",
				rd.Name, entry.Name, ErrCatalogMissingRoleService,
			)
		}
	}

	entry.ServiceFiles = svcByRole
	return entry, nil
}
