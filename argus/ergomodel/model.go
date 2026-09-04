package ergomodel

import (
	"go/ast"
	"go/build"
	"go/token"
	"go/types"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

type Callback struct {
	Decl     *ast.FuncDecl
	Kind     CallbackKind
	Name     string
	Behavior *types.Named
	Recv     string
	Meta     bool

	Application bool
}

type SendSite struct {
	Call    *ast.CallExpr
	Method  string
	Payload ast.Expr
	In      *Callback
}

type Model struct {
	pass *analysis.Pass
	cfg  *Config
	fset *token.FileSet

	Callbacks []*Callback
	SendSites []*SendSite

	shapes       map[types.Type]Shape
	guards       map[types.Type]Guardedness
	inProgress   map[types.Type]bool
	usedBackEdge int

	senders map[string]int
	cbNames map[string]bool
	ifaces  map[string]*types.Interface

	funcs           map[*types.Func]FuncBehavior
	underCallback   map[*types.Func]bool
	declOf          map[*types.Func]*ast.FuncDecl
	goSites         []*GoSite
	blockSites      []*BlockSite
	spawnSites      []*SpawnSite
	specs           []*SupervisorSpec
	behaviorSpecs   []*SupervisorSpec
	events          []*EventRegistration
	registered      map[string]bool
	registeredNamed []RegisteredType

	registeredErrors map[types.Object]bool
	registersErrors  bool
	errorListOpaque  bool
	wireErrorSites   []*WireErrorSite
	appMembers       []*AppMember
	factories        map[types.Object]FactoryInfo
	spawnPoints      []*SpawnPoint
	docs             map[token.Pos]string
	subscriptions    []*EventSubscription
	publications     []*EventPublication
	assigned         map[types.Object]bool

	pkgAllowReason string
	pkgAllowed     bool

	baseline *baseline
	debt2    *debtCounter

	suppress map[int]map[string]bool
	debt     []DebtEntry
	frozen   bool
}

func newModel(pass *analysis.Pass, cfg *Config) *Model {
	m := &Model{
		pass:       pass,
		cfg:        cfg,
		fset:       pass.Fset,
		shapes:     make(map[types.Type]Shape),
		guards:     make(map[types.Type]Guardedness),
		inProgress: make(map[types.Type]bool),
		ifaces:     make(map[string]*types.Interface),
		declOf:     make(map[*types.Func]*ast.FuncDecl),
		suppress:   make(map[int]map[string]bool),
		debt2:      newDebtCounter(),
	}
	m.pkgAllowReason, m.pkgAllowed = cfg.PackageAllowed(pass.Pkg.Path())
	return m
}

func (m *Model) editable() bool {
	name := m.firstFile()
	if name == "" {
		return true
	}

	if strings.Contains(name, "/pkg/mod/") {
		return false
	}
	if goroot := build.Default.GOROOT; goroot != "" && strings.HasPrefix(name, goroot+string(os.PathSeparator)) {
		return false
	}
	return true
}

func (m *Model) stdlib() bool {
	path := m.pass.Pkg.Path()
	first := path
	if i := strings.IndexByte(path, '/'); i >= 0 {
		first = path[:i]
	}
	return strings.Contains(first, ".") == false
}

func (m *Model) firstFile() string {
	if len(m.pass.Files) == 0 {
		return ""
	}
	return m.fset.Position(m.pass.Files[0].Pos()).Filename
}

func (m *Model) Config() *Config { return m.cfg }

func (m *Model) PackageAllowed() (string, bool) { return m.pkgAllowReason, m.pkgAllowed }

func (m *Model) build(insp *inspector.Inspector) {
	info := m.pass.TypesInfo
	decls := m.funcDecls(insp)

	for _, fn := range decls {
		if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
			m.declOf[obj] = fn
		}
	}

	byDecl := make(map[*ast.FuncDecl]*Callback)
	names := m.callbackIndex()
	for _, fn := range decls {
		if names[fn.Name.Name] == false {
			continue
		}
		kind, ok := callbackKinds[fn.Name.Name]
		if ok == false {
			continue
		}
		recv, ok := receiverType(info, fn)
		if ok == false || m.isBehavior(recv) == false {
			continue
		}
		cb := &Callback{
			Decl:        fn,
			Kind:        kind,
			Name:        fn.Name.Name,
			Behavior:    recv,
			Recv:        receiverName(fn),
			Meta:        embedsMetaProcess(recv),
			Application: isApplicationBehavior(recv),
		}
		m.Callbacks = append(m.Callbacks, cb)
		byDecl[fn] = cb
	}

	for _, fn := range decls {
		enclosing := byDecl[fn]
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			if payload, method, ok := m.sendPayload(call); ok {
				m.SendSites = append(m.SendSites, &SendSite{
					Call: call, Method: method, Payload: payload, In: enclosing,
				})
			}
			if args, method, ok := m.spawnArgs(call); ok {
				m.spawnSites = append(m.spawnSites, &SpawnSite{
					Call: call, Method: method, Args: args, In: enclosing,
				})
			}
			return true
		})
	}

	m.collectSuppressions()
	m.buildBehavior(decls, byDecl)
	m.buildSpecs(decls)
	m.buildEvents(decls)
	m.buildRegistrations(decls)
	m.buildAppMembers(decls)
	m.buildFactories(decls)
	m.buildSpawnPoints(decls)
	m.buildDocs()
	m.buildSubscriptions(decls)
	m.buildPublications(decls)
	m.buildAssigned()
	m.buildWireErrorSites(decls)
}

