// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"internal/testenv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const alignDirectiveMain = `package main

import "align.test/lib"

//go:align 8
//go:noinline
func Align8(x int) int { return x + 1 }

//go:align 16
//go:noinline
func Align16(x int) int { return x + 1 }

//go:align 64
//go:noinline
func Align64(x int) int { return x + 1 }

//go:align 256
//go:noinline
func Align256(x int) int { return x + 1 }

//go:align 2048
//go:noinline
func Align2048(x int) int { return x + 1 }

//go:align 2048
//go:inline
func InlineOnly(x int) int { return x + 1 }

//go:align 256
//go:inline
func InlineEmitted(x int) int { return x + 1 }

//go:align 64
//go:inline
//go:noinline
func Both(x int) int { return x + 1 }

//go:noinline
func Caller1(x int) int { return lib.InlineOnly(InlineEmitted(InlineOnly(x))) }

//go:noinline
func Caller2(x int) int { return lib.InlineOnly(InlineEmitted(InlineOnly(x))) }

//go:noinline
func Plain1(x int) int { return x + 3 }

//go:noinline
func Plain2(x int) int { return x + 3 }

type Value int

//go:align 64
//go:noinline
func (v Value) Method(x int) int { return int(v) + x }

//go:align 256
//go:noinline
func Generic[T ~int](x T) T { return x + 1 }

var functions = []func(int) int{
	Align8, Align16, Align64, Align256, Align2048, InlineEmitted, Both,
	Caller1, Caller2, Plain1, Plain2, Generic[int], lib.Generic[int],
}
var methods = []func(lib.Box[int], int) int{lib.Box[int].Method}
var sink int

func main() {
	for _, f := range functions { sink += f(sink) }
	for _, f := range methods { sink += f(lib.Box[int]{}, sink) }
	sink += Value(sink).Method(sink) + Both(sink) + Align64(sink)
}
`

const alignDirectiveLibrary = `package lib

//go:align 2048
//go:inline
func InlineOnly(x int) int { return x + 1 }

//go:align 256
//go:noinline
func Generic[T ~int](x T) T { return x + 1 }

type Box[T ~int] struct { Value T }

