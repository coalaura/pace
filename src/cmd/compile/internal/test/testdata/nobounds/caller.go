// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package caller

import "nobounds"

func InlineUnchecked(data []byte, index int) byte {
	return nobounds.Unchecked(data, index)
}

//go:nobounds
func InlineChecked(data []byte, index int) byte {
	return nobounds.Checked(data, index)
}

func InlineForced(data []byte, index int) byte {
	return nobounds.Forced(data, index)
}
