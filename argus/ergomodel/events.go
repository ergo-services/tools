package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

type EventRegistration struct {
	Call     *ast.CallExpr
	Pos      token.Pos
	Name     string
	Notify   FieldValue
	Buffer   FieldValue
	Open     FieldValue
	In       *Callback
	Behavior *types.Named
}

func (m *Model) Events() []*EventRegistration { return m.events }

type EventPublication struct {
	Call     *ast.CallExpr
	Pos      token.Pos
	Name     string
	Token    ast.Expr
	Behavior *types.Named
	In       *Callback
	Decl     *ast.FuncDecl
}

func (m *Model) EventPublications() []*EventPublication { return m.publications }

func (m *Model) Assigned(obj types.Object) bool { return m.assigned[obj] }

func (m *Model) buildAssigned() {
	m.assigned = map[types.Object]bool{}

	var note func(e ast.Expr)
	note = func(e ast.Expr) {
		var id *ast.Ident
		switch x := unwrap(e).(type) {
		case *ast.Ident:
			id = x
		case *ast.SelectorExpr:
			id = x.Sel
		case *ast.IndexExpr:

			note(x.X)
			return
		default:
			return
		}
		if obj := m.pass.TypesInfo.Uses[id]; obj != nil {
			m.assigned[obj] = true
		}
		if obj := m.pass.TypesInfo.Defs[id]; obj != nil {
			m.assigned[obj] = true
		}
	}

	for _, file := range m.pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range x.Lhs {
					note(lhs)
				}
			case *ast.IncDecStmt:
				note(x.X)
			case *ast.ValueSpec:
				if len(x.Values) > 0 {
					for _, name := range x.Names {
						note(name)
					}
				}
			case *ast.UnaryExpr:

				if x.Op == token.AND {
					note(x.X)
				}
			case *ast.CompositeLit:

				for _, elt := range x.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						note(kv.Key)
					}
				}
			}
			return true
		})
	}
}

func (m *Model) buildPublications(decls []*ast.FuncDecl) {
	byDecl := map[*ast.FuncDecl]*Callback{}
	for _, cb := range m.Callbacks {
		byDecl[cb.Decl] = cb
	}

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		cb := byDecl[decl]
		var behavior *types.Named
		if cb != nil {
			behavior = cb.Behavior
		} else if recv, ok := receiverType(m.pass.TypesInfo, decl); ok {
			behavior = recv
		}

		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			fn, isFunc := m.calleeFunc(call)
			if isFunc == false || fn.Name() != "SendEvent" || fn.Pkg() == nil {
				return true
			}
			if strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
				return true
			}
			if len(call.Args) < 2 {
				return true
			}
			pub := &EventPublication{
				Call: call, Pos: call.Pos(), Token: call.Args[1],
				Behavior: behavior, In: cb, Decl: decl,
			}
			if v := m.resolveValue(call.Args[0]); v.HasStr {
				pub.Name = v.Str
			}
			m.publications = append(m.publications, pub)
			return true
		})
	}
}

func (m *Model) Registered(t types.Type) bool {
	named, ok := deref(t).(*types.Named)
	if ok == false {
		return false
	}
	return m.registered[typePath(named)]
}

type RegisteredType struct {
	Named *types.Named
	Pos   token.Pos
}

func (m *Model) RegisteredTypes() []RegisteredType { return m.registeredNamed }

func (m *Model) DeclOf(fn *types.Func) *ast.FuncDecl {
	if fn == nil {
		return nil
	}
	return m.declOf[fn]
}

func (m *Model) RegisteredErrors() map[types.Object]bool { return m.registeredErrors }

func (m *Model) RegistersErrors() bool { return m.registersErrors }

