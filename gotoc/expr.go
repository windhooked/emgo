package gotoc

import (
	"bytes"
	"fmt"
	"go/ast"
	"strconv"

	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
)

func writeInt(w *bytes.Buffer, ev exact.Value, k types.BasicKind) {
	if k == types.Uintptr {
		u, _ := exact.Uint64Val(ev)
		w.WriteString("0x")
		w.WriteString(strconv.FormatUint(u, 16))
		return
	}
	s := ev.String()
	if s[0] == '-' {
		w.WriteByte('(')
	}
	switch k {
	case types.Int32:
		if s == "-2147483648" {
			w.WriteString("-2147483647L-1L")
		} else {
			w.WriteString(s + "L")
		}
	case types.Uint32:
		w.WriteString(s + "UL")
	case types.Int64:
		if s == "-9223372036854775808" {
			w.WriteString("-9223372036854775807LL-1LL")
		} else {
			w.WriteString(s + "LL")
		}
	case types.Uint64:
		w.WriteString(s + "ULL")
	default:
		w.WriteString(s)
	}
	if s[0] == '-' {
		w.WriteByte(')')
	}
}

func writeFloat(w *bytes.Buffer, ev exact.Value, k types.BasicKind) {
	n, _ := exact.Int64Val(exact.Num(ev))
	if n < 0 {
		w.WriteByte('(')
	}
	w.WriteString(strconv.FormatInt(n, 10))
	d, _ := exact.Int64Val(exact.Denom(ev))
	if d != 1 {
		w.WriteByte('/')
		w.WriteString(strconv.FormatInt(d, 10))
	}
	w.WriteByte('.')
	if k == types.Float32 {
		w.WriteByte('F')
	}
	if n < 0 {
		w.WriteByte(')')
	}
}

func (cdd *CDD) Value(w *bytes.Buffer, ev exact.Value, t types.Type) {
	k := t.Underlying().(*types.Basic).Kind()

	// TODO: use t instead ev.Kind() in following switch
	switch ev.Kind() {
	case exact.Int:
		writeInt(w, ev, k)

	case exact.Float:
		writeFloat(w, ev, k)

	case exact.Complex:
		switch k {
		case types.Complex64:
			k = types.Float32
		case types.Complex128:
			k = types.Float64
		default:
			k = types.UntypedFloat
		}
		writeFloat(w, exact.Real(ev), k)
		im := exact.Imag(ev)
		if exact.Sign(im) != -1 {
			w.WriteByte('+')
		}
		writeFloat(w, im, k)
		w.WriteByte('i')

	case exact.String:
		w.WriteString("EGSTR(")
		w.WriteString(ev.String())
		w.WriteByte(')')

	default:
		w.WriteString(ev.String())
	}
}

func (cdd *CDD) Name(w *bytes.Buffer, obj types.Object, direct bool) {
	if obj == nil {
		w.WriteByte('_')
		return
	}
	switch o := obj.(type) {
	case *types.PkgName:
		// Imported package name in SelectorExpr: pkgname.Name
		w.WriteString(upath(o.Pkg().Path()))
		return

	case *types.Func:
		s := o.Type().(*types.Signature)
		if r := s.Recv(); r != nil {
			t := r.Type()
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
				direct = false
			}
			cdd.Type(w, t)
			w.WriteByte('$')
			w.WriteString(o.Name())
			if !cdd.gtc.isLocal(t.(*types.Named).Obj()) {
				cdd.addObject(o, direct)
			}
			return
		}
	}

	if p := obj.Pkg(); p != nil && !cdd.gtc.isLocal(obj) {
		cdd.addObject(obj, direct)
		w.WriteString(upath(obj.Pkg().Path()))
		w.WriteByte('$')
	}
	name := obj.Name()
	switch name {
	case "init":
		w.WriteString(cdd.gtc.uniqueId() + name)

	default:
		w.WriteString(name)
		if cdd.gtc.isLocal(obj) {
			w.WriteByte('$')
		}
	}
}

func (cdd *CDD) NameStr(o types.Object, direct bool) string {
	buf := new(bytes.Buffer)
	cdd.Name(buf, o, direct)
	return buf.String()
}

