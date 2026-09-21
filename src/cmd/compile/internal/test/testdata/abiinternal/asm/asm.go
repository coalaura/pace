// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package asm

//go:abiinternal addr=DX new=BX width=CX signed=DI -> old=AX
func Optimized(addr *uint64, new uint64, width uint8, signed bool) (old uint64)

//go:abiinternal addr=BX new=DX width=R8 signed=R9 -> old=R10
func Different(addr *uint64, new uint64, width uint8, signed bool) (old uint64)

//go:abiinternal a=BX b=CX c=AX -> x=BX y=CX z=AX
func Cycle(a, b, c uint64) (x, y, z uint64)

//go:abiinternal a=AX -> result=AX
func Identity(a uint64) (result uint64)

//go:abiinternal a=AX b=BX -> x=AX y=BX
func Narrow(a uint16, b uint32) (x uint16, y uint32)

//go:abiinternal a=DX -> result=DX
func Pointer(a *uint64) (result *uint64)

// An ordinary ABI0 definition and an ABI0 caller of the annotated Identity.
func Ordinary(a uint64) (result uint64)
func Legacy(a uint64) (result uint64)

//go:abiinternal a=AX -> result=AX
func Referenced(a uint64) (result uint64)
