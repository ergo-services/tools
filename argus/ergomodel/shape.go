package ergomodel

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"
)

type Shape struct {
	Aliasing bool
	Opaque   bool
	WireOK   bool
	Witness  string

	Ref types.Type
}

const recursiveWitness = "recursive type"

func (m *Model) shapeOf(t types.Type) Shape {
	if t == nil {
		return Shape{Aliasing: true, Opaque: true}
	}
	t = types.Unalias(t)
	if s, ok := m.cachedShape(t); ok {
		return s
	}
	edges := func(x types.Type) []types.Type {
		var out []types.Type
		for _, c := range m.shapeEdges(x, false) {
			if _, done := m.cachedShape(c); done == false {
				out = append(out, c)
			}
		}
		return out
	}
	components([]types.Type{t}, edges, m.settle)
	s, _ := m.cachedShape(t)
	return s
}

func (m *Model) cachedShape(t types.Type) (Shape, bool) {
	if s, ok := m.shapes[t]; ok {
		return s, true
	}
	s, ok := m.sealed[t]
	return s, ok
}

func (m *Model) settled(t types.Type) Shape {
	t = types.Unalias(t)
	if s, ok := m.cachedShape(t); ok {
		return s
	}
	return m.shapeOf(t)
}

func (m *Model) shapeEdges(t types.Type, wire bool) []types.Type {
	var out []types.Type
	switch u := t.(type) {
	case *types.Slice:
		out = append(out, u.Elem())
	case *types.Map:
		out = append(out, u.Key(), u.Elem())
	case *types.Array:
		out = append(out, u.Elem())
	case *types.Pointer:
		out = append(out, u.Elem())
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if wire && reflect.StructTag(u.Tag(i)).Get("edf") == "-" {
				continue
			}
			out = append(out, u.Field(i).Type())
		}
	case *types.Named:
		if _, leaf := m.namedLeaf(u); leaf {
			return nil
		}
		if wire && m.hasMarshalerPair(u) {
			return nil
		}
		out = append(out, u.Underlying())
	}
	for i := range out {
		out[i] = types.Unalias(out[i])
	}
	return out
}

func (m *Model) settle(scc []types.Type) {
	if len(scc) == 1 && m.selfEdge(scc[0]) == false {
		m.shapes[scc[0]] = m.computeShape(scc[0], m.settled)
		return
	}

	member := make(map[types.Type]bool, len(scc))
	for _, t := range scc {
		member[t] = true
	}
	preds := map[types.Type][]types.Type{}
	wires := map[types.Type][]types.Type{}
	for _, t := range scc {
		for _, c := range m.shapeEdges(t, false) {
			if member[c] {
				preds[c] = append(preds[c], t)
			}
		}
		for _, c := range m.shapeEdges(t, true) {
			if member[c] {
				wires[t] = append(wires[t], c)
			}
		}
	}

	cyclic := map[types.Type]bool{}
	components(scc, func(t types.Type) []types.Type { return wires[t] }, func(comp []types.Type) {
		if len(comp) == 1 && containsType(wires[comp[0]], comp[0]) == false {
			return
		}
		for _, t := range comp {
			cyclic[t] = true
		}
	})

	current := make(map[types.Type]Shape, len(scc))
	for _, t := range scc {
		current[t] = Shape{WireOK: true}
	}
	approx := func(c types.Type) Shape {
		c = types.Unalias(c)
		if member[c] {
			s := current[c]
			s.Witness = recursiveWitness
			return s
		}
		return m.settled(c)
	}

	queue := append([]types.Type(nil), scc...)
	queued := make(map[types.Type]bool, len(scc))
	for _, t := range scc {
		queued[t] = true
	}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		queued[t] = false

		s := m.computeShape(t, approx)
		if cyclic[t] {
			s.WireOK = false
		}
		prev := current[t]
		current[t] = s
		if s.Aliasing == prev.Aliasing && s.Opaque == prev.Opaque && s.WireOK == prev.WireOK {
			continue
		}
		for _, p := range preds[t] {
			if queued[p] == false {
				queue = append(queue, p)
				queued[p] = true
			}
		}
	}

	const (
		pending = iota
		building
		built
	)
	state := make(map[types.Type]int, len(scc))
	var witnessed func(t types.Type) Shape
	witnessed = func(t types.Type) Shape {
		switch state[t] {
		case building:
			s := current[t]
			s.Witness = recursiveWitness
			return s
		case built:
			return m.shapes[t]
		}
		state[t] = building
		s := m.computeShape(t, func(c types.Type) Shape {
			c = types.Unalias(c)
			if member[c] {
				return witnessed(c)
			}
			return m.settled(c)
		})
		if cyclic[t] {
			s.WireOK = false
		}
		state[t] = built
		m.shapes[t] = s
		return s
	}
	for i := len(scc) - 1; i >= 0; i-- {
		witnessed(scc[i])
	}
}