func (cdd *CDD) SelectorExpr(w *bytes.Buffer, e *ast.SelectorExpr) (fun, recvt types.Type, recvs string) {
	sel := cdd.gtc.ti.Selections[e]
	if sel == nil {
		cdd.Name(w, cdd.object(e.Sel), true)
		return
	}
	s := cdd.ExprStr(e.X, nil)
	index := sel.Index()
	rt := sel.Recv()
	for _, id := range index[:len(index)-1] {
		if p, ok := rt.(*types.Pointer); ok {
			rt = p.Elem()
			s += "->"
		} else {
			s += "."
		}
		f := underlying(rt).(*types.Struct).Field(id)
		s += f.Name()
		rt = f.Type()
	}
	rpt, isPtr := rt.(*types.Pointer)
	switch sel.Kind() {
	case types.FieldVal:
		w.WriteString(s)
		if isPtr {
			w.WriteString("->")
		} else {
			w.WriteByte('.')
		}
		w.WriteString(e.Sel.Name)

	case types.MethodVal:
		fun = sel.Obj().Type()
		rtyp := fun.(*types.Signature).Recv().Type()

		switch rtyp.Underlying().(type) {
		case *types.Interface:
			// Method with interface receiver.
			w.WriteString(e.Sel.Name)
			recvs = s
			recvt = rt

		case *types.Pointer:
			// Method with pointer receiver.
			cdd.Name(w, sel.Obj(), true)
			if isPtr {
				recvs = s
				recvt = rt
			} else {
				recvs = "&" + s
				recvt = types.NewPointer(rt)
			}
		default:
			// Method with non-pointer receiver.
			cdd.Name(w, sel.Obj(), true)
			if isPtr {
				recvs = "*" + s
				recvt = rpt.Elem()
			} else {
				recvs = s
				recvt = rt
			}
		}

	case types.MethodExpr:
		cdd.Name(w, sel.Obj(), true)

	default:
		cdd.notImplemented(e)
	}
	return
}

func (cdd *CDD) SelectorExprStr(e *ast.SelectorExpr) (s string, fun, recvt types.Type, recvs string) {
	buf := new(bytes.Buffer)
	fun, recvt, recvs = cdd.SelectorExpr(buf, e)
	s = buf.String()
	return
}

func (cdd *CDD) builtin(b *types.Builtin, args []ast.Expr) (fun, recv string) {
	name := b.Name()

	switch name {
	case "len":
		switch t := underlying(cdd.exprType(args[0])).(type) {
		case *types.Slice, *types.Basic: // Basic == String
			return "len", ""

		case *types.Array:
			return "", strconv.FormatInt(t.Len(), 10)

		case *types.Chan:
			return "clen", ""

		default:
			cdd.notImplemented(ast.NewIdent("len"), t)
		}

	case "cap":
		switch t := underlying(cdd.exprType(args[0])).(type) {
		case *types.Slice:
			return "cap", ""

		case *types.Chan:
			return "ccap", ""

		default:
			cdd.notImplemented(ast.NewIdent("cap"), t)
		}

	case "copy":
		switch t := underlying(cdd.exprType(args[1])).(type) {
		case *types.Basic: // string
			return "STRCPY", ""

		case *types.Slice:
			typ, dim := cdd.TypeStr(t.Elem())
			return "SLICPY", typ + dimFuncPtr("", dim)

		default:
			panic(t)
		}

	case "new":
		typ, dim := cdd.TypeStr(cdd.exprType(args[0]))
		args[0] = nil
		return "NEW", typ + dimFuncPtr("", dim)

	case "make":
		a0t := cdd.exprType(args[0])
		args[0] = nil

		switch t := underlying(a0t).(type) {
		case *types.Slice:
			typ, dim := cdd.TypeStr(t.Elem())
			name := "MAKESLI"
			if len(args) == 3 {
				name = "MAKESLIC"
			}
			return name, typ + dimFuncPtr("", dim)

		case *types.Chan:
			typ, dim := cdd.TypeStr(t.Elem())
			typ += dimFuncPtr("", dim)
			if len(args) == 1 {
				typ += ", 0"
			}
			return "MAKECHAN", typ

		case *types.Map:
			typ, dim := cdd.TypeStr(t.Key())
			k := typ + dimFuncPtr("", dim)
			typ, dim = cdd.TypeStr(t.Elem())
			e := typ + dimFuncPtr("", dim)
			name := "MAKEMAP"
			if len(args) == 2 {
				name = "MAKEMAPC"
			}
			return name, k + ", " + e

		default:
			cdd.notImplemented(ast.NewIdent(name))
		}

	}

	return name, ""
}

