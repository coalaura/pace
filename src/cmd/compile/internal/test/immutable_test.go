// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"debug/elf"
	"internal/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const immutableLibrary = `package library
type Value struct { Field int; Array [2]int }
func (value Value) Read() int { return value.Field }
func (value *Value) Write() { value.Field++ }
//go:immutable
var Table = [4]uint64{10, 20, 30, 40}
//go:immutable
var Data = []byte{1, 2, 3, 4}
//go:immutable
var Record = Value{Field: 7}
//go:immutable
var Zero int
//go:immutable
var Dynamic = runtimeValue()
//go:noinline
func runtimeValue() int { return 19 }
func Read(index int) uint64 { return Table[index] }
func Generic[T ~int](input T) T {
	//go:immutable
	value := input
	return value
}
func Local() int {
	//go:immutable
	data := []int{3, 5}
	return data[0] + data[1]
}
`

type immutableCase struct {
	name   string
	source string
	want   string
}

func TestImmutableDiagnostics(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	cases := []immutableCase{
		{"arguments", "//go:immutable x\nvar x = 1", "takes no arguments"},
		{"tab argument", "//go:immutable\tx\nvar x = 1", "takes no arguments"},
		{"duplicate", "//go:immutable\n//go:immutable\nvar x = 1", "duplicate go:immutable"},
		{"function", "//go:immutable\nfunc F() {}", "misplaced go:immutable"},
		{"type", "//go:immutable\ntype T int", "misplaced go:immutable"},
		{"const", "//go:immutable\nconst x = 1", "misplaced go:immutable"},
		{"dangling", "//go:immutable\n", "misplaced go:immutable"},
		{"multiple", "//go:immutable\nvar x, y = 1, 2", "single-name"},
		{"blank", "//go:immutable\nvar _ = 1", "single-name"},
		{"group", "//go:immutable\nvar (x = 1)", "misplaced go:immutable"},
		{"blank line", "//go:immutable\n\nvar x = 1", "immediately following"},
		{"comment", "//go:immutable\n// comment\nvar x = 1", "immediately following"},
		{"assignment directive", "func F() { x := 1\n//go:immutable\nx = 2; _ = x }", "single-name"},
		{"local multiple", "func F() {\n//go:immutable\nx, y := 1, 2; _, _ = x, y }", "single-name"},
		{"pointer", "//go:immutable\nvar x = new(int)", "pointer-free"},
		{"map", "//go:immutable\nvar x = map[int]int{}", "pointer-free"},
		{"string", "//go:immutable\nvar x = \"text\"", "pointer-free"},
		{"reference field", "//go:immutable\nvar x = struct{ P *int }{}", "pointer-free"},
		{"reference element", "//go:immutable\nvar x = []*int{nil}", "pointer-free"},
		{"slice alias", "var original = []int{1}\n//go:immutable\nvar x = original", "fresh slice"},
		{"slice call", "func makeSlice() []int { return []int{1} }\n//go:immutable\nvar x = makeSlice()", "fresh slice"},
		{"slice conversion", "//go:immutable\nvar x = []byte(\"abc\")", "fresh slice"},
		{"slice shadow make", "func make([]int, int) []int { return nil }\n//go:immutable\nvar x = make([]int{}, 1)", "fresh slice"},
		{"makenozero", "func F() {\n//go:makenozero\n//go:immutable\nx := make([]int, 1); _ = x }", "misplaced go:makenozero"},
		{"muststack", "func F() {\n//go:muststack\n//go:immutable\nx := make([]int, 1); _ = x }", "misplaced go:muststack"},
		{"assign", "//go:immutable\nvar x = 1\nfunc F() { x = 2 }", "cannot assign"},
		{"increment", "//go:immutable\nvar x = 1\nfunc F() { x++ }", "cannot assign"},
		{"compound", "//go:immutable\nvar x = 1\nfunc F() { x += 2 }", "cannot assign"},
		{"multi assign", "//go:immutable\nvar x = 1\nfunc F() { x, _ = 2, 3 }", "cannot assign"},
		{"reuse short", "func F() {\n//go:immutable\nx := 1\nx, y := 2, 3; _, _ = x, y }", "cannot assign"},
		{"range assign", "//go:immutable\nvar x = 1\nfunc F() { for x = range 3 {} }", "cannot assign"},
		{"select assign", "//go:immutable\nvar x = 1\nfunc F(c chan int) { select { case x = <-c: } }", "cannot assign"},
		{"forward write", "func F() { x++ }\n//go:immutable\nvar x = 1", "cannot assign"},
		{"closure write", "func F() {\n//go:immutable\nx := 1\nf := func(){ x++ }; f() }", "cannot assign"},
		{"field", "//go:immutable\nvar x = struct{ A [2]int }{}\nfunc F() { x.A[0]++ }", "cannot assign"},
		{"pointer alias", "//go:immutable\nvar x = 1\nvar alias = &x", "cannot take address"},
		{"field address", "//go:immutable\nvar x = struct{ A int }{}\nvar alias = &x.A", "cannot take address"},
		{"array slice", "//go:immutable\nvar x = [2]int{}\nvar alias = (x)[:]", "cannot slice"},
		{"slice write", "//go:immutable\nvar x = []int{1}\nfunc F() { x[0]++ }", "cannot assign"},
		{"slice field", "//go:immutable\nvar x = []struct{ A int }{{}}\nfunc F() { x[0].A = 3 }", "cannot assign"},
		{"slice element address", "//go:immutable\nvar x = []int{1}\nvar alias = &x[0]", "cannot take address"},
		{"slice escape", "//go:immutable\nvar x = []int{1}\nvar alias = x", "mutable alias"},
		{"slice return", "//go:immutable\nvar x = []int{1}\nfunc F() []int { return x }", "mutable alias"},
		{"slice append", "//go:immutable\nvar x = make([]int, 1, 4)\nvar alias = append(x, 2)", "mutable alias"},
		{"slice clear", "//go:immutable\nvar x = []int{1}\nfunc F() { clear(x) }", "mutable alias"},
		{"slice copy destination", "//go:immutable\nvar x = []int{1}\nfunc F() { copy(x, []int{2}) }", "mutable alias"},
		{"slice array pointer", "//go:immutable\nvar x = []int{1}\nvar alias = (*[1]int)(x)", "mutable alias"},
		{"slice interface", "//go:immutable\nvar x = []int{1}\nvar alias any = x", "mutable alias"},
		{"slice call argument", "//go:immutable\nvar x = []int{1}\nfunc mutate(s []int) { s[0]++ }\nfunc F() { mutate(x) }", "mutable alias"},
		{"slice variadic", "//go:immutable\nvar x = []int{1}\nfunc mutate(s ...int) { s[0]++ }\nfunc F() { mutate(x...) }", "mutable alias"},
		{"slice go call", "//go:immutable\nvar x = []int{1}\nfunc mutate(s []int) { s[0]++ }\nfunc F() { go mutate(x) }", "mutable alias"},
		{"slice defer call", "//go:immutable\nvar x = []int{1}\nfunc mutate(s []int) { s[0]++ }\nfunc F() { defer mutate(x) }", "mutable alias"},
		{"slice send", "//go:immutable\nvar x = []int{1}\nfunc F(c chan []int) { c <- x }", "mutable alias"},
		{"slice shadow len", "//go:immutable\nvar x = []int{1}\nfunc len(s []int) int { s[0]++; return 1 }\nfunc F() { len(x) }", "mutable alias"},
		{"slice readonly call", "//go:immutable\nvar x = []int{1}\n//go:readonly s\nfunc read(s []int)\nfunc F() { read(x) }", "mutable alias"},
		{"slice method", "type S []int\nfunc (s S) Write() { s[0]++ }\n//go:immutable\nvar x = S{1}\nfunc F() { x.Write() }", "reference-receiver"},
		{"pointer method", "type T int\nfunc (t *T) Write() { *t++ }\n//go:immutable\nvar x T\nfunc F() { x.Write() }", "reference-receiver"},
		{"pointer method value", "type T int\nfunc (t *T) Write() { *t++ }\n//go:immutable\nvar x T\nvar method = x.Write", "reference-receiver"},
		{"promoted method", "type T int\nfunc (t *T) Write() { *t++ }\n//go:immutable\nvar x struct{ T }\nfunc F() { x.Write() }", "reference-receiver"},
		{"generic uninstantiated", "func F[T ~int](input T) {\n//go:immutable\nx := input\nx++ }", "cannot assign"},
		{"generic unknown", "func F[T any](input T) T {\n//go:immutable\nx := input\nreturn x }", "pointer-free"},
		{"generic alias", "func F[T ~[]int](input T) int {\n//go:immutable\nx := input\nreturn len(x) }", "fresh slice"},
		{"generic slice method", "func F[V interface{ ~[]int; Write() }]() {\n//go:immutable\nx := make(V, 1)\nx.Write() }", "reference-receiver"},
		{"linkname", "import _ \"unsafe\"\n//go:linkname x\n//go:immutable\nvar x = 1", "cannot have a linkname"},
		{"local var", "func F() int {\n//go:immutable\nvar x = 1\nreturn x }", ""},
		{"group member", "var (\n//go:immutable\nx = 1\n)", ""},
		{"shadow", "func F() int {\n//go:immutable\nx := 1\n{ x := 2; x++ }; return x }", ""},
		{"range shadow", "func F() int {\n//go:immutable\nx := 1\nfor x := range 3 { x++; _ = x }; return x }", ""},
		{"value copy", "//go:immutable\nvar x = [2]int{}\nfunc F() { y := x; y[0]++ }", ""},
		{"value call", "//go:immutable\nvar x = [2]int{}\nfunc mutate(a [2]int) { a[0]++ }\nfunc F() { mutate(x) }", ""},
		{"value initialization alias", "var original = [2]int{1}\n//go:immutable\nvar x = original\nfunc F() { original[0]++ }", ""},
		{"value interface", "//go:immutable\nvar x = [2]int{1}\nvar copy any = x", ""},
		{"slice reads", "//go:immutable\nvar x = []byte{1, 2}\nfunc F(dst []byte) int { _ = x; _ = (x); _ = string(x); _ = x == nil; for _, value := range x { _ = value }; return len(x) + cap(x) + int(x[0]) + copy(dst, x) }", ""},
		{"nil slice", "//go:immutable\nvar x []int\n//go:immutable\nvar y []int = nil", ""},
		{"generic slice make", "func F[T ~int](n int) T {\n//go:immutable\nx := make([]T, n)\nreturn x[0] }", ""},
	}
	variants := []string{"inline", "noinline"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			var flags []string
			if variant == "noinline" {
				flags = append(flags, "-l")
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					output := compileImmutable(t, "package p\n"+test.source+"\n", test.want != "", flags...)
					if !strings.Contains(output, test.want) {
						t.Fatalf("missing %q\n%s", test.want, output)
					}
				})
			}
		})
	}
}

func TestImmutableImports(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	directory := t.TempDir()
	archive := filepath.Join(directory, "library.a")
	compileImmutable(t, immutableLibrary, false, "-p", "library", "-o", archive)
	importcfg := filepath.Join(directory, "importcfg")
	err := os.WriteFile(importcfg, []byte("packagefile library="+archive+"\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	cases := []immutableCase{
		{"write", "func F() { library.Table[0]++ }", "cannot assign"},
		{"assign", "func F() { library.Table = [4]uint64{} }", "cannot assign"},
		{"address", "var alias = &library.Table", "cannot take address"},
		{"slice", "var alias = library.Table[:]", "cannot slice"},
		{"slice write", "func F() { library.Data[0]++ }", "cannot assign"},
		{"slice alias", "var alias = library.Data", "mutable alias"},
		{"method", "func F() { library.Record.Write() }", "reference-receiver"},
		{"dynamic", "func F() { library.Dynamic++ }", "cannot assign"},
		{"read", "func F(index int) uint64 { return library.Table[index] + library.Read(index) + uint64(library.Generic(3)) + uint64(library.Local()) + uint64(library.Record.Read()) + uint64(library.Data[0]) }", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			output := compileImmutable(t, "package caller\nimport \"library\"\n"+test.source, test.want != "", "-p", "caller", "-importcfg", importcfg, "-m")
			if !strings.Contains(output, test.want) {
				t.Fatalf("missing %q\n%s", test.want, output)
			}
			if test.name == "read" && !strings.Contains(output, "inlining call to library.Read") {
				t.Fatalf("cross-package inlining was not exercised\n%s", output)
			}
		})
	}
}

func TestImmutableStorage(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	t.Setenv("GOARCH", "amd64")
	source := immutableLibrary + `
