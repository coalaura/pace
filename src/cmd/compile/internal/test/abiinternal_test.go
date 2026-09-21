// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"cmd/internal/obj/x86"
	"cmd/internal/objabi"
	"encoding/json"
	"internal/testenv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type abiInternalDiagnostic struct {
	name       string
	source     string
	want       string
	arch       string
	noAssembly bool
}

type abiInternalAssemblyDiagnostic struct {
	name   string
	source string
	want   string
}

func TestABIInternalAssembly(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	if runtime.GOARCH != "amd64" {
		t.Skip("amd64 only")
	}
	goTool := testenv.GoToolPath(t)
	executable := filepath.Join(t.TempDir(), "abiinternal.exe")
	build := testenv.Command(t, goTool, "build", "-o", executable, ".")
	build.Dir = "testdata/abiinternal"
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	output, err = testenv.Command(t, executable).CombinedOutput()
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, output)
	}
	output, err = testenv.Command(t, goTool, "tool", "objdump", "-s", `abiinternaltest/asm\.`, executable).CombinedOutput()
	if err != nil {
		t.Fatalf("objdump: %v\n%s", err, output)
	}
	disassembly := string(output)
	if strings.Contains(disassembly, "Optimized.abi0") || strings.Contains(disassembly, "Different.abi0") || strings.Contains(disassembly, "Identity.abi0") {
		t.Fatal("annotated definition retained ABI0 call path")
	}
	if !strings.Contains(disassembly, "Ordinary.abi0") || !strings.Contains(disassembly, "Referenced.abi0") {
		t.Fatal("ordinary ABI0 definition or reverse wrapper missing")
	}
	names := []string{"Optimized", "Different", "Identity", "Cycle", "Narrow"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			start := strings.Index(disassembly, "TEXT abiinternaltest/asm."+name+"(SB)")
			if start < 0 {
				t.Fatal("missing ABIInternal definition")
			}
			body := disassembly[start:]
			end := strings.Index(body, "\n\n")
			if end >= 0 {
				body = body[:end]
			}
			wantMoves := 0
			if name == "Optimized" {
				wantMoves = 2 // AX -> DX and the authored memory load
				if !strings.Contains(body, "MOVQ AX, DX") {
					t.Fatal("missing AX -> DX adaptation")
				}
			}
			if name == "Different" {
				wantMoves = 6 // four entry copies, one memory load, one return copy
			}
			if name == "Cycle" {
				wantMoves = 12 // four copies per cycle: entry and two returns
				if strings.Count(body, "RET") != 2 || strings.Count(body, "R12") != 6 {
					t.Fatalf("missing cycle adaptation:\n%s", body)
				}
			}
			if strings.Count(body, "MOVQ") != wantMoves || strings.Contains(body, "(SP)") || strings.Contains(body, "CALL") {
				t.Fatalf("unexpected moves or ABI0 path:\n%s", body)
			}
			if strings.Contains(body, "MOVB ") || strings.Contains(body, "MOVW ") || strings.Contains(body, "MOVL ") {
				t.Fatalf("mapped identity move survived:\n%s", body)
			}
			if name == "Identity" && !strings.Contains(body, "RET") {
				t.Fatal("missing return")
			}
		})
	}
}

