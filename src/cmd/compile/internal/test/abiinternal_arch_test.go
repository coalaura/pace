// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"cmd/internal/objabi"
	"fmt"
	"internal/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type abiInternalArchitectureTest struct {
	name      string
	registers [3]string // first three natural ABIInternal registers, for cycle coverage
	forbidden string
}

var abiInternalArchitectures = []abiInternalArchitectureTest{
	{"amd64", [3]string{"AX", "BX", "CX"}, "SP BP R12 R14 R15 X0"},
	{"arm64", [3]string{"R0", "R1", "R2"}, "R16 R18 R18_PLATFORM R27 R28 g R29 R30 LR R31 ZR RSP F0 V0"},
	{"loong64", [3]string{"R4", "R5", "R6"}, "R0 R1 R2 R3 R20 R22 g R29 R30 R31 F0"},
	{"ppc64", [3]string{"R3", "R4", "R5"}, "R0 R1 R2 R13 R30 g R31 LR CTR F0"},
	{"ppc64le", [3]string{"R3", "R4", "R5"}, "R0 R1 R2 R13 R30 g R31 LR CTR F0"},
	{"riscv64", [3]string{"X10", "X11", "X12"}, "X0 X1 X2 X3 X4 X27 g X31 SP RA A0 T0 F0"},
	{"s390x", [3]string{"R2", "R3", "R4"}, "R0 R1 R10 R11 R12 R13 g R14 LR R15 F0 V0"},
}

func TestABIInternalArchitectureRegisters(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	goTool := testenv.GoToolPath(t)
	for _, architecture := range abiInternalArchitectures {
		t.Run(architecture.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "test.go")
			symabis := filepath.Join(dir, "symabis")
			metadata := filepath.Join(dir, "abiinternal")
			description := objabi.ABIInternalArchitecture(architecture.name)
			registers := description.Registers + " " + architecture.forbidden
			forbidden := " " + architecture.forbidden + " "
			for register := range strings.FieldsSeq(registers) {
				t.Run(register, func(t *testing.T) {
					text := fmt.Sprintf("package p\n//go:abiinternal a=%s -> r=%s\nfunc F(a uintptr) (r uintptr)\n", register, register)
					writeABIInternalTestFile(t, source, text)
					writeABIInternalTestFile(t, symabis, "def p.F ABI0\n")
					command := testenv.Command(t, goTool, "tool", "compile", "-p", "p", "-symabis", symabis,
						"-abiinternal", metadata, "-o", filepath.Join(dir, "test.o"), source)
					command.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+architecture.name)
					output, err := command.CombinedOutput()
					if !strings.Contains(forbidden, " "+register+" ") {
						if err != nil {
							t.Fatalf("compile: %v\n%s", err, output)
						}
					} else if err == nil || !strings.Contains(string(output), "invalid go:abiinternal register") {
						t.Fatalf("compile: %v; want invalid register diagnostic\n%s", err, output)
					}
				})
			}
		})
	}
}

