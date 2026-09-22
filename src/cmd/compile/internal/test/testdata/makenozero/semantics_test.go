// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package makenozero_test

import (
	"fmt"
	"runtime"
	"strconv"
	"testing"
)

type makeNoZeroDimensions struct {
	length   int64
	capacity int64
}

type makeNoZeroInteger interface {
	~int | ~uint | ~int64 | ~uint64 | ~int8 | ~uint8
}

func TestMakeNoZero(t *testing.T) {
	buffer := makeNoZeroHeap(16, 32)
	if len(buffer) != 16 || cap(buffer) != 32 {
		t.Fatalf("got len=%d cap=%d, want 16, 32", len(buffer), cap(buffer))
	}
	full := buffer[:cap(buffer)]
	for index := range full {
		full[index] = uint64(index + 1)
	}
	for index, value := range full {
		if value != uint64(index+1) {
			t.Fatalf("element %d = %d", index, value)
		}
	}

	ordinary := makeZeroHeap(16, 32)
	for index, value := range ordinary[:cap(ordinary)] {
		if value != 0 {
			t.Fatalf("ordinary element %d = %d", index, value)
		}
	}

	// No assertions about the initial contents of uninitialized storage.
	// Zero-sized types still retain the ordinary len/cap and panic semantics.
	//go:makenozero
	empty := make([]struct{}, 16, 32)
	if len(empty) != 16 || cap(empty) != 32 {
		t.Fatal("zero-sized element dimensions changed")
	}
	if makeNoZeroHeap(0, 0) == nil {
		t.Fatal("zero-sized allocation is nil")
	}
}

func TestMakeNoZeroPanics(t *testing.T) {
	dimensions := []makeNoZeroDimensions{
		{0, 0}, {0, 4}, {4, 4}, {-1, -1}, {-1, 0}, {0, -1}, {2, 1}, {1, -1},
		{1, 1 << 61}, {1 << 61, 1 << 61}, {-1, 1 << 32}, {1 << 32, 0},
		{1, 1<<63 - 1}, {-1, 1<<63 - 1},
	}
	if strconv.IntSize == 32 {
		// These products overflow uintptr on 32-bit targets. Do not execute
		// them on 64-bit targets, where they would request enormous allocations.
		dimensions = append(dimensions, makeNoZeroDimensions{0, 1 << 29}, makeNoZeroDimensions{1 << 29, 1 << 29})
	}
	for _, dimensions := range dimensions {
		t.Run(fmt.Sprintf("%d_%d", dimensions.length, dimensions.capacity), func(t *testing.T) {
			compareMakeNoZeroPanics(t, dimensions.length, dimensions.capacity)
			compareMakeNoZeroPanics(t, uint64(dimensions.length), uint64(dimensions.capacity))
			compareMakeNoZeroPanics(t, int(dimensions.length), int(dimensions.capacity))
			compareMakeNoZeroPanics(t, uint(dimensions.length), uint(dimensions.capacity))
		})
	}
}

func TestMakeNoZeroMixedWidths(t *testing.T) {
	compareMakeNoZeroPanics(t, int8(-1), uint64(1<<32))
	compareMakeNoZeroPanics(t, uint64(1<<32), int8(-1))
	compareMakeNoZeroPanics(t, uint64(1<<63), uint8(0))
	compareMakeNoZeroPanics(t, uint8(0), uint64(1<<63))
	compareMakeNoZeroPanics(t, int8(4), uint64(8))
	compareMakeNoZeroPanics(t, uint64(4), uint8(8))
}

func TestMakeNoZeroArithmeticOverflow(t *testing.T) {
	// A non-power-of-two element size exercises the exact multiplication bound.
	// Both dimensions still fit in int; only their byte sizes overflow uintptr.
	overflow := int(^uint(0)/3 + 1)
	lengthPanic := "runtime error: makeslice: len out of range"
	capacityPanic := "runtime error: makeslice: cap out of range"
	got := makeNoZeroPanic(func() { makeNoZeroTriples(0, overflow) })
	if got != capacityPanic {
		t.Fatalf("capacity multiplication overflow: %q, want %q", got, capacityPanic)
	}
	got = makeNoZeroPanic(func() { makeNoZeroTriples(overflow, overflow) })
	if got != lengthPanic {
		t.Fatalf("length multiplication overflow: %q, want %q", got, lengthPanic)
	}
	got = makeNoZeroPanic(func() { makeNoZeroTriples(overflow, 0) })
	if got != lengthPanic {
		t.Fatalf("length overflow with smaller capacity: %q, want %q", got, lengthPanic)
	}
	got = makeNoZeroPanic(func() { makeNoZeroImplicit(overflow) })
	if got != lengthPanic {
		t.Fatalf("implicit capacity overflow: %q, want %q", got, lengthPanic)
	}
}