func (cdd *CDD) funStr(fe ast.Expr, args []ast.Expr) (fs string, ft types.Type, rs string, rt types.Type) {
	switch f := fe.(type) {
	case *ast.SelectorExpr:
		buf := new(bytes.Buffer)
		ft, rt, rs = cdd.SelectorExpr(buf, f)
		fs = buf.String()
		return

	case *ast.Ident:
		switch o := cdd.object(f).(type) {
		case *types.Builtin:
			fs, rs = cdd.builtin(o, args)

		default:
			fs = cdd.NameStr(o, true)
			ft = o.Type()
		}
		return
	}
	fs = cdd.ExprStr(fe, nil)
	ft = cdd.exprType(fe)
	return
}

func (cdd *CDD) CallExpr(w *bytes.Buffer, e *ast.CallExpr) {
	switch t := cdd.exprType(e.Fun).(type) {
	case *types.Signature:
		fun, _, recvs, recvt := cdd.funStr(e.Fun, e.Args)

		var comma, reci bool
		if recvt != nil {
			_, reci = recvt.Underlying().(*types.Interface)
		}
		if reci {
			if _, ok := e.Fun.(*ast.SelectorExpr).X.(*ast.Ident); ok {
				reci = false
				w.WriteString(recvs + "." + fun + "(" + recvs + ".val$")
			} else {
				w.WriteString("({")
				dim := cdd.Type(w, recvt)
				w.WriteString(" " + dimFuncPtr("r", dim) + " = ")
				w.WriteString(recvs)
				w.WriteString("; r." + fun + "(r.val$")
			}
			comma = true
		} else {
			if fun == "" {
				w.WriteString(recvs)
			}
			w.WriteString(fun)

			w.WriteByte('(')
			if recvs != "" {
				w.WriteString(recvs)
				comma = true
			}
		}
		tup := t.Params()
		for i, a := range e.Args {
			if a == nil {
				// builtin can set type args to nil
				continue
			}
			if comma {
				w.WriteString(", ")
			} else {
				comma = true
			}
			var at types.Type
			// Builtin functions may not spefify type for all parameters.
			if i < tup.Len() {
				at = tup.At(i).Type()
			}
			cdd.interfaceExpr(w, a, at)
			i++
		}
		w.WriteByte(')')
		if reci {
			w.WriteString(";})")
		}

	default:
		arg := e.Args[0]
		switch typ := underlying(t).(type) {
		case *types.Slice:
			switch underlying(cdd.exprType(arg)).(type) {
			case *types.Basic: // string
				w.WriteString("NEWSTR(")
				cdd.Expr(w, arg, typ)
				w.WriteByte(')')

			default: // slice
				w.WriteByte('(')
				cdd.Expr(w, arg, typ)
				w.WriteByte(')')
			}

		case *types.Interface:
			cdd.interfaceExpr(w, arg, t)

		default:
			w.WriteString("((")
			dim := cdd.Type(w, t)
			w.WriteString(dimFuncPtr("", dim))
			w.WriteString(")(")
			cdd.Expr(w, arg, t)
			w.WriteString("))")
		}
	}
}

