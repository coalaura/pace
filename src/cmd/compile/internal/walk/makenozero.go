// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package walk

import (
	"cmd/compile/internal/base"
	"cmd/compile/internal/ir"
	"cmd/compile/internal/typecheck"
	"cmd/compile/internal/types"
)

// walkMakeSliceNoZero validates heap slice dimensions before allocating unscanned,
// uninitialized storage. Its arguments have the same signed type that an ordinary
// make would pass to makeslice or makeslice64.
func walkMakeSliceNoZero(n *ir.MakeExpr, length, capacity ir.Node, init *ir.Nodes) *ir.CallExpr {
	length = cheapExpr(length, init)
	capacity = cheapExpr(capacity, init)
	intType := types.Types[types.TINT]
	uintptrType := types.Types[types.TUINTPTR]

	// Match makeslice64's ordered narrowing checks, including when a uint64
	// dimension was first converted to int64. Both checks precede validation of
	// the narrowed values: a negative length with a wide capacity is a cap panic.
	if length.Type().Size() > intType.Size() {
		narrowLength := typecheck.Conv(typecheck.Conv(length, intType), length.Type())
		checkMakeSliceNoZero(ir.NewBinaryExpr(base.Pos, ir.ONE, narrowLength, length), "panicmakeslicelen", init)
		narrowCapacity := typecheck.Conv(typecheck.Conv(capacity, intType), capacity.Type())
		checkMakeSliceNoZero(ir.NewBinaryExpr(base.Pos, ir.ONE, narrowCapacity, capacity), "panicmakeslicecap", init)
	}

	length = typecheck.Conv(length, intType)
	capacity = typecheck.Conv(capacity, intType)
	var invalidLength ir.Node = ir.NewBinaryExpr(base.Pos, ir.OLT, length, ir.NewInt(base.Pos, 0))
	var invalidCapacity ir.Node = ir.NewBinaryExpr(base.Pos, ir.OGT, length, capacity)
	elementSize := n.Type().Elem().Size()

	if elementSize > 1 {
		// Dividing the target's uintptr maximum by the constant element size
		// detects multiplication overflow without overflowing the check itself.
		// This is an arithmetic bound, deliberately independent of runtime's
		// address-space/allocation limits. Sizes zero and one cannot overflow.
		maxUintptr := ^uint64(0) >> (64 - uint(types.PtrSize)*8)
		maxElements := ir.NewUintptr(base.Pos, int64(maxUintptr/uint64(elementSize)))
		lengthOverflow := ir.NewBinaryExpr(base.Pos, ir.OGT, typecheck.Conv(length, uintptrType), maxElements)
		capacityOverflow := ir.NewBinaryExpr(base.Pos, ir.OGT, typecheck.Conv(capacity, uintptrType), maxElements)
		invalidLength = ir.NewLogicalExpr(base.Pos, ir.OOROR, invalidLength, lengthOverflow)
		invalidCapacity = ir.NewLogicalExpr(base.Pos, ir.OOROR, invalidCapacity, capacityOverflow)
	}

	// For nonnegative dimensions, length overflow implies capacity overflow or
	// length > capacity. Checking length first preserves makeslice's preference
	// for a length panic when both dimensions are invalid (also for implicit cap).
	checkMakeSliceNoZero(invalidLength, "panicmakeslicelen", init)
	checkMakeSliceNoZero(invalidCapacity, "panicmakeslicecap", init)

	size := ir.NewBinaryExpr(base.Pos, ir.OMUL, typecheck.Conv(capacity, uintptrType), ir.NewUintptr(base.Pos, elementSize))
	return mkcall1(typecheck.LookupRuntime("mallocgc"), types.Types[types.TUNSAFEPTR], init,
		size, typecheck.NodNil(), ir.NewBool(base.Pos, false))
}

func checkMakeSliceNoZero(condition ir.Node, panicName string, init *ir.Nodes) {
	check := ir.NewIfStmt(base.Pos, condition, nil, nil)
	check.Body.Append(mkcall(panicName, nil, &check.Body))
	appendWalkStmt(init, check)
}