func (m *Model) spawnArgs(call *ast.CallExpr) ([]ast.Expr, string, bool) {
	fn, ok := m.calleeFunc(call)
	if ok == false {
		return nil, "", false
	}
	if fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return nil, "", false
	}
	from, ok := spawnMethods[fn.Name()]
	if ok == false {
		return nil, "", false
	}
	if from >= len(call.Args) {
		return nil, fn.Name(), true
	}
	return call.Args[from:], fn.Name(), true
}

func (m *Model) funcDecls(insp *inspector.Inspector) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		out = append(out, n.(*ast.FuncDecl))
	})
	return out
}

func isApplicationBehavior(n *types.Named) bool {
	if n == nil {
		return false
	}
	ms := types.NewMethodSet(types.NewPointer(n))
	for i := 0; i < ms.Len(); i++ {
		if ms.At(i).Obj().Name() == "PreLoad" {
			return true
		}
	}
	return false
}

func embedsMetaProcess(n *types.Named) bool {
	st, ok := n.Underlying().(*types.Struct)
	if ok == false {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Anonymous() == false {
			continue
		}
		if named, ok := deref(f.Type()).(*types.Named); ok {
			if typePath(named) == "ergo.services/ergo/gen.MetaProcess" {
				return true
			}
		}
	}
	return false
}

func (m *Model) precompute() {

	scope := m.pass.Pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		tn, ok := obj.(*types.TypeName)
		if ok == false {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if ok == false {
			continue
		}
		if named.TypeParams() != nil && named.TypeParams().Len() > 0 {
			continue
		}
		s := m.shapeOf(named)
		m.guards[named] = m.Guard(named)
		m.publishShape(tn, named, s)
	}

	for _, site := range m.SendSites {
		t := m.pass.TypesInfo.TypeOf(site.Payload)
		if t == nil {
			continue
		}
		m.shapeOf(t)
		m.guards[types.Unalias(t)] = m.Guard(t)
	}

	for _, site := range m.spawnSites {
		for _, arg := range site.Args {
			t := m.pass.TypesInfo.TypeOf(arg)
			if t == nil {
				continue
			}
			m.shapeOf(t)
			m.guards[types.Unalias(t)] = m.Guard(t)
		}
	}
}

func (m *Model) publishShape(tn *types.TypeName, named *types.Named, s Shape) {
	marked, local, allowed := m.markers(tn)
	clone := methodSetHas(named, "CloneMessage")
	if marked == false && allowed == false && clone == false && local == false {
		return
	}
	m.pass.ExportObjectFact(tn, &ShapeFact{
		Aliasing: s.Aliasing,
		Opaque:   s.Opaque,
		WireOK:   s.WireOK,
		Message:  marked,
		Local:    local,
		Allowed:  allowed,
		Clone:    clone,
		Witness:  s.Witness,
	})
}

func (m *Model) markers(tn *types.TypeName) (message bool, local bool, allowed bool) {
	pos := m.fset.Position(tn.Pos())
	for line := pos.Line - 1; line >= pos.Line-3 && line > 0; line-- {
		ids, ok := m.suppress[line]
		if ok == false {
			continue
		}
		if ids["message"] {
			message = true
		}
		if ids["message local"] {
			message, local = true, true
		}

		for id := range ids {
			if id == "" || isRuleID(id) {
				allowed = true
			}
		}
	}
	return message, local, allowed
}

