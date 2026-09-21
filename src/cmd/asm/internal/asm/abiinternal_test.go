// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package asm

import (
	"cmd/asm/internal/lex"
	"cmd/internal/obj"
	"cmd/internal/objabi"
	"fmt"
	"strings"
	"testing"
)

type abiInternalOperandTest struct {
	architecture string
	body         string
}

// Exercise overlapping copies, disjoint cycles, identities, and destinations
// outside the input set by choosing every ordered subset of these registers.
func TestABIInternalCopies(t *testing.T) {
	architectures := []string{"amd64", "arm64", "loong64", "ppc64", "ppc64le", "riscv64", "s390x"}
	for _, architecture := range architectures {
		t.Run(architecture, func(t *testing.T) { testABIInternalCopies(t, architecture) })
	}
}

func TestABIInternalMapping(t *testing.T) {
	architectures := []string{"amd64", "arm64", "loong64", "ppc64", "ppc64le", "riscv64", "s390x"}
	for _, architecture := range architectures {
		t.Run(architecture, func(t *testing.T) {
			parser := newParser(architecture)
			description := objabi.ABIInternalArchitecture(architecture)
			registers := strings.Fields(description.Registers)
			scratch := parser.arch.Register[description.Scratch]
			seen := map[int16]bool{scratch: true}
			for _, name := range registers {
				register := parser.arch.Register[name]
				if register == 0 || seen[register] {
					t.Fatalf("invalid or aliased register %s", name)
				}
				seen[register] = true
			}
			if scratch == 0 {
				t.Fatal("missing scratch register")
			}
			var source strings.Builder
			source.WriteString("TEXT ·F(SB), 4, $0-48\n")
			for instruction := range strings.FieldsSeq(description.IdentityMoves) {
				if parser.arch.Instructions[instruction] == 0 {
					t.Fatalf("missing identity instruction %s", instruction)
				}
				fmt.Fprintf(&source, "%s a+0(FP), %s\n", instruction, registers[1])
				fmt.Fprintf(&source, "%s %s, x+24(FP)\n", instruction, registers[1])
			}
			source.WriteString("JMP done\nRET\ndone:\nRET\n")
			inputs := make([]objabi.ABIInternalValue, 3)
			results := make([]objabi.ABIInternalValue, 3)
			for index := range inputs {
				inputs[index] = objabi.ABIInternalValue{
					Name: string(rune('a' + index)), Offset: int64(index * 8),
					Reg: registers[(index+1)%3], Natural: parser.arch.Register[registers[index]],
				}
				results[index] = inputs[index]
				results[index].Name = string(rune('x' + index))
				results[index].Offset += 24
			}
			parser.lex = lex.NewTokenizer("abiinternal.s", strings.NewReader(source.String()), nil)
			parser.abiInternal = map[string]objabi.ABIInternalFunc{"pkg.F": {Args: 24, Inputs: inputs, Results: results}}
			first, ok := parser.Parse()
			if !ok {
				t.Fatal("parse failed")
			}
			copies, returns, metadata, scratchUses := 0, 0, 0, 0
			for prog := first; prog != nil; prog = prog.Link {
				switch prog.As {
				case obj.AFUNCDATA:
					metadata++
				case obj.ARET:
					returns++
				case obj.AJMP:
					target, _ := prog.To.Val.(*obj.Prog)
					if target == nil || target.As != parser.arch.Instructions[description.Move] {
						t.Fatal("branch bypasses return adaptation")
					}
				case obj.ATEXT, obj.ANOP:
				default:
					if prog.As != parser.arch.Instructions[description.Move] || prog.From.Type != obj.TYPE_REG || prog.To.Type != obj.TYPE_REG {
						t.Fatalf("unexpected mapped instruction: %v", prog)
					}
					copies++
					if prog.From.Reg == scratch || prog.To.Reg == scratch {
						scratchUses++
					}
				}
			}
			if copies != 12 || returns != 2 || metadata != 2 || scratchUses != 6 || first.To.Val != int32(24) {
				t.Fatalf("copies=%d returns=%d metadata=%d scratchUses=%d args=%v", copies, returns, metadata, scratchUses, first.To.Val)
			}
		})
	}
}

