// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import (
	"cmd/compile/internal/syntax"
	"cmd/compile/internal/types"
	"cmd/compile/internal/types2"
	"internal/pkgbits"
)

func (pw *pkgWriter) checkImmutableDecl(declaration *syntax.VarDecl) {
	pragma, _ := declaration.Pragma.(*pragmas)
	if pragma == nil || !pragma.ImmutablePos.IsKnown() {
		return
	}

	position := pragma.ImmutablePos
	pragma.ImmutablePos = syntax.Pos{}
	if len(declaration.NameList) != 1 || len(pragma.Embeds) != 0 {
		pw.errorf(position, "go:immutable requires a single-name variable declaration without go:embed")
		return
	}
	pw.recordImmutable(position, declaration.Pos(), declaration.NameList[0], declaration.Values)
}

func (pw *pkgWriter) recordImmutable(position, declaration syntax.Pos, name *syntax.Name, value syntax.Expr) {
	if position.Base() != declaration.Base() || position.Line()+1 != declaration.Line() || name.Value == "_" {
		pw.errorf(position, "go:immutable requires an immediately following single-name variable declaration")
		return
	}

	variable, ok := pw.info.Defs[name].(*types2.Var)
	if !ok {
		pw.errorf(position, "go:immutable requires a single-name variable declaration")
		return
	}

	if !pointerFree(variable.Type()) {
		slice, ok := types2.CoreType(variable.Type()).(*types2.Slice)
		if !ok || !pointerFree(slice.Elem()) {
			pw.errorf(position, "go:immutable requires a pointer-free value or a slice of pointer-free elements")
			return
		}
		if value != nil && !pw.freshImmutableSlice(value) {
			pw.errorf(position, "go:immutable slice requires a fresh slice literal or make expression")
			return
		}
	}

	if pw.immutable == nil {
		pw.immutable = make(map[*types2.Var]bool)
	}
	pw.immutable[variable] = true
}

func (pw *pkgWriter) freshImmutableSlice(expression syntax.Expr) bool {
	expression = syntax.Unparen(expression)
	if isNil(pw, expression) {
		return true
	}
	switch expression := expression.(type) {
	case *syntax.CompositeLit:
		return true
	case *syntax.CallExpr:
		return pw.isBuiltin(expression.Fun, "make")
	}
	return false
}

// Check typed source before serializing any bodies, including uninstantiated
// generics. No mutable aliases are admitted, so inlining and escape analysis
// need no separate ownership or alias propagation machinery.
func (pw *pkgWriter) checkImmutable(noders []*noder) {
	for variable, immutable := range pw.immutable {
		if _, linked := pw.linknames[variable]; immutable && linked {
			pw.errorf(variable.Pos(), "go:immutable variable cannot have a linkname")
		}
	}

	var ancestors []syntax.Node
	for _, noder := range noders {
		syntax.Inspect(noder.file, func(node syntax.Node) bool {
			if node == nil {
				ancestors = ancestors[:len(ancestors)-1]
				return false
			}

			switch node := node.(type) {
			case *syntax.AssignStmt:
				pw.checkImmutableAssignment(node.Lhs)
			case *syntax.RangeClause:
				pw.checkImmutableAssignment(node.Lhs)
			case *syntax.Operation:
				if node.Op == syntax.And && node.Y == nil {
					pw.rejectImmutable(node.X, "take address of")
				}
			case *syntax.SliceExpr:
				pw.rejectImmutable(node.X, "slice")
			case *syntax.SelectorExpr:
				selection := pw.info.Selections[node]
				if selection != nil && selection.Kind() == types2.MethodVal {
					signature := selection.Obj().Type().(*types2.Signature)
					if !pointerFree(signature.Recv().Type()) {
						pw.rejectImmutable(node.X, "call or capture a reference-receiver method on")
					}
				}
			}

			expression, ok := node.(syntax.Expr)
			if ok && len(ancestors) != 0 {
				parent := ancestors[len(ancestors)-1]
				// The Sel name is part of its qualified expression, not a use
				// of the imported variable on its own.
				selector, selected := parent.(*syntax.SelectorExpr)
				if !selected || selector.Sel != expression {
					variable := pw.immutableRoot(expression)
					if variable != nil {
						_, slice := types2.CoreType(pw.typeOf(expression)).(*types2.Slice)
						if slice && !pw.immutableSliceRead(expression, parent) {
							pw.errorf(expression.Pos(), "cannot expose mutable alias of go:immutable variable %s", variable.Name())
						}
					}
				}
			}

			ancestors = append(ancestors, node)
			return true
		})
	}
}