func (m *Model) MessageMarked(tn *types.TypeName) (message bool, local bool) {
	if tn.Pkg() == m.pass.Pkg {
		msg, loc, _ := m.markers(tn)
		return msg, loc
	}
	var f ShapeFact
	if m.pass.ImportObjectFact(tn, &f) {
		return f.Message, f.Local
	}
	return false, false
}

func (m *Model) Shape(t types.Type) Shape {
	if t == nil {
		return Shape{Aliasing: true, Opaque: true}
	}
	if s, ok := m.shapes[types.Unalias(t)]; ok {
		return s
	}
	if m.frozen {

		sub := &Model{pass: m.pass, cfg: m.cfg, fset: m.fset,
			shapes: map[types.Type]Shape{}, inProgress: map[types.Type]bool{}}
		return sub.shapeOf(t)
	}
	return m.shapeOf(t)
}

func (m *Model) Guarded(t types.Type) Guardedness {
	if g, ok := m.guards[types.Unalias(deref(t))]; ok {
		return g
	}
	if g, ok := m.guards[types.Unalias(t)]; ok {
		return g
	}
	return m.Guard(t)
}

func (m *Model) AllowedType(t types.Type) (string, bool) {
	if t == nil {
		return "", false
	}
	if named, ok := deref(t).(*types.Named); ok {
		if reason, ok := m.cfg.AllowTypes[typePath(named)]; ok {
			return reason, true
		}
	}
	reason, ok := m.cfg.AllowTypes[types.TypeString(types.Unalias(t), nil)]
	return reason, ok
}

func (m *Model) freeze() { m.frozen = true }

type DebtEntry struct {
	Pos     token.Pos
	Rules   []string
	Reason  string
	Blanket bool
}

func (m *Model) Debt() []DebtEntry { return m.debt }

func (m *Model) collectSuppressions() {
	for _, f := range m.pass.Files {
		for _, group := range f.Comments {
			for _, c := range group.List {
				text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
				line := m.fset.Position(c.Pos()).Line

				switch {
				case strings.HasPrefix(text, "argus:allow"):
					rest := strings.TrimSpace(strings.TrimPrefix(text, "argus:allow"))
					ids := leadingRuleIDs(rest)
					reason := rest
					if len(ids) > 0 {
						reason = strings.TrimSpace(strings.TrimPrefix(rest, strings.Fields(rest)[0]))
					}
					m.debt = append(m.debt, DebtEntry{Pos: c.Pos(), Rules: ids, Reason: reason})
					if len(ids) == 0 {
						m.addSuppress(line, "")
						continue
					}
					for _, id := range ids {
						m.addSuppress(line, id)
					}
				case strings.HasPrefix(text, "argus:ignore"):
					reason := strings.TrimSpace(strings.TrimPrefix(text, "argus:ignore"))
					m.debt = append(m.debt, DebtEntry{Pos: c.Pos(), Reason: reason, Blanket: true})
					m.addSuppress(line, "")
				case strings.HasPrefix(text, "argus:message"):
					rest := strings.TrimSpace(strings.TrimPrefix(text, "argus:message"))
					if rest == "local" {
						m.addSuppress(line, "message local")
					}
					m.addSuppress(line, "message")
				}
			}
		}
	}
}

func (m *Model) addSuppress(line int, id string) {
	if m.suppress[line] == nil {
		m.suppress[line] = make(map[string]bool)
	}
	m.suppress[line][id] = true
}

func leadingRuleIDs(rest string) []string {
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return nil
	}
	parts := strings.Split(fields[0], ",")
	for _, p := range parts {
		if isRuleID(p) == false {
			return nil
		}
	}
	return parts
}

func isRuleID(s string) bool {
	if len(s) < 5 || s[0] != 'A' {
		return false
	}
	for _, r := range s[1:5] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (m *Model) Suppressed(pos token.Pos, ruleID string) bool {
	line := m.fset.Position(pos).Line
	for _, l := range []int{line, line - 1} {
		ids, ok := m.suppress[l]
		if ok == false {
			continue
		}
		if ids[""] || ids[ruleID] {
			return true
		}
	}
	return false
}