func TestABIInternalDiagnostics(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	goTool := testenv.GoToolPath(t)
	tests := []abiInternalDiagnostic{
		{name: "missing input", source: "//go:abiinternal -> r=AX\nfunc F(a uint64) (r uint64)", want: `missing go:abiinternal mapping for "a"`},
		{name: "missing result", source: "//go:abiinternal a=AX ->\nfunc F(a uint64) (r uint64)", want: `missing go:abiinternal mapping for "r"`},
		{name: "unknown", source: "//go:abiinternal extra=AX ->\nfunc F()", want: `unknown go:abiinternal name "extra"`},
		{name: "duplicate name", source: "//go:abiinternal a=AX a=BX ->\nfunc F(a uint64)", want: `duplicate go:abiinternal name "a"`},
		{name: "duplicate input register", source: "//go:abiinternal a=AX b=AX ->\nfunc F(a, b uint64)", want: `duplicate go:abiinternal register "AX"`},
		{name: "duplicate result register", source: "//go:abiinternal -> a=AX b=AX\nfunc F() (a, b uint64)", want: `duplicate go:abiinternal register "AX"`},
		{name: "malformed", source: "//go:abiinternal a=AX=BX ->\nfunc F(a uint64)", want: "malformed go:abiinternal assignment"},
		{name: "no equals", source: "//go:abiinternal a ->\nfunc F(a uint64)", want: "malformed go:abiinternal assignment"},
		{name: "no arrow", source: "//go:abiinternal a=AX\nfunc F(a uint64)", want: "requires one -> separator"},
		{name: "multiple arrows", source: "//go:abiinternal -> ->\nfunc F()", want: "requires one -> separator"},
		{name: "bare", source: "//go:abiinternal\nfunc F()", want: "requires one -> separator"},
		{name: "unnamed input", source: "//go:abiinternal ->\nfunc F(uint64)", want: "requires named parameters and results"},
		{name: "unnamed result", source: "//go:abiinternal ->\nfunc F() uint64", want: "requires named parameters and results"},
		{name: "blank input", source: "//go:abiinternal _=AX ->\nfunc F(_ uint64)", want: "requires named parameters and results"},
		{name: "scratch", source: "//go:abiinternal a=R12 ->\nfunc F(a uint64)", want: `invalid go:abiinternal register "R12"`},
		{name: "reserved", source: "//go:abiinternal a=R14 ->\nfunc F(a uint64)", want: `invalid go:abiinternal register "R14"`},
		{name: "vector", source: "//go:abiinternal a=X0 ->\nfunc F(a float64)", want: `invalid go:abiinternal register "X0"`},
		{name: "float", source: "//go:abiinternal a=AX ->\nfunc F(a float64)", want: "requires a single integer register"},
		{name: "string", source: "//go:abiinternal a=AX ->\nfunc F(a string)", want: "requires a single integer register"},
		{name: "slice", source: "//go:abiinternal a=AX ->\nfunc F(a []byte)", want: "requires a single integer register"},
		{name: "interface", source: "//go:abiinternal a=AX ->\nfunc F(a any)", want: "requires a single integer register"},
		{name: "struct", source: "type T struct { A uint64 }\n//go:abiinternal a=AX ->\nfunc F(a T)", want: "requires a single integer register"},
		{name: "array", source: "//go:abiinternal a=AX ->\nfunc F(a [2]uint64)", want: "requires a single integer register"},
		{name: "stack input", source: "//go:abiinternal a=AX b=BX c=CX d=DX e=DI f=SI g=R8 h=R9 i=R10 j=R11 ->\nfunc F(a,b,c,d,e,f,g,h,i,j uint64)", want: `requires a single integer register for "j"`},
		{name: "stack result", source: "//go:abiinternal -> a=AX b=BX c=CX d=DX e=DI f=SI g=R8 h=R9 i=R10 j=R11\nfunc F() (a,b,c,d,e,f,g,h,i,j uint64)", want: `requires a single integer register for "j"`},
		{name: "body", source: "//go:abiinternal ->\nfunc F() {}", want: "requires a bodyless function"},
		{name: "method", source: "type T int\n//go:abiinternal ->\nfunc (t T) F()", want: "does not support methods"},
		{name: "blank function", source: "//go:abiinternal ->\nfunc _()", want: "requires a named function"},
		{name: "init function", source: "//go:abiinternal ->\nfunc init()", want: "func init must have a body"},
		{name: "misplaced", source: "//go:abiinternal ->\nvar F int", want: "misplaced go:abiinternal directive"},
		{name: "repeated", source: "//go:abiinternal ->\n//go:abiinternal ->\nfunc F()", want: "duplicate go:abiinternal directive"},
		{name: "missing assembly", source: "//go:abiinternal ->\nfunc F()", want: "requires an assembly definition", noAssembly: true},
	}
	unsupported := []string{"386", "arm", "mips", "mipsle", "mips64", "mips64le", "wasm"}
	for _, architecture := range unsupported {
		tests = append(tests, abiInternalDiagnostic{
			name: architecture, source: "//go:abiinternal ->\nfunc F()",
			want: "requires an architecture with integer register ABIInternal", arch: architecture,
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "test.go")
			err := os.WriteFile(source, []byte("package p\n"+test.source+"\n"), 0600)
			if err != nil {
				t.Fatal(err)
			}
			symabis := filepath.Join(dir, "symabis")
			defs := "def p.F ABI0\n"
			if test.noAssembly {
				defs = ""
			}
			err = os.WriteFile(symabis, []byte(defs), 0600)
			if err != nil {
				t.Fatal(err)
			}
			arch := test.arch
			if arch == "" {
				arch = "amd64"
			}
			command := testenv.Command(t, goTool, "tool", "compile", "-p", "p", "-symabis", symabis,
				"-abiinternal", filepath.Join(dir, "abiinternal"), "-o", filepath.Join(dir, "test.o"), source)
			command.Env = append(os.Environ(), "GOARCH="+arch)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) || strings.Contains(string(output), "internal compiler error") {
				t.Fatalf("compile: %v; want %q\n%s", err, test.want, output)
			}
		})
	}
}

