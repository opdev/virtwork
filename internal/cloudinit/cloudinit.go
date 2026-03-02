// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package cloudinit

import (
	"maps"

	"gopkg.in/yaml.v3"
)

// WriteFile represents a file to be written by cloud-init.
type WriteFile struct {
	Path        string `yaml:"path"`
	Content     string `yaml:"content"`
	Permissions string `yaml:"permissions"`
}

// CloudConfigOpts holds the options for building a cloud-config YAML document.
type CloudConfigOpts struct {
	Packages          []string
	WriteFiles        []WriteFile
	RunCmd            [][]string
	Extra             map[string]interface{}
	SSHUser           string
	SSHPassword       string
	SSHAuthorizedKeys []string
}

// BuildCloudConfig produces a cloud-init YAML document from the given options.
// The output begins with the required "#cloud-config\n" header.
// Empty/nil fields are omitted from the output.
func BuildCloudConfig(opts CloudConfigOpts) (string, error) {
	doc := make(map[string]interface{})

	if len(opts.Packages) > 0 {
		doc["packages"] = opts.Packages
	}

	if len(opts.WriteFiles) > 0 {
		doc["write_files"] = opts.WriteFiles
	}

	if len(opts.RunCmd) > 0 {
		doc["runcmd"] = opts.RunCmd
	}

	// SSH user block
	if opts.SSHUser != "" {
		user := map[string]interface{}{
			"name":  opts.SSHUser,
			"sudo":  "ALL=(ALL) NOPASSWD:ALL",
			"shell": "/bin/bash",
		}

		if opts.SSHPassword != "" {
			user["lock_passwd"] = false
			user["plain_text_passwd"] = opts.SSHPassword
			doc["ssh_pwauth"] = true
		} else {
			user["lock_passwd"] = true
		}

		if len(opts.SSHAuthorizedKeys) > 0 {
			user["ssh_authorized_keys"] = opts.SSHAuthorizedKeys
		}

		doc["users"] = []map[string]interface{}{user}
	}

	// Merge extra keys at top level
	maps.Copy(doc, opts.Extra)

	if len(doc) == 0 {
		return "#cloud-config\n", nil
	}

	yamlBytes, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}

	return "#cloud-config\n" + string(yamlBytes), nil
}
