// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package constants

import "time"

// KubeVirt API coordinates.
const (
	KubevirtAPIGroup   = "kubevirt.io"
	KubevirtAPIVersion = "v1"
	KubevirtVMPlural   = "virtualmachines"
	KubevirtVMIPlural  = "virtualmachineinstances"
)

// CDI (Containerized Data Importer) API coordinates.
const (
	CDIAPIGroup   = "cdi.kubevirt.io"
	CDIAPIVersion = "v1beta1"
	CDIDVPlural   = "datavolumes"
)

// Default resource values.
const (
	DefaultContainerDiskImage = "quay.io/containerdisks/fedora:41"
	DefaultNamespace          = "virtwork"
	DefaultCPUCores           = 2
	DefaultMemory             = "2Gi"
	DefaultDiskSize           = "10Gi"
	DefaultSSHUser            = "virtwork"
)

// KubeVirt disk, volume, and network names.
const (
	DiskNameContainerDisk = "containerdisk"
	DiskNameCloudInit     = "cloudinitdisk"
	DiskNameData          = "datadisk"
	DiskBusVirtio         = "virtio"
	NetworkNameDefault    = "default"
)

// Secret data keys.
const SecretKeyUserdata = "userdata"

// SSH cloud-init user configuration.
const (
	SSHSudoRule     = "ALL=(ALL) NOPASSWD:ALL"
	SSHDefaultShell = "/bin/bash"
)

// Kubernetes recommended labels.
const (
	LabelAppName   = "app.kubernetes.io/name"
	LabelManagedBy = "app.kubernetes.io/managed-by"
	LabelComponent = "app.kubernetes.io/component"
	ManagedByValue = "virtwork"
	LabelRunID     = "virtwork/run-id"
)

// Kubernetes Secret size limit (1 MiB).
const MaxSecretDataSize = 1 << 20

// Audit defaults.
const (
	DefaultAuditDBPath = "virtwork.db"
)

// Polling defaults for VMI readiness.
const (
	DefaultReadyTimeout = 600 * time.Second
	DefaultPollInterval = 15 * time.Second
)
