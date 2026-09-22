// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import (
	"go/token"
	"slices"
	"strings"

	"cmd/compile/internal/ir"
	"cmd/compile/internal/syntax"
)

// readOnlyParams bridges the local Unified IR round trip by body index.
// Only the resulting escape tags, not the directive, are exported.
var readOnlyParams map[index][]string

func (pw *pkgWriter) checkReadOnly(decl *syntax.FuncDecl) {
	pragma, _ := decl.Pragma.(*pragmas)
	if pragma == nil || pragma.ReadOnly == nil {
		return
	}

	if decl.Body != nil {
		pw.errorf(pragma.ReadOnlyPos, "go:readonly requires a bodyless function")
		return
	}

	for index, name := range pragma.ReadOnly {
		name = strings.TrimSpace(name)
		pragma.ReadOnly[index] = name

		if !token.IsIdentifier(name) || name == "_" {
			pw.errorf(pragma.ReadOnlyPos, "malformed go:readonly parameter list")
			continue
		}

		if slices.Contains(pragma.ReadOnly[:index], name) {
			pw.errorf(pragma.ReadOnlyPos, "duplicate go:readonly parameter %q", name)
			continue
		}

		found := false
		for _, param := range decl.Type.ParamList {
			if param.Name != nil && param.Name.Value == name {
				found = true
				break
			}
		}

		if !found {
			pw.errorf(pragma.ReadOnlyPos, "unknown go:readonly input parameter %q", name)
		}
	}
}

func (w *writer) recordReadOnly(decl *syntax.FuncDecl, body index) {
	pragma, _ := decl.Pragma.(*pragmas)
	if pragma == nil || pragma.ReadOnly == nil {
		return
	}

	if readOnlyParams == nil {
		readOnlyParams = make(map[index][]string)
	}

	readOnlyParams[body] = pragma.ReadOnly
}

func (r *reader) applyReadOnly(fn *ir.Func, body index) {
	if r.p != localPkgReader {
		return
	}

	names := readOnlyParams[body]
	if len(names) == 0 {
		return
	}

	for _, param := range fn.Type().Params() {
		if param.Sym != nil && slices.Contains(names, param.Sym.Name) {
			param.SetReadOnly(true)
		}
	}
}
