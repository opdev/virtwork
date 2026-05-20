// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"bytes"
	"errors"
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

// BinaryPath returns the path to the virtwork binary for E2E tests.
// It is safe for concurrent use and performs at most one build per test run.
//
// Resolution order:
//  1. VIRTWORK_BINARY environment variable (if set)
//  2. Build from source (on first call, cached thereafter)
//
// The binary is built with CGO_ENABLED=0 for portability and placed in a
// temporary directory. The build finds the module root automatically by
// walking up from this file's location.
//
// Example:
//
//	BeforeSuite(func() {
//	    // Trigger build early so first test doesn't pay the cost
//	    _, err := testutil.BinaryPath()
//	    Expect(err).NotTo(HaveOccurred())
//	})
//
// Returns an error if the build fails. Subsequent calls return the cached
// error, so build failures are deterministic across test runs.
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

	//nolint:gosec
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
// returns stdout, stderr, exit code, and any execution error. This is the
// primary function for E2E tests that exercise the CLI as a black box.
//
// The binary is obtained via BinaryPath(), which builds from source on first
// call if VIRTWORK_BINARY is not set. Build errors are returned as err.
//
// Exit code handling:
//  - exitCode is 0 and err is nil on successful execution
//  - exitCode is non-zero and err is nil on command failure (expected failures)
//  - exitCode is -1 and err is non-nil if the command could not be executed
//
// Example:
//
//	stdout, stderr, exitCode, err := testutil.RunVirtwork("run", "--dry-run")
//	Expect(err).NotTo(HaveOccurred())
//	Expect(exitCode).To(Equal(0))
//	Expect(stdout).To(ContainSubstring("VirtualMachine"))
//
// Both stdout and stderr are captured and returned even when the command fails,
// enabling assertion on error messages in tests.
func RunVirtwork(args ...string) (stdout string, stderr string, exitCode int, err error) {
	binaryPath, err := BinaryPath()
	if err != nil {
		return "", "", -1, fmt.Errorf("getting binary path: %w", err)
	}

	//nolint:gosec
	cmd := exec.Command(binaryPath, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if runErr != nil {
		exitErr := &exec.ExitError{}
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			return stdout, stderr, exitCode, nil
		}
		return stdout, stderr, -1, runErr
	}

	return stdout, stderr, 0, nil
}
