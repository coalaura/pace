// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nobounds

import "unsafe"

func Checked(data []byte, index int) byte {
	return data[index]
}

func CheckedSlice(data []byte, low, high int) []byte {
	return data[low:high]
}

func CheckedConversion(data []byte) [4]byte {
	return [4]byte(data)
}

//go:nobounds
func Unchecked(data []byte, index int) byte {
	return data[index]
}

//go:nobounds
func Slice(data []byte, low, high int) []byte {
	return data[low:high]
}

//go:nobounds
func Slice3(data []byte, low, high, maximum int) []byte {
	return data[low:high:maximum]
}

//go:nobounds
func StringIndex(data string, index int) byte {
	return data[index]
}

//go:nobounds
func StringSlice(data string, low, high int) string {
	return data[low:high]
}

//go:nobounds
func ArrayIndex(data [8]byte, index int) byte {
	return data[index]
}

//go:nobounds
func ArraySlice(data *[8]byte, low, high int) []byte {
	return data[low:high]
}

//go:nobounds
func ArraySlice3(data *[8]byte, low, high, maximum int) []byte {
	return data[low:high:maximum]
}

//go:nobounds
func WideIndex(data []byte, index uint64) byte {
	return data[index]
}

//go:nobounds
func WideSlice(data []byte, low, high uint64) []byte {
	return data[low:high]
}

//go:nobounds
func ArrayReadZero(data [0]byte, index int) byte {
	return data[index]
}

//go:nobounds
func ArrayWriteZero(data [0]byte, index int) [0]byte {
	data[index] = 1
	return data
}

//go:nobounds
func ArrayWriteOne(data [1]byte, index int) [1]byte {
	data[index] = 1
	return data
}

//go:nobounds
func ArrayWriteEmpty(data [8]struct{}, index int) [8]struct{} {
	data[index] = struct{}{}
	return data
}

//go:nobounds
func ArrayConversion(data []byte) [4]byte {
	return [4]byte(data)
}

//go:nobounds
func ArrayPointerConversion(data []byte) *[4]byte {
	return (*[4]byte)(data)
}

func InlineUnchecked(data []byte, index int) byte {
	return Unchecked(data, index)
}

//go:nobounds
func InlineChecked(data []byte, index int) byte {
	return Checked(data, index)
}

//go:inline
//go:nobounds
func Forced(data []byte, index int) byte {
	// The three calls exceed the ordinary inlining budget.
	index = Cost(Cost(Cost(index)))
	return data[index]
}

//go:noinline
func Cost(index int) int {
	return index
}

func InlineForced(data []byte, index int) byte {
	return Forced(data, index)
}

//go:noinline
func NoInlineChecked(data []byte, index int) byte {
	return data[index]
}

//go:nobounds
func CallChecked(data []byte, index int) byte {
	return NoInlineChecked(data, index)
}

//go:nobounds
func Closure(data []byte) func(int) byte {
	return func(index int) byte { return data[index] }
}

//go:nobounds
func CheckPointer(pointer unsafe.Pointer) *[2]int {
	return (*[2]int)(pointer)
}