func TestABIInternalAssemblyDiagnostics(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	goTool := testenv.GoToolPath(t)
	dir := t.TempDir()
	metadata := filepath.Join(dir, "abiinternal")
	mappings := map[string]objabi.ABIInternalFunc{
		"p.F": {Inputs: []objabi.ABIInternalValue{{Name: "a", Reg: "AX", Natural: x86.REG_AX}}},
	}
	data, err := json.Marshal(mappings)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(metadata, data, 0600)
	if err != nil {
		t.Fatal(err)
	}
	tests := []abiInternalAssemblyDiagnostic{
		{name: "splitting", source: "TEXT ·F(SB), $0-0\nRET", want: "ABIInternal requires NOSPLIT"},
		{name: "frame", source: "TEXT ·F(SB), 4, $8-0\nRET", want: "requires zero-frame NOSPLIT"},
		{name: "call", source: "TEXT ·F(SB), 4, $0-0\nCALL ·G(SB)\nRET", want: "requires leaf assembly"},
		{name: "tail return", source: "TEXT ·F(SB), 4, $0-0\nRET ·G(SB)", want: "requires leaf assembly"},
		{name: "tail jump", source: "TEXT ·F(SB), 4, $0-0\nJMP ·G(SB)", want: "branches must target local labels"},
		{name: "indirect jump", source: "TEXT ·F(SB), 4, $0-0\nJMP AX", want: "branches must target local labels"},
		{name: "push", source: "TEXT ·F(SB), 4, $0-0\nPUSHQ AX\nRET", want: "does not support stack operations"},
		{name: "stack operand", source: "TEXT ·F(SB), 4, $0-0\nMOVQ 8(SP), AX\nRET", want: "does not support stack operands"},
		{name: "unknown FP", source: "TEXT ·F(SB), 4, $0-0\nMOVQ bad+0(FP), AX\nRET", want: "invalid go:abiinternal FP operand"},
		{name: "wrong offset", source: "TEXT ·F(SB), 4, $0-0\nMOVQ a+8(FP), AX\nRET", want: "invalid go:abiinternal FP operand"},
		{name: "address of FP", source: "TEXT ·F(SB), 4, $0-0\nMOVQ $a+0(FP), AX\nRET", want: "invalid go:abiinternal FP operand"},
		{name: "ABI selector", source: "TEXT ·F<ABIInternal>(SB), 4, $0-0\nRET", want: "ABI selector only permitted when compiling runtime"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := filepath.Join(dir, "test.s")
			err := os.WriteFile(source, []byte(test.source+"\n"), 0600)
			if err != nil {
				t.Fatal(err)
			}
			command := testenv.Command(t, goTool, "tool", "asm", "-p", "p", "-abiinternal", metadata, "-o", filepath.Join(dir, "test.o"), source)
			command.Env = append(os.Environ(), "GOARCH=amd64")
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("asm: %v; want %q\n%s", err, test.want, output)
			}
		})
	}
}
