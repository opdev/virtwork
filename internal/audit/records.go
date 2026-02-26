// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package audit

// WorkloadRecord holds data for inserting a workload_details row.
type WorkloadRecord struct {
	WorkloadType    string
	Enabled         bool
	VMCount         int
	CPUCores        int
	Memory          string
	HasDataDisk     bool
	DataDiskSize    string
	RequiresService bool
}

// VMRecord holds data for inserting a vm_details row.
type VMRecord struct {
	VMName             string
	Namespace          string
	Component          string
	Role               string
	CPUCores           int
	Memory             string
	ContainerDiskImage string
	HasDataDisk        bool
	DataDiskSize       string
}

// ResourceRecord holds data for inserting a resource_details row.
type ResourceRecord struct {
	ResourceType string
	ResourceName string
	Namespace    string
}

// EventRecord holds data for inserting an events row.
type EventRecord struct {
	VMID        *int64
	WorkloadID  *int64
	EventType   string
	Message     string
	ErrorDetail string
}
