// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"internal/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoBounds(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	goTool := testenv.GoToolPath(t)
	directory := t.TempDir()
	archive := filepath.Join(directory, "nobounds.a")
	importcfg := filepath.Join(directory, "importcfg")
	err := os.WriteFile(importcfg, []byte("packagefile nobounds="+archive+"\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Check a 32-bit target too: wide indexes have an additional high-word check.
	architectures := []string{"amd64", "386", "arm64"}
	for _, architecture := range architectures {
		t.Run(architecture, func(t *testing.T) {
			compile := func(packagePath, source, output string, flags ...string) string {
				arguments := []string{"tool", "compile", "-S", "-m", "-d=checkptr=1", "-p", packagePath, "-o", output}
				arguments = append(arguments, flags...)
				arguments = append(arguments, filepath.Join("testdata", "nobounds", source))
				command := testenv.Command(t, goTool, arguments...)
				command.Env = append(command.Environ(), "GOARCH="+architecture)
				result, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("compile %s: %v\n%s", source, err, result)
				}
				return string(result)
			}

			library := compile("nobounds", "nobounds.go", archive)
			checked := []string{"Checked", "CheckedSlice", "CheckedConversion", "InlineChecked", "NoInlineChecked", "Closure.func1"}
			unchecked := []string{
				"Unchecked", "Slice", "Slice3", "StringIndex", "StringSlice", "ArrayIndex", "ArraySlice", "ArraySlice3",
				"WideIndex", "WideSlice", "ArrayReadZero", "ArrayWriteZero", "ArrayWriteOne", "ArrayWriteEmpty",
				"ArrayConversion", "ArrayPointerConversion", "InlineUnchecked", "Forced", "InlineForced", "CallChecked",
			}
			for _, name := range checked {
				checkNoBoundsAssembly(t, library, "nobounds."+name, true)
			}
			for _, name := range unchecked {
				checkNoBoundsAssembly(t, library, "nobounds."+name, false)
			}
			inlined := []string{"Unchecked", "Checked", "Forced"}
			for _, name := range inlined {
				if !strings.Contains(library, "inlining call to "+name) {
					t.Errorf("local call to %s was not inlined", name)
				}
			}
			if !strings.Contains(noBoundsAssembly(t, library, "nobounds.CallChecked"), "nobounds.NoInlineChecked") {
				t.Error("missing ordinary checked call")
			}
			if !strings.Contains(noBoundsAssembly(t, library, "nobounds.CheckPointer"), "runtime.checkptrAlignment") {
				t.Error("nobounds suppressed checkptr instrumentation")
			}
			if architecture == "amd64" && !strings.Contains(noBoundsAssembly(t, library, "nobounds.ArraySlice"), "TESTB\tAL, (AX)") {
				t.Error("nobounds suppressed the array pointer nil check")
			}

			caller := compile("caller", "caller.go", filepath.Join(directory, "caller.o"), "-importcfg", importcfg)
			checkNoBoundsAssembly(t, caller, "caller.InlineUnchecked", false)
			checkNoBoundsAssembly(t, caller, "caller.InlineChecked", true)
			checkNoBoundsAssembly(t, caller, "caller.InlineForced", false)
			for _, name := range inlined {
				if !strings.Contains(caller, "inlining call to nobounds."+name) {
					t.Errorf("imported call to %s was not inlined", name)
				}
			}

			// The existing global switch must continue to suppress ordinary checks too.
			global := compile("nobounds", "nobounds.go", archive, "-B")
			for _, name := range checked {
				checkNoBoundsAssembly(t, global, "nobounds."+name, false)
			}
		})
	}
}

func checkNoBoundsAssembly(t *testing.T, assembly, name string, wantCheck bool) {
	t.Helper()
	body := noBoundsAssembly(t, assembly, name)
	hasCheck := strings.Contains(body, "runtime.panicBounds") || strings.Contains(body, "runtime.panicExtend")
	if hasCheck != wantCheck {
		t.Errorf("%s: bounds check = %v, want %v\n%s", name, hasCheck, wantCheck, body)
	}
}

func noBoundsAssembly(t *testing.T, assembly, name string) string {
	t.Helper()
	start := strings.Index(assembly, "\n"+name+" STEXT ")
	if start < 0 {
		t.Fatalf("missing assembly for %s", name)
	}
	body := assembly[start+1:]
	// Instructions and relocations are indented; the next symbol is not.
	for offset := 0; offset < len(body); {
		next := strings.IndexByte(body[offset:], '\n')
		if next < 0 {
			break
		}
		offset += next + 1
		if offset < len(body) && body[offset] != '\t' {
			return body[:offset]
		}
	}
	return body
}
