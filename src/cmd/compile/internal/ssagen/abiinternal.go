// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssagen

import (
	"cmd/compile/internal/abi"
	"cmd/compile/internal/base"
	"cmd/compile/internal/ir"
	"cmd/compile/internal/ssa"
	"cmd/compile/internal/typecheck"
	"cmd/compile/internal/types"
	"cmd/internal/obj"
	"cmd/internal/objabi"
	"encoding/json"
	"internal/buildcfg"
	"os"
	"strings"
)

var abiInternalFuncs = make(map[*ir.Func]bool)

// ApplyABIInternal overrides only annotated assembly definitions, before the
// normal wrapper pass. ABI0 references still get the normal reverse wrapper.
func (s *SymABIs) ApplyABIInternal(directives map[string]string) {
	if len(directives) == 0 && base.Flag.ABIInternal == "" {
		return
	}

	mappings := make(map[string]objabi.ABIInternalFunc, len(directives))
	for _, fn := range typecheck.Target.Funcs {
		directive, ok := directives[fn.Sym().Name]
		if !ok {
			continue
		}
		architecture := objabi.ABIInternalArchitecture(buildcfg.GOARCH)
		if architecture.Scratch == "" || !buildcfg.Experiment.RegabiArgs {
			base.ErrorfAt(fn.Pos(), 0, "go:abiinternal requires an architecture with integer register ABIInternal")
			continue
		}
		if fn.Sym().Linkname != "" || fn.Pragma&ir.CgoUnsafeArgs != 0 {
			base.ErrorfAt(fn.Pos(), 0, "go:abiinternal cannot be combined with linkname or cgo_unsafe_args")
			continue
		}
		if !s.HasDef(fn.Sym()) {
			base.ErrorfAt(fn.Pos(), 0, "go:abiinternal requires an assembly definition")
			continue
		}
		parts := strings.Split(strings.TrimPrefix(directive, "go:abiinternal"), "->")
		if len(parts) != 2 {
			base.ErrorfAt(fn.Pos(), 0, "go:abiinternal requires one -> separator")
			continue
		}
		natural := ssaConfig.ABI1.ABIAnalyzeFuncType(fn.Type())
		stack := ssaConfig.ABI0.ABIAnalyzeFuncType(fn.Type())
		before := base.Errors()
		mapping := objabi.ABIInternalFunc{
			Args:    natural.ArgWidth(),
			Inputs:  abiInternalValues(fn, parts[0], fn.Type().Params(), natural.InParams(), stack.InParams()),
			Results: abiInternalValues(fn, parts[1], fn.Type().Results(), natural.OutParams(), stack.OutParams()),
		}
		if base.Errors() != before {
			continue
		}
		symName := fn.Sym().Pkg.Prefix + "." + fn.Sym().Name
		s.defs[symName] = obj.ABIInternal
		abiInternalFuncs[fn] = true
		mappings[symName] = mapping
	}
	base.ExitIfErrors()
	if base.Flag.ABIInternal == "" {
		if len(mappings) != 0 {
			base.Errorf("go:abiinternal requires -abiinternal output file")
			base.ExitIfErrors()
		}
		return
	}
	data, err := json.Marshal(mappings)
	if err == nil {
		err = os.WriteFile(base.Flag.ABIInternal, data, 0666)
	}
	if err != nil {
		base.Fatalf("writing go:abiinternal mappings: %v", err)
	}
}

func HasABIInternalMapping(fn *ir.Func) bool {
	return abiInternalFuncs[fn]
}

func abiInternalValues(fn *ir.Func, text string, fields []*types.Field, natural, stack []abi.ABIParamAssignment) []objabi.ABIInternalValue {
	architecture := objabi.ABIInternalArchitecture(buildcfg.GOARCH)
	assignments := make(map[string]string, len(fields))
	used := make(map[string]bool, len(fields))
	for assignment := range strings.FieldsSeq(text) {
		name, register, ok := strings.Cut(assignment, "=")
		if !ok || name == "" || register == "" || strings.Contains(register, "=") {
			base.ErrorfAt(fn.Pos(), 0, "malformed go:abiinternal assignment %q", assignment)
			continue
		}
		if !architecture.AllowsRegister(register) {
			base.ErrorfAt(fn.Pos(), 0, "invalid go:abiinternal register %q", register)
			continue
		}
		if _, exists := assignments[name]; exists {
			base.ErrorfAt(fn.Pos(), 0, "duplicate go:abiinternal name %q", name)
		}
		if used[register] {
			base.ErrorfAt(fn.Pos(), 0, "duplicate go:abiinternal register %q", register)
		}
		assignments[name] = register
		used[register] = true
	}
	values := make([]objabi.ABIInternalValue, 0, len(fields))
	for index, field := range fields {
		if field.Sym == nil || field.Sym.Name == "_" || strings.HasPrefix(field.Sym.Name, "~") {
			base.ErrorfAt(fn.Pos(), 0, "go:abiinternal requires named parameters and results")
			continue
		}
		name := field.Sym.Name
		reg, ok := assignments[name]
		if !ok {
			base.ErrorfAt(fn.Pos(), 0, "missing go:abiinternal mapping for %q", name)
			continue
		}
		delete(assignments, name)
		param := &natural[index]
		typ := field.Type
		if !(typ.IsInteger() || typ.IsBoolean() || typ.IsPtrShaped()) || len(param.Registers) != 1 {
			base.ErrorfAt(fn.Pos(), 0, "go:abiinternal requires a single integer register for %q", name)
			continue
		}
		values = append(values, objabi.ABIInternalValue{
			// Named FP operands exclude the architecture's fixed linkage area.
			Name: name, Offset: int64(stack[index].Offset()) - ssaConfig.ABI0.LocalsOffset(), Reg: reg,
			Natural: ssa.ObjRegForAbiReg(param.Registers[0], ssaConfig),
		})
	}
	for name := range assignments {
		base.ErrorfAt(fn.Pos(), 0, "unknown go:abiinternal name %q", name)
	}
	return values
}