func (cdd *CDD) Expr(w *bytes.Buffer, expr ast.Expr, nilT types.Type) {
	cdd.Complexity++

	if t := cdd.gtc.ti.Types[expr]; t.Value != nil {
		// Constant expression
		cdd.Value(w, t.Value, t.Type)
		return
	}

	switch e := expr.(type) {
	case *ast.BinaryExpr:
		op := e.Op.String()
		ltyp := cdd.exprType(e.X)
		rtyp := cdd.exprType(e.Y)

		lhs := cdd.ExprStr(e.X, ltyp)
		rhs := cdd.ExprStr(e.Y, rtyp)

		if op == "==" || op == "!=" {
			eq(w, lhs, op, rhs, ltyp, rtyp)
			break
		}
		// BUG: strings
		if op == "&^" {
			op = "&~"
		}
		w.WriteString("(" + lhs + op + rhs + ")")

	case *ast.UnaryExpr:
		op := e.Op.String()

		if op == "<-" {
			t := cdd.exprType(e.X).(*types.Chan).Elem()
			if tup, ok := cdd.exprType(e).(*types.Tuple); ok {
				tn, _ := cdd.tupleName(tup)
				w.WriteString("RECVOK(" + tn + ", ")
				cdd.Expr(w, e.X, nil)
				w.WriteByte(')')
			} else {
				w.WriteString("RECV(")
				dim := cdd.Type(w, t)
				w.WriteString(dimFuncPtr("", dim))
				w.WriteString(", ")
				cdd.Expr(w, e.X, nil)
				w.WriteString(", ")
				zeroVal(w, t)
				w.WriteByte(')')
			}
			break
		}

		if op == "^" {
			op = "~"
		}
		w.WriteString(op)
		cdd.Expr(w, e.X, nil)

	case *ast.CallExpr:
		cdd.CallExpr(w, e)

	case *ast.Ident:
		if e.Name == "nil" {
			cdd.Nil(w, nilT)
		} else {
			cdd.Name(w, cdd.object(e), true)
		}

	case *ast.IndexExpr:
		cdd.indexExpr(w, cdd.exprType(e.X), cdd.ExprStr(e.X, nil), e.Index)

	case *ast.KeyValueExpr:
		kt := cdd.exprType(e.Key)
		if i, ok := e.Key.(*ast.Ident); ok && kt == nil {
			// e.Key is field name
			w.WriteByte('.')
			w.WriteString(i.Name)
		} else {
			w.WriteByte('[')
			cdd.Expr(w, e.Key, kt)
			w.WriteByte(']')
		}
		w.WriteString(" = ")
		cdd.Expr(w, e.Value, nilT)

	case *ast.ParenExpr:
		w.WriteByte('(')
		cdd.Expr(w, e.X, nilT)
		w.WriteByte(')')

	case *ast.SelectorExpr:
		s, fun, recvt, recvs := cdd.SelectorExprStr(e)
		if recvt == nil {
			w.WriteString(s)
			break
		}
		sig := fun.(*types.Signature)
		w.WriteString("({")
		res, params := cdd.signature(sig, false, numNames)
		w.WriteString(res.typ)
		w.WriteByte(' ')
		w.WriteString(dimFuncPtr("func"+params.String(), res.dim))
		w.WriteString(" { return " + s + "(" + recvs)
		if p := sig.Params(); p != nil {
			for i := 1; i <= p.Len(); i++ {
				w.WriteString(", _" + strconv.Itoa(i))
			}
		}
		w.WriteString("); } func;})")

	case *ast.SliceExpr:
		cdd.SliceExpr(w, e)

	case *ast.StarExpr:
		w.WriteByte('*')
		cdd.Expr(w, e.X, nil)

	case *ast.TypeAssertExpr:
		cdd.notImplemented(e)

	case *ast.CompositeLit:
		w.WriteByte('(')

		typ := cdd.exprType(e)

		switch t := underlying(typ).(type) {
		case *types.Array:
			w.WriteString("(")
			dim := cdd.Type(w, t.Elem())
			dim = append([]string{"[]"}, dim...)
			w.WriteString("(" + dimFuncPtr("", dim) + "))")
			w.WriteByte('{')
			nilT = t.Elem()
		case *types.Slice:
			w.WriteString("(slice){(")
			dim := cdd.Type(w, t.Elem())
			dim = append([]string{"[]"}, dim...)
			w.WriteString(dimFuncPtr("", dim))
			w.WriteString("){")
			nilT = t.Elem()
		case *types.Struct:
			w.WriteByte('(')
			cdd.Type(w, typ)
			w.WriteString("){")
			nilT = nil
		default:
			cdd.notImplemented(e, t)
		}

		for i, el := range e.Elts {
			if i > 0 {
				w.WriteString(", ")
			}
			if nilT != nil {
				cdd.Expr(w, el, nilT)
			} else {
				cdd.Expr(w, el, underlying(typ).(*types.Struct).Field(i).Type())
			}
		}

		switch underlying(typ).(type) {
		case *types.Slice:
			w.WriteByte('}')
			plen := ", " + strconv.Itoa(len(e.Elts))
			w.WriteString(plen)
			w.WriteString(plen)
			w.WriteByte('}')

		default:
			w.WriteByte('}')
		}

		w.WriteByte(')')

	case *ast.FuncLit:
		fname := "func"

		fd := &ast.FuncDecl{
			Name: &ast.Ident{NamePos: e.Type.Func, Name: fname},
			Type: e.Type,
			Body: e.Body,
		}
		sig := cdd.exprType(e).(*types.Signature)
		cdd.gtc.ti.Defs[fd.Name] = types.NewFunc(e.Type.Func, cdd.gtc.pkg, fname, sig)

		w.WriteString("({\n")
		cdd.il++

		cdds := cdd.gtc.FuncDecl(fd, cdd.il)
		for _, c := range cdds {
			for u, typPtr := range c.FuncBodyUses {
				cdd.FuncBodyUses[u] = typPtr
			}
			cdd.indent(w)
			w.Write(c.Def)
		}

		cdd.indent(w)
		w.WriteString(fname + "$;\n")

		cdd.il--
		cdd.indent(w)
		w.WriteString("})")

	default:
		fmt.Fprintf(w, "!%v<%T>!", e, e)
	}
	return
}