func TestABIInternalArchitectureBuilds(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	goTool := testenv.GoToolPath(t)
	for _, architecture := range abiInternalArchitectures {
		t.Run(architecture.name, func(t *testing.T) {
			dir := t.TempDir()
			description := objabi.ABIInternalArchitecture(architecture.name)
			registers := architecture.registers
			writeABIInternalTestFile(t, filepath.Join(dir, "go.mod"), "module abiinternalarch\n\ngo 1.27\n")
			// Both entry and return adaptation need a three-register cycle. Two
			// returns exercise branch targets that must retain their adaptation.
			source := fmt.Sprintf(`package main
//go:abiinternal a=%[2]s b=%[3]s c=%[1]s -> x=%[2]s y=%[3]s z=%[1]s
func Cycle(a, b, c uint64) (x, y, z uint64)
//go:abiinternal p=%[1]s -> r=%[1]s
func Pointer(p *byte) (r *byte)
//go:abiinternal a=%[1]s -> r=%[1]s
func Narrow(a int8) (r int8)
//go:abiinternal a=%[1]s -> r=%[1]s
func Boolean(a bool) (r bool)
func main() {
 a, b, c := Cycle(1, 2, 3)
 var value byte
 if a != 1 || b != 2 || c != 3 || Pointer(&value) != &value || Narrow(-1) != -1 || !Boolean(true) { panic("mapping") }
}
`, registers[0], registers[1], registers[2])
			writeABIInternalTestFile(t, filepath.Join(dir, "main.go"), source)
			assembly := fmt.Sprintf(`#include "textflag.h"
TEXT ·Cycle(SB), NOSPLIT, $0-48
 %[4]s a+0(FP), %[2]s
 %[4]s b+8(FP), %[3]s
 %[4]s c+16(FP), %[1]s
 JMP done
 RET
done:
 %[4]s %[2]s, x+24(FP)
 %[4]s %[3]s, y+32(FP)
 %[4]s %[1]s, z+40(FP)
 RET
TEXT ·Pointer(SB), NOSPLIT, $0-16
 %[4]s p+0(FP), %[1]s
 %[4]s %[1]s, r+8(FP)
 RET
TEXT ·Narrow(SB), NOSPLIT, $0-9
 MOVB a+0(FP), %[1]s
 MOVB %[1]s, r+8(FP)
 RET
TEXT ·Boolean(SB), NOSPLIT, $0-9
 MOVB a+0(FP), %[1]s
 MOVB %[1]s, r+8(FP)
 RET
`, registers[0], registers[1], registers[2], description.Move)
			writeABIInternalTestFile(t, filepath.Join(dir, "main.s"), assembly)
			command := testenv.Command(t, goTool, "build", "-o", filepath.Join(dir, "main"), ".")
			command.Dir = dir
			command.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+architecture.name, "CGO_ENABLED=0", "GOWORK=off")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("cross-build: %v\n%s", err, output)
			}
			command = testenv.Command(t, goTool, "tool", "objdump", "-s", `main\.(Cycle|Pointer|Narrow|Boolean)`, filepath.Join(dir, "main"))
			output, err = command.CombinedOutput()
			if err != nil {
				t.Fatalf("objdump: %v\n%s", err, output)
			}
			checkABIInternalDisassembly(t, architecture.name, string(output))
		})
	}
}

func checkABIInternalDisassembly(t *testing.T, architecture, disassembly string) {
	t.Helper()
	description := objabi.ABIInternalArchitecture(architecture)
	copyInstruction := description.Move
	switch architecture {
	case "ppc64", "ppc64le":
		copyInstruction = "OR"
	case "riscv64":
		copyInstruction = "ADD"
	}
	names := []string{"Cycle", "Pointer", "Narrow", "Boolean"}
	for _, name := range names {
		if !strings.Contains(disassembly, "TEXT main."+name+"(SB)") {
			t.Fatalf("missing ABIInternal definition %s:\n%s", name, disassembly)
		}
	}
	if strings.Contains(disassembly, ".abi0") {
		t.Fatalf("unexpected ABI0 definition:\n%s", disassembly)
	}
	cycle := false
	copies, returns, jumps, scratchUses := 0, 0, 0, 0
	for line := range strings.SplitSeq(disassembly, "\n") {
		if strings.HasPrefix(line, "TEXT ") {
			cycle = strings.HasPrefix(line, "TEXT main.Cycle(")
			continue
		}
		fieldIndex := 0
		for field := range strings.FieldsSeq(line) {
			fieldIndex++
			if fieldIndex != 4 {
				continue
			}
			switch field {
			case copyInstruction:
				if !cycle {
					t.Fatalf("identity copy survived:\n%s", disassembly)
				}
				copies++
				if strings.Contains(line, description.Scratch) {
					scratchUses++
				}
			case "RET":
				returns++
			case "JMP", "BR":
				jumps++
			case "?": // zero alignment padding on arm64 and s390x
			default:
				t.Fatalf("unexpected instruction %s:\n%s", field, disassembly)
			}
			break
		}
	}
	if copies != 12 || returns != 5 || jumps != 1 || scratchUses != 6 {
		t.Fatalf("unexpected adaptation: copies=%d returns=%d jumps=%d scratchUses=%d\n%s", copies, returns, jumps, scratchUses, disassembly)
	}
}

func writeABIInternalTestFile(t *testing.T, path, contents string) {
	t.Helper()
	err := os.WriteFile(path, []byte(contents), 0600)
	if err != nil {
		t.Fatal(err)
	}
}