func (pw *pkgWriter) checkImmutableAssignment(expression syntax.Expr) {
	if expression == nil {
		return
	}
	for _, target := range syntax.UnpackListExpr(expression) {
		pw.rejectImmutable(target, "assign to")
	}
}

func (pw *pkgWriter) rejectImmutable(expression syntax.Expr, action string) {
	variable := pw.immutableRoot(expression)
	if variable != nil {
		pw.errorf(expression.Pos(), "cannot %s go:immutable variable %s", action, variable.Name())
	}
}

// immutableRoot follows only storage projections. Conversions and calls that
// copy pointer-free values deliberately end the path to the original storage.
func (pw *pkgWriter) immutableRoot(expression syntax.Expr) *types2.Var {
	expression = syntax.Unparen(expression)
	var object types2.Object
	switch expression := expression.(type) {
	case *syntax.Name:
		object = pw.info.Uses[expression]
	case *syntax.SelectorExpr:
		selection := pw.info.Selections[expression]
		if selection != nil {
			if selection.Kind() == types2.FieldVal {
				return pw.immutableRoot(expression.X)
			}
			return nil
		}
		object = pw.info.Uses[expression.Sel]
	case *syntax.IndexExpr:
		return pw.immutableRoot(expression.X)
	}

	variable, ok := object.(*types2.Var)
	if !ok {
		return nil
	}
	if immutable, known := pw.immutable[variable]; known {
		if immutable {
			return variable
		}
		return nil
	}
	if variable.Pkg() == nil || variable.Pkg() == pw.curpkg || variable.Parent() != variable.Pkg().Scope() {
		return nil
	}

	// Use the existing private Unified IR extension. The public Go type format
	// remains unchanged, and re-exports retain the defining package's contract.
	symbol := types.NewPkg(variable.Pkg().Path(), variable.Pkg().Name()).Lookup(variable.Name())
	location := objReader[symbol]
	reader := location.pr.newReader(pkgbits.SectionObjExt, location.idx, pkgbits.SyncObject1)
	reader.Sync(pkgbits.SyncVarExt)
	immutable := reader.Bool()
	if pw.immutable == nil {
		pw.immutable = make(map[*types2.Var]bool)
	}
	pw.immutable[variable] = immutable
	if immutable {
		return variable
	}
	return nil
}

func (pw *pkgWriter) immutableSliceRead(expression syntax.Expr, parent syntax.Node) bool {
	switch parent := parent.(type) {
	case *syntax.ParenExpr:
		return true // The enclosing expression is checked separately.
	case *syntax.IndexExpr:
		return parent.X == expression
	case *syntax.RangeClause:
		return parent.X == expression
	case *syntax.CallExpr:
		if len(parent.ArgList) == 1 && parent.ArgList[0] == expression {
			if pw.isBuiltin(parent.Fun, "len") || pw.isBuiltin(parent.Fun, "cap") {
				return true
			}
			return pw.typeAndValue(parent.Fun).IsType() && types2.Identical(types2.CoreType(pw.typeOf(parent)), types2.Typ[types2.String])
		}
		return pw.isBuiltin(parent.Fun, "copy") && len(parent.ArgList) == 2 && parent.ArgList[1] == expression
	case *syntax.Operation:
		if parent.Op == syntax.Eql || parent.Op == syntax.Neq {
			return isNil(pw, parent.X) || isNil(pw, parent.Y)
		}
	case *syntax.AssignStmt:
		name, blank := parent.Lhs.(*syntax.Name)
		return blank && name.Value == "_" && parent.Rhs == expression
	}
	return false
}