func (cdd *CDD) indexExpr(w *bytes.Buffer, typ types.Type, xs string, idx ast.Expr) {
	pt, isPtr := typ.(*types.Pointer)
	if isPtr {
		w.WriteString("(*")
		typ = pt.Elem()
	}

	var indT types.Type

	switch t := typ.Underlying().(type) {
	case *types.Basic: // string
		w.WriteString(xs + ".str")

	case *types.Slice:
		w.WriteString("((")
		dim := cdd.Type(w, t.Elem())
		dim = append([]string{"*"}, dim...)
		w.WriteString(dimFuncPtr("", dim))
		w.WriteByte(')')
		w.WriteString(xs + ".arr)")

	case *types.Array:
		w.WriteString(xs)

	case *types.Map:
		indT = t.Key()
		cdd.notImplemented(&ast.IndexExpr{}, t)

	default:
		panic(t)
	}

	if isPtr {
		w.WriteByte(')')
	}
	w.WriteByte('[')
	cdd.Expr(w, idx, indT)
	w.WriteByte(']')
}

func (cdd *CDD) SliceExpr(w *bytes.Buffer, e *ast.SliceExpr) {
	sx := cdd.ExprStr(e.X, nil)

	typ := cdd.exprType(e.X)
	pt, isPtr := typ.(*types.Pointer)
	if isPtr {
		typ = pt.Elem()
		sx = "(*" + sx + ")"
	}

	switch t := typ.(type) {
	case *types.Slice:
		if e.Low == nil && e.High == nil && e.Max == nil {
			w.WriteString(sx)
			break
		}

		if e.Low != nil {
			switch {
			case e.High == nil && e.Max == nil:
				w.WriteString("SLICEL(")

			case e.High != nil && e.Max == nil:
				w.WriteString("SLICELH(")

			case e.High == nil && e.Max != nil:
				w.WriteString("SLICEM(")

			default:
				w.WriteString("SLICELHM(")
			}
			w.WriteString(sx)
			w.WriteString(", ")
			dim := cdd.Type(w, t.Elem())
			dim = append([]string{"*"}, dim...)
			w.WriteString(dimFuncPtr("", dim))
			w.WriteString(", ")
			cdd.Expr(w, e.Low, nil)
		} else {
			switch {
			case e.High != nil && e.Max == nil:
				w.WriteString("SLICEH(")

			case e.High == nil && e.Max != nil:
				w.WriteString("SLICEM(")

			default:
				w.WriteString("SLICEHM(")
			}
			w.WriteString(sx)
		}

		if e.High != nil {
			w.WriteString(", ")
			cdd.Expr(w, e.High, nil)
		}
		if e.Max != nil {
			w.WriteString(", ")
			cdd.Expr(w, e.Max, nil)
		}

		w.WriteByte(')')

	case *types.Array:
		alen := strconv.FormatInt(t.Len(), 10) + ", "
		if e.Low != nil {
			switch {
			case e.High == nil && e.Max == nil:
				w.WriteString("ASLICEL(" + alen)

			case e.High != nil && e.Max == nil:
				w.WriteString("ASLICELH(" + alen)

			case e.High == nil && e.Max != nil:
				w.WriteString("ASLICEM(")

			default:
				w.WriteString("ASLICELHM(")
			}
			w.WriteString(sx)
			w.WriteString(", ")
			cdd.Expr(w, e.Low, nil)
		} else {
			switch {
			case e.High == nil && e.Max == nil:
				w.WriteString("ASLICE(" + alen)

			case e.High != nil && e.Max == nil:
				w.WriteString("ASLICEH(" + alen)

			case e.High == nil && e.Max != nil:
				w.WriteString("ASLICEM(")

			default:
				w.WriteString("ASLICEHM(")
			}
			w.WriteString(sx)
		}

		if e.High != nil {
			w.WriteString(", ")
			cdd.Expr(w, e.High, nil)
		}
		if e.Max != nil {
			w.WriteString(", ")
			cdd.Expr(w, e.Max, nil)
		}

		w.WriteByte(')')

	case *types.Basic: // string
		if e.Low == nil && e.High == nil {
			w.WriteString(sx)
			break
		}
		switch {
		case e.Low == nil:
			w.WriteString("SSLICEH(")

		case e.High == nil:
			w.WriteString("SSLICEL(")

		default:
			w.WriteString("SSLICELH(")
		}

		w.WriteString(sx)

		if e.Low != nil {
			w.WriteString(", ")
			cdd.Expr(w, e.Low, nil)
		}
		if e.High != nil {
			w.WriteString(", ")
			cdd.Expr(w, e.High, nil)
		}

		w.WriteByte(')')

	default:
		panic(e)
	}
}

