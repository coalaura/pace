// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pace contains the distribution policy for the three-tool PACE overlay.
// The ordinary source-tree go command does not enable this policy.
package pace

import (
	"fmt"
	"go/version"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Enabled reports whether this command was invoked under its release name.
func Enabled() bool {
	name := filepath.Base(os.Args[0])
	return name == "pace" || name == "pace.exe"
}

// Check validates the overlay before command execution or toolchain selection.
func Check(goroot string) error {
	if !Enabled() {
		return nil
	}

	err := checkVersion(goroot, runtime.Version())
	if err != nil {
		return err
	}

	tools := [...]string{"compile", "asm"}
	for _, tool := range tools {
		_, err = ToolPath(tool)
		if err != nil {
			return err
		}
	}

	return nil
}

// ToolPath returns an override only for PACE's compiler and assembler.
// All other tools continue to come from the selected standard GOROOT.
func ToolPath(tool string) (string, error) {
	if !Enabled() || (tool != "compile" && tool != "asm") {
		return "", nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("pace: locating executable: %w", err)
	}

	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("pace: resolving executable: %w", err)
	}

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}

	name := tool + "pe" + suffix
	path := filepath.Join(filepath.Dir(executable), name)
	info, err := os.Stat(path)
	if err != nil {
		return path, fmt.Errorf("pace: required helper %s must be beside pace: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return path, fmt.Errorf("pace: required helper %s is not a regular file: %s", name, path)
	}

	return path, nil
}

func checkVersion(goroot, required string) error {
	// Development toolchains need not have a release VERSION file. Normal
	// bootstrap binaries are named go and never reach this check at all.
	if !version.IsValid(required) {
		return nil
	}

	data, err := os.ReadFile(filepath.Join(goroot, "VERSION"))
	if err != nil {
		return fmt.Errorf("pace: this build requires Go %s; reading GOROOT VERSION: %w", strings.TrimPrefix(required, "go"), err)
	}

	installed, _, _ := strings.Cut(string(data), "\n")
	installed = strings.TrimSpace(installed)
	if installed != required {
		return fmt.Errorf("pace: this build requires Go %s; GOROOT contains Go %s",
			strings.TrimPrefix(required, "go"), strings.TrimPrefix(installed, "go"))
	}

	return nil
}
