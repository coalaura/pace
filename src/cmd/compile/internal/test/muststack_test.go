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

type mustStackCase struct {
	name string
	body string
	want string
}

func TestMustStack(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	flagSets := [][]string{nil, {"-l"}}
	cases := []mustStackCase{
		{"value", "//go:muststack\nvalue := BigStruct{}; use(&value)", ""},
		{"slice", "//go:muststack\nbuf := make([]byte, 4096); useSlice(buf)", ""},
		{"capacity", "//go:muststack\nbuf := make([]byte, n, 4096); useSlice(buf)", ""},
		{"folded", "size := 4096\n//go:muststack\nbuf := make([]byte, size); useSlice(buf)", ""},
		{"new", "//go:muststack\nptr := new(BigStruct); use(ptr)", ""},
		{"literal", "//go:muststack\nptr := &BigStruct{}; use(ptr)", ""},
		{"slice-literal", "//go:muststack\nbuf := []byte{1, 2}; useSlice(buf)", ""},
		{"scope", "//go:muststack\nvalue := BigStruct{}; use(&value); other := BigStruct{}; pointerSink = &other", ""},
		{"existing-storage", "//go:muststack\nptr := pointerSink; pointerSink = ptr", ""},
		{"callee", "//go:muststack\nvalue := allocatingCall(); _ = value", ""},
		{"argument-escape", "//go:muststack\nvalue := escapingArgument(new(BigStruct)); _ = value", "initial storage for value"},
		{"small-map", "//go:muststack\nvalue := make(map[int]int); _ = value", ""},
		{"map-literal", "//go:muststack\nvalue := map[int]int{1: 2}; _ = value", ""},
		{"large-map", "//go:muststack\nvalue := make(map[int]int, 100); _ = value", "initial storage for value"},
		{"dynamic-map", "//go:muststack\nvalue := make(map[int]int, n); _ = value", "initial storage for value"},
		{"channel", "//go:muststack\nvalue := make(chan int); _ = value", "initial storage for value"},
		{"bytes", "//go:muststack\nbuf := []byte(\"hello\"); useSlice(buf)", ""},
		{"small-interface", "//go:muststack\nvalue := any(n); useAny(value)", ""},
		{"large-interface", "//go:muststack\nvalue := any(BigStruct{}); useAny(value)", "initial storage for value"},
		{"both", "//go:muststack\n//go:makenozero\nbuf := make([]byte, 4096); useSlice(buf)", ""},
		{"reverse", "//go:makenozero\n//go:muststack\nbuf := make([]byte, 4096); useSlice(buf)", ""},
		{"value-escape", "//go:muststack\nvalue := BigStruct{}; pointerSink = &value", "variable value escapes to heap"},
		{"slice-escape", "//go:muststack\nbuf := make([]byte, 4096); sliceSink = buf", "initial storage for buf"},
		{"slice-header-escape", "//go:muststack\nbuf := make([]byte, 4096); headerSink = &buf", "variable buf escapes to heap"},
		{"new-escape", "//go:muststack\nptr := new(BigStruct); pointerSink = ptr", "initial storage for ptr"},
		{"literal-escape", "//go:muststack\nptr := &BigStruct{}; pointerSink = ptr", "initial storage for ptr"},
		{"slice-literal-escape", "//go:muststack\nbuf := []byte{1, 2}; sliceSink = buf", "initial storage for buf"},
		{"later-escape", "//go:muststack\nbuf := make([]byte, 4096); useSlice(buf); sliceSink = buf", "initial storage for buf"},
		{"closure-escape", "//go:muststack\nvalue := BigStruct{}; closureSink = func() { use(&value) }", "variable value escapes to heap"},
		{"nested-escape", "//go:muststack\nvalue := struct{ P *BigStruct }{new(BigStruct)}; pointerSink = value.P", "initial storage for value"},
		{"dynamic", "//go:muststack\nbuf := make([]byte, n); useSlice(buf)", "initial storage for buf"},
		{"dynamic-capacity", "//go:muststack\nbuf := make([]byte, 1, n); useSlice(buf)", "initial storage for buf"},
		{"large-slice", "//go:muststack\nbuf := make([]byte, 1<<20); useSlice(buf)", "initial storage for buf"},
		{"large-new", "//go:muststack\nptr := new([1<<20]byte); _ = ptr", "initial storage for ptr"},
		{"large-literal", "//go:muststack\nptr := &[1<<20]byte{}; _ = ptr", "initial storage for ptr"},
		{"large-value", "//go:muststack\nvalue := [1<<24]byte{}; _ = value", "variable value escapes to heap"},
		{"both-escape", "//go:muststack\n//go:makenozero\nbuf := make([]byte, 4096); sliceSink = buf", "initial storage for buf"},
		{"reverse-escape", "//go:makenozero\n//go:muststack\nbuf := make([]byte, 4096); sliceSink = buf", "initial storage for buf"},
		{"arguments", "//go:muststack extra\nvalue := 1; _ = value", "takes no arguments"},
		{"duplicate", "//go:muststack\n//go:muststack\nvalue := 1; _ = value", "duplicate go:muststack"},
		{"duplicate-block", "//go:muststack\n//go:makenozero\n//go:muststack\nbuf := make([]byte, 1); _ = buf", "duplicate go:muststack"},
		{"multiple", "//go:muststack\na, b := 1, 2; _, _ = a, b", "single-name"},
		{"reassignment", "value := 1\n//go:muststack\nvalue = 2; _ = value", "single-name"},
		{"redeclaration", "value := 1\n//go:muststack\nvalue := 2; _ = value", "no new variables"},
		{"blank", "//go:muststack\n_ := 1", "no new variables"},
		{"var", "//go:muststack\nvar value = 1; _ = value", "misplaced go:muststack"},
		{"return", "//go:muststack\nreturn", "misplaced go:muststack"},
		{"dangling", "//go:muststack\n", "misplaced go:muststack"},
		{"gap", "//go:muststack\n\nvalue := 1; _ = value", "immediately following"},
		{"comment", "//go:muststack\n// comment\nvalue := 1; _ = value", "immediately following"},
		{"block-gap", "//go:muststack\n\n//go:makenozero\nbuf := make([]byte, 1); _ = buf", "immediately following"},
		{"block-comment", "//go:makenozero\n// comment\n//go:muststack\nbuf := make([]byte, 1); _ = buf", "immediately following"},
		{"block-trailing-gap", "//go:muststack\n//go:makenozero\n\nbuf := make([]byte, 1); _ = buf", "immediately following"},
		{"unrelated-directive", "//go:muststack\n//go:unknown\nvalue := 1; _ = value", "immediately following"},
		{"label", "//go:muststack\nlabel: value := 1; _ = value; goto label", "misplaced go:muststack"},
		{"if", "//go:muststack\nif value := 1; value > 0 {}", "misplaced go:muststack"},
		{"inside", "value := new(\n//go:muststack\nint); _ = value", "misplaced go:muststack"},
		{"trailing", "value := 1 //go:muststack\n_ = value", "misplaced compiler directive"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			source := `package muststack
type BigStruct [4096]byte
var pointerSink *BigStruct
var sliceSink []byte
var headerSink *[]byte
var closureSink func()
func allocatingCall() int { pointerSink = new(BigStruct); return 1 }
func escapingArgument(ptr *BigStruct) int { pointerSink = ptr; return 1 }
//go:noescape
func use(*BigStruct)
//go:noescape
func useSlice([]byte)
//go:noescape
func useAny(any)
func f(n int) {
` + test.body + "\n}\n"
			for _, flags := range flagSets {
				output := compileMustStack(t, source, test.want != "", flags...)
				if !strings.Contains(output, test.want) {
					t.Fatalf("want %q\n%s", test.want, output)
				}
			}
		})
	}
}

