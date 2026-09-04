package eventprotocol

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2007"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2007",
	Doc: `A2007: an event that can never be published, or a name registered twice.

RegisterEvent returns a token, and the token is the capability to publish: the
producer entry keeps it and PublishEvent rejects any other value with ErrEventOwner
unless the event was registered Open. So an event whose token was thrown away is
registered, visible, subscribable, and mute. Subscribers link successfully and then
wait forever, which is the part that makes this expensive to find by hand: nothing
fails at the producer except one error return that is usually discarded too.

Three literal forms, all decided inside the producer's own package because that is
where the name literal, the options and the publish site are visible together.

The token was discarded at registration. The result went to _ or the call was a
statement, and the same behavior publishes that name. There is no expression in the
program that could hold the right token.

The token cannot hold a token. The publish passes gen.Ref{}, or a variable this
package never writes: not assigned, not declared with a value, not address-taken. An
exported field is left alone, since another package can write it. This one needs the
event's own registration to be visible with Open unset, because Open turns the check
off and any Ref then works.

The same name is registered twice. LoadOrStore returns ErrTaken on the second call,
so one of the two registrations does nothing and, if the second one is the one whose
token got kept, publishing uses a token no entry holds. Reported only for two calls
in one statement list with no UnregisterEvent in that function, which is the form
where both definitely run in that order.

The fix for the first two is to keep the token, or to register with Open: true when
any local process is meant to publish. For the third it is to register once.

Source: events.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2007",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const refTypePath = "ergo.services/ergo/gen.Ref"

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	discardedTokens(pass, m)
	unusableTokens(pass, m)
	duplicateNames(pass, m)
	return nil, nil
}

func discardedTokens(pass *analysis.Pass, m *ergomodel.Model) {
	for _, reg := range m.Events() {
		if reg.Name == "" || reg.Behavior == nil {
			continue
		}
		if tokenKept(pass, reg.Call) {
			continue
		}

		if reg.Open.Set && reg.Open.Bool {
			continue
		}
		pub := publicationOf(m, reg.Behavior, reg.Name)
		if pub == nil {
			continue
		}
		m.Report(pass, reg.Pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindEvent, Tier: 2,
				ID: eventID(pass, reg.Behavior, reg.Name), Witness: "token discarded",
			},
			"this registration of %q discards the token RegisterEvent returns, and %s publishes that event, so every publish is rejected with gen.ErrEventOwner while subscribers link successfully and receive nothing; keep the token, or register with Open: true if any local process is meant to publish",
			reg.Name, publisherName(pub))
	}
}

func unusableTokens(pass *analysis.Pass, m *ergomodel.Model) {
	for _, pub := range m.EventPublications() {
		if pub.Name == "" {
			continue
		}

		reg := registrationOf(m, pub.Name)
		if reg == nil || (reg.Open.Set && reg.Open.Bool) {
			continue
		}
		witness, ok := zeroRef(pass, m, pub.Token)
		if ok == false {
			continue
		}

		if tokenKept(pass, reg.Call) == false {
			continue
		}
		m.Report(pass, pub.Pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindEvent, Tier: 2,
				ID: eventID(pass, pub.Behavior, pub.Name), Witness: witness,
			},
			"this publish of %q passes %s, and the event is not registered Open, so PublishEvent rejects it with gen.ErrEventOwner; pass the token RegisterEvent returned",
			pub.Name, witness)
	}
}

func duplicateNames(pass *analysis.Pass, m *ergomodel.Model) {
	byDecl := map[*ast.FuncDecl][]*ergomodel.EventRegistration{}
	for _, reg := range m.Events() {
		if reg.Name == "" || reg.In == nil {
			continue
		}
		byDecl[reg.In.Decl] = append(byDecl[reg.In.Decl], reg)
	}

	for decl, regs := range byDecl {
		if len(regs) < 2 || unregisters(pass, decl) {
			continue
		}
		for _, list := range statementLists(decl.Body) {
			seen := map[string]*ergomodel.EventRegistration{}
			for _, stmt := range list {

				if directCall(stmt) == false {
					continue
				}
				for _, reg := range regs {
					if contains(stmt, reg.Pos) == false {
						continue
					}
					first, dup := seen[reg.Name]
					if dup == false {
						seen[reg.Name] = reg
						continue
					}
					m.Report(pass, reg.Pos,
						ergomodel.Finding{
							Rule: ruleID, Kind: ergomodel.KindEvent, Tier: 2,
							ID: eventID(pass, reg.Behavior, reg.Name), Witness: "registered twice",
						},
						"%q is already registered at line %d of this same block, and a second registration of one name returns gen.ErrTaken, so this call does nothing and the token it would have returned is empty; register the event once",
						reg.Name, pass.Fset.Position(first.Pos).Line)
				}
			}
		}
	}
}

func tokenKept(pass *analysis.Pass, call *ast.CallExpr) bool {
	kept := false
	found := false
	for _, file := range pass.Files {
		if found {
			break
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if found {
				return false
			}
			switch x := n.(type) {
			case *ast.ExprStmt:
				if x.X == ast.Expr(call) {
					found, kept = true, false
					return false
				}
			case *ast.AssignStmt:
				if len(x.Rhs) != 1 || x.Rhs[0] != ast.Expr(call) {
					return true
				}
				found = true
				if len(x.Lhs) == 0 {
					return false
				}
				id, isIdent := x.Lhs[0].(*ast.Ident)
				kept = isIdent == false || id.Name != "_"
				return false
			}
			return true
		})
	}

	if found == false {
		return true
	}
	return kept
}

func zeroRef(pass *analysis.Pass, m *ergomodel.Model, e ast.Expr) (string, bool) {
	if isRefType(pass, e) == false {
		return "", false
	}
	switch x := e.(type) {
	case *ast.CompositeLit:
		if len(x.Elts) == 0 {
			return "gen.Ref{}", true
		}
	case *ast.Ident, *ast.SelectorExpr:
		obj := objectOf(pass, e)
		if obj == nil {
			return "", false
		}

		if obj.Exported() {
			return "", false
		}
		if m.Assigned(obj) {
			return "", false
		}
		return "the never assigned " + obj.Name(), true
	}
	return "", false
}

func isRefType(pass *analysis.Pass, e ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	if ok == false || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path()+"."+named.Obj().Name() == refTypePath
}

func objectOf(pass *analysis.Pass, e ast.Expr) types.Object {
	switch x := e.(type) {
	case *ast.Ident:
		if obj := pass.TypesInfo.Uses[x]; obj != nil {
			return obj
		}
		return pass.TypesInfo.Defs[x]
	case *ast.SelectorExpr:
		return pass.TypesInfo.Uses[x.Sel]
	}
	return nil
}

func publicationOf(m *ergomodel.Model, behavior *types.Named, name string) *ergomodel.EventPublication {
	for _, pub := range m.EventPublications() {
		if pub.Name == name && pub.Behavior == behavior {
			return pub
		}
	}
	return nil
}

func registrationOf(m *ergomodel.Model, name string) *ergomodel.EventRegistration {
	for _, reg := range m.Events() {
		if reg.Name == name {
			return reg
		}
	}
	return nil
}

func publisherName(pub *ergomodel.EventPublication) string {
	if pub.In != nil {
		return pub.In.Name
	}
	if pub.Decl != nil {
		return pub.Decl.Name.Name
	}
	return "this behavior"
}

func eventID(pass *analysis.Pass, behavior *types.Named, name string) string {
	if behavior != nil {
		return ergomodel.TypeID(behavior) + ":" + name
	}
	return pass.Pkg.Path() + ":" + name
}

func unregisters(pass *analysis.Pass, decl *ast.FuncDecl) bool {
	found := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if ok == false || sel.Sel.Name != "UnregisterEvent" {
			return true
		}
		found = true
		return false
	})
	return found
}

func statementLists(body *ast.BlockStmt) [][]ast.Stmt {
	var out [][]ast.Stmt
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BlockStmt:
			out = append(out, x.List)
		case *ast.CaseClause:
			out = append(out, x.Body)
		case *ast.CommClause:
			out = append(out, x.Body)
		}
		return true
	})
	return out
}

func directCall(stmt ast.Stmt) bool {
	switch stmt.(type) {
	case *ast.ExprStmt, *ast.AssignStmt:
		return true
	}
	return false
}

func contains(stmt ast.Stmt, pos token.Pos) bool {
	return pos >= stmt.Pos() && pos < stmt.End()
}
