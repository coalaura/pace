// errorcheck -0 -m

// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inline

// Calls to cost push the functions below over the inlining budget.
// go:noinline takes precedence over go:inline.
//
//go:inline
//go:noinline
func cost(x int) int {
	return x + 1
}

func normal(x int) int {
	return cost(cost(cost(x)))
}

//go:inline
func forced(x int) int { // ERROR "can inline forced"
	return cost(cost(cost(x)))
}

// The body must still be checked for unsupported operations after exceeding
// the budget.
//
//go:inline
func unsupported(x int) int {
	x = cost(cost(cost(x)))
	defer cost(x) // ERROR "can inline unsupported.deferwrap1"
	return x
}

func calls(x int) int {
	return normal(x) + forced(x) + unsupported(x) // ERROR "inlining call to forced"
}
