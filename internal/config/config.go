// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/opdev/virtwork/internal/constants"
)

var (
	ErrInvalidNamespace = errors.New("invalid config: namespace must not be empty")
	ErrInvalidCpuCores  = errors.New("invalid config: cpu-cores must be at least 1, got")
	ErrInvalidMemory    = errors.New("invalid config: not a valid value for memory")
	ErrInvalidDiskSize  = errors.New("invalid config: not a valid value for disk-size")
	ErrInvalidTimeout   = errors.New(
		"invalid config: timeout must be at least 1 when wait-for-ready is enabled, got",
	)
	ErrParamMissingWorkload = errors.New("missing workload prefix (expected workload.key=value)")
	ErrParamEmptyWorkload   = errors.New("empty workload name")
	ErrParamMissingEquals   = errors.New("missing '=' in param (expected workload.key=value)")
	ErrParamEmptyKey        = errors.New("empty param key")
)

// WorkloadConfig holds per-workload configuration.
type WorkloadConfig struct {
	Enabled  *bool             `mapstructure:"enabled"`
	VMCount  int               `mapstructure:"vm-count"`
	CPUCores int               `mapstructure:"cpu-cores"`
	Memory   string            `mapstructure:"memory"`
	Params   map[string]string `mapstructure:"params"`
}

// Validate checks the assembled Config for semantic errors and returns
// a clear message naming the invalid field, the value, and what is expected.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Namespace) == "" {
		return ErrInvalidNamespace
	}
	if c.CPUCores < 1 {
		return fmt.Errorf("%w %d", ErrInvalidCpuCores, c.CPUCores)
	}
	if _, err := resource.ParseQuantity(c.Memory); err != nil {
		return fmt.Errorf("%w (%q): %w", ErrInvalidMemory, c.Memory, err)
	}
	if _, err := resource.ParseQuantity(c.DataDiskSize); err != nil {
		return fmt.Errorf("%w (%q): %w", ErrInvalidDiskSize, c.DataDiskSize, err)
	}
	if c.WaitForReady && c.ReadyTimeoutSeconds < 1 {
		return fmt.Errorf("%w %d", ErrInvalidTimeout, c.ReadyTimeoutSeconds)
	}
	return nil
}

// Config holds the complete application configuration.
type Config struct {
	Namespace           string                    `mapstructure:"namespace"`
	ContainerDiskImage  string                    `mapstructure:"container-disk-image"`
	DataDiskSize        string                    `mapstructure:"disk-size"`
	CPUCores            int                       `mapstructure:"cpu-cores"`
	Memory              string                    `mapstructure:"memory"`
	Workloads           map[string]WorkloadConfig `mapstructure:"workloads"`
	KubeconfigPath      string                    `mapstructure:"kubeconfig"`
	ClusterContext      string                    // Runtime-only: current kubeconfig context
	CleanupMode         string                    `mapstructure:"cleanup-mode"`
	WaitForReady        bool                      `mapstructure:"wait-for-ready"`
	ReadyTimeoutSeconds int                       `mapstructure:"timeout"`
	DryRun              bool                      `mapstructure:"dry-run"`
	Verbose             bool                      `mapstructure:"verbose"`
	SSHUser             string                    `mapstructure:"ssh-user"`
	SSHPassword         string                    `mapstructure:"ssh-password"`
	SSHAuthorizedKeys   []string                  `mapstructure:"ssh-authorized-keys"`
	AuditEnabled        bool                      `mapstructure:"audit"`
	AuditDBPath         string                    `mapstructure:"audit-db"`
	VMConcurrency       int                       `mapstructure:"vm-concurrency"`
	CatalogDir          string                    `mapstructure:"catalog-dir"`
	FromCatalog         []string                  `mapstructure:"from-catalog"`
}

