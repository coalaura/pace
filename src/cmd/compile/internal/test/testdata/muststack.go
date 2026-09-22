// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package muststack

//go:noescape
func use([]byte)

func Ordinary() {
	buf := make([]byte, 64)
	use(buf)
}

func Stack() {
	//go:muststack
	buf := make([]byte, 64)
	use(buf)
}

func Both() {
	//go:muststack
	//go:makenozero
	buf := make([]byte, 64)
	use(buf)
}

func Reverse() {
	//go:makenozero
	//go:muststack
	buf := make([]byte, 64)
	use(buf)
}