func (m *Model) buildRegistrations(decls []*ast.FuncDecl) {
	m.registered = map[string]bool{}
	m.registeredErrors = map[types.Object]bool{}

	noteType := func(e ast.Expr) {
		t := m.pass.TypesInfo.TypeOf(e)
		if t == nil {
			return
		}
		named, ok := deref(t).(*types.Named)
		if ok == false {
			return
		}
		if m.registered[typePath(named)] == false {
			m.registeredNamed = append(m.registeredNamed,
				RegisteredType{Named: named, Pos: e.Pos()})
		}
		m.registered[typePath(named)] = true
	}
	noteErr := func(e ast.Expr) {
		m.registersErrors = true
		if obj := m.errorObject(e); obj != nil {
			m.registeredErrors[obj] = true
		}
	}

	noteErrList := func(e ast.Expr) {
		m.registersErrors = true
		lit, ok := unwrap(e).(*ast.CompositeLit)
		if ok == false {
			m.errorListOpaque = true
			return
		}
		for _, elt := range lit.Elts {
			noteErr(elt)
		}
	}

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			fn, ok := m.calleeFunc(call)
			if ok == false || fn.Pkg() == nil {
				return true
			}
			if strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
				return true
			}
			switch fn.Name() {
			case "RegisterType", "RegisterTypeOf":
				for _, arg := range call.Args {
					noteType(arg)
				}
			case "RegisterTypes", "RegisterTypesOf":
				for _, arg := range call.Args {
					lit, ok := arg.(*ast.CompositeLit)
					if ok == false {
						continue
					}
					for _, elt := range lit.Elts {
						noteType(elt)
					}
				}
			case "RegisterError":
				for _, arg := range call.Args {
					noteErr(arg)
				}
			case "RegisterErrors":
				for _, arg := range call.Args {
					noteErrList(arg)
				}
			}
			return true
		})
	}

	for _, file := range m.pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			kv, ok := n.(*ast.KeyValueExpr)
			if ok == false {
				return true
			}
			key, ok := kv.Key.(*ast.Ident)
			if ok == false {
				return true
			}
			switch key.Name {
			case "RegisterTypes", "Types":
				lit, isLit := kv.Value.(*ast.CompositeLit)
				if isLit == false {
					return true
				}
				for _, elt := range lit.Elts {
					noteType(elt)
				}
			case "RegisterErrors":
				noteErrList(kv.Value)
			}
			return true
		})
	}
}

func (m *Model) errorObject(e ast.Expr) types.Object {
	switch x := unwrap(e).(type) {
	case *ast.Ident:
		if obj := m.pass.TypesInfo.Uses[x]; obj != nil {
			return obj
		}
		return m.pass.TypesInfo.Defs[x]
	case *ast.SelectorExpr:
		return m.pass.TypesInfo.Uses[x.Sel]
	}
	return nil
}

func (m *Model) ErrorListOpaque() bool { return m.errorListOpaque }

func (m *Model) buildEvents(decls []*ast.FuncDecl) {
	byDecl := map[*ast.FuncDecl]*Callback{}
	for _, cb := range m.Callbacks {
		byDecl[cb.Decl] = cb
	}

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		cb := byDecl[decl]
		var behavior *types.Named
		if cb != nil {
			behavior = cb.Behavior
		} else if recv, ok := receiverType(m.pass.TypesInfo, decl); ok {
			behavior = recv
		}

		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			fn, ok := m.calleeFunc(call)
			if ok == false || fn.Name() != "RegisterEvent" {
				return true
			}
			if fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
				return true
			}

			reg := &EventRegistration{
				Call: call, Pos: call.Pos(), In: cb, Behavior: behavior,
			}
			if len(call.Args) > 0 {
				if v := m.resolveValue(call.Args[0]); v.HasStr {
					reg.Name = v.Str
				}
			}
			if len(call.Args) > 1 {
				opts := &SupervisorSpec{fields: map[string]FieldValue{}}
				m.foldLiteral(opts, call.Args[1], "")
				reg.Notify = opts.fields["Notify"]
				reg.Buffer = opts.fields["Buffer"]
				reg.Open = opts.fields["Open"]
			}
			m.events = append(m.events, reg)
			return true
		})
	}
}

func (m *Model) HandlesEventLifecycle(behavior *types.Named) bool {
	if behavior == nil {
		return false
	}
	for _, cb := range m.Callbacks {
		if cb.Behavior != behavior {
			continue
		}
		if referencesEventLifecycle(cb.Decl, m.pass.TypesInfo) {
			return true
		}
	}
	return false
}

