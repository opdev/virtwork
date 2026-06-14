// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/constants"
)

var (
	ErrCatalogEntryNotFound      = errors.New("catalog entry not found")
	ErrCatalogNoServices         = errors.New("catalog entry has no .service files")
	ErrCatalogManifestRequired   = errors.New("workload.yaml is required for multi-role catalog entries")
	ErrCatalogMissingRoleService = errors.New("missing .service file for declared role")

	ErrStorageNameEmpty        = errors.New("storage name must not be empty")
	ErrStorageNameReserved     = errors.New("storage name conflicts with reserved disk name")
	ErrStorageNameDuplicate    = errors.New("duplicate storage name")
	ErrStorageSizeInvalid      = errors.New("storage size must be a valid quantity")
	ErrStorageSerialEmpty      = errors.New("storage serial must not be empty")
	ErrStorageSerialTooLong    = errors.New("storage serial must be at most 20 characters")
	ErrStorageMountNotAbsolute = errors.New("storage mount must be an absolute path")
	ErrServicePortsEmpty       = errors.New("service must declare at least one port")
	ErrServicePortRange        = errors.New("service port must be between 1 and 65535")
	ErrServiceProtocol         = errors.New("service protocol must be TCP or UDP")
)

// RoleDefinition declares a role and its default VM count in a catalog manifest.
type RoleDefinition struct {
	Name    string `yaml:"name"`
	VMCount int    `yaml:"vm-count"`
}

// StorageDefinition declares a persistent storage volume in a catalog manifest.
type StorageDefinition struct {
	Name   string `yaml:"name"`
	Size   string `yaml:"size"`
	Serial string `yaml:"serial"`
	Mount  string `yaml:"mount"`
}

// ServicePort declares a port for a K8s Service in a catalog manifest.
type ServicePort struct {
	Name     string `yaml:"name"`
	Port     int32  `yaml:"port"`
	Protocol string `yaml:"protocol"`
}

// ServiceDefinition declares a K8s Service in a catalog manifest.
type ServiceDefinition struct {
	Ports        []ServicePort `yaml:"ports"`
	SelectorRole string        `yaml:"selector-role"`
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
	Description string              `yaml:"description"`
	Packages    []string            `yaml:"packages"`
	Params      []catalogParamDef   `yaml:"params"`
	Roles       []RoleDefinition    `yaml:"roles"`
	Storage     []StorageDefinition `yaml:"storage"`
	Service     *ServiceDefinition  `yaml:"service"`
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
		return NewGenericWorkload(cfg, entry, opts.Namespace,
			opts.SSHUser, opts.SSHPassword, opts.SSHAuthorizedKeys)
	}
}

var placeholderRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func ValidatePlaceholders(entry *CatalogEntry) (errs []string, warnings []string) {
	declaredKeys := make(map[string]bool, len(entry.Manifest.Params))
	for _, p := range entry.Manifest.Params {
		declaredKeys[p.Key] = true
	}

	usedKeys := make(map[string]bool)
	for name, content := range entry.ServiceFiles {
		for _, match := range placeholderRe.FindAllStringSubmatch(content, -1) {
			key := match[1]
			usedKeys[key] = true
			if !declaredKeys[key] {
				errs = append(
					errs,
					fmt.Sprintf("placeholder {{%s}} in %s does not match any declared param", key, name),
				)
			}
		}
	}

	for key := range declaredKeys {
		if !usedKeys[key] {
			warnings = append(warnings, fmt.Sprintf("declared param %q is not used in any service file", key))
		}
	}

	sort.Strings(errs)
	sort.Strings(warnings)
	return errs, warnings
}

var reservedDiskNames = map[string]bool{
	"containerdisk": true,
	"cloudinitdisk": true,
	"datadisk":      true,
}

func validateManifest(m *CatalogManifest) error {
	seen := make(map[string]bool, len(m.Storage))
	for _, s := range m.Storage {
		if s.Name == "" {
			return ErrStorageNameEmpty
		}
		if reservedDiskNames[s.Name] {
			return fmt.Errorf("%q: %w", s.Name, ErrStorageNameReserved)
		}
		if seen[s.Name] {
			return fmt.Errorf("%q: %w", s.Name, ErrStorageNameDuplicate)
		}
		if _, err := resource.ParseQuantity(s.Size); err != nil {
			return fmt.Errorf("%q: %w", s.Size, ErrStorageSizeInvalid)
		}
		if s.Serial == "" {
			return ErrStorageSerialEmpty
		}
		if len(s.Serial) > 20 {
			return fmt.Errorf("%q: %w", s.Serial, ErrStorageSerialTooLong)
		}
		if !filepath.IsAbs(s.Mount) {
			return fmt.Errorf("%q: %w", s.Mount, ErrStorageMountNotAbsolute)
		}
		seen[s.Name] = true
	}

	if m.Service != nil {
		if len(m.Service.Ports) == 0 {
			return ErrServicePortsEmpty
		}
		for _, p := range m.Service.Ports {
			if p.Port < 1 || p.Port > 65535 {
				return fmt.Errorf("%d: %w", p.Port, ErrServicePortRange)
			}
			proto := strings.ToUpper(p.Protocol)
			if proto != "" && proto != "TCP" && proto != "UDP" {
				return fmt.Errorf("%q: %w", p.Protocol, ErrServiceProtocol)
			}
		}
	}
	return nil
}

func convertServicePorts(ports []ServicePort) []corev1.ServicePort {
	result := make([]corev1.ServicePort, len(ports))
	for i, p := range ports {
		proto := corev1.ProtocolTCP
		if strings.ToUpper(p.Protocol) == "UDP" {
			proto = corev1.ProtocolUDP
		}
		result[i] = corev1.ServicePort{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: intstr.FromInt32(p.Port),
			Protocol:   proto,
		}
	}
	return result
}

func buildCatalogServiceSpec(name, namespace string, def *ServiceDefinition) *corev1.Service {
	if def == nil {
		return nil
	}
	selector := map[string]string{
		constants.LabelAppName: "virtwork-" + name,
	}
	if def.SelectorRole != "" {
		selector = map[string]string{
			"virtwork/role": def.SelectorRole,
		}
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "virtwork-" + name,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:   constants.ManagedByValue,
				constants.LabelManagedBy: constants.ManagedByValue,
				constants.LabelComponent: name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports:    convertServicePorts(def.Ports),
		},
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
		if err := validateManifest(&entry.Manifest); err != nil {
			return nil, fmt.Errorf("validating manifest for %q: %w", entryName, err)
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