func TestMustStackPackagePragmas(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	sources := []string{
		"//go:muststack\npackage p\n",
		"package p\n//go:muststack\nvar value = 1\n",
		"package p\n//go:muststack\nfunc f() {}\n",
		"package p\n//go:muststack\ntype T int\n",
	}
	for _, source := range sources {
		output := compileMustStack(t, source, true)
		if !strings.Contains(output, "misplaced go:muststack") {
			t.Fatalf("missing misplaced diagnostic\n%s", output)
		}
	}
}

func TestMustStackComposition(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	source, err := os.ReadFile(filepath.Join("testdata", "muststack.go"))
	if err != nil {
		t.Fatal(err)
	}

	architectures := []string{"amd64", "386", "arm64"}
	for _, architecture := range architectures {
		t.Run(architecture, func(t *testing.T) {
			t.Setenv("GOARCH", architecture)
			assembly := compileMustStack(t, string(source), false, "-S", "-m", "-p", "muststack")
			checkMakeNoZeroStack(t, assembly, "muststack.Ordinary", architecture, true)
			checkMakeNoZeroStack(t, assembly, "muststack.Stack", architecture, true)
			checkMakeNoZeroStack(t, assembly, "muststack.Both", architecture, false)
			checkMakeNoZeroStack(t, assembly, "muststack.Reverse", architecture, false)
		})
	}
}

