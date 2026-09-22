// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"internal/testenv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type makeNoZeroErrorCase struct {
	name string
	body string
	want string
}

func TestMakeNoZero(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	goTool := testenv.GoToolPath(t)
	directory := t.TempDir()
	archive := filepath.Join(directory, "makenozero.a")
	importcfg := filepath.Join(directory, "importcfg")
	err := os.WriteFile(importcfg, []byte("packagefile makenozero="+archive+"\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	architectures := []string{"amd64", "386", "arm64"}
	for _, architecture := range architectures {
		t.Run(architecture, func(t *testing.T) {
			compile := func(packagePath, source, output string, flags ...string) string {
				arguments := []string{"tool", "compile", "-S", "-m", "-p", packagePath, "-o", output}
				arguments = append(arguments, flags...)
				arguments = append(arguments, filepath.Join("testdata", "makenozero", source))
				command := testenv.Command(t, goTool, arguments...)
				command.Env = append(command.Environ(), "GOARCH="+architecture)
				result, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("compile %s: %v\n%s", source, err, result)
				}
				return string(result)
			}

			library := compile("makenozero", "makenozero.go", archive)
			heapFunctions := []string{"Heap", "HeapWide", "HeapCapacity", "Scalar", "Array", "Struct", "Copy", "ZeroSize", "AddressSpaceBoundary"}
			for _, name := range heapFunctions {
				body := makeNoZeroAssembly(t, library, "makenozero."+name)
				if !strings.Contains(body, "runtime.mallocgc(SB)") || strings.Contains(body, "runtime.makeslice") ||
					strings.Contains(body, "runtime.growslice") || strings.Contains(body, "bytealg.MakeNoZero") {
					t.Errorf("%s: missing uninitialized allocation\n%s", name, body)
				}
			}
			wide := makeNoZeroAssembly(t, library, "makenozero.OrdinaryHeapWide")
			helper := "runtime.makeslice"
			if architecture == "386" {
				helper += "64"
			}
			if !strings.Contains(wide, helper+"(SB)") {
				t.Errorf("ordinary wide make: missing %s\n%s", helper, wide)
			}
			boundary := makeNoZeroAssembly(t, library, "makenozero.AddressSpaceBoundary")
			if strings.Contains(boundary, "runtime.panicmakeslice") {
				t.Errorf("representable byte size rejected by compiler\n%s", boundary)
			}

			checkMakeNoZeroStack(t, library, "makenozero.Stack", architecture, false)
			checkMakeNoZeroStack(t, library, "makenozero.OrdinaryStack", architecture, true)
			checkMakeNoZeroStack(t, library, "makenozero.LocalInline", architecture, false)
			for _, name := range []string{"VariableStack", "OrdinaryVariableStack"} {
				body := makeNoZeroAssembly(t, library, "makenozero."+name)
				ordinary := name == "OrdinaryVariableStack"
				checkMakeNoZeroInitialization(t, body, name, architecture, ordinary)
				helper := "runtime.mallocgc(SB)"
				if ordinary {
					helper = "runtime.makeslice(SB)"
				}
				if !strings.Contains(body, helper) || !strings.Contains(body, ".autotmp_") ||
					!strings.Contains(body, "runtime.panicmakeslicelen") || !strings.Contains(body, "runtime.panicmakeslicecap") {
					t.Errorf("%s: missing stack/heap allocation or validation\n%s", name, body)
				}
			}
			capacity := makeNoZeroAssembly(t, library, "makenozero.HeapCapacity")
			capacity, _, _ = strings.Cut(capacity, "CALL\truntime.mallocgc(SB)")
			capacityArgument := map[string]string{
				"amd64": "MOVL\t$32, AX",
				"386":   "MOVL\t$32, (SP)",
				"arm64": "MOVD\t$32, R0",
			}[architecture]
			if !strings.Contains(capacity, capacityArgument) {
				t.Errorf("heap allocation did not receive the full capacity\n%s", capacity)
			}
			if !strings.Contains(library, "inlining call to HeapCapacity") {
				t.Error("missing local inlining")
			}
			ordinary := makeNoZeroAssembly(t, library, "makenozero.OrdinaryHeap")
			if !strings.Contains(ordinary, "runtime.makeslice(SB)") {
				t.Errorf("ordinary make changed\n%s", ordinary)
			}
			mixed := makeNoZeroAssembly(t, library, "makenozero.Mixed")
			if !strings.Contains(mixed, "runtime.makeslice(SB)") || !strings.Contains(mixed, "runtime.mallocgc(SB)") {
				t.Errorf("marker leaked to another allocation\n%s", mixed)
			}

			caller := compile("caller", "caller.go", filepath.Join(directory, "caller.o"), "-importcfg", importcfg)
			checkMakeNoZeroStack(t, caller, "caller.Stack", architecture, false)
			for _, name := range []string{"Heap", "Generic"} {
				body := makeNoZeroAssembly(t, caller, "caller."+name)
				if !strings.Contains(body, "runtime.mallocgc(SB)") {
					t.Errorf("imported %s lost marker\n%s", name, body)
				}
			}
			if !strings.Contains(caller, "inlining call to makenozero.HeapCapacity") ||
				!strings.Contains(caller, "inlining call to makenozero.Heap") {
				t.Error("missing cross-package inlining")
			}
		})
	}
}

func checkMakeNoZeroStack(t *testing.T, assembly, name, architecture string, zero bool) {
	t.Helper()
	body := makeNoZeroAssembly(t, assembly, name)
	if strings.Contains(body, "runtime.makeslice") || strings.Contains(body, "runtime.newobject") || strings.Contains(body, "runtime.mallocgc") {
		t.Errorf("%s: backing storage escaped\n%s", name, body)
	}
	checkMakeNoZeroInitialization(t, body, name, architecture, zero)
}

func checkMakeNoZeroInitialization(t *testing.T, body, name, architecture string, zero bool) {
	t.Helper()
	// These fixtures expose all backing bytes to an opaque noescape function.
	// Zeroing cannot be dead-store eliminated, and no uninitialized value is read.
	pattern := `DUFFZERO|STOSL|MOVUPS\s+X15,|MOVQ\s+\$0,.*autotmp`
	if architecture == "arm64" {
		pattern = `DUFFZERO|STP(?:\.P)?\s+\(ZR, ZR\)|MOVD\s+ZR,.*autotmp`
	}
	if regexp.MustCompile(pattern).MatchString(body) != zero {
		t.Errorf("%s: zeroing mismatch (want %v)\n%s", name, zero, body)
	}
}

func TestMakeNoZeroErrors(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	goTool := testenv.GoToolPath(t)
	directory := t.TempDir()
	source := filepath.Join(directory, "invalid.go")
	object := filepath.Join(directory, "invalid.o")
	cases := []makeNoZeroErrorCase{
		{"arguments", "//go:makenozero extra\nb := make([]byte, 1); _ = b", "takes no arguments"},
		{"duplicate", "//go:makenozero\n//go:makenozero\nb := make([]byte, 1); _ = b", "duplicate go:makenozero"},
		{"multiple", "//go:makenozero\na, b := make([]byte, 1), make([]byte, 2); _, _ = a, b", "single-name"},
		{"assignment", "var b []byte\n//go:makenozero\nb = make([]byte, 1); _ = b", "single-name"},
		{"declaration", "//go:makenozero\nvar b = make([]byte, 1); _ = b", "misplaced go:makenozero"},
		{"return", "//go:makenozero\nreturn", "misplaced go:makenozero"},
		{"dangling", "//go:makenozero\n", "misplaced go:makenozero"},
		{"gap", "//go:makenozero\n\nb := make([]byte, 1); _ = b", "immediately following"},
		{"comment", "//go:makenozero\n// intervening comment\nb := make([]byte, 1); _ = b", "immediately following"},
		{"label", "//go:makenozero\nlabel: b := make([]byte, 1); _ = b; goto label", "misplaced go:makenozero"},
		{"if", "//go:makenozero\nif b := make([]byte, 1); b != nil {}", "misplaced go:makenozero"},
		{"header", "if true &&\n//go:makenozero\ntrue { b := make([]byte, 1); _ = b }", "misplaced go:makenozero"},
		{"case", "switch { case\n//go:makenozero\ntrue: b := make([]byte, 1); _ = b }", "misplaced go:makenozero"},
		{"closure", "_ = func(\n//go:makenozero\n) { b := make([]byte, 1); _ = b }", "misplaced go:makenozero"},
		{"inside", "b := make(\n//go:makenozero\n[]byte, 1); _ = b", "misplaced go:makenozero"},
		{"trailing", "b := make([]byte, 1) //go:makenozero\n_ = b", "misplaced compiler directive"},
		{"map", "//go:makenozero\nb := make(map[int]int); _ = b", "slice make expression"},
		{"channel", "//go:makenozero\nb := make(chan int); _ = b", "slice make expression"},
		{"nested", "//go:makenozero\nb := append(make([]byte, 1), 2); _ = b", "slice make expression"},
		{"shadowed", "make := func() []byte { return nil }\n//go:makenozero\nb := make(); _ = b", "slice make expression"},
		{"negative", "//go:makenozero\nb := make([]byte, -1); _ = b", "must not be negative"},
		{"capacity", "//go:makenozero\nb := make([]byte, 2, 1); _ = b", "length and capacity swapped"},
		{"overflow", "//go:makenozero\nb := make([]byte, 1<<100); _ = b", "overflows int"},
	}
	for _, element := range []string{"*int", "string", "any", "[]byte", "map[int]int", "func()", "chan int", "struct{ P *int }", "[2]*int", "[0]*int"} {
		cases = append(cases, makeNoZeroErrorCase{element, "//go:makenozero\nb := make([]" + element + ", 1); _ = b", "pointer-free"})
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := os.WriteFile(source, []byte("package invalid\nfunc f() {\n"+test.body+"\n}\n"), 0600)
			if err != nil {
				t.Fatal(err)
			}
			command := testenv.Command(t, goTool, "tool", "compile", "-o", object, source)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) || strings.Contains(string(output), "internal compiler error") {
				t.Fatalf("compile error = %v; want %q\n%s", err, test.want, output)
			}
		})
	}
}

func TestMakeNoZeroSemantics(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	command := testenv.Command(t, testenv.GoToolPath(t), "test", "-vet=off", "-count=1",
		filepath.Join("testdata", "makenozero", "semantics_test.go"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("makenozero semantics: %v\n%s", err, output)
	}
}

func makeNoZeroAssembly(t *testing.T, assembly, name string) string {
	t.Helper()
	start := strings.Index(assembly, "\n"+name+" STEXT ")
	if start < 0 {
		t.Fatalf("missing assembly for %s", name)
	}
	body := assembly[start+1:]
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
