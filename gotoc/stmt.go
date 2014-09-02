package gotoc

import (
	"bytes"
	"go/ast"
	"go/token"
	"strconv"

	"code.google.com/p/go.tools/go/types"
)

func (cdd *CDD) ReturnStmt(w *bytes.Buffer, s *ast.ReturnStmt, resultT string, tup *types.Tuple) (end bool) {
	switch len(s.Results) {
	case 0:
		if resultT == "void" {
			w.WriteString("return;\n")
		} else {
			w.WriteString("goto end;\n")
			end = true
		}

	case 1:
		w.WriteString("return ")
		var retTyp types.Type
		if tup.Len() != 1 {
			retTyp = tup
		} else {
			retTyp = tup.At(0).Type()
		}
		cdd.interfaceExpr(w, s.Results[0], retTyp)
		w.WriteString(";\n")

	default:
		w.WriteString("return (" + resultT + "){")
		for i, expr := range s.Results {
			if i > 0 {
				w.WriteString(", ")
			}
			cdd.interfaceExpr(w, expr, tup.At(i).Type())
		}
		w.WriteString("};\n")
	}
	return
}

func (cdd *CDD) label(w *bytes.Buffer, label, suffix string) {
	cdd.il--
	cdd.indent(w)
	w.WriteString(label)
	w.WriteString(suffix)
	w.WriteString(":;\n")
	cdd.il++
}

