// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/opdev/virtwork/internal/constants"
)

// WorkloadConfig holds per-workload configuration.
type WorkloadConfig struct {
	Enabled  bool              `mapstructure:"enabled"`
	VMCount  int               `mapstructure:"vm-count"`
	CPUCores int               `mapstructure:"cpu-cores"`
	Memory   string            `mapstructure:"memory"`
	Params   map[string]string `mapstructure:"params"`
}

// Config holds the complete application configuration.
type Config struct {
	Namespace           string                    `mapstructure:"namespace"`
	ContainerDiskImage  string                    `mapstructure:"container-disk-image"`
	DataDiskSize        string                    `mapstructure:"data-disk-size"`
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
}

// SetDefaults registers Viper defaults.
func SetDefaults(v *viper.Viper) {
	v.SetDefault("namespace", constants.DefaultNamespace)
	v.SetDefault("container-disk-image", constants.DefaultContainerDiskImage)
	v.SetDefault("data-disk-size", constants.DefaultDiskSize)
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
}

// BindFlags registers Cobra flags on the given command.
func BindFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.String("namespace", "", "Kubernetes namespace for VMs")
	f.String("kubeconfig", "", "Path to kubeconfig file")
	f.String("config", "", "Path to YAML config file")
	f.String("container-disk-image", "", "Container disk image for VMs")
	f.String("data-disk-size", "", "Data disk size")
	f.Int("cpu-cores", 0, "CPU cores per VM")
	f.String("memory", "", "Memory per VM (e.g., 2Gi)")
	f.Bool("dry-run", false, "Print specs without creating resources")
	f.Bool("no-wait", false, "Skip waiting for VM readiness")
	f.Int("timeout", 0, "Readiness timeout in seconds")
	f.Bool("verbose", false, "Enable verbose output")
	f.String("ssh-user", "", "SSH user for VMs")
	f.String("ssh-password", "", "SSH password for VMs")
	f.StringSlice("ssh-key", nil, "SSH authorized key (repeatable)")
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
	bindFlagIfSet(v, cmd, "data-disk-size")
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

	// Build the Config struct
	cfg := &Config{}
	cfg.Namespace = v.GetString("namespace")
	cfg.ContainerDiskImage = v.GetString("container-disk-image")
	cfg.DataDiskSize = v.GetString("data-disk-size")
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

	// Handle SSH authorized keys: CLI flags, env var (comma-split), or YAML list
	cfg.SSHAuthorizedKeys = resolveSSHKeys(v, cmd)

	// Unmarshal workloads map if present in config file
	workloads := make(map[string]WorkloadConfig)
	if v.IsSet("workloads") {
		if err := v.UnmarshalKey("workloads", &workloads); err != nil {
			return nil, fmt.Errorf("parsing workloads config: %w", err)
		}
	}
	cfg.Workloads = workloads

	return cfg, nil
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
func resolveSSHKeys(v *viper.Viper, cmd *cobra.Command) []string {
	// CLI flags take highest priority
	if cmd.Flags().Changed("ssh-key") {
		keys, _ := cmd.Flags().GetStringSlice("ssh-key")
		if len(keys) > 0 {
			return keys
		}
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
			return keys
		}
	}

	// Fall back to YAML config list
	return v.GetStringSlice("ssh-authorized-keys")
}