func referencesEventLifecycle(decl *ast.FuncDecl, info *types.Info) bool {
	found := false
	ast.Inspect(decl, func(n ast.Node) bool {
		if found {
			return false
		}
		id, ok := n.(*ast.Ident)
		if ok == false {
			return true
		}
		switch id.Name {
		case "MessageEventStart", "MessageEventStop":
		default:
			return true
		}
		obj := info.Uses[id]
		if obj == nil || obj.Pkg() == nil {
			return true
		}
		if obj.Pkg().Path() == "ergo.services/ergo/gen" {
			found = true
		}
		return true
	})
	return found
}

type AppMember struct {
	Pos         token.Pos
	Name        FieldValue
	Factory     FieldValue
	InitTimeout FieldValue
	In          *ast.FuncDecl
}

func (m *Model) AppMembers() []*AppMember { return m.appMembers }

const appMemberTypePath = "ergo.services/ergo/gen.ApplicationMemberSpec"

func (m *Model) buildAppMembers(decls []*ast.FuncDecl) {
	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if ok == false {
				return true
			}
			t := m.pass.TypesInfo.TypeOf(lit)
			named, isNamed := deref(t).(*types.Named)
			if isNamed == false || typePath(named) != appMemberTypePath {
				return true
			}
			fields := &SupervisorSpec{fields: map[string]FieldValue{}}
			m.foldLiteral(fields, lit, "")
			m.appMembers = append(m.appMembers, &AppMember{
				Pos:         lit.Pos(),
				Name:        fields.fields["Name"],
				Factory:     fields.fields["Factory"],
				InitTimeout: fields.fields["Options.InitTimeout"],
				In:          decl,
			})
			return true
		})
	}
}

func (m *Model) DocOf(tn *types.TypeName) string {
	if doc, ok := m.docs[tn.Pos()]; ok {
		return doc
	}
	return ""
}

func (m *Model) buildDocs() {
	m.docs = map[token.Pos]string{}
	for _, file := range m.pass.Files {
		for _, d := range file.Decls {
			gd, ok := d.(*ast.GenDecl)
			if ok == false || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok == false {
					continue
				}

				doc := ts.Doc
				if doc == nil {
					doc = gd.Doc
				}
				if doc != nil {
					m.docs[ts.Name.Pos()] = doc.Text()
				}
			}
		}
	}
}

func (m *Model) MessageTypes() map[*types.TypeName]bool {
	out := map[*types.TypeName]bool{}
	scope := m.pass.Pkg.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if ok == false {
			continue
		}
		if marked, _ := m.MessageMarked(tn); marked {
			out[tn] = true
		}
	}
	for _, site := range m.SendSites {
		t := m.pass.TypesInfo.TypeOf(site.Payload)
		if t == nil {
			continue
		}
		named, ok := deref(t).(*types.Named)
		if ok == false || named.Obj() == nil || named.Obj().Pkg() != m.pass.Pkg {
			continue
		}
		out[named.Obj()] = true
	}
	return out
}

func (m *Model) RegisteredNames() map[string]bool { return m.registered }

type frameworkEvent struct {
	Buffer     int
	Reportable bool
	Why        string
}

var frameworkEvents = map[string]frameworkEvent{
	"core": {Buffer: 1000, Reportable: true, Why: "the node event bus, gen.CoreEvent"},
}

const inspectEventPrefix = "inspect_"

type EventSubscription struct {
	Call      *ast.CallExpr
	Method    string
	Pos       token.Pos
	Discarded bool
	Name      FieldValue
	Behavior  *types.Named
	In        *Callback
}

func (m *Model) EventSubscriptions() []*EventSubscription { return m.subscriptions }

func (m *Model) EventBuffer(name string) (size int, source string, reportable bool, ok bool) {
	if fe, found := frameworkEvents[name]; found {
		return fe.Buffer, fe.Why, fe.Reportable, true
	}
	if strings.HasPrefix(name, inspectEventPrefix) {
		return 1, "an inspect snapshot, re-published every period", false, true
	}

	best := -1
	for _, reg := range m.events {
		if reg.Name != name || reg.Buffer.HasInt == false {
			continue
		}
		if best < 0 || int(reg.Buffer.Int) < best {
			best = int(reg.Buffer.Int)
		}
	}
	if best > 0 {
		return best, "a producer in this package", true, true
	}
	return 0, "", false, false
}

