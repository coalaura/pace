// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objabi

import (
	"os"
	"path/filepath"
	"strings"
)

// PACETool reports whether this is a compiler or assembler release executable.
// It controls release-only telemetry policy, not tool version identity.
func PACETool() bool {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	return name == "compilepe" || name == "asmpe"
}

// paceToolIdentity returns the canonical tool name for cmd/go's tool ID parser
// and whether PACE identity applies, including source-tree executables.
func paceToolIdentity(name string) (string, bool) {
	switch name {
	case "compile", "compilepe":
		return "compile", true
	case "asm", "asmpe":
		return "asm", true
	}
	return name, false
}
