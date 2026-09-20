// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package asm

import (
	"cmd/internal/obj"
	"cmd/internal/objabi"
	"encoding/json"
	"internal/abi"
	"log"
	"os"
	"strings"
)

type abiInternalCopy struct {
	from int16
	to   int16
}

func (p *Parser) ReadABIInternal(file string) {
	data, err := os.ReadFile(file)
	if err == nil {
		err = json.Unmarshal(data, &p.abiInternal)
	}
	if err != nil {
		log.Fatalf("-abiinternal: %v", err)
	}
	architecture := objabi.ABIInternalArchitecture(p.arch.Name)
	if len(p.abiInternal) != 0 && architecture.Scratch == "" {
		log.Fatal("-abiinternal requires an architecture with integer register ABIInternal")
	}
}

func (p *Parser) applyABIInternal() {
	if len(p.abiInternal) == 0 {
		return
	}

	for text := p.firstProg; text != nil; text = text.Link {
		if text.As != obj.ATEXT {
			continue
		}
		mapping, ok := p.abiInternal[text.From.Sym.Name]
		if !ok {
			continue
		}
		p.mapABIInternal(text, mapping)
	}
}

func (p *Parser) mapABIInternal(text *obj.Prog, mapping objabi.ABIInternalFunc) {
	architecture := objabi.ABIInternalArchitecture(p.arch.Name)
	if !text.From.Sym.NoSplit() || text.To.Offset != 0 {
		p.errorf("go:abiinternal requires zero-frame NOSPLIT assembly")
		return
	}
	// The source's argument size describes ABI0. Runtime metadata must describe
	// the actual ABIInternal stack layout, including its input spill area.
	text.To.Val = int32(mapping.Args)

	values := make(map[string]objabi.ABIInternalValue, len(mapping.Inputs)+len(mapping.Results))
	for _, value := range mapping.Inputs {
		values[value.Name] = value
	}
	for _, value := range mapping.Results {
		values[value.Name] = value
	}
	scratch := p.arch.Register[architecture.Scratch]
	entry := p.abiInternalCopies(mapping.Inputs, false, scratch)
	exit := p.abiInternalCopies(mapping.Results, true, scratch)
	// Retain original Prog identities, including identity copies (as zero-width
	// NOPs), so branches to labels on rewritten instructions remain valid.
	local := make(map[*obj.Prog]bool)
	foundArgMap, foundArgInfo := false, false
	for prog := text.Link; prog != nil && prog.As != obj.ATEXT; prog = prog.Link {
		local[prog] = true
		if prog.As == obj.AFUNCDATA && prog.From.Type == obj.TYPE_CONST {
			foundArgMap = foundArgMap || prog.From.Offset == abi.FUNCDATA_ArgsPointerMaps
			foundArgInfo = foundArgInfo || prog.From.Offset == abi.FUNCDATA_ArgInfo
		}
	}
	for prog := text.Link; prog != nil && prog.As != obj.ATEXT; {
		next := prog.Link
		for instruction := range strings.FieldsSeq(architecture.LinkBranches) {
			if prog.As == p.arch.Instructions[instruction] {
				if architecture.NoLinkRegister != "" && prog.From.Type == obj.TYPE_REG && prog.From.Reg == p.arch.Register[architecture.NoLinkRegister] {
					continue // e.g. RISC-V JAL X0, label is an ordinary jump
				}
				p.errorf("go:abiinternal requires leaf assembly without tail calls")
				return
			}
		}
		if prog.As == obj.ACALL || prog.As == obj.ARET && prog.To.Type != obj.TYPE_NONE {
			p.errorf("go:abiinternal requires leaf assembly without tail calls")
			return
		}
		if p.arch.IsJump(prog.As.String()) && prog.As != obj.ARET {
			target, _ := prog.To.Val.(*obj.Prog)
			if !local[target] {
				p.errorf("go:abiinternal branches must target local labels")
				return
			}
		}
		// Keep the initial subset frameless, including implicit stack operations.
		op := prog.As.String()
		if strings.HasPrefix(op, "PUSH") || strings.HasPrefix(op, "POP") || op == "ADJSP" || op == "ENTER" || op == "LEAVE" {
			p.errorf("go:abiinternal does not support stack operations")
			return
		}
		if p.abiInternalStackRegister(prog.Reg, architecture) || p.abiInternalStackRegister(prog.RegTo2, architecture) {
			p.errorf("go:abiinternal does not support stack operands")
		}
		mapped := p.mapABIInternalOperand(&prog.From, values, architecture)
		mapped = p.mapABIInternalOperand(&prog.To, values, architecture) || mapped
		for index := range prog.RestArgs {
			mapped = p.mapABIInternalOperand(&prog.RestArgs[index].Addr, values, architecture) || mapped
		}
		if mapped && prog.From.Type == obj.TYPE_REG && prog.To.Type == obj.TYPE_REG && prog.From.Reg == prog.To.Reg {
			for instruction := range strings.FieldsSeq(architecture.IdentityMoves) {
				if prog.As == p.arch.Instructions[instruction] {
					prog.As = obj.ANOP
					prog.From = obj.Addr{}
					prog.To = obj.Addr{}
					break
				}
			}
		}
		if prog.As == obj.ARET && len(exit) != 0 {
			// Branches to RET must execute the return adaptation as well.
			ret := *prog
			last := prog
			for index, move := range exit {
				if index != 0 {
					last = obj.Appendp(last, p.ctxt.NewProg)
				}
				setABIInternalCopy(last, move, p.arch.Instructions[architecture.Move])
			}
			last.Link = &ret
		}
		prog = next
	}
	// Use the compiler's ordinary argument metadata, now computed for ABIInternal.
	last := text
	if !foundArgMap {
		last = p.abiInternalFuncData(last, abi.FUNCDATA_ArgsPointerMaps, text.From.Sym.Name+".args_stackmap")
	}
	if !foundArgInfo {
		last = p.abiInternalFuncData(last, abi.FUNCDATA_ArgInfo, text.From.Sym.Name+".arginfo1")
	}
	for _, move := range entry {
		last = obj.Appendp(last, p.ctxt.NewProg)
		setABIInternalCopy(last, move, p.arch.Instructions[architecture.Move])
	}
}