func (cdd *CDD) ExprStr(expr ast.Expr, nilT types.Type) string {
	buf := new(bytes.Buffer)
	cdd.Expr(buf, expr, nilT)
	return buf.String()
}

func (cdd *CDD) Nil(w *bytes.Buffer, t types.Type) {
	switch underlying(t).(type) {
	case *types.Slice:
		w.WriteString("NILSLICE")

	case *types.Map:
		w.WriteString("NILMAP")

	case *types.Chan:
		w.WriteString("NILCHAN")

	case *types.Pointer, *types.Basic, *types.Signature:
		// Pointer or unsafe.Pointer
		w.WriteString("nil")

	case *types.Interface:
		w.WriteByte('(')
		cdd.Type(w, t)
		w.WriteString("){}")

	default:
		w.WriteString("'unknown nil")
	}
}

func eq(w *bytes.Buffer, lhs, op, rhs string, ltyp, rtyp types.Type) {
	typ := ltyp
	if typ == types.Typ[types.UntypedNil] {
		typ = rtyp
	}
	switch t := underlying(typ).(type) {
	case *types.Slice:
		nilv := "nil"
		sel := ".arr"
		if rtyp == types.Typ[types.UntypedNil] {
			lhs += sel
			rhs = nilv
		} else {
			lhs = nilv
			rhs += sel
		}
	case *types.Interface:
		nilv := "NILI"
		sel := ""
		if !t.Empty() {
			sel = ".interface"
		}
		if op == "!=" {
			w.WriteByte('!')
		}
		if rtyp == types.Typ[types.UntypedNil] {
			lhs += sel
			rhs = nilv
		} else {
			lhs = nilv
			rhs += sel
		}
		w.WriteString("EQUALI(" + lhs + ", " + rhs + ")")
		return
	case *types.Basic:
		if t.Kind() != types.String {
			break
		}
		if op == "!=" {
			w.WriteByte('!')
		}
		w.WriteString("equals(" + lhs + ", " + rhs + ")")
		return
	}
	w.WriteString("(" + lhs + " " + op + " " + rhs + ")")
}

