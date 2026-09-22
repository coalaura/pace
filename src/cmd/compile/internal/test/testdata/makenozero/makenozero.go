// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package makenozero

type Record struct {
	Hash uint64
	Size uint32
}

//go:noescape
func use([]byte)

func Stack() {
	//go:makenozero
	buffer := make([]byte, 16, 1024)
	use(buffer[:cap(buffer)])
}

func OrdinaryStack() {
	buffer := make([]byte, 16, 1024)
	use(buffer[:cap(buffer)])
}

func VariableStack(length, capacity int) {
	//go:makenozero
	buffer := make([]byte, length, capacity)
	use(buffer)
}

func OrdinaryVariableStack(length, capacity int) {
	buffer := make([]byte, length, capacity)
	use(buffer)
}

func Heap(length, capacity int) []byte {
	//go:makenozero
	buffer := make([]byte, length, capacity)
	return buffer
}

func HeapWide(length, capacity int64) []byte {
	//go:makenozero
	buffer := make([]byte, length, capacity)
	return buffer
}

func OrdinaryHeapWide(length, capacity int64) []byte {
	return make([]byte, length, capacity)
}

func HeapCapacity() []byte {
	//go:makenozero
	buffer := make([]byte, 16, 32)
	return buffer
}

func Scalar(length int) []uint64 {
	//go:makenozero
	buffer := make([]uint64, length)
	return buffer
}

func Array(length int) [][32]byte {
	//go:makenozero
	buffer := make([][32]byte, length)
	return buffer
}

func Struct(length int) []Record {
	//go:makenozero
	buffer := make([]Record, length)
	return buffer
}

func Generic[T ~uint64](length int) []T {
	//go:makenozero
	buffer := make([]T, length)
	return buffer
}

func LocalInline() {
	buffer := HeapCapacity()
	use(buffer[:cap(buffer)])
}

func Copy(source []byte, length int) []byte {
	//go:makenozero
	buffer := make([]byte, length)
	copy(buffer, source)
	return buffer
}

func OrdinaryHeap(length, capacity int) []byte {
	return make([]byte, length, capacity)
}

func Mixed(length int) ([]byte, []byte) {
	//go:makenozero
	buffer := make([]byte, length)
	ordinary := make([]byte, length)
	return buffer, ordinary
}

func ZeroSize(length, capacity uint64) []struct{} {
	//go:makenozero
	buffer := make([]struct{}, length, capacity)
	return buffer
}

// AddressSpaceBoundary is compiled but never executed. The byte size is exactly
// the target's uintptr maximum, which is representable but exceeds runtime
// allocation limits. The compiler must leave that limit to mallocgc.
func AddressSpaceBoundary() [][3]byte {
	capacity := int(^uint(0) / 3)
	//go:makenozero
	buffer := make([][3]byte, 0, capacity)
	return buffer
}
