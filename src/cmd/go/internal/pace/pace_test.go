// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnabled(t *testing.T) {
	original := os.Args[0]
	t.Cleanup(func() { os.Args[0] = original })
	names := []string{"pace", "pace.exe", "go", "go.exe", "notpace", "pace.test"}
	for _, name := range names {
		os.Args[0] = filepath.Join("bin", name)
		want := name == "pace" || name == "pace.exe"
		if Enabled() != want {
			t.Errorf("Enabled(%q) = %v, want %v", name, Enabled(), want)
		}
	}
}

func TestCheckVersion(t *testing.T) {
	root := t.TempDir()
	versions := []string{"go1.27.1\ntime 2026-08-28T16:20:06Z\n", "go1.27.1\r\n", "go1.27.2\n", "go1.27.1-custom\n", ""}
	for _, installed := range versions {
		err := os.WriteFile(filepath.Join(root, "VERSION"), []byte(installed), 0666)
		if err != nil {
			t.Fatal(err)
		}

		err = checkVersion(root, "go1.27.1")
		wantMatch := strings.HasPrefix(installed, "go1.27.1\n") || strings.HasPrefix(installed, "go1.27.1\r\n")
		if (err == nil) != wantMatch {
			t.Errorf("VERSION %q: %v", installed, err)
		}
	}

	err := checkVersion(t.TempDir(), "go1.27.1")
	if err == nil || !strings.Contains(err.Error(), "requires Go 1.27.1") {
		t.Fatalf("missing VERSION: %v", err)
	}

	err = checkVersion(t.TempDir(), "devel go1.28-abcdef")
	if err != nil {
		t.Fatalf("development build: %v", err)
	}
}