func (m *Model) selfEdge(t types.Type) bool {
	return containsType(m.shapeEdges(t, false), t)
}

func containsType(list []types.Type, t types.Type) bool {
	for _, x := range list {
		if x == t {
			return true
		}
	}
	return false
}

func components(roots []types.Type, edges func(types.Type) []types.Type, found func([]types.Type)) {
	index := map[types.Type]int{}
	low := map[types.Type]int{}
	onStack := map[types.Type]bool{}
	var stack []types.Type
	next := 0

	var visit func(t types.Type)
	visit = func(t types.Type) {
		index[t], low[t] = next, next
		next++
		stack = append(stack, t)
		onStack[t] = true

		for _, c := range edges(t) {
			if _, seen := index[c]; seen == false {
				visit(c)
				low[t] = min(low[t], low[c])
				continue
			}
			if onStack[c] {
				low[t] = min(low[t], index[c])
			}
		}
		if low[t] != index[t] {
			return
		}

		var comp []types.Type
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			comp = append(comp, top)
			if top == t {
				break
			}
		}
		found(comp)
	}
	for _, t := range roots {
		if _, seen := index[t]; seen == false {
			visit(t)
		}
	}
}

func (m *Model) computeShape(t types.Type, get func(types.Type) Shape) Shape {
	switch u := t.(type) {
	case *types.Basic:
		return basicShape(u)

	case *types.Slice:
		return alias(get(u.Elem()), "[]"+short(u.Elem()), u)

	case *types.Map:
		k := get(u.Key())
		v := get(u.Elem())
		s := alias(v, fmt.Sprintf("map[%s]%s", short(u.Key()), short(u.Elem())), u)
		s.WireOK = s.WireOK && k.WireOK
		return s

	case *types.Array:
		return get(u.Elem())

	case *types.Chan:
		return Shape{Aliasing: true, WireOK: false, Witness: "chan " + short(u.Elem()), Ref: u}

	case *types.Signature:
		return Shape{Aliasing: true, WireOK: false, Witness: "func", Ref: u}

	case *types.Pointer:
		elem := types.Unalias(u.Elem())
		s := alias(get(elem), "*"+short(elem), u)

		if _, nested := elem.Underlying().(*types.Pointer); nested {
			s.WireOK = false
			s.Witness = "**" + short(elem)
		}
		return s

	case *types.Struct:
		return m.structShape(u, get)

	case *types.Interface:
		return m.interfaceShape(u)

	case *types.Named:
		return m.namedShape(u, get)

	case *types.TypeParam:
		return Shape{Aliasing: true, Opaque: true, WireOK: false, Witness: "type parameter"}
	}
	return Shape{Aliasing: true, Opaque: true, WireOK: false}
}

func basicShape(b *types.Basic) Shape {
	switch b.Kind() {
	case types.UnsafePointer, types.Uintptr:
		return Shape{Aliasing: true, Opaque: true, WireOK: false, Witness: b.Name()}
	case types.Invalid:
		return Shape{Aliasing: true, Opaque: true, WireOK: false}
	}

	return Shape{WireOK: true}
}

func alias(inner Shape, hop string, ref types.Type) Shape {
	s := inner
	s.Aliasing = true
	s.Ref = ref
	if inner.Witness != "" && inner.Witness != recursiveWitness {
		s.Witness = hop + " -> " + inner.Witness
	} else if inner.Witness == recursiveWitness {
		s.Witness = hop + " (recursive)"
	} else {
		s.Witness = hop
	}
	return s
}