// SetDefaults registers Viper defaults.
func SetDefaults(v *viper.Viper) {
	v.SetDefault("namespace", constants.DefaultNamespace)
	v.SetDefault("container-disk-image", constants.DefaultContainerDiskImage)
	v.SetDefault("disk-size", constants.DefaultDiskSize)
	v.SetDefault("cpu-cores", constants.DefaultCPUCores)
	v.SetDefault("memory", constants.DefaultMemory)
	v.SetDefault("wait-for-ready", true)
	v.SetDefault("timeout", 600)
	v.SetDefault("dry-run", false)
	v.SetDefault("verbose", false)
	v.SetDefault("ssh-user", constants.DefaultSSHUser)
	v.SetDefault("ssh-password", "")
	v.SetDefault("kubeconfig", "")
	v.SetDefault("cleanup-mode", "")
	v.SetDefault("audit", true)
	v.SetDefault("audit-db", constants.DefaultAuditDBPath)
	v.SetDefault("vm-concurrency", constants.DefaultVMConcurrency)
}

// BindPersistentFlags registers persistent flags shared across all subcommands.
func BindPersistentFlags(cmd *cobra.Command) {
	pf := cmd.PersistentFlags()
	pf.String("namespace", "", "Kubernetes namespace for VMs")
	pf.String("kubeconfig", "", "Path to kubeconfig file")
	pf.String("config", "", "Path to YAML config file")
	pf.Bool("verbose", false, "Enable verbose output")
	pf.Bool("audit", true, "Enable audit logging to SQLite")
	pf.Bool("no-audit", false, "Disable audit logging")
	pf.String("audit-db", "", "Path to audit database file")
}

// BindRunFlags registers flags specific to the "run" subcommand.
// defaultWorkloads sets the default value for the --workloads flag.
func BindRunFlags(cmd *cobra.Command, defaultWorkloads []string) {
	f := cmd.Flags()
	f.StringSlice("workloads", defaultWorkloads, "Workloads to deploy (comma-separated)")
	f.Int("vm-count", 1, "Number of VMs per workload")
	f.Int("cpu-cores", 0, "CPU cores per VM")
	f.String("memory", "", "Memory per VM (e.g., 2Gi)")
	f.String("disk-size", "", "Data disk size")
	f.String("container-disk-image", "", "Container disk image for VMs")
	f.Bool("dry-run", false, "Print specs without creating resources")
	f.Bool("no-wait", false, "Skip waiting for VM readiness")
	f.Int("timeout", 0, "Readiness timeout in seconds")
	f.String("ssh-user", "", "SSH user for VMs")
	f.String("ssh-password", "", "SSH password for VMs")
	f.StringSlice("ssh-key", nil, "SSH authorized key (repeatable)")
	f.StringSlice("ssh-key-file", nil, "SSH key file path (repeatable)")
	f.Int("vm-concurrency", constants.DefaultVMConcurrency, "Max concurrent VM creation operations")
	f.String("params", "", "Per-workload params (comma-separated workload.key=value pairs)")
	f.String("catalog-dir", "", "Path to catalog directory (default ~/.virtwork/catalog)")
	f.StringSlice("from-catalog", nil, "Catalog entries to load (comma-separated)")
}

// BindCleanupFlags registers flags specific to the "cleanup" subcommand.
func BindCleanupFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.Bool("delete-namespace", false, "Also delete the namespace")
	f.String("run-id", "", "Only delete resources from this specific run (UUID)")
	f.Bool("dry-run", false, "Print intent without destroying resources")
	f.BoolP("yes", "y", false, "Skip confirmation prompt and proceed with cleanup")
}

func BindValidateFlags(cmd *cobra.Command) {
	cmd.Flags().String("catalog-dir", "", "Path to catalog directory (default ~/.virtwork/catalog)")
}