func TestMakeNoZeroZeroSize(t *testing.T) {
	maximum := int(^uint(0) >> 1)
	buffer := makeNoZeroEmpty(maximum, maximum)
	if buffer == nil || len(buffer) != maximum || cap(buffer) != maximum {
		t.Fatal("zero-sized elements lost dimensions or non-nil storage")
	}
	dimensions := []makeNoZeroDimensions{{-1, -1}, {0, -1}, {2, 1}, {1 << 32, 0}, {-1, 1 << 32}}
	for _, dimensions := range dimensions {
		want := makeNoZeroPanic(func() { makeZeroEmpty(dimensions.length, dimensions.capacity) })
		got := makeNoZeroPanic(func() { makeNoZeroEmpty(dimensions.length, dimensions.capacity) })
		if got != want {
			t.Errorf("zero-sized make(%d, %d): %q, want %q", dimensions.length, dimensions.capacity, got, want)
		}
	}
}

func TestMakeNoZeroEvaluation(t *testing.T) {
	calls := 0
	dimension := func() int {
		calls++
		return calls
	}
	//go:makenozero
	buffer := make([]byte, dimension(), dimension())
	if calls != 2 || len(buffer) != 1 || cap(buffer) != 2 {
		t.Fatalf("dimensions evaluated incorrectly: calls=%d len=%d cap=%d", calls, len(buffer), cap(buffer))
	}
	//go:makenozero
	implicit := make([]byte, dimension())
	if calls != 3 || len(implicit) != 3 || cap(implicit) != 3 {
		t.Fatal("implicit capacity reevaluated length")
	}
}

//go:noinline
func makeNoZeroTriples(length, capacity int) [][3]byte {
	//go:makenozero
	buffer := make([][3]byte, length, capacity)
	return buffer
}

//go:noinline
func makeNoZeroImplicit(length int) [][3]byte {
	//go:makenozero
	buffer := make([][3]byte, length)
	return buffer
}

//go:noinline
func makeNoZeroEmpty[Integer makeNoZeroInteger](length, capacity Integer) []struct{} {
	//go:makenozero
	buffer := make([]struct{}, length, capacity)
	return buffer
}

//go:noinline
func makeZeroEmpty[Integer makeNoZeroInteger](length, capacity Integer) []struct{} {
	return make([]struct{}, length, capacity)
}

//go:noinline
func makeNoZeroHeap[Length, Capacity makeNoZeroInteger](length Length, capacity Capacity) []uint64 {
	//go:makenozero
	buffer := make([]uint64, length, capacity)
	return buffer
}

//go:noinline
func makeZeroHeap[Length, Capacity makeNoZeroInteger](length Length, capacity Capacity) []uint64 {
	return make([]uint64, length, capacity)
}

//go:noinline
func makeNoZeroStack[Length, Capacity makeNoZeroInteger](length Length, capacity Capacity) {
	//go:makenozero
	buffer := make([]uint64, length, capacity)
	runtime.KeepAlive(buffer)
}

//go:noinline
func makeZeroStack[Length, Capacity makeNoZeroInteger](length Length, capacity Capacity) {
	buffer := make([]uint64, length, capacity)
	runtime.KeepAlive(buffer)
}

func compareMakeNoZeroPanics[Length, Capacity makeNoZeroInteger](t *testing.T, length Length, capacity Capacity) {
	t.Helper()
	want := makeNoZeroPanic(func() { makeZeroHeap(length, capacity) })
	got := makeNoZeroPanic(func() { makeNoZeroHeap(length, capacity) })
	if got != want {
		t.Errorf("heap make(%T(%v), %v): panic %q, want %q", length, length, capacity, got, want)
	}
	want = makeNoZeroPanic(func() { makeZeroStack(length, capacity) })
	got = makeNoZeroPanic(func() { makeNoZeroStack(length, capacity) })
	if got != want {
		t.Errorf("stack make(%T(%v), %v): panic %q, want %q", length, length, capacity, got, want)
	}
}

func makeNoZeroPanic(operation func()) (message string) {
	defer func() {
		failure := recover()
		if failure != nil {
			message = fmt.Sprint(failure)
		}
	}()
	operation()
	return
}