//go:immutable
var Partial = [2]int{1, runtimeValue()}
//go:immutable
var PartialSlice = []int{1, runtimeValue()}
//go:immutable
var Made = make([]int, 2)
var Mutable = [4]uint64{10, 20, 30, 40}
func Fold() uint64 { return Table[2] }
func AcrossCall() (uint64, uint64) { before := Table[2]; runtimeValue(); return before, Table[2] }
func MutableLoad() uint64 { return Mutable[2] }
`
	output := compileImmutable(t, source, false, "-p", "library", "-S")
	want := []string{"library.Table SRODATA", "library.Data SRODATA", "library..stmp_0 SRODATA static size=4", "library.Record SRODATA", "library.Zero SRODATA", "library.Dynamic SNOPTRBSS", "library.Partial SNOPTRDATA", "library.PartialSlice SDATA", "library..stmp_1 SNOPTRDATA", "library.Made SBSS", "library.Mutable SNOPTRDATA"}
	for _, fragment := range want {
		if !strings.Contains(output, fragment) {
			t.Errorf("missing %q", fragment)
		}
	}
	fold := makeNoZeroAssembly(t, output, "library.Fold")
	across := makeNoZeroAssembly(t, output, "library.AcrossCall")
	mutable := makeNoZeroAssembly(t, output, "library.MutableLoad")
	if !strings.Contains(fold, "$30") || strings.Contains(fold, "library.Table") {
		t.Errorf("immutable load not folded\n%s", fold)
	}
	if !strings.Contains(across, "$30") || strings.Contains(across, "library.Table") {
		t.Errorf("immutable loads across call not folded\n%s", across)
	}
	if !strings.Contains(mutable, "library.Mutable") {
		t.Errorf("missing ordinary mutable load\n%s", mutable)
	}
	if t.Failed() {
		t.Log(output)
	}
}

func TestImmutableRuntime(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	directory := t.TempDir()
	filename := filepath.Join(directory, "immutable_test.go")
	source := strings.Replace(immutableLibrary, "package library", "package immutable\nimport \"testing\"", 1) + `