func (cdd *CDD) Stmt(w *bytes.Buffer, stmt ast.Stmt, label, resultT string, tup *types.Tuple) (end bool) {
	updateEnd := func(e bool) {
		if e {
			end = true
		}
	}

	cdd.Complexity++

	switch s := stmt.(type) {
	case *ast.DeclStmt:
		cdds := cdd.gtc.Decl(s.Decl, cdd.il)
		for _, c := range cdds {
			for u, typPtr := range c.FuncBodyUses {
				cdd.FuncBodyUses[u] = typPtr
			}
			w.Write(c.Decl)
		}
		for _, c := range cdds {
			w.Write(c.Def)
		}

	case *ast.AssignStmt:
		rhs := make([]string, len(s.Lhs))
		typ := make([]types.Type, len(s.Lhs))

		rhsIsTuple := len(s.Lhs) > 1 && len(s.Rhs) == 1

		if rhsIsTuple {
			tup := cdd.exprType(s.Rhs[0]).(*types.Tuple)
			tupName, _ := cdd.tupleName(tup)
			w.WriteString(tupName)
			tupName = "tmp" + cdd.gtc.uniqueId()
			w.WriteString(" " + tupName + " = ")
			cdd.Expr(w, s.Rhs[0], nil)
			w.WriteString(";\n")
			cdd.indent(w)
			for i, n := 0, tup.Len(); i < n; i++ {
				rhs[i] = tupName + "._" + strconv.Itoa(i)
				if s.Tok == token.DEFINE {
					if o := cdd.gtc.ti.Defs[s.Lhs[i].(*ast.Ident)]; o != nil {
						typ[i] = o.Type()
					} // else Lhs[i] was defined before.
				}
			}
		} else {
			for i, e := range s.Rhs {
				if s.Tok == token.DEFINE {
					var t types.Type
					if o := cdd.gtc.ti.Defs[s.Lhs[i].(*ast.Ident)]; o != nil {
						t = o.Type()
						typ[i] = t
					} else {
						// Lhs[i] was defined before.
						t = cdd.exprType(s.Lhs[i])
					}
					rhs[i] = cdd.ExprStr(e, t)
					continue
				}
				t := cdd.exprType(s.Lhs[i])
				rhs[i] = cdd.interfaceExprStr(e, t)
				typ[i] = t
			}
		}

		lhs := make([]string, len(s.Lhs))

		if s.Tok == token.DEFINE {
			for i, e := range s.Lhs {
				name := cdd.NameStr(cdd.object(e.(*ast.Ident)), true)
				if name == "_$" {
					lhs[i] = "_"
				} else if typ[i] != nil {
					t, dim := cdd.TypeStr(typ[i])
					lhs[i] = t + " " + dimFuncPtr(name, dim)
				} else {
					lhs[i] = name
				}
			}
		} else {
			for i, e := range s.Lhs {
				lhs[i] = cdd.ExprStr(e, nil)
			}
		}

		if len(s.Rhs) == len(s.Lhs) && len(s.Lhs) > 1 && s.Tok != token.DEFINE {
			for i, t := range typ {
				if i > 0 {
					cdd.indent(w)
				}
				if lhs[i] == "_" {
					w.WriteString("(void)(")
					w.WriteString(rhs[i])
					w.WriteString(");\n")
				} else {
					dim := cdd.Type(w, t)
					tmp := "tmp" + cdd.gtc.uniqueId()
					w.WriteString(" " + dimFuncPtr(tmp, dim))
					w.WriteString(" = " + rhs[i] + ";\n")
					rhs[i] = tmp
				}
			}
			cdd.indent(w)
		}

		var atok string
		switch s.Tok {
		case token.DEFINE:
			atok = " = "

		case token.AND_NOT_ASSIGN:
			atok = " &= "
			rhs[0] = "~(" + rhs[0] + ")"

		default:
			atok = " " + s.Tok.String() + " "
		}
		indent := false
		for i := 0; i < len(lhs); i++ {
			li := lhs[i]
			if li == "_" && rhsIsTuple {
				continue
			}
			if indent {
				cdd.indent(w)
			} else {
				indent = true
			}
			if li == "_" {
				w.WriteString("(void)(")
				w.WriteString(rhs[i])
				w.WriteString(");\n")
			} else {
				w.WriteString(li)
				w.WriteString(atok)
				w.WriteString(rhs[i])
				w.WriteString(";\n")
			}
		}

	case *ast.ExprStmt:
		cdd.Expr(w, s.X, nil)
		w.WriteString(";\n")

	case *ast.IfStmt:
		if s.Init != nil {
			w.WriteString("{\n")
			cdd.il++
			cdd.indent(w)
			updateEnd(cdd.Stmt(w, s.Init, "", resultT, tup))
			cdd.indent(w)
		}

		w.WriteString("if (")
		cdd.Expr(w, s.Cond, nil)
		w.WriteString(") ")
		updateEnd(cdd.BlockStmt(w, s.Body, resultT, tup))
		if s.Else == nil {
			w.WriteByte('\n')
		} else {
			w.WriteString(" else ")
			updateEnd(cdd.Stmt(w, s.Else, "", resultT, tup))
		}

		if s.Init != nil {
			cdd.il--
			cdd.indent(w)
			w.WriteString("}\n")
		}

	case *ast.IncDecStmt:
		w.WriteString(s.Tok.String())
		w.WriteByte('(')
		cdd.Expr(w, s.X, nil)
		w.WriteString(");\n")

	case *ast.BlockStmt:
		updateEnd(cdd.BlockStmt(w, s, resultT, tup))
		w.WriteByte('\n')

	case *ast.ForStmt:
		if s.Init != nil {
			w.WriteString("{\n")
			cdd.il++
			cdd.indent(w)
			updateEnd(cdd.Stmt(w, s.Init, "", resultT, tup))
			cdd.indent(w)
		}

		if label != "" && s.Post == nil {
			w.WriteString(label + "_continue: ")
		}

		w.WriteString("while (")
		if s.Cond != nil {
			cdd.Expr(w, s.Cond, nil)
		} else {
			w.WriteString("true")
		}
		w.WriteString(") ")

		if s.Post != nil {
			w.WriteString("{\n")
			cdd.il++
			cdd.indent(w)
		}
		updateEnd(cdd.BlockStmt(w, s.Body, resultT, tup))
		w.WriteByte('\n')

		if s.Post != nil {
			if label != "" {
				cdd.label(w, label, "_continue")
			}
			cdd.indent(w)
			updateEnd(cdd.Stmt(w, s.Post, "", resultT, tup))
			cdd.il--
			cdd.indent(w)
			w.WriteString("}\n")
		}

		if s.Init != nil {
			cdd.il--
			cdd.indent(w)
			w.WriteString("}\n")
		}

		if label != "" {
			cdd.label(w, label, "_break")
		}

	case *ast.RangeStmt:
		w.WriteString("{\n")
		cdd.il++
		xt := cdd.exprType(s.X)
		xs := "x"
		xl := ""

		array := false
		switch t := xt.(type) {
		case *types.Array:
			array = true
			xl = strconv.FormatInt(t.Len(), 10)

		case *types.Pointer:
			array = true
			xl = strconv.FormatInt(t.Elem().(*types.Array).Len(), 10)
		}

		if v, ok := s.Value.(*ast.Ident); ok && v.Name == "_" {
			s.Value = nil
		}

		switch e := s.X.(type) {
		case *ast.Ident:
			xs = cdd.NameStr(cdd.object(e), true)

		default:
			if s.Value != nil || !array {
				cdd.indent(w)
				cdd.varDecl(w, xt, false, xs, e)
			}
		}

		if !array {
			xl = "len(" + xs + ")"
		}

		switch xt.(type) {
		case *types.Slice, *types.Array, *types.Pointer:
			cdd.indent(w)

			ks := cdd.ExprStr(s.Key, nil)

			if s.Tok == token.DEFINE {
				w.WriteString("int ")
			}
			w.WriteString(ks + " = 0;\n")

			if label != "" {
				cdd.label(w, label, "_continue")
			}

			cdd.indent(w)
			w.WriteString("for (; " + ks + " < " + xl + "; ++" + ks + ") ")

			if s.Value != nil {
				w.WriteString("{\n")
				cdd.il++

				cdd.indent(w)
				if s.Tok == token.DEFINE {
					t := xt
					if pt, ok := xt.(*types.Pointer); ok {
						t = pt.Elem()
					}
					vt := t.(interface {
						Elem() types.Type
					}).Elem()
					dim := cdd.Type(w, vt)
					w.WriteByte(' ')
					w.WriteString(dimFuncPtr(cdd.ExprStr(s.Value, nil), dim))
				} else {
					cdd.Expr(w, s.Value, nil)
				}

				w.WriteString(" = ")
				cdd.indexExpr(w, xt, xs, s.Key)
				w.WriteString(";\n")

				cdd.indent(w)
			}

		default:
			cdd.notImplemented(s, xt)
		}

		updateEnd(cdd.BlockStmt(w, s.Body, resultT, tup))
		w.WriteByte('\n')

		if s.Value != nil {
			cdd.il--
			cdd.indent(w)
			w.WriteString("}\n")
		}

		cdd.il--
		cdd.indent(w)
		w.WriteString("}\n")

		if label != "" {
			cdd.label(w, label, "_break")
		}

	case *ast.ReturnStmt:
		updateEnd(cdd.ReturnStmt(w, s, resultT, tup))

	case *ast.SwitchStmt:
		w.WriteString("switch(0){case 0:{\n")
		cdd.il++

		if s.Init != nil {
			cdd.indent(w)
			updateEnd(cdd.Stmt(w, s.Init, "", resultT, tup))
		}

		cdd.indent(w)

		var typ types.Type
		if s.Tag != nil {
			typ = cdd.exprType(s.Tag)
			cdd.varDecl(w, typ, false, "tag", s.Tag)
		} else {
			typ = types.Typ[types.Bool]
			w.WriteString("bool tag = true;\n")
		}

		for _, stmt := range s.Body.List {
			cdd.indent(w)

			cs := stmt.(*ast.CaseClause)
			if cs.List != nil {
				w.WriteString("if (")
				for i, e := range cs.List {
					if i != 0 {
						w.WriteString(" || ")
					}
					eq(w, "tag", "==", cdd.ExprStr(e, typ), typ, typ)
				}
				w.WriteString(") ")
			}
			w.WriteString("{\n")
			cdd.il++

			brk := true
			if n := len(cs.Body) - 1; n >= 0 {
				bs, ok := cs.Body[n].(*ast.BranchStmt)
				if ok && bs.Tok == token.FALLTHROUGH {
					brk = false
					cs.Body = cs.Body[:n]
				}
			}
			for _, s := range cs.Body {
				cdd.indent(w)
				updateEnd(cdd.Stmt(w, s, "", resultT, tup))
			}
			if brk {
				cdd.indent(w)
				w.WriteString("break;\n")
			}

			cdd.il--
			cdd.indent(w)
			w.WriteString("}\n")
		}

		cdd.il--
		cdd.indent(w)
		w.WriteString("}}\n")

		if label != "" {
			cdd.label(w, label, "_break")
		}

	case *ast.BranchStmt:
		if s.Label == nil {
			w.WriteString(s.Tok.String())
		} else {
			w.WriteString("goto " + s.Label.Name + "$")
			switch s.Tok {
			case token.BREAK:
				w.WriteString("_break")
			case token.CONTINUE:
				w.WriteString("_continue")
			}
		}
		w.WriteString(";\n")

	case *ast.GoStmt:
		cdd.GoStmt(w, s)

	case *ast.SendStmt:
		et := cdd.exprType(s.Chan).(*types.Chan).Elem()
		w.WriteString("SEND(")
		cdd.Expr(w, s.Chan, nil)
		w.WriteString(", ")
		dim := cdd.Type(w, et)
		w.WriteString(dimFuncPtr("", dim))
		w.WriteString(", ")
		cdd.Expr(w, s.Value, et)
		w.WriteString(");\n")

	case *ast.SelectStmt:
		w.WriteString("switch(0){case 0:{\n")
		cdd.il++
		cdd.indent(w)
		w.WriteString("__label__ ")
		dflt := false
		for i, stmt := range s.Body.List {
			if i != 0 {
				w.WriteString(", ")
			}
			if stmt.(*ast.CommClause).Comm == nil {
				dflt = true
				w.WriteString("dflt")
			} else {
				w.WriteString("case" + strconv.Itoa(i))
			}
		}
		w.WriteString(";\n")

		for i, stmt := range s.Body.List {
			switch s := stmt.(*ast.CommClause).Comm.(type) {
			case nil:

			case *ast.SendStmt:
				cdd.indent(w)
				w.WriteString("SENDINIT(" + strconv.Itoa(i) + ", ")
				cdd.Expr(w, s.Chan, nil)
				w.WriteString(", ")
				et := cdd.exprType(s.Chan).(*types.Chan).Elem()
				dim := cdd.Type(w, et)
				dimFuncPtr("", dim)
				w.WriteString(", ")
				cdd.Expr(w, s.Value, et)
				w.WriteString(");\n")

			default:
				cdd.indent(w)
				w.WriteString("RECVINIT(" + strconv.Itoa(i) + ", ")
				var c ast.Expr
				switch r := s.(type) {
				case *ast.AssignStmt:
					c = r.Rhs[0].(*ast.UnaryExpr).X
				case *ast.ExprStmt:
					c = r.X.(*ast.UnaryExpr).X
				default:
					cdd.notImplemented(s)
				}
				cdd.Expr(w, c, nil)
				w.WriteString(", ")
				et := cdd.exprType(c).(*types.Chan).Elem()
				dim := cdd.Type(w, et)
				dimFuncPtr("", dim)
				w.WriteString(");\n")
			}
		}

		cdd.indent(w)
		n := len(s.Body.List)
		if dflt {
			w.WriteString("NBSELECT(\n")
			n--
		} else {
			w.WriteString("SELECT(\n")
		}

		cdd.il++
		for i, stmt := range s.Body.List {
			s := stmt.(*ast.CommClause).Comm
			switch s.(type) {
			case nil:
				continue

			case *ast.SendStmt:
				cdd.indent(w)
				w.WriteString("SENDCOMM(" + strconv.Itoa(i) + ")")

			default:
				cdd.indent(w)
				w.WriteString("RECVCOMM(" + strconv.Itoa(i) + ")")
			}
			if n--; n > 0 {
				w.WriteByte(',')
			}
			w.WriteByte('\n')
		}
		cdd.il--
		cdd.indent(w)
		w.WriteString(");\n")

		for i, stmt := range s.Body.List {
			cc := stmt.(*ast.CommClause)
			s := cc.Comm
			cdd.indent(w)
			switch s.(type) {
			case nil:
				w.WriteString("dflt")
			default:
				w.WriteString("case" + strconv.Itoa(i))
			}
			w.WriteString(":{\n")
			cdd.il++
			switch s := s.(type) {
			case nil:

			case *ast.SendStmt:
				cdd.indent(w)
				w.WriteString("SELSEND(" + strconv.Itoa(i) + ");\n")

			case *ast.AssignStmt:
				cdd.indent(w)
				name := cdd.ExprStr(s.Lhs[0], nil)
				if len(s.Lhs) == 1 {
					if name != "_$" {
						if s.Tok == token.DEFINE {
							dim := cdd.Type(w, cdd.exprType(s.Rhs[0]))
							w.WriteString(" " + dimFuncPtr(name, dim))
						} else {
							w.WriteString(name)
						}
						w.WriteString(" = ")
					}
					w.WriteString("SELRECV(" + strconv.Itoa(i) + ");\n")
				} else {
					ok := cdd.ExprStr(s.Lhs[1], nil)
					tmp := ""
					var tup *types.Tuple
					if name != "_$" || ok != "_$" {
						tup = cdd.exprType(s.Rhs[0]).(*types.Tuple)
						tupName, _ := cdd.tupleName(tup)
						w.WriteString(tupName + " ")
						tmp = "tmp" + cdd.gtc.uniqueId()
						w.WriteString(tmp + " = ")
					}
					w.WriteString("SELRECVOK(" + strconv.Itoa(i) + ");\n")
					if name != "_$" {
						cdd.indent(w)
						if s.Tok == token.DEFINE {
							dim := cdd.Type(w, tup.At(0).Type())
							w.WriteString(" " + dimFuncPtr(name, dim))
						} else {
							w.WriteString(name)
						}
						w.WriteString(" = " + tmp + "._0;\n")
					}
					if ok != "_$" {
						cdd.indent(w)
						if s.Tok == token.DEFINE {
							w.WriteString("bool ")
						}
						w.WriteString(ok + " = " + tmp + "._1;\n")
					}
				}

			case *ast.ExprStmt:
				cdd.indent(w)
				w.WriteString("SELRECV(" + strconv.Itoa(i) + ");\n")

			default:
				cdd.notImplemented(s)
			}
			for _, s = range cc.Body {
				cdd.indent(w)
				updateEnd(cdd.Stmt(w, s, "", resultT, tup))
			}
			cdd.indent(w)
			w.WriteString("break;\n")
			cdd.il--
			cdd.indent(w)
			w.WriteString("}\n")
		}
		cdd.il--
		cdd.indent(w)
		w.WriteString("}}\n")

	default:
		cdd.notImplemented(s)
	}
	return
}

