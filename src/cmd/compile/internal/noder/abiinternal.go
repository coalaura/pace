// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import "cmd/compile/internal/syntax"

// ABIInternal holds directives for local declarations only. They deliberately
// do not enter Unified IR: callers use the ordinary ABIInternal convention.
var ABIInternal = make(map[string]string)

func (pw *pkgWriter) recordABIInternal(decl *syntax.FuncDecl) {
	pragma, _ := decl.Pragma.(*pragmas)
	if pragma == nil || pragma.ABIInternal == "" {
		return
	}
	if decl.Body != nil {
		pw.errorf(pragma.ABIInternalPos, "go:abiinternal requires a bodyless function")
		return
	}
	if decl.Recv != nil {
		pw.errorf(pragma.ABIInternalPos, "go:abiinternal does not support methods")
		return
	}
	if decl.Name.Value == "_" {
		pw.errorf(pragma.ABIInternalPos, "go:abiinternal requires a named function")
		return
	}
	ABIInternal[decl.Name.Value] = pragma.ABIInternal
}
