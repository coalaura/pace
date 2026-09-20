// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package b

import "./a"

func Imported(ptr *uint64) {
	a.Cas64(ptr, 0, 1) // ERROR "intrinsic substitution for Cas64"
	a.Plain(ptr, 1, 2)
	a.Unknown(ptr, 2, 3)
}
