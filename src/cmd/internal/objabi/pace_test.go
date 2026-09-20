// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objabi

import "testing"

func TestPACEToolIdentity(t *testing.T) {
	tests := []struct {
		name string
		want string
		pace bool
	}{
		{"compile", "compile", true},
		{"compilepe", "compile", true},
		{"asm", "asm", true},
		{"asmpe", "asm", true},
		{"link", "link", false},
		{"vet", "vet", false},
		{"cover", "cover", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, pace := paceToolIdentity(test.name)
			if name != test.want || pace != test.pace {
				t.Fatalf("paceToolIdentity(%q) = %q, %v; want %q, %v", test.name, name, pace, test.want, test.pace)
			}
		})
	}
}