//go:immutable
var Partial = [2]int{1, runtimeValue()}
//go:immutable
var PartialSlice = []int{1, runtimeValue()}
//go:immutable
var Made = make([]int, 2)
var Original = [2]int{11, 13}
//go:immutable
var Copy = Original
func init() { Original[0] = 99 }
func TestValues(t *testing.T) {
	if Table[2] != 30 || Data[3] != 4 || Record.Read() != 7 || Zero != 0 || Dynamic != 19 {
		t.Fatal("package initialization")
	}
	if Partial[1] != 19 || PartialSlice[1] != 19 || Made[1] != 0 {
		t.Fatal("runtime initialization")
	}
	if Original[0] != 99 || Copy[0] != 11 { t.Fatal("initializer copy") }
	if Generic(13) != 13 || Local() != 8 {
		t.Fatal("generic or inline local")
	}
	//go:immutable
	local := []byte{3, 5, 7}
	copyOf := make([]byte, len(local))
	if copy(copyOf, local) != 3 { t.Fatal("copy") }
	copyOf[0] = 99
	if local[0] != 3 || string(local) != "\x03\x05\x07" { t.Fatal("alias") }
	sum := 0
	for _, value := range local { sum += int(value) }
	closure := func() int { return sum + int(local[0]) }
	if closure() != 18 { t.Fatal("range or capture") }
	valueCopy := Record
	valueCopy.Write()
	if Record.Field != 7 || valueCopy.Field != 8 { t.Fatal("value copy") }
	//go:immutable
	runtimeLocal := runtimeValue()
	if runtimeLocal != 19 { t.Fatal("local runtime initializer") }
}
`
	err := os.WriteFile(filename, []byte(source), 0600)
	if err != nil {
		t.Fatal(err)
	}
	variants := []string{"", "-gcflags=all=-l"}
	for _, variant := range variants {
		arguments := []string{"test", "-vet=off", "-count=1"}
		if variant != "" {
			arguments = append(arguments, variant)
		}
		arguments = append(arguments, filename)
		output, err := testenv.Command(t, testenv.GoToolPath(t), arguments...).CombinedOutput()
		if err != nil {
			t.Fatalf("runtime test %s: %v\n%s", variant, err, output)
		}
	}
}

func TestImmutableImportRuntime(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	directory := t.TempDir()
	bridgeDirectory := filepath.Join(directory, "bridge")
	err := os.Mkdir(bridgeDirectory, 0700)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":     "module immutabletest\n\ngo 1.27\n",
		"library.go": immutableLibrary,
		"bridge/bridge.go": `package bridge
