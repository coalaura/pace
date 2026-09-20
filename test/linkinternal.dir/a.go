// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package a

import "sync/atomic"

//go:linkinternal internal/runtime/atomic.Cas64
func Cas64(ptr *uint64, old, new uint64) bool {
	return atomic.CompareAndSwapUint64(ptr, old, new) // ERROR "intrinsic substitution for CompareAndSwapUint64"
}

func Plain(ptr *uint64, old, new uint64) bool {
	return atomic.CompareAndSwapUint64(ptr, old, new) // ERROR "intrinsic substitution for CompareAndSwapUint64"
}

// Unknown targets, including paths with dots, retain their fallback.
//
//go:linkinternal example.com/atomic.Cas64
func Unknown(ptr *uint64, old, new uint64) bool {
	return atomic.CompareAndSwapUint64(ptr, old, new) // ERROR "intrinsic substitution for CompareAndSwapUint64"
}

// Inlining is disabled so substitution cannot come from inlining the fallback.
func Local(ptr *uint64) {
	Cas64(ptr, 0, 1) // ERROR "intrinsic substitution for Cas64"
	Plain(ptr, 1, 2)
	Unknown(ptr, 2, 3)
}
