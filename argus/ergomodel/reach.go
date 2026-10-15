package ergomodel

import (
	"go/ast"
	"go/token"
)

func ancestors(root ast.Node, pos token.Pos) []ast.Node {
	var chain []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil || pos < n.Pos() || pos >= n.End() {
			return false
		}
		chain = append(chain, n)
		return true
	})
	return chain
}

func stmtList(n ast.Node) []ast.Stmt {
	switch x := n.(type) {
	case *ast.BlockStmt:
		return x.List
	case *ast.CaseClause:
		return x.Body
	case *ast.CommClause:
		return x.Body
	}
	return nil
}

func hasLabels(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if _, isLabel := n.(*ast.LabeledStmt); isLabel {
			found = true
		}
		return found == false
	})
	return found
}

func insideClosure(chain []ast.Node) bool {
	for _, n := range chain {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return true
		}
	}
	return false
}

func dominates(body *ast.BlockStmt, stmt ast.Stmt, pos token.Pos) bool {
	if stmt == nil {
		return false
	}
	chain := ancestors(body, pos)
	for i, n := range chain {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		if i+1 >= len(chain) {
			break
		}
		for _, s := range stmtList(n) {
			if s == chain[i+1] {
				break
			}
			if s == stmt {
				return true
			}
		}
	}
	return false
}

func unreachable(body *ast.BlockStmt, at token.Pos, store token.Pos) bool {
	from := ancestors(body, at)
	to := ancestors(body, store)
	common := 0
	for common < len(from) && common < len(to) && from[common] == to[common] {
		common++
	}
	if common == 0 || common == len(from) || common == len(to) {
		return false
	}
	for _, n := range from[:common] {
		switch n.(type) {
		case *ast.ForStmt, *ast.RangeStmt, *ast.FuncLit:
			return false
		}
	}
	if insideClosure(from[common:]) || insideClosure(to[common:]) {
		return false
	}
	if exclusive(from[common-1], from[common], to[common]) {
		return true
	}
	if store < at {
		return false
	}
	return returnsBefore(from, common-1, at, to[common])
}

func exclusive(parent ast.Node, a ast.Node, b ast.Node) bool {
	switch p := parent.(type) {
	case *ast.IfStmt:
		return (a == p.Body && b == p.Else) || (a == p.Else && b == p.Body)
	case *ast.BlockStmt:
		_, aCase := a.(*ast.CaseClause)
		_, bCase := b.(*ast.CaseClause)
		if aCase && bCase {
			return fallsThrough(p) == false
		}
		_, aComm := a.(*ast.CommClause)
		_, bComm := b.(*ast.CommClause)
		return aComm && bComm
	}
	return false
}

func fallsThrough(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(c ast.Node) bool {
		if branch, isBranch := c.(*ast.BranchStmt); isBranch && branch.Tok == token.FALLTHROUGH {
			found = true
		}
		return found == false
	})
	return found
}

func returnsBefore(chain []ast.Node, level int, at token.Pos, stop ast.Node) bool {
	for i := level; i+1 < len(chain); i++ {
		list := stmtList(chain[i])
		if list == nil {
			continue
		}
		child := chain[i+1]
		if branchesAfter(child, at) {
			return false
		}
		after := false
		for _, s := range list {
			if s == child {
				after = true
				continue
			}
			if after == false {
				continue
			}
			if s == stop || branchesAfter(s, token.NoPos) {
				break
			}
			if terminates(s) {
				return true
			}
		}
	}
	return false
}

func branchesAfter(n ast.Node, at token.Pos) bool {
	found := false
	ast.Inspect(n, func(c ast.Node) bool {
		switch x := c.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BranchStmt:
			if x.Pos() > at {
				found = true
			}
		}
		return found == false
	})
	return found
}

func terminates(s ast.Stmt) bool {
	switch x := s.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.ExprStmt:
		call, isCall := x.X.(*ast.CallExpr)
		if isCall == false {
			return false
		}
		id, isIdent := call.Fun.(*ast.Ident)
		return isIdent && id.Name == "panic"
	}
	return false
}
