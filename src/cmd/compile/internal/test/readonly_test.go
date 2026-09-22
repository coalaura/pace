// Copyright 2026 coalaura. All rights reserved.
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

const readOnlyDeclarations = `
type Table struct { Data []byte }
//go:noescape
//go:readonly src, table
func Read(dst, src []byte, table *Table)
//go:readonly src
func Retain(src []byte)
func Ordinary(src []byte)
//go:noescape
func Mutable(src []byte)
//go:readonly callback
//go:noescape
func Callback(callback func())
`

const readOnlyCallers = `
func ReadWrapper(dstMutable, srcReadOnly []byte, tableReadOnly *$Table) {
	$Read(dstMutable, srcReadOnly, tableReadOnly)
}
func RetainWrapper(retained []byte) { $Retain(retained) }
func OrdinaryWrapper(ordinary []byte) { $Ordinary(ordinary) }
func MutableWrapper(mutable []byte) { $Mutable(mutable) }
func CallbackWrapper(callbackReadOnly func()) { $Callback(callbackReadOnly) }
func Convert(readText, writeText, aliasText, retainedText, ordinaryText, mutableText string) {
	$Read(nil, []byte(readText), nil)
	$Read([]byte(writeText), nil, nil)
	alias := []byte(aliasText)
	$Read(alias, alias, nil)
	$Retain([]byte(retainedText))
	$Ordinary([]byte(ordinaryText))
	$Mutable([]byte(mutableText))
}
`

type readOnlyDiagnostic struct {
	name   string
	source string
	want   string
}

func TestReadOnlyEscapeSummaries(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	directory := t.TempDir()
	archive := filepath.Join(directory, "library.a")
	library := "package library\n" + readOnlyDeclarations
	compileReadOnly(t, library, false, "-p", "library", "-o", archive)
	importcfg := filepath.Join(directory, "importcfg")
	err := os.WriteFile(importcfg, []byte("packagefile library="+archive+"\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	variants := []string{"local", "imported"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			flags := []string{"-m", "-l", "-d=escapemutationscalls=1"}
			source := library + strings.ReplaceAll(readOnlyCallers, "$", "")
			if variant == "imported" {
				flags = append(flags, "-importcfg", importcfg)
				source = "package caller\nimport \"library\"\n" + strings.ReplaceAll(readOnlyCallers, "$", "library.")
			}

			output := compileReadOnly(t, source, false, flags...)
			want := []string{
				"mutates param: dstMutable derefs=0",
				"mutates param: mutable derefs=0",
				"calls param: srcReadOnly derefs=0",
				"calls param: tableReadOnly derefs=0",
				"calls param: callbackReadOnly derefs=0",
				"leaking param: retained",
				"leaking param: ordinary",
				"([]byte)(retainedText) escapes to heap",
				"([]byte)(ordinaryText) escapes to heap",
				"([]byte)(readText) does not escape",
				"([]byte)(writeText) does not escape",
				"([]byte)(aliasText) does not escape",
				"([]byte)(mutableText) does not escape",
			}
			for _, diagnostic := range want {
				if !strings.Contains(output, diagnostic) {
					t.Errorf("missing %q\n%s", diagnostic, output)
				}
			}

			unchanged := []string{"srcReadOnly", "tableReadOnly", "callbackReadOnly"}
			for _, name := range unchanged {
				if strings.Contains(output, "mutates param: "+name) || strings.Contains(output, "leaking param: "+name) {
					t.Errorf("readonly noescape parameter %s mutates or escapes\n%s", name, output)
				}
			}

			// Only readText may be reused: writeText and aliasText have mutable
			// aliases, retainedText and ordinaryText escape, and Mutable has no promise.
			if strings.Count(output, "zero-copy string->[]byte conversion") != 1 {
				t.Errorf("want exactly one zero-copy conversion\n%s", output)
			}

			readLine := strings.Count(source[:strings.Index(source, "[]byte(readText)")], "\n") + 1
			readLocation := "readonly.go:" + strconv.Itoa(readLine) + ":"
			for line := range strings.SplitSeq(output, "\n") {
				if strings.Contains(line, "zero-copy string->[]byte conversion") && !strings.Contains(line, readLocation) {
					t.Errorf("zero-copy conversion outside the readonly argument: %s", line)
				}
			}
		})
	}
}