// LoadConfig loads configuration from flags, environment variables, config file,
// and defaults using the Viper priority chain: flags > env > file > defaults.
func LoadConfig(cmd *cobra.Command) (*Config, error) {
	v := viper.New()

	// Set defaults first (lowest priority)
	SetDefaults(v)

	// Environment variables (middle priority)
	v.SetEnvPrefix("VIRTWORK")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	// Config file (if specified via --config flag)
	configPath, _ := cmd.Flags().GetString("config")
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	// Bind flags (highest priority — only overrides when explicitly set)
	bindFlagIfSet(v, cmd, "namespace")
	bindFlagIfSet(v, cmd, "kubeconfig")
	bindFlagIfSet(v, cmd, "container-disk-image")
	bindFlagIfSet(v, cmd, "disk-size")
	bindFlagIfSet(v, cmd, "memory")
	bindFlagIfSet(v, cmd, "ssh-user")
	bindFlagIfSet(v, cmd, "ssh-password")

	if cmd.Flags().Changed("cpu-cores") {
		val, _ := cmd.Flags().GetInt("cpu-cores")
		v.Set("cpu-cores", val)
	}
	if cmd.Flags().Changed("timeout") {
		val, _ := cmd.Flags().GetInt("timeout")
		v.Set("timeout", val)
	}
	if cmd.Flags().Changed("dry-run") {
		val, _ := cmd.Flags().GetBool("dry-run")
		v.Set("dry-run", val)
	}
	if cmd.Flags().Changed("verbose") {
		val, _ := cmd.Flags().GetBool("verbose")
		v.Set("verbose", val)
	}
	if cmd.Flags().Changed("no-wait") {
		val, _ := cmd.Flags().GetBool("no-wait")
		v.Set("wait-for-ready", !val)
	}
	if cmd.Flags().Changed("vm-concurrency") {
		val, _ := cmd.Flags().GetInt("vm-concurrency")
		v.Set("vm-concurrency", val)
	}

	// Build the Config struct
	cfg := &Config{}
	cfg.Namespace = v.GetString("namespace")
	cfg.ContainerDiskImage = v.GetString("container-disk-image")
	cfg.DataDiskSize = v.GetString("disk-size")
	cfg.CPUCores = v.GetInt("cpu-cores")
	cfg.Memory = v.GetString("memory")
	cfg.KubeconfigPath = v.GetString("kubeconfig")
	cfg.CleanupMode = v.GetString("cleanup-mode")
	cfg.WaitForReady = v.GetBool("wait-for-ready")
	cfg.ReadyTimeoutSeconds = v.GetInt("timeout")
	cfg.DryRun = v.GetBool("dry-run")
	cfg.Verbose = v.GetBool("verbose")
	cfg.SSHUser = v.GetString("ssh-user")
	cfg.SSHPassword = v.GetString("ssh-password")
	cfg.AuditEnabled = v.GetBool("audit")
	cfg.AuditDBPath = v.GetString("audit-db")
	cfg.VMConcurrency = v.GetInt("vm-concurrency")

	// Catalog settings
	bindFlagIfSet(v, cmd, "catalog-dir")
	cfg.CatalogDir = v.GetString("catalog-dir")
	if cfg.CatalogDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home directory for catalog-dir: %w", err)
		}
		cfg.CatalogDir = filepath.Join(home, ".virtwork", "catalog")
	}

	if cmd.Flags().Changed("from-catalog") {
		cfg.FromCatalog, _ = cmd.Flags().GetStringSlice("from-catalog")
	} else {
		cfg.FromCatalog = v.GetStringSlice("from-catalog")
	}

	// Handle SSH authorized keys: CLI flags, env var (comma-split), or YAML list
	sshKeys, err := resolveSSHKeys(v, cmd)
	if err != nil {
		return nil, err
	}
	cfg.SSHAuthorizedKeys = sshKeys

	// Unmarshal workloads map if present in config file
	workloads := make(map[string]WorkloadConfig)
	if v.IsSet("workloads") {
		if err := v.UnmarshalKey("workloads", &workloads); err != nil {
			return nil, fmt.Errorf("parsing workloads config: %w", err)
		}
	}
	cfg.Workloads = workloads

	// Merge CLI/env params into workload configs (CLI flag > env var > YAML)
	rawParams := resolveRawParams(cmd)
	if rawParams != "" {
		parsed, err := ParseParams(rawParams)
		if err != nil {
			return nil, fmt.Errorf("parsing --params: %w", err)
		}
		for wl, params := range parsed {
			wlCfg := cfg.Workloads[wl]
			if wlCfg.Params == nil {
				wlCfg.Params = make(map[string]string)
			}
			maps.Copy(wlCfg.Params, params)
			cfg.Workloads[wl] = wlCfg
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ParseParams parses a comma-separated string of "workload.key=value" pairs
// into a per-workload param map.
func ParseParams(raw string) (map[string]map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return make(map[string]map[string]string), nil
	}

	result := make(map[string]map[string]string)
	for pair := range strings.SplitSeq(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		workload, rest, hasDot := strings.Cut(pair, ".")
		if !hasDot {
			return nil, fmt.Errorf("%w: %q", ErrParamMissingWorkload, pair)
		}
		if workload == "" {
			return nil, fmt.Errorf("%w in %q", ErrParamEmptyWorkload, pair)
		}

		key, value, hasEq := strings.Cut(rest, "=")
		if !hasEq {
			return nil, fmt.Errorf("%w: %q", ErrParamMissingEquals, pair)
		}
		if key == "" {
			return nil, fmt.Errorf("%w in %q", ErrParamEmptyKey, pair)
		}

		if result[workload] == nil {
			result[workload] = make(map[string]string)
		}
		result[workload][key] = value
	}
	return result, nil
}

// bindFlagIfSet sets a Viper key from a Cobra flag only when the flag was explicitly provided.
func bindFlagIfSet(v *viper.Viper, cmd *cobra.Command, name string) {
	if cmd.Flags().Changed(name) {
		val, _ := cmd.Flags().GetString(name)
		v.Set(name, val)
	}
}

// resolveSSHKeys resolves SSH authorized keys from CLI flags, env vars, or config file.
// Priority: CLI --ssh-key flags > VIRTWORK_SSH_AUTHORIZED_KEYS env var > YAML config.
func resolveSSHKeys(v *viper.Viper, cmd *cobra.Command) ([]string, error) {
	// CLI flags take highest priority — merge inline keys and file-based keys
	var cliKeys []string

	if cmd.Flags().Changed("ssh-key") {
		keys, _ := cmd.Flags().GetStringSlice("ssh-key")
		cliKeys = append(cliKeys, keys...)
	}

	if cmd.Flags().Changed("ssh-key-file") {
		paths, _ := cmd.Flags().GetStringSlice("ssh-key-file")
		for _, p := range paths {
			data, err := os.ReadFile(
				filepath.Clean(p),
			) //nolint:gosec // CLI user supplies the path intentionally
			if err != nil {
				return nil, fmt.Errorf("reading SSH key file %s: %w", p, err)
			}
			key := strings.TrimSpace(string(data))
			if key != "" {
				cliKeys = append(cliKeys, key)
			}
		}
	}

	if len(cliKeys) > 0 {
		return cliKeys, nil
	}

	// Check env var with comma splitting
	envVal := os.Getenv("VIRTWORK_SSH_AUTHORIZED_KEYS")
	if envVal != "" {
		parts := strings.Split(envVal, ",")
		keys := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				keys = append(keys, trimmed)
			}
		}
		if len(keys) > 0 {
			return keys, nil
		}
	}

	// Fall back to YAML config list
	return v.GetStringSlice("ssh-authorized-keys"), nil
}

// resolveRawParams returns the raw params string from CLI flag or env var.
// Priority: CLI --params flag > VIRTWORK_PARAMS env var.
func resolveRawParams(cmd *cobra.Command) string {
	if cmd.Flags().Changed("params") {
		val, _ := cmd.Flags().GetString("params")
		return val
	}
	return os.Getenv("VIRTWORK_PARAMS")
}