func (m *Model) ConsumesEvents(named *types.Named) bool {
	if named == nil {
		return false
	}
	ms := types.NewMethodSet(types.NewPointer(named))
	for i := 0; i < ms.Len(); i++ {
		sel := ms.At(i)
		if sel.Obj().Name() != "HandleEvent" {
			continue
		}
		pkg := sel.Obj().Pkg()

		if pkg != nil && strings.HasPrefix(pkg.Path(), ergoPrefix) == false {
			return true
		}
	}
	return false
}

func (m *Model) buildSubscriptions(decls []*ast.FuncDecl) {
	byDecl := map[*ast.FuncDecl]*Callback{}
	for _, cb := range m.Callbacks {
		byDecl[cb.Decl] = cb
	}

	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		cb := byDecl[decl]
		var behavior *types.Named
		if cb != nil {
			behavior = cb.Behavior
		} else if recv, ok := receiverType(m.pass.TypesInfo, decl); ok {
			behavior = recv
		}
		singles := m.singleAssignedLiterals(decl.Body)

		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, discarded, ok := subscriptionStatement(n)
			if ok == false {
				return true
			}
			fn, isFunc := m.calleeFunc(call)
			if isFunc == false || fn.Pkg() == nil {
				return true
			}
			if strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
				return true
			}
			if fn.Name() != "LinkEvent" && fn.Name() != "MonitorEvent" {
				return true
			}
			sub := &EventSubscription{
				Call: call, Method: fn.Name(), Pos: call.Pos(),
				Discarded: discarded, Behavior: behavior, In: cb,
			}
			if len(call.Args) > 0 {
				sub.Name = m.resolveEventName(call.Args[0], singles)
			}
			m.subscriptions = append(m.subscriptions, sub)
			return true
		})
	}
}

func subscriptionStatement(n ast.Node) (*ast.CallExpr, bool, bool) {
	switch x := n.(type) {
	case *ast.ExprStmt:
		if call, ok := x.X.(*ast.CallExpr); ok {
			return call, true, true
		}
	case *ast.AssignStmt:
		if len(x.Rhs) != 1 || len(x.Lhs) != 2 {
			return nil, false, false
		}
		call, ok := x.Rhs[0].(*ast.CallExpr)
		if ok == false {
			return nil, false, false
		}
		id, isIdent := x.Lhs[0].(*ast.Ident)
		return call, isIdent && id.Name == "_", true
	}
	return nil, false, false
}

func (m *Model) resolveEventName(arg ast.Expr, singles map[string]ast.Expr) FieldValue {
	if lit, ok := arg.(*ast.CompositeLit); ok {
		return m.eventLiteralName(lit)
	}
	if id, ok := arg.(*ast.Ident); ok {
		if rhs, found := singles[id.Name]; found {
			if lit, isLit := rhs.(*ast.CompositeLit); isLit {
				return m.eventLiteralName(lit)
			}
		}
	}
	return FieldValue{}
}

func (m *Model) eventLiteralName(lit *ast.CompositeLit) FieldValue {
	named, ok := deref(m.pass.TypesInfo.TypeOf(lit)).(*types.Named)
	if ok == false || typePath(named) != "ergo.services/ergo/gen.Event" {
		return FieldValue{}
	}
	fields := &SupervisorSpec{fields: map[string]FieldValue{}}
	m.foldLiteral(fields, lit, "")
	return fields.fields["Name"]
}

func (m *Model) singleAssignedLiterals(body *ast.BlockStmt) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	count := map[string]int{}
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		if id, isIdent := as.Lhs[0].(*ast.Ident); isIdent {
			count[id.Name]++
			out[id.Name] = as.Rhs[0]
		}
		return true
	})
	for name, n := range count {
		if n > 1 {
			delete(out, name)
		}
	}
	return out
}
