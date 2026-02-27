// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	builtBinaryPath string
	buildOnce       sync.Once
	errBuild        error
)

// BinaryPath returns the path to the virtwork binary. It checks the
// VIRTWORK_BINARY environment variable first, then falls back to building
// the binary on first call. The build is performed only once per test run.
func BinaryPath() (string, error) {
	if p := os.Getenv("VIRTWORK_BINARY"); p != "" {
		return p, nil
	}
	buildOnce.Do(func() {
		builtBinaryPath, errBuild = buildBinary()
	})
	return builtBinaryPath, errBuild
}

// buildBinary compiles the virtwork binary into a temp directory and returns
// its path.
func buildBinary() (string, error) {
	tmpDir, err := os.MkdirTemp("", "virtwork-e2e-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	binaryName := "virtwork"
	if runtime.GOOS == "windows" {
		binaryName = "virtwork.exe"
	}
	outputPath := filepath.Join(tmpDir, binaryName)

	// Find the module root by walking up from this file's directory
	_, thisFile, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

	cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/virtwork")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("building virtwork binary: %w\nstderr: %s", err, stderr.String())
	}

	return outputPath, nil
}

// RunVirtwork executes the virtwork binary with the given arguments and
// returns stdout, stderr, and the exit code. The binary is built on first
// call if not provided via VIRTWORK_BINARY.
func RunVirtwork(args ...string) (stdout string, stderr string, exitCode int, err error) {
	binaryPath, err := BinaryPath()
	if err != nil {
		return "", "", -1, fmt.Errorf("getting binary path: %w", err)
	}

	cmd := exec.Command(binaryPath, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			return stdout, stderr, exitCode, nil
		}
		return stdout, stderr, -1, runErr
	}

	return stdout, stderr, 0, nil
}
