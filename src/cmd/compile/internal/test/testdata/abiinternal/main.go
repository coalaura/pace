// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"abiinternaltest/asm"
	"reflect"
)

func main() {
	value := uint64(1000)
	signedValues := []bool{false, true}
	for _, signed := range signedValues {
		want := uint64(1261)
		if signed {
			want++
		}
		if asm.Optimized(&value, 6, 255, signed) != want || asm.Different(&value, 6, 255, signed) != want {
			panic("direct call")
		}
		functions := []func(*uint64, uint64, uint8, bool) uint64{asm.Optimized, asm.Different}
		for _, function := range functions {
			if function(&value, 6, 255, signed) != want {
				panic("function value")
			}
			args := []reflect.Value{reflect.ValueOf(&value), reflect.ValueOf(uint64(6)), reflect.ValueOf(uint8(255)), reflect.ValueOf(signed)}
			result := reflect.ValueOf(function).Call(args)
			if result[0].Uint() != want {
				panic("reflection")
			}
		}
	}
	inputs := []uint64{0, 1, 0xfedcba9876543210}
	for _, input := range inputs {
		x, y, z := asm.Cycle(input, 123, 456)
		if x != input || y != 123 || z != 456 {
			panic("parallel copy cycle")
		}
		if asm.Identity(input) != input || asm.Ordinary(input) != input || asm.Legacy(input) != input {
			panic("identity/ABI0")
		}
	}
	if asm.Pointer(&value) != &value {
		panic("pointer result")
	}
	narrow16, narrow32 := asm.Narrow(0xffff, 0xffffffff)
	if narrow16 != 0xffff || narrow32 != 0xffffffff {
		panic("narrow values")
	}
}
