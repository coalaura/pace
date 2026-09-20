// errorcheck

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p

// ERROR "usage: //go:linkinternal import/path.Function"
// ERROR "usage: //go:linkinternal import/path.Function"
// ERROR "usage: //go:linkinternal import/path.Function"
// ERROR "usage: //go:linkinternal import/path.Function"
// ERROR "usage: //go:linkinternal import/path.Function"
// ERROR "usage: //go:linkinternal import/path.Function"
// ERROR "invalid go:linkinternal target"
// ERROR "duplicate go:linkinternal directive"

//go:linkinternal runtime.One
//line linkinternal_errors.go:9
//go:linkinternal
//go:linkinternal runtime.One runtime.Two
//go:linkinternal runtime
//go:linkinternal .Function
//go:linkinternal runtime.
//go:linkinternal runtime.123
//go:linkinternal runtime/(*T).Method
//go:linkinternal runtime.Two
func invalid() {}