func TestReadOnlyDiagnostics(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	cases := []readOnlyDiagnostic{
		{"body", "//go:readonly src\nfunc F(src []byte) {}", "requires a bodyless function"},
		{"blank body", "//go:readonly src\nfunc _(src []byte) {}", "requires a bodyless function"},
		{"unknown", "//go:readonly missing\nfunc F(src []byte)", `unknown go:readonly input parameter "missing"`},
		{"result", "//go:readonly result\nfunc F(src []byte) (result []byte)", `unknown go:readonly input parameter "result"`},
		{"receiver", "type T int\n//go:readonly recv\nfunc (recv *T) F(src []byte)", `unknown go:readonly input parameter "recv"`},
		{"duplicate", "//go:readonly src, src\nfunc F(src []byte)", `duplicate go:readonly parameter "src"`},
		{"repeated", "//go:readonly src\n//go:readonly src\nfunc F(src []byte)", "duplicate go:readonly directive"},
		{"bare", "//go:readonly\nfunc F(src []byte)", "malformed go:readonly parameter list"},
		{"empty", "//go:readonly \t\nfunc F(src []byte)", "malformed go:readonly parameter list"},
		{"leading comma", "//go:readonly ,src\nfunc F(src []byte)", "malformed go:readonly parameter list"},
		{"trailing comma", "//go:readonly src,\nfunc F(src []byte)", "malformed go:readonly parameter list"},
		{"empty name", "//go:readonly src,,dst\nfunc F(src, dst []byte)", "malformed go:readonly parameter list"},
		{"missing comma", "//go:readonly src dst\nfunc F(src, dst []byte)", "malformed go:readonly parameter list"},
		{"punctuation", "//go:readonly src;dst\nfunc F(src, dst []byte)", "malformed go:readonly parameter list"},
		{"expression", "//go:readonly src[0]\nfunc F(src []byte)", "malformed go:readonly parameter list"},
		{"keyword", "//go:readonly func\nfunc F(src []byte)", "malformed go:readonly parameter list"},
		{"blank", "//go:readonly _\nfunc F(_ []byte)", "malformed go:readonly parameter list"},
		{"unnamed", "//go:readonly src\nfunc F([]byte)", `unknown go:readonly input parameter "src"`},
		{"var", "//go:readonly src\nvar src []byte", "misplaced go:readonly directive"},
		{"type", "//go:readonly src\ntype src []byte", "misplaced go:readonly directive"},
		{"const", "//go:readonly src\nconst src = 1", "misplaced go:readonly directive"},
		{"statement", "func F() {\n//go:readonly src\nsrc := 1; _ = src\n}", "misplaced go:readonly directive"},
		{"dangling", "//go:readonly src\n", "misplaced go:readonly directive"},
		{"trailing", "func F(src []byte) //go:readonly src", "misplaced compiler directive"},
		{"whitespace", "//go:readonly\t src ,\t dst\nfunc F(src, dst []byte)", ""},
		{"unicode", "//go:readonly données\nfunc F(données []byte)", ""},
		{"variadic", "//go:noescape\n//go:readonly src\nfunc F(src ...*byte)", ""},
		{"scalar", "//go:readonly count\nfunc F(count int)", ""},
		{"method input", "type T int\n//go:noescape\n//go:readonly src\nfunc (recv *T) F(src []byte)", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			output := compileReadOnly(t, "package p\n"+test.source+"\n", test.want != "")
			if !strings.Contains(output, test.want) {
				t.Fatalf("missing %q\n%s", test.want, output)
			}
		})
	}

	output := compileReadOnly(t, "//go:readonly src\npackage p\n", true)
	if !strings.Contains(output, "misplaced go:readonly directive") {
		t.Fatalf("missing package diagnostic\n%s", output)
	}
}

func TestReadOnlyABIInternal(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	t.Setenv("GOARCH", "amd64")
	directory := t.TempDir()
	symabis := filepath.Join(directory, "symabis")
	err := os.WriteFile(symabis, []byte("def p.Read ABI0\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	source := `package p
//go:noescape
//go:readonly src
//go:abiinternal dst=BX src=AX ->
func Read(dst, src *byte)
func Caller(dstMutable, srcReadOnly *byte) { Read(dstMutable, srcReadOnly) }
`
	output := compileReadOnly(t, source, false, "-p", "p", "-m", "-l", "-d=escapemutationscalls=1",
		"-symabis", symabis, "-abiinternal", filepath.Join(directory, "abiinternal"))
	if !strings.Contains(output, "mutates param: dstMutable derefs=0") ||
		!strings.Contains(output, "calls param: srcReadOnly derefs=0") ||
		strings.Contains(output, "mutates param: srcReadOnly") {
		t.Fatalf("unexpected mutation summaries with go:abiinternal\n%s", output)
	}
}

func compileReadOnly(t *testing.T, source string, fail bool, flags ...string) string {
	t.Helper()
	directory := t.TempDir()
	filename := filepath.Join(directory, "readonly.go")
	err := os.WriteFile(filename, []byte(source), 0600)
	if err != nil {
		t.Fatal(err)
	}

	arguments := make([]string, 0, 5+len(flags))
	arguments = append(arguments, "tool", "compile", "-o", filepath.Join(directory, "readonly.o"))
	arguments = append(arguments, flags...)
	arguments = append(arguments, filename)
	output, err := testenv.Command(t, testenv.GoToolPath(t), arguments...).CombinedOutput()
	if (err != nil) != fail || strings.Contains(string(output), "internal compiler error") {
		t.Fatalf("compile: %v (want failure: %v)\n%s", err, fail, output)
	}

	return string(output)
}