func (p *Parser) mapABIInternalOperand(addr *obj.Addr, values map[string]objabi.ABIInternalValue, architecture objabi.ABIInternalArch) bool {
	if addr.Name != obj.NAME_PARAM {
		if addr.Name == obj.NAME_AUTO || p.abiInternalStackRegister(addr.Reg, architecture) ||
			p.abiInternalStackRegister(addr.Index, architecture) {
			p.errorf("go:abiinternal does not support stack operands")
		}
		return false
	}
	if addr.Sym != nil {
		if value, ok := values[addr.Sym.Name]; ok && addr.Type == obj.TYPE_MEM && addr.Offset == value.Offset && addr.Index == 0 {
			*addr = obj.Addr{Type: obj.TYPE_REG, Reg: p.arch.Register[value.Reg]}
			return true
		}
	}
	p.errorf("invalid go:abiinternal FP operand")
	return false
}

func (p *Parser) abiInternalStackRegister(register int16, architecture objabi.ABIInternalArch) bool {
	if register == 0 {
		return false
	}
	for name := range strings.FieldsSeq(architecture.StackRegisters) {
		if register == p.arch.Register[name] {
			return true
		}
	}
	return false
}

func (p *Parser) abiInternalFuncData(after *obj.Prog, index int64, name string) *obj.Prog {
	prog := obj.Appendp(after, p.ctxt.NewProg)
	prog.As = obj.AFUNCDATA
	prog.From = obj.Addr{Type: obj.TYPE_CONST, Offset: index}
	prog.To = obj.Addr{Type: obj.TYPE_MEM, Name: obj.NAME_EXTERN, Sym: p.ctxt.Lookup(name)}
	return prog
}

// abiInternalCopies schedules a parallel copy using one permanent ABI scratch
// register. A destination is ready when no remaining copy reads it. If none is
// ready, save one source in scratch to break the cycle, then resume that same rule.
func (p *Parser) abiInternalCopies(values []objabi.ABIInternalValue, result bool, scratch int16) []abiInternalCopy {
	pending := make([]abiInternalCopy, 0, len(values))
	for _, value := range values {
		move := abiInternalCopy{from: value.Natural, to: p.arch.Register[value.Reg]}
		if result {
			move.from, move.to = move.to, move.from
		}
		if move.from != move.to {
			pending = append(pending, move)
		}
	}
	moves := make([]abiInternalCopy, 0, len(pending)+len(pending)/2)
	for len(pending) != 0 {
		ready := -1
		for index, move := range pending {
			live := false
			for _, other := range pending {
				if other.from == move.to {
					live = true
					break
				}
			}
			if !live {
				ready = index
				break
			}
		}
		if ready >= 0 {
			moves = append(moves, pending[ready])
			pending = append(pending[:ready], pending[ready+1:]...)
			continue
		}
		source := pending[0].from
		moves = append(moves, abiInternalCopy{from: source, to: scratch})
		for index := range pending {
			if pending[index].from == source {
				pending[index].from = scratch
			}
		}
	}
	return moves
}

func setABIInternalCopy(prog *obj.Prog, move abiInternalCopy, instruction obj.As) {
	prog.As = instruction
	prog.From = obj.Addr{Type: obj.TYPE_REG, Reg: move.from}
	prog.To = obj.Addr{Type: obj.TYPE_REG, Reg: move.to}
}