func TestMustStackUnannotated(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	source := `package p
var sink any
func f(n int) {
 value := [4096]byte{}
 sink = &value
 buf := make([]byte, 4096)
 sink = buf
 ptr := new([4096]byte)
 sink = ptr
 literal := &[4096]byte{}
 sink = literal
 dynamic := make([]byte, n)
 sink = dynamic
}
`
	output := compileMustStack(t, source, false, "-m", "-l")
	diagnostics := []string{"moved to heap: value", "make([]byte, 4096) escapes to heap", "new([4096]byte) escapes to heap", "&[4096]byte{} escapes to heap", "make([]byte, n) escapes to heap"}
	for _, diagnostic := range diagnostics {
		if !strings.Contains(output, diagnostic) {
			t.Errorf("missing %q\n%s", diagnostic, output)
		}
	}
}

func TestMustStackGeneric(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	flagSets := [][]string{nil, {"-l"}}
	directory := t.TempDir()
	archive := filepath.Join(directory, "library.a")
	library := `package library
func Value[T any]() {
 //go:muststack
 value := *new(T)
 _ = value
}
func Slice[T any]() {
 //go:muststack
 buf := make([]T, 16)
 _ = buf
}
func Pointer[T any]() *T {
 //go:muststack
 ptr := new(T)
 return ptr
}
func NoZero[T ~uint64]() {
 //go:makenozero
 //go:muststack
 buf := make([]T, 16)
 _ = buf
}
`
	compileMustStack(t, library, false, "-p", "library", "-o", archive)
	importcfg := filepath.Join(directory, "importcfg")
	err := os.WriteFile(importcfg, []byte("packagefile library="+archive+"\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	cases := []mustStackCase{
		{"value", "Value[int]()", ""},
		{"slice", "Slice[byte]()", ""},
		{"nozero", "NoZero[uint64]()", ""},
		{"large-value", "Value[[1<<24]byte]()", "variable value escapes to heap"},
		{"large-slice", "Slice[[1<<20]byte]()", "initial storage for buf"},
		{"pointer-escape", "Pointer[int]()", "initial storage for ptr"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			for _, flags := range flagSets {
				local := compileMustStack(t, library+"\nfunc local() { "+test.body+" }\n", test.want != "", flags...)
				importFlags := append([]string{"-importcfg", importcfg}, flags...)
				imported := compileMustStack(t, "package caller\nimport \"library\"\nfunc f() { library."+test.body+" }\n", test.want != "", importFlags...)
				if !strings.Contains(local, test.want) || !strings.Contains(imported, test.want) {
					t.Fatalf("want %q\nlocal:\n%s\nimported:\n%s", test.want, local, imported)
				}
			}
		})
	}
}

func compileMustStack(t *testing.T, source string, fail bool, flags ...string) string {
	t.Helper()
	directory := t.TempDir()
	filename := filepath.Join(directory, "muststack.go")
	err := os.WriteFile(filename, []byte(source), 0600)
	if err != nil {
		t.Fatal(err)
	}

	arguments := []string{"tool", "compile", "-o", filepath.Join(directory, "muststack.o")}
	arguments = append(arguments, flags...)
	arguments = append(arguments, filename)
	command := testenv.Command(t, testenv.GoToolPath(t), arguments...)
	output, err := command.CombinedOutput()
	if (err != nil) != fail || strings.Contains(string(output), "internal compiler error") {
		t.Fatalf("compile error = %v, want failure %v\n%s", err, fail, output)
	}
	return string(output)
}