func (cdd *CDD) GoStmt(w *bytes.Buffer, s *ast.GoStmt) {
	c := s.Call
	fs, ft, rs, rt := cdd.funStr(c.Fun, c.Args)

	type arg struct {
		l string
		r string
		t types.Type
	}

	var f arg
	if rt == nil {
		// Call of function or function variable.
		f = arg{"_f", fs, ft}
		if fident, ok := c.Fun.(*ast.Ident); ok {
			if _, ok = cdd.object(fident).(*types.Var); !ok {
				// Ordinary function call
				f = arg{fs, "", nil}
			}
		}
	} else {
		// Method call.
		f = arg{fs, "", nil}
	}

	n := len(c.Args)
	if f.t != nil {
		n++
	}
	if rt != nil {
		n++
	}

	if n == 0 {
		// Fast path for ordinary function without parameters.
		w.WriteString("GO(" + f.l + "());\n")
		return
	}

	argv := make([]arg, n)
	n = 0

	if f.t != nil {
		argv[n] = f
		n++
	}
	if rt != nil {
		argv[n] = arg{"_r", rs, rt}
		n++
	}
	tup := cdd.exprType(c.Fun).(*types.Signature).Params()
	for i, a := range c.Args {
		s := cdd.ExprStr(a, tup.At(i).Type())
		t := cdd.exprType(a)
		argv[n] = arg{"_" + strconv.Itoa(i), s, t}
		n++
	}

	w.WriteString("{\n")
	cdd.il++

	cdd.indent(w)
	w.WriteString("void wrap(")
	for i, arg := range argv {
		if i > 0 {
			w.WriteString(", ")
		}
		t, dim := cdd.TypeStr(arg.t)
		w.WriteString(t + " " + dimFuncPtr(arg.l, dim))
	}
	w.WriteString(") {\n")
	cdd.il++
	cdd.indent(w)
	w.WriteString("goready();\n")
	cdd.indent(w)
	w.WriteString(f.l + "(")
	args := argv
	if f.t != nil {
		args = argv[1:]
	}
	for i, arg := range args {
		if i > 0 {
			w.WriteString(", ")
		}
		w.WriteString(arg.l)
	}
	w.WriteString(");\n")
	cdd.il--
	cdd.indent(w)
	w.WriteString("}\n")

	for _, arg := range argv {
		t, dim := cdd.TypeStr(arg.t)
		cdd.indent(w)
		w.WriteString(t + " " + dimFuncPtr(arg.l, dim) + " = " + arg.r + ";\n")
	}

	cdd.indent(w)
	w.WriteString("GOWAIT(wrap(")
	for i, arg := range argv {
		if i > 0 {
			w.WriteString(", ")
		}
		w.WriteString(arg.l)
	}
	w.WriteString("));\n")

	cdd.il--
	cdd.indent(w)
	w.WriteString("}\n")

	return
}

func (cdd *CDD) BlockStmt(w *bytes.Buffer, bs *ast.BlockStmt, resultT string, tup *types.Tuple) (end bool) {
	updateEnd := func(e bool) {
		if e {
			end = true
		}
	}

	w.WriteString("{\n")
	cdd.il++
	for _, stmt := range bs.List {
		switch s := stmt.(type) {
		case *ast.LabeledStmt:
			label := s.Label.Name + "$"
			cdd.label(w, label, "")
			cdd.indent(w)
			updateEnd(cdd.Stmt(w, s.Stmt, label, resultT, tup))

		default:
			cdd.indent(w)
			updateEnd(cdd.Stmt(w, s, "", resultT, tup))
		}
	}
	cdd.il--
	cdd.indent(w)
	w.WriteString("}")
	return
}
