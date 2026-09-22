// errorcheck

// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:nobounds // ERROR "misplaced compiler directive"
package nobounds

//go:nobounds // ERROR "misplaced compiler directive"
var value int

//go:nobounds // ERROR "misplaced compiler directive"
const constant = 1

//go:nobounds // ERROR "misplaced compiler directive"
type T int

//go:nobounds
func valid(data []byte, index int) byte {
	//go:nobounds // ERROR "misplaced compiler directive"
	var local int
	return data[index+local]
}

//go:nobounds
func (T) method(data []byte, index int) byte {
	return data[index]
}
