// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objabi

import "strings"

// ABIInternalFunc is the compiler-to-assembler description of a go:abiinternal
// function. The JSON sidecar is package-local build data, not export data.
type ABIInternalFunc struct {
	Args    int64 // ABIInternal stack argument and spill area size
	Inputs  []ABIInternalValue
	Results []ABIInternalValue
}

type ABIInternalValue struct {
	Name    string
	Offset  int64  // ABI0 FP offset, for validating stock-compatible operands
	Reg     string // authored body's register name, resolved by the assembler
	Natural int16  // compiler-assigned ABIInternal register
}

// ABIInternalArch describes only the machine-specific parts of explicit scalar
// register mappings. Register lists use canonical assembler names, not aliases.
// Natural argument/result assignments still come from the compiler's ABI analysis.
type ABIInternalArch struct {
	Registers      string
	Scratch        string
	Move           string
	IdentityMoves  string
	StackRegisters string
	LinkBranches   string // instructions with an explicit or implicit link destination
	NoLinkRegister string // explicit link destination that discards the return address
}

func (arch ABIInternalArch) AllowsRegister(name string) bool {
	for register := range strings.FieldsSeq(arch.Registers) {
		if register == name {
			return true
		}
	}
	return false
}

func ABIInternalArchitecture(name string) ABIInternalArch {
	switch name {
	case "amd64":
		// Exclude SP, BP, g (R14), the dynlink temporary R15, and our scratch.
		return ABIInternalArch{
			Registers: "AX BX CX DX DI SI R8 R9 R10 R11 R13", Scratch: "R12", Move: "MOVQ",
			IdentityMoves: "MOVB MOVW MOVL MOVQ", StackRegisters: "SP BP SPB BPB",
		}
	case "arm64":
		// R18 is platform-reserved; R27 is the assembler temporary; R28 is g;
		// R29/R30/RSP are frame/link/stack registers. R31 is the zero register.
		return ABIInternalArch{
			Registers: "R0 R1 R2 R3 R4 R5 R6 R7 R8 R9 R10 R11 R12 R13 R14 R15 R17 R19 R20 R21 R22 R23 R24 R25 R26",
			Scratch:   "R16", Move: "MOVD", IdentityMoves: "MOVB MOVBU MOVH MOVHU MOVW MOVWU MOVD", StackRegisters: "RSP R29",
		}
	case "loong64":
		// Exclude zero/link/reserved/stack R0-R3, g (R22), the fixed closure
		// context R29, and assembler temporaries R30/R31. R20 is ABI scratch.
		return ABIInternalArch{
			Registers: "R4 R5 R6 R7 R8 R9 R10 R11 R12 R13 R14 R15 R16 R17 R18 R19 R21 R23 R24 R25 R26 R27 R28",
			Scratch:   "R20", Move: "MOVV", IdentityMoves: "MOVB MOVBU MOVH MOVHU MOVW MOVWU MOVV", StackRegisters: "R3",
			LinkBranches: "JIRL",
		}
	case "ppc64", "ppc64le":
		// Exclude zero/stack/TOC R0-R2, TLS R13, g (R30), and assembler
		// scratch R31. LR and other special registers are not mapping targets.
		return ABIInternalArch{
			Registers: "R3 R4 R5 R6 R7 R8 R9 R10 R11 R12 R14 R15 R16 R17 R18 R19 R20 R21 R22 R23 R24 R25 R26 R27 R28 R29",
			Scratch:   "R31", Move: "MOVD", IdentityMoves: "MOVB MOVBZ MOVH MOVHZ MOVW MOVWZ MOVD", StackRegisters: "R1",
			LinkBranches: "BCL",
		}
	case "riscv64":
		// Exclude zero/link/stack/global/TLS X0-X4, g (X27), and assembler
		// scratch X31. X8 is an ordinary ABI argument register, not a Go FP.
		return ABIInternalArch{
			Registers: "X5 X6 X7 X8 X9 X10 X11 X12 X13 X14 X15 X16 X17 X18 X19 X20 X21 X22 X23 X24 X25 X26 X28 X29 X30",
			Scratch:   "X31", Move: "MOV", IdentityMoves: "MOVB MOVBU MOVH MOVHU MOVW MOVWU MOV", StackRegisters: "X2",
			LinkBranches: "JAL JALR CJALR", NoLinkRegister: "X0",
		}
	case "s390x":
		// R0 is zero, R10/R11 assembler temporaries, R12 fixed closure context,
		// R13 g, R14 link, R15 stack. R1 is the ABI scratch register.
		return ABIInternalArch{
			Registers: "R2 R3 R4 R5 R6 R7 R8 R9", Scratch: "R1", Move: "MOVD",
			IdentityMoves: "MOVB MOVBZ MOVH MOVHZ MOVW MOVWZ MOVD", StackRegisters: "R15",
			LinkBranches: "BCL",
		}
	}

	return ABIInternalArch{}
}
