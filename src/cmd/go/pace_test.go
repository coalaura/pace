// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main_test

import (
	"internal/testenv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPACEOverlay builds real release-named tools outside GOROOT. Set
// PACE_TEST_GOROOT to test against a matching, unmodified Go installation.
func TestPACEOverlay(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	if testing.Short() {
		t.Skip("builds the three overlay executables")
	}

	directory := t.TempDir()
	goTool := testenv.GoToolPath(t)
	tools := []string{"go", "compile", "asm"}
	for _, tool := range tools {
		command := testenv.Command(t, goTool, "build", "-o", filepath.Join(directory, tool+exeSuffix), "cmd/"+tool)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("building %s: %v\n%s", tool, err, output)
		}
	}

	copyPACEFile(t, filepath.Join(directory, "go"+exeSuffix), filepath.Join(directory, "pace"+exeSuffix))
	copyPACEFile(t, filepath.Join(directory, "compile"+exeSuffix), filepath.Join(directory, "compilepe"+exeSuffix))
	copyPACEFile(t, filepath.Join(directory, "asm"+exeSuffix), filepath.Join(directory, "asmpe"+exeSuffix))

	root := os.Getenv("PACE_TEST_GOROOT")
	if root == "" {
		root = testGOROOT
	}
	t.Setenv("GOROOT", root)
	// Stock Go, source-tree PACE, and the overlay share this build cache.
	t.Setenv("GOCACHE", t.TempDir())
	t.Setenv("GOENV", "off")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GO_TELEMETRY_CHILD", "2") // No uploads from stock controls either.
	telemetryDirectory := t.TempDir()
	t.Setenv("TEST_TELEMETRY_DIR", telemetryDirectory)
	err := os.WriteFile(filepath.Join(telemetryDirectory, "mode"), []byte("local\n"), 0666)
	if err != nil {
		t.Fatal(err)
	}

	paceTool := filepath.Join(directory, "pace"+exeSuffix)
	stockGo := filepath.Join(root, "bin", "go"+exeSuffix)
	stockTools := filepath.Join(root, "pkg", "tool", runtime.GOOS+"_"+runtime.GOARCH)
	version := "go version " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
	run := func(t *testing.T, executable string, args ...string) string {
		t.Helper()
		command := testenv.Command(t, executable, args...)
		command.Dir = directory
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", executable, args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	fail := func(t *testing.T, args ...string) string {
		t.Helper()
		command := testenv.Command(t, paceTool, args...)
		command.Dir = directory
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatalf("pace %v unexpectedly succeeded: %s", args, output)
		}
		return string(output)
	}

	t.Run("identity-and-telemetry", func(t *testing.T) {
		got := run(t, paceTool, "version")
		if got != version+" (pace)" {
			t.Fatalf("pace version = %q", got)
		}
		if run(t, paceTool, "env", "GOVERSION") != runtime.Version() || run(t, paceTool, "env", "GOROOT") != root {
			t.Fatal("PACE changed GOVERSION or GOROOT")
		}

		helpers := []string{"compile", "asm"}
		for _, helper := range helpers {
			got = run(t, filepath.Join(directory, helper+"pe"+exeSuffix), "-V=full")
			if !strings.HasPrefix(got, helper+" version "+runtime.Version()+" pace") {
				t.Fatalf("PACE tool identity = %q", got)
			}
			if !strings.Contains(runtime.Version(), "devel") && strings.Contains(got, " buildID=") {
				t.Fatalf("PACE release tool identity includes an executable build ID: %q", got)
			}
		}
		// The stock linker also opens counters unless the parent supplies an
		// isolated, disabled configuration.
		got = run(t, paceTool, "tool", "link", "-V=full")
		if strings.Contains(got, " pace") {
			t.Fatalf("link acquired PACE identity: %q", got)
		}

		got = fail(t, "telemetry", "on")
		if !strings.Contains(got, "pace: telemetry is disabled") {
			t.Fatal(got)
		}
		mode, err := os.ReadFile(filepath.Join(telemetryDirectory, "mode"))
		if err != nil || string(mode) != "local\n" {
			t.Fatalf("telemetry mode changed: %q, %v", mode, err)
		}
		entries, err := os.ReadDir(telemetryDirectory)
		if err != nil || len(entries) != 1 {
			t.Fatalf("PACE created telemetry files: %v, %v", entries, err)
		}

		// Even a sidecar invocation must remain an ordinary PACE invocation.
		t.Setenv("GO_TELEMETRY_CHILD", "1")
		if run(t, paceTool, "version") != version+" (pace)" {
			t.Fatal("PACE entered the telemetry sidecar")
		}
		t.Setenv("GO_TELEMETRY_CHILD", "2")
		if run(t, filepath.Join(directory, "go"+exeSuffix), "version") != version || run(t, stockGo, "version") != version {
			t.Fatal("stock go version changed")
		}
		counts, err := filepath.Glob(filepath.Join(telemetryDirectory, "local", "*.count"))
		if err != nil || len(counts) == 0 {
			t.Fatalf("stock go did not open telemetry counters: %v, %v", counts, err)
		}

		for _, helper := range helpers {
			ordinary := run(t, filepath.Join(directory, helper+exeSuffix), "-V=full")
			overlay := run(t, paceTool, "tool", helper, "-V=full")
			if ordinary != overlay {
				t.Fatalf("source-tree %s identity = %q, overlay = %q", helper, ordinary, overlay)
			}
			if os.Getenv("PACE_TEST_GOROOT") != "" {
				stock := run(t, filepath.Join(stockTools, helper+exeSuffix), "-V=full")
				if ordinary == stock {
					t.Fatalf("PACE shares %s cache tool ID with stock Go: %q", helper, stock)
				}
			}
		}
	})

	t.Run("dispatch", func(t *testing.T) {
		names := []string{"compile", "asm", "link"}
		for _, name := range names {
			stock := filepath.Join(stockTools, name+exeSuffix)
			want := stock
			if name != "link" {
				want = filepath.Join(directory, name+"pe"+exeSuffix)
			}
			if run(t, paceTool, "tool", "-n", name) != want {
				t.Fatalf("PACE %s did not resolve to %s", name, want)
			}
			if run(t, filepath.Join(directory, "go"+exeSuffix), "tool", "-n", name) != stock || run(t, stockGo, "tool", "-n", name) != stock {
				t.Fatalf("stock %s dispatch changed", name)
			}
		}
	})

	t.Run("missing-helpers", func(t *testing.T) {
		names := []string{"compilepe", "asmpe"}
		for _, name := range names {
			path := filepath.Join(directory, name+exeSuffix)
			err := os.Rename(path, path+".saved")
			if err != nil {
				t.Fatal(err)
			}
			output := fail(t, "version")
			err = os.Rename(path+".saved", path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, "required helper "+name) {
				t.Fatal(output)
			}
		}
	})

	t.Run("goroot-mismatch", func(t *testing.T) {
		other := t.TempDir()
		err := os.WriteFile(filepath.Join(other, "VERSION"), []byte("go1.999.0\n"), 0666)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("GOROOT", other)
		output := fail(t, "version")
		if !strings.Contains(output, "requires Go "+strings.TrimPrefix(runtime.Version(), "go")) || !strings.Contains(output, "GOROOT contains Go 1.999.0") {
			t.Fatal(output)
		}
	})

	t.Run("build-and-cache", func(t *testing.T) {
		if os.Getenv("PACE_TEST_GOROOT") == "" {
			t.Skip("requires PACE_TEST_GOROOT pointing to a matching, unmodified Go installation")
		}

		err := os.WriteFile(filepath.Join(directory, "main.go"), []byte(`package main
import "fmt"
//go:noinline
func buffer(length int) []uint64 {
	//go:makenozero
	result := make([]uint64, length, 2*length)
	return result
}
func main() {
	data := buffer(16)
	for index := range data[:cap(data)] {
		data[:cap(data)][index] = uint64(index)
	}
	fmt.Println(data[15])
}
`), 0666)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module overlaytest\ngo 1.27.0\n"), 0666)
		if err != nil {
			t.Fatal(err)
		}
		toolchains := []struct {
			name string
			tool string
			root string
		}{
			{"stock", stockGo, root},
			{"source", goTool, testGOROOT},
			{"overlay", paceTool, root},
		}
		ids := make(map[string]string, len(toolchains))
		runtimeIDs := make(map[string]string, len(toolchains))
		for round := range 2 {
			for _, toolchain := range toolchains {
				t.Setenv("GOROOT", toolchain.root)
				run(t, toolchain.tool, "build", "-o", filepath.Join(directory, toolchain.name+exeSuffix), ".")
				id := run(t, toolchain.tool, "list", "-export", "-f", "{{.BuildID}}", ".")
				if id == "" || (toolchain.name != "stock" && id == ids["stock"]) {
					t.Fatalf("shared build cache identity: %s %q, stock %q", toolchain.name, id, ids["stock"])
				}
				if round != 0 && id != ids[toolchain.name] {
					t.Fatalf("%s cache identity changed: %q != %q", toolchain.name, id, ids[toolchain.name])
				}
				ids[toolchain.name] = id

				runtimeID := run(t, toolchain.tool, "list", "-export", "-f", "{{.BuildID}}", "runtime")
				if runtimeID == "" || (toolchain.name != "stock" && runtimeID == runtimeIDs["stock"]) {
					t.Fatalf("shared runtime cache identity: %s %q, stock %q", toolchain.name, runtimeID, runtimeIDs["stock"])
				}
				if round != 0 && runtimeID != runtimeIDs[toolchain.name] {
					t.Fatalf("%s runtime cache identity changed: %q != %q", toolchain.name, runtimeID, runtimeIDs[toolchain.name])
				}
				runtimeIDs[toolchain.name] = runtimeID
			}
		}
	})

	t.Run("makenozero", func(t *testing.T) {
		if os.Getenv("PACE_TEST_GOROOT") == "" {
			t.Skip("requires PACE_TEST_GOROOT pointing to a matching, unmodified Go installation")
		}
		// Execute compiler-lowered allocations against the untouched installed runtime.
		run(t, paceTool, "test", "-vet=off", filepath.Join(testGOROOT, "src", "cmd", "compile", "internal", "test", "testdata", "makenozero", "semantics_test.go"),
			"-run", "^TestMakeNoZero", "-count=1")
	})

	t.Run("no-toolchain-switch", func(t *testing.T) {
		settings := []string{"auto", "path", "go1.999.0", "go1.999.0+auto", "go1.999.0+path"}
		for _, setting := range settings {
			t.Setenv("GOTOOLCHAIN", setting)
			if run(t, paceTool, "version") != version+" (pace)" {
				t.Fatalf("switched with GOTOOLCHAIN=%s", setting)
			}
		}
		err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module overlaytest\ngo 1.999.0\n"), 0666)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("GOTOOLCHAIN", "auto")
		output := fail(t, "build", ".")
		if !strings.Contains(output, "requires go >= 1.999.0") || !strings.Contains(output, "GOTOOLCHAIN=local") || strings.Contains(output, "downloading") {
			t.Fatal(output)
		}
	})
}

func copyPACEFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(destination, data, 0777)
	if err != nil {
		t.Fatal(err)
	}
}
