// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import "cmd/compile/internal/ir"

// markNoBounds runs after type checking, before any calls in body are inlined.
// The inliner also calls it when re-reading a body, using the callee's pragma.
// This preserves the origin of each operation without changing the export format.
func markNoBounds(fn *ir.Func, body []ir.Node) {
	if fn.Pragma&ir.NoBounds == 0 {
		return
	}

	// VisitList does not descend into function literals, which have their own bodies.
	ir.VisitList(body, func(node ir.Node) {
		switch node.Op() {
		case ir.OINDEX:
			node.(*ir.IndexExpr).SetBounded(true)
		case ir.OSLICE, ir.OSLICEARR, ir.OSLICESTR, ir.OSLICE3, ir.OSLICE3ARR:
			node.(*ir.SliceExpr).SetBounded(true)
		case ir.OSLICE2ARR, ir.OSLICE2ARRPTR:
			node.(*ir.ConvExpr).SetBounded(true)
		}
	})
}
