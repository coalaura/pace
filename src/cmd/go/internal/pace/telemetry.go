// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pace

import (
	"os"
	"path/filepath"
)

// DisableChildTelemetry gives unmodified GOROOT tools (such as link and cgo)
// a private, disabled telemetry configuration. The upstream directory override
// is inherited by children; the user's stock-Go configuration is never touched.
// PACE itself and its helpers skip telemetry initialization entirely.
func DisableChildTelemetry() (cleanup func(), err error) {
	directory, err := os.MkdirTemp("", "pace-telemetry-")
	if err != nil {
		return nil, err
	}

	cleanup = func() { os.RemoveAll(directory) }
	err = os.WriteFile(filepath.Join(directory, "mode"), []byte("off\n"), 0600)
	if err == nil {
		err = os.Setenv("TEST_TELEMETRY_DIR", directory)
	}
	if err != nil {
		cleanup()
		return nil, err
	}

	return cleanup, nil
}
