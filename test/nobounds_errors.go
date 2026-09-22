// errorcheck

// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nobounds

//go:nobounds
func invalidConstants(data []byte, array [4]byte) {
	_ = data[-1]    // ERROR "invalid argument: index -1.*must not be negative"
	_ = array[4]    // ERROR "invalid argument: index 4 out of bounds"
	_ = "abc"[3]    // ERROR "invalid argument: index 3 out of bounds"
	_ = array[:5]   // ERROR "invalid argument: index 5 out of bounds"
	_ = "abc"[:4]   // ERROR "invalid argument: index 4 out of bounds"
	_ = data[2:1]   // ERROR "invalid slice indices: 1 < 2"
	_ = data[0:2:1] // ERROR "invalid slice indices: 1 < 2"
}
