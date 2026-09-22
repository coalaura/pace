// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import (
	"cmd/compile/internal/syntax"
	"cmd/compile/internal/types2"
)

// checkStatementPragmas binds a statement directive to its one allocation,
// before Unified IR serializes the body for local or cross-package inlining.
func (pw *pkgWriter) checkStatementPragmas(statement *syntax.AssignStmt) {
	pragma, _ := statement.Pragma.(*pragmas)
	if pragma == nil {
		return
	}

	position := pragma.MakeNoZeroPos
	pragma.MakeNoZeroPos = syntax.Pos{}
	pw.checkPragmas(pragma, 0, false)
	if !position.IsKnown() {
		return
	}

	_, singleName := statement.Lhs.(*syntax.Name)
	call, singleCall := statement.Rhs.(*syntax.CallExpr)
	if statement.Op != syntax.Def || !singleName || !singleCall ||
		position.Base() != statement.Lhs.Pos().Base() || position.Line()+1 != statement.Lhs.Pos().Line() {
		pw.errorf(position, "go:makenozero requires an immediately following single-name := make([]T, len[, cap]) statement")
		return
	}

	if !pw.isBuiltin(call.Fun, "make") {
		pw.errorf(position, "go:makenozero requires a slice make expression")
		return
	}

	slice, ok := types2.CoreType(pw.typeOf(call)).(*types2.Slice)
	if !ok {
		pw.errorf(position, "go:makenozero requires a slice make expression")
		return
	}
	if !pointerFree(slice.Elem()) {
		pw.errorf(position, "go:makenozero requires a pointer-free slice element type")
		return
	}

	if pw.makeNoZero == nil {
		pw.makeNoZero = make(map[*syntax.CallExpr]bool)
	}
	pw.makeNoZero[call] = true
}

// A type parameter must prove the property from its constraint, even if the
// function is never instantiated. Unknown or mixed core types are rejected.
func pointerFree(typ types2.Type) bool {
	switch underlying := types2.CoreType(typ).(type) {
	case *types2.Basic:
		return underlying.Info()&(types2.IsBoolean|types2.IsNumeric) != 0
	case *types2.Array:
		return pointerFree(underlying.Elem())
	case *types2.Struct:
		for index := 0; index < underlying.NumFields(); index++ {
			if !pointerFree(underlying.Field(index).Type()) {
				return false
			}
		}
		return true
	}
	return false
}