//go:align 64
//go:noinline
func (b Box[T]) Method(x T) T { return b.Value + x }
`

type alignDirectiveError struct {
	name   string
	source string
	want   string
}

type alignDirectiveSymbol struct {
	address uint64
	size    uint64
}

func TestAlignDirectiveErrors(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	t.Parallel()

	cases := []alignDirectiveError{
		{"missing", "//go:align\nfunc f() {}", "usage: //go:align N"},
		{"extra", "//go:align 64 128\nfunc f() {}", "usage: //go:align N"},
		{"malformed", "//go:align nope\nfunc f() {}", "power-of-two integer"},
		{"fraction", "//go:align 16.0\nfunc f() {}", "power-of-two integer"},
		{"zero", "//go:align 0\nfunc f() {}", "power-of-two integer"},
		{"negative", "//go:align -64\nfunc f() {}", "power-of-two integer"},
		{"small", "//go:align 4\nfunc f() {}", "power-of-two integer"},
		{"nonpower", "//go:align 24\nfunc f() {}", "power-of-two integer"},
		{"large", "//go:align 4096\nfunc f() {}", "power-of-two integer"},
		{"overflow", "//go:align 18446744073709551616\nfunc f() {}", "power-of-two integer"},
		{"duplicate", "//go:align 64\n//go:align 64\nfunc f() {}", "duplicate go:align directive"},
		{"conflicting", "//go:align 16\n//go:align 64\nfunc f() {}", "duplicate go:align directive"},
		{"variable", "//go:align 64\nvar v int", "misplaced go:align directive"},
		{"type", "//go:align 64\ntype T int", "misplaced go:align directive"},
		{"statement", "func f() {\n//go:align 64\nprintln(1)\n}", "misplaced go:align directive"},
		{"trailing", "func f() {}\n//go:align 64", "misplaced go:align directive"},
		{"bodyless", "//go:align 64\nfunc f()", "go:align requires a function body"},
		{"bodylessMethod", "type T int\n//go:align 64\nfunc (T) f()", "go:align requires a function body"},
		{"blankBodyless", "//go:align 64\nfunc _()", "go:align requires a function body"},
		{"hex", "//go:align 0x40\nfunc f() {}", ""},
		{"tab", "//go:align\t64\nfunc f() {}", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "align.go")
			err := os.WriteFile(source, []byte("package p\n"+test.source+"\n"), 0666)
			if err != nil {
				t.Fatal(err)
			}

			cmd := testenv.Command(t, testenv.GoToolPath(t), "tool", "compile", "-o", filepath.Join(dir, "align.o"), source)
			output, err := cmd.CombinedOutput()
			if test.want == "" {
				if err != nil {
					t.Fatalf("compile: %v\n%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), test.want) || strings.Contains(string(output), "internal compiler error") {
				t.Fatalf("compile: %v\n%s\nwant %q", err, output, test.want)
			}
		})
	}
}

func TestAlignDirectiveLinked(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	t.Parallel()

	// Exercise different assemblers, including ppc64's own text-alignment handling.
	arches := []string{"amd64", "arm64", "ppc64le"}
	for _, arch := range arches {
		t.Run(arch, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			err := os.Mkdir(filepath.Join(dir, "lib"), 0777)
			if err != nil {
				t.Fatal(err)
			}

			aligned, diagnostics := buildAlignDirective(t, dir, arch, true)
			baseline, _ := buildAlignDirective(t, dir, arch, false)
			alignments := map[string]uint64{
				"main.Align8":                             8,
				"main.Align16":                            16,
				"main.Align64":                            64,
				"main.Align256":                           256,
				"main.Align2048":                          2048,
				"main.InlineEmitted":                      256,
				"main.Both":                               64,
				"main.Value.Method":                       64,
				"main.Generic[go.shape.int]":              256,
				"main.Generic[int]":                       256,
				"align.test/lib.Generic[go.shape.int]":    256,
				"align.test/lib.Generic[int]":             256,
				"align.test/lib.Box[go.shape.int].Method": 64,
				"align.test/lib.Box[int].Method":          64,
			}
			for name, alignment := range alignments {
				symbol, ok := aligned[name]
				if !ok {
					t.Errorf("missing symbol %s", name)
				} else if symbol.address%alignment != 0 {
					t.Errorf("%s: address %#x is not aligned to %d", name, symbol.address, alignment)
				}
			}

			inlined := []string{"InlineOnly", "InlineEmitted", "lib.InlineOnly"}
			for _, name := range inlined {
				if !strings.Contains(diagnostics, "inlining call to "+name) {
					t.Errorf("missing inlining of %s:\n%s", name, diagnostics)
				}
			}
			if strings.Contains(diagnostics, "inlining call to Both") || strings.Contains(diagnostics, "inlining call to Align64") {
				t.Errorf("go:noinline not respected:\n%s", diagnostics)
			}
			if _, ok := aligned["main.InlineOnly"]; ok {
				t.Error("fully inlined helper was emitted")
			}
			if _, ok := aligned["align.test/lib.InlineOnly"]; ok {
				t.Error("fully inlined imported helper was emitted")
			}

			// Compare identical unannotated functions with and without directives.
			// Entry spacing checks catch leaked alignment even when code size is unchanged.
			pairs := []string{"main.Plain", "main.Caller"}
			for _, prefix := range pairs {
				first, second := prefix+"1", prefix+"2"
				if aligned[first].size == 0 || aligned[second].size == 0 || baseline[first].size == 0 || baseline[second].size == 0 {
					t.Fatalf("missing nonempty symbols for %s", prefix)
				}
				if aligned[first].size != baseline[first].size || aligned[second].size != baseline[second].size {
					t.Errorf("alignment changed code size for %s", prefix)
				}
				spacing := aligned[second].address - aligned[first].address
				want := baseline[second].address - baseline[first].address
				if spacing != want || spacing >= 2048 {
					t.Errorf("%s entry spacing = %d, want baseline %d (< 2048)", prefix, spacing, want)
				}
			}
		})
	}
}

func buildAlignDirective(t *testing.T, dir, arch string, annotated bool) (map[string]alignDirectiveSymbol, string) {
	t.Helper()
	files := map[string]string{
		"go.mod":     "module align.test\n\ngo 1.27\n",
		"main.go":    alignDirectiveMain,
		"lib/lib.go": alignDirectiveLibrary,
	}
	for name, source := range files {
		if !annotated {
			source = strings.ReplaceAll(source, "//go:align ", "// alignment ")
		}
		err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0666)
		if err != nil {
			t.Fatal(err)
		}
	}

	executable := filepath.Join(dir, "align.exe")
	cmd := testenv.Command(t, testenv.GoToolPath(t), "build", "-gcflags=align.test=-m", "-o", executable, ".")
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOENV=off", "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	diagnostics := string(output)

	cmd = testenv.Command(t, testenv.GoToolPath(t), "tool", "nm", "-size", executable)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nm: %v\n%s", err, output)
	}
	symbols := make(map[string]alignDirectiveSymbol)
	for line := range strings.SplitSeq(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[2] != "T" {
			continue
		}
		address, err := strconv.ParseUint(fields[0], 16, 64)
		if err != nil {
			t.Fatal(err)
		}
		size, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		symbols[fields[3]] = alignDirectiveSymbol{address, size}
	}
	return symbols, diagnostics
}