func (m *Model) structShape(st *types.Struct, get func(types.Type) Shape) Shape {
	out := Shape{WireOK: true}

	aliasWitness := ""
	wireWitness := ""

	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("edf")
		fs := get(f.Type())

		wire := tag != "-"

		if wire && f.Exported() == false {
			out.WireOK = false
			if wireWitness == "" {
				wireWitness = f.Name() + " (unexported)"
			}
		}
		if wire && fs.WireOK == false {
			out.WireOK = false
			if wireWitness == "" {
				wireWitness = describeField(f, fs)
			}
		}
		if wire && fs.Opaque {
			out.Opaque = true
		}

		if fs.Aliasing {
			out.Aliasing = true
			if aliasWitness == "" {
				aliasWitness = describeField(f, fs)

				out.Ref = fs.Ref
				if out.Ref == nil {
					out.Ref = f.Type()
				}
			}
		}
	}

	out.Witness = aliasWitness
	if out.Witness == "" {
		out.Witness = wireWitness
	}
	return out
}

func describeField(f *types.Var, fs Shape) string {
	w := f.Name() + " " + short(f.Type())
	if fs.Witness != "" && fs.Witness != short(f.Type()) {
		w += " -> " + fs.Witness
	}
	return w
}

func (m *Model) interfaceShape(it *types.Interface) Shape {
	if it.Empty() {
		return Shape{Aliasing: true, Opaque: true, WireOK: true, Witness: "any"}
	}
	return Shape{Aliasing: true, Opaque: true, WireOK: false, Witness: "interface"}
}

func (m *Model) namedShape(n *types.Named, get func(types.Type) Shape) Shape {
	if s, leaf := m.namedLeaf(n); leaf {
		return s
	}
	s := get(n.Underlying())
	if m.hasMarshalerPair(n) {
		s.WireOK = true
	}
	return s
}

func (m *Model) namedLeaf(n *types.Named) (Shape, bool) {
	path := typePath(n)

	if frameworkValueTypes[path] {
		return Shape{WireOK: true}, true
	}

	if path == "error" {
		if m.cfg.AcceptIfaces["error"] {
			return Shape{WireOK: true}, true
		}
		return Shape{Aliasing: true, Opaque: true, WireOK: true, Witness: "error"}, true
	}
	if _, ok := m.cfg.AllowTypes[path]; ok {
		return Shape{WireOK: true}, true
	}

	if strings.HasPrefix(n.Obj().Name(), "_Ctype_") {
		return Shape{Aliasing: true, Opaque: true, WireOK: false, Witness: "cgo type"}, true
	}

	if m.hasMarshalerPair(n) {
		return Shape{}, false
	}

	if obj := n.Obj(); obj != nil && obj.Pkg() != nil && obj.Pkg() != m.pass.Pkg {
		var f ShapeFact
		if m.pass.ImportObjectFact(obj, &f) {
			return Shape{Aliasing: f.Aliasing, Opaque: f.Opaque, WireOK: f.WireOK, Witness: f.Witness}, true
		}
	}
	return Shape{}, false
}

func (m *Model) hasMarshalerPair(n *types.Named) bool {
	value := methodSetHas(n, "MarshalEDF") || methodSetHas(n, "MarshalBinary")
	ptr := methodSetHas(types.NewPointer(n), "UnmarshalEDF") || methodSetHas(types.NewPointer(n), "UnmarshalBinary")
	return value && ptr
}

func methodSetHas(t types.Type, name string) bool {
	ms := types.NewMethodSet(t)
	for i := 0; i < ms.Len(); i++ {
		if ms.At(i).Obj().Name() == name {
			return true
		}
	}
	return false
}

var frameworkValueTypes = map[string]bool{
	"ergo.services/ergo/gen.Atom":      true,
	"ergo.services/ergo/gen.PID":       true,
	"ergo.services/ergo/gen.ProcessID": true,
	"ergo.services/ergo/gen.Ref":       true,
	"ergo.services/ergo/gen.Alias":     true,
	"ergo.services/ergo/gen.Event":     true,
	"time.Time":                        true,
	"time.Duration":                    true,
}

func typePath(n *types.Named) string {
	obj := n.Obj()
	if obj == nil {
		return ""
	}
	if obj.Pkg() == nil {
		return obj.Name()
	}
	return obj.Pkg().Path() + "." + obj.Name()
}

func short(t types.Type) string {
	return types.TypeString(types.Unalias(t), func(p *types.Package) string { return p.Name() })
}
