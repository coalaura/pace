// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package escape

import (
	"go/constant"
	"internal/abi"

	"cmd/compile/internal/base"
	"cmd/compile/internal/ir"
	"cmd/compile/internal/types"
)

// checkMustStack runs only after every location in the batch has its final
// escape state. It diagnoses assertions without changing any escape decisions.
func checkMustStack(fn *ir.Func) {
	for _, variable := range fn.Dcl {
		statement, ok := variable.Defn.(*ir.AssignStmt)
		if !ok || !statement.MustStack {
			continue
		}

		name := variable.Sym().Name
		if variable.Esc() == ir.EscHeap {
			base.ErrorfAt(statement.Pos(), 0, "go:muststack: variable %s escapes to heap", name)
			continue
		}

		if mustStackHeap(statement.Y) {
			base.ErrorfAt(statement.Pos(), 0, "go:muststack: initial storage for %s may be heap allocated", name)
		}
	}
}

// Only allocations in the initializer are checked, not storage reached through
// existing names or the bodies of called functions. Aggregate literals naturally
// include their directly-created element/field allocations in this traversal.
func mustStackHeap(node ir.Node) bool {
	if node == nil {
		return false
	}
	if mustStackAllocationHeap(node) {
		return true
	}
	if node.Op() == ir.OINLCALL {
		// Inlining must not extend the assertion into the callee body, but
		// its arguments (now in Init) still belong to the initializer.
		for _, statement := range node.Init() {
			if mustStackHeap(statement) {
				return true
			}
		}
		return false
	}
	return ir.DoChildren(node, mustStackHeap)
}

func mustStackAllocationHeap(node ir.Node) bool {
	switch node.Op() {
	case ir.ONEW, ir.OPTRLIT, ir.OSLICELIT, ir.OCLOSURE, ir.OMETHVALUE, ir.ORUNESTR:
		return node.Esc() == ir.EscHeap

	case ir.OCONVIFACE:
		valueType := node.(*ir.ConvExpr).X.Type()
		// walk.dataWord only uses a stack temporary for values up to 1024 bytes.
		return node.Esc() == ir.EscHeap || !valueType.IsInterface() && !types.IsDirectIface(valueType) && valueType.Size() > 1024

	case ir.OMAKESLICE:
		allocation := node.(*ir.MakeExpr)
		capacity := allocation.Cap
		if capacity == nil {
			capacity = allocation.Len
		}

		// EscNone permits a variable-sized make to use a stack buffer with a
		// heap fallback. Only a constant capacity guarantees stack storage.
		// Escape analysis has already folded capacities it can prove constant.
		return node.Esc() != ir.EscNone || !ir.IsSmallIntConst(capacity)

	case ir.OMAKEMAP:
		allocation := node.(*ir.MakeExpr)
		// Escape state describes the map header; larger or dynamic hints can
		// still allocate groups in runtime.makemap (see walkMakeMap).
		return node.Esc() != ir.EscNone || !ir.IsSmallIntConst(allocation.Len) || ir.Int64Val(allocation.Len) > abi.MapGroupSlots

	case ir.OMAPLIT:
		allocation := node.(*ir.CompLitExpr)
		return node.Esc() != ir.EscNone || len(allocation.List) > abi.MapGroupSlots ||
			node.Type().Key().Size() > abi.MapMaxKeyBytes || node.Type().Elem().Size() > abi.MapMaxElemBytes

	case ir.OMAKECHAN:
		// Channels have no stack allocation path.
		return true

	case ir.OSTR2BYTES:
		value := node.(*ir.ConvExpr).X
		// Constant strings have a fixed-size allocation in walkStringToBytes.
		return node.Esc() != ir.EscNone || !ir.IsConst(value, constant.String) || int64(len(ir.StringVal(value))) > ir.MaxImplicitStackVarSize

	case ir.OAPPEND, ir.OADDSTR, ir.OSTR2RUNES, ir.OBYTES2STR, ir.ORUNES2STR:
		// These operations may call allocating runtime helpers even with
		// EscNone; escape state alone cannot guarantee stack-only storage.
		return true
	}
	return false
}