import library "immutabletest"
func Read(index int) uint64 { return library.Table[index] + uint64(library.Data[index]) }
func Generic[T ~int](input T) T { return library.Generic(input) }
`,
		"library_test.go": `package library_test
import (
	library "immutabletest"
	"immutabletest/bridge"
	"testing"
)
func TestImports(t *testing.T) {
	for index := range 4 {
		want := uint64(10 * (index + 1))
		if library.Table[index] != want || library.Read(index) != want || bridge.Read(index) != want + uint64(index + 1) {
			t.Fatal("imported immutable data or inlined load", index)
		}
	}
	if library.Dynamic != 19 || library.Record.Read() != 7 || library.Zero != 0 || bridge.Generic(17) != 17 || library.Local() != 8 {
		t.Fatal("imported initialization or generics")
	}
}
`,
	}
	for filename, contents := range files {
		err = os.WriteFile(filepath.Join(directory, filename), []byte(contents), 0600)
		if err != nil {
			t.Fatal(err)
		}
	}
	variants := []string{"", "-gcflags=all=-l"}
	for _, variant := range variants {
		arguments := []string{"test", "-vet=off", "-count=1"}
		if variant != "" {
			arguments = append(arguments, variant)
		}
		command := testenv.Command(t, testenv.GoToolPath(t), arguments...)
		command.Dir = directory
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("import runtime test %s: %v\n%s", variant, err, output)
		}
	}

	// Use one object format on every host to inspect both the slice header and
	// its anonymous backing allocation, which is omitted from the symbol table.
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "amd64")
	t.Setenv("CGO_ENABLED", "0")
	executable := filepath.Join(directory, "immutable.test")
	command := testenv.Command(t, testenv.GoToolPath(t), "test", "-vet=off", "-c", "-o", executable)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build linked storage test: %v\n%s", err, output)
	}
	image, err := elf.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	symbols, err := image.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	rodata := image.Section(".rodata")
	if rodata == nil || rodata.Flags&elf.SHF_WRITE != 0 {
		t.Fatal("missing readonly .rodata section")
	}
	data, err := rodata.Data()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"immutabletest.Table": true, "immutabletest.Data": true, "immutabletest.Dynamic": false}
	for _, symbol := range symbols {
		readonly, found := want[symbol.Name]
		if !found {
			continue
		}
		section := image.Sections[symbol.Section]
		if (section == rodata) != readonly || (section.Flags&elf.SHF_WRITE == 0) != readonly {
			t.Fatalf("unexpected section %s for %s", section.Name, symbol.Name)
		}
		if symbol.Name == "immutabletest.Data" {
			backing := image.ByteOrder.Uint64(data[symbol.Value-rodata.Addr:])
			if backing < rodata.Addr || backing+4 > rodata.Addr+rodata.Size {
				t.Fatal("slice backing storage is outside .rodata")
			}
			if string(data[backing-rodata.Addr:backing-rodata.Addr+4]) != "\x01\x02\x03\x04" {
				t.Fatal("wrong slice backing storage contents")
			}
		}
		delete(want, symbol.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing linked symbols: %v", want)
	}
}

func compileImmutable(t *testing.T, source string, fail bool, flags ...string) string {
	t.Helper()
	directory := t.TempDir()
	filename := filepath.Join(directory, "immutable.go")
	err := os.WriteFile(filename, []byte(source), 0600)
	if err != nil {
		t.Fatal(err)
	}

	arguments := make([]string, 0, 5+len(flags))
	arguments = append(arguments, "tool", "compile", "-o", filepath.Join(directory, "immutable.o"))
	arguments = append(arguments, flags...)
	arguments = append(arguments, filename)
	output, err := testenv.Command(t, testenv.GoToolPath(t), arguments...).CombinedOutput()
	if (err != nil) != fail || strings.Contains(string(output), "internal compiler error") {
		t.Fatalf("compile: %v (want failure: %v)\n%s", err, fail, output)
	}
	return string(output)
}
