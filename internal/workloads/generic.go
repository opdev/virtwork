// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads

import (
	"sort"
	"strings"

	"github.com/opdev/virtwork/internal/config"
)

// GenericWorkload implements Workload for single-role catalog entries.
type GenericWorkload struct {
	BaseWorkload
	entryName    string
	serviceFiles map[string]string
	packages     []string
}

// NewGenericWorkload creates a GenericWorkload from a loaded catalog entry.
func NewGenericWorkload(
	cfg config.WorkloadConfig,
	entry *CatalogEntry,
	sshUser, sshPassword string,
	sshKeys []string,
) *GenericWorkload {
	return &GenericWorkload{
		BaseWorkload: BaseWorkload{
			Config:            cfg,
			ParamSchema:       entry.Schema(),
			SSHUser:           sshUser,
			SSHPassword:       sshPassword,
			SSHAuthorizedKeys: sshKeys,
		},
		entryName:    entry.Name,
		serviceFiles: entry.ServiceFiles,
		packages:     entry.Manifest.Packages,
	}
}

// Name returns the catalog entry name.
func (w *GenericWorkload) Name() string {
	return w.entryName
}

// CloudInitUserdata returns cloud-init YAML with the entry's service files installed.
func (w *GenericWorkload) CloudInitUserdata() (string, error) {
	names := make([]string, 0, len(w.serviceFiles))
	for name := range w.serviceFiles {
		names = append(names, name)
	}
	sort.Strings(names)

	writeFiles := make([]WriteFile, 0, len(names))
	runcmd := [][]string{{"systemctl", "daemon-reload"}}

	for _, name := range names {
		content := w.substituteParams(w.serviceFiles[name])
		writeFiles = append(writeFiles, WriteFile{
			Path:        "/etc/systemd/system/" + name,
			Content:     content,
			Permissions: "0644",
		})
		runcmd = append(runcmd, []string{"systemctl", "enable", "--now", name})
	}

	return w.BuildCloudConfig(CloudConfigOpts{
		Packages:   w.packages,
		WriteFiles: writeFiles,
		RunCmd:     runcmd,
	})
}

func (w *GenericWorkload) substituteParams(content string) string {
	for _, p := range w.ParamSchema {
		content = strings.ReplaceAll(content, "{{"+p.Key+"}}", w.GetParam(p.Key))
	}
	return content
}