func findMethod(t *types.Named, name string) *types.Func {
	for i := 0; i < t.NumMethods(); i++ {
		f := t.Method(i)
		if f.Name() == name {
			return f
		}
	}
	return nil
}

func (cdd *CDD) interfaceExpr(w *bytes.Buffer, expr ast.Expr, ityp types.Type) {
	etyp := cdd.exprType(expr)
	e := cdd.ExprStr(expr, ityp)
	if ityp == nil || etyp == nil {
		w.WriteString(e)
		return
	}
	if _, ok := ityp.Underlying().(*types.Interface); !ok || types.Identical(ityp, etyp) {
		w.WriteString(e)
		return
	}
	if b, ok := (etyp).(*types.Basic); ok && b.Kind() == types.UntypedNil {
		w.WriteString(e)
		return
	}

	_, eii := etyp.Underlying().(*types.Interface)
	if !eii && cdd.gtc.siz.Sizeof(etyp) > cdd.gtc.sizPtr {
		cdd.exit(
			expr.Pos(), "can't assign value of type %v to interface of type %v",
			etyp, ityp,
		)
	}

	ets, edim := cdd.TypeStr(etyp)
	tid := "0x" + strconv.FormatUint(cdd.gtc.typeHash(ets, edim), 16)
	it := ityp.Underlying().(*types.Interface)

	if eii {
		if it.Empty() {
			w.WriteString(e + ".interface")
		} else {
			w.WriteString("({\n")
			cdd.il++
			cdd.indent(w)
			w.WriteString(ets + " e = " + e + ";\n")
			cdd.indent(w)
			w.WriteByte('(')
			cdd.Type(w, ityp)
			w.WriteString("){\n")
			cdd.il++
			cdd.indent(w)
			w.WriteString(".interface = e.interface")
			for i := 0; i < it.NumMethods(); i++ {
				f := it.Method(i)
				w.WriteString(",\n")
				cdd.indent(w)
				fname := f.Name()
				w.WriteString("." + fname + " = e." + fname)
			}
			w.WriteByte('\n')
			cdd.il--
			cdd.indent(w)
			w.WriteString("}\n")
			cdd.il--
			cdd.indent(w)
			w.WriteString("})")
		}
	} else {
		if it.Empty() {
			w.WriteString("INTERFACE(" + e + ", " + tid + ")")
		} else {
			w.WriteByte('(')
			cdd.Type(w, ityp)
			w.WriteString("){\n")
			cdd.il++
			cdd.indent(w)
			w.WriteString(".interface = INTERFACE(" + e + ", " + tid + ")")
			for i := 0; i < it.NumMethods(); i++ {
				f := it.Method(i)
				w.WriteString(",\n")
				cdd.indent(w)
				fname := f.Name()
				w.WriteString("." + fname + " = ")
				if t, ok := etyp.(*types.Pointer); ok {
					etyp = t.Elem()
				}
				m := findMethod(etyp.(*types.Named), fname)
				recv := m.Type().(*types.Signature).Recv().Type()
				if cdd.gtc.siz.Sizeof(recv) != cdd.gtc.sizPtr {
					cdd.Name(w, m, true)
					w.WriteByte('$')
					continue
				}
				w.WriteByte('(')
				dim := cdd.Type(w, f.Type())
				w.WriteString(dimFuncPtr("", dim))
				w.WriteByte(')')
				cdd.Name(w, m, true)
			}
			w.WriteByte('\n')
			cdd.il--
			cdd.indent(w)
			w.WriteByte('}')
		}
	}
	return
}

func (cdd *CDD) interfaceExprStr(expr ast.Expr, ityp types.Type) string {
	buf := new(bytes.Buffer)
	cdd.interfaceExpr(buf, expr, ityp)
	return buf.String()
}