func TestABIInternalStackOperands(t *testing.T) {
	tests := []abiInternalOperandTest{
		{"amd64", "MOVQ SP, AX"},
		{"amd64", "MOVQ 8(BP), AX"},
		{"arm64", "MOVD RSP, R0"},
		{"arm64", "MOVD (R29), R0"},
		{"arm64", "ADD $8, RSP, R0"},
		{"loong64", "ADDV $8, R3, R4"},
		{"ppc64", "ADD $8, R1, R3"},
		{"ppc64le", "MOVD (R1), R3"},
		{"riscv64", "ADD $8, X2, X10"},
		{"s390x", "MOVD (R15), R2"},
	}
	for _, test := range tests {
		t.Run(test.architecture+"/"+test.body, func(t *testing.T) {
			parser := newParser(test.architecture)
			var diagnostics strings.Builder
			parser.errorWriter = &diagnostics
			parser.lex = lex.NewTokenizer("abiinternal.s", strings.NewReader("TEXT ·F(SB), 4, $0-0\n"+test.body+"\nRET\n"), nil)
			parser.abiInternal = map[string]objabi.ABIInternalFunc{"pkg.F": {}}
			_, ok := parser.Parse()
			if ok || !strings.Contains(diagnostics.String(), "does not support stack operands") {
				t.Fatalf("parse=%v: %s", ok, diagnostics.String())
			}
		})
	}
}

func TestABIInternalLinkBranches(t *testing.T) {
	tests := []abiInternalOperandTest{
		{"arm64", "BL done"},
		{"loong64", "JAL done"},
		{"ppc64", "BCL $20, CR0LT, done"},
		{"ppc64le", "BCL $20, CR0LT, done"},
		{"riscv64", "JAL X1, done"},
		{"riscv64", "JALR X1, (X5)"},
		{"riscv64", "CJALR X5"},
		{"s390x", "BL done"},
		{"s390x", "BCL done"},
	}
	for _, test := range tests {
		t.Run(test.architecture+"/"+test.body, func(t *testing.T) {
			parser := newParser(test.architecture)
			var diagnostics strings.Builder
			parser.errorWriter = &diagnostics
			parser.lex = lex.NewTokenizer("abiinternal.s", strings.NewReader("TEXT ·F(SB), 4, $0-0\n"+test.body+"\ndone:\nRET\n"), nil)
			parser.abiInternal = map[string]objabi.ABIInternalFunc{"pkg.F": {}}
			_, ok := parser.Parse()
			if ok || !strings.Contains(diagnostics.String(), "requires leaf assembly") {
				t.Fatalf("parse=%v: %s", ok, diagnostics.String())
			}
		})
	}
	t.Run("riscv64/no link", func(t *testing.T) {
		parser := newParser("riscv64")
		parser.lex = lex.NewTokenizer("abiinternal.s", strings.NewReader("TEXT ·F(SB), 4, $0-0\nJAL X0, done\ndone:\nRET\n"), nil)
		parser.abiInternal = map[string]objabi.ABIInternalFunc{"pkg.F": {}}
		_, ok := parser.Parse()
		if !ok {
			t.Fatal("JAL with discarded link must be allowed to target local labels")
		}
	})
}

func testABIInternalCopies(t *testing.T, architecture string) {
	parser := newParser(architecture)
	description := objabi.ABIInternalArchitecture(architecture)
	registers := strings.Fields(description.Registers)[:5]
	if architecture == "amd64" {
		registers = []string{"AX", "BX", "CX", "DX", "R13"}
	}
	scratch := parser.arch.Register[description.Scratch]
	values := make([]objabi.ABIInternalValue, 4)
	for index := range values {
		values[index].Natural = parser.arch.Register[registers[index]]
	}
	var visit func(int)
	visit = func(index int) {
		if index != len(values) {
			for _, register := range registers {
				used := false
				for prior := range index {
					used = used || values[prior].Reg == register
				}
				if !used {
					values[index].Reg = register
					visit(index + 1)
				}
			}
			return
		}
		directions := []bool{false, true}
		for _, result := range directions {
			state := make(map[int16]int16, len(registers)+1)
			for _, register := range registers {
				number := parser.arch.Register[register]
				state[number] = number
			}
			moves := parser.abiInternalCopies(values, result, scratch)
			for _, move := range moves {
				if move.from == move.to {
					t.Fatalf("identity copy: %+v", move)
				}
				state[move.to] = state[move.from]
			}
			for _, value := range values {
				from, to := value.Natural, parser.arch.Register[value.Reg]
				if result {
					from, to = to, from
				}
				if state[to] != from {
					t.Fatalf("values=%+v result=%v moves=%+v: register %d has %d, want %d", values, result, moves, to, state[to], from)
				}
			}
		}
	}
	visit(0)
}
