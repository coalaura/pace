// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package caller

import "makenozero"

//go:noescape
func use([]byte)

func Stack() {
	buffer := makenozero.HeapCapacity()
	use(buffer[:cap(buffer)])
}

func Heap(length, capacity int) []byte {
	return makenozero.Heap(length, capacity)
}

func Generic(length int) []uint64 {
	return makenozero.Generic[uint64](length)
}
