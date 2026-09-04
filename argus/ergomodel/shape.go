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

func (m *Model) shapeOf(t types.Type) Shape {
	if t == nil {
		return Shape{Aliasing: true, Opaque: true}
	}
	t = types.Unalias(t)
	if s, ok := m.shapes[t]; ok {
		return s
	}
	if m.inProgress[t] {
		m.usedBackEdge++
		return Shape{WireOK: false, Witness: "recursive type"}
	}

	m.inProgress[t] = true
	before := m.usedBackEdge
	s := m.computeShape(t)
	delete(m.inProgress, t)

	if m.usedBackEdge == before {
		m.shapes[t] = s
	}
	return s
}

func (m *Model) computeShape(t types.Type) Shape {
	switch u := t.(type) {
	case *types.Basic:
		return basicShape(u)

	case *types.Slice:
		return alias(m.shapeOf(u.Elem()), "[]"+short(u.Elem()), u)

	case *types.Map:
		k := m.shapeOf(u.Key())
		v := m.shapeOf(u.Elem())
		s := alias(v, fmt.Sprintf("map[%s]%s", short(u.Key()), short(u.Elem())), u)
		s.WireOK = s.WireOK && k.WireOK
		return s

	case *types.Array:
		return m.shapeOf(u.Elem())

	case *types.Chan:
		return Shape{Aliasing: true, WireOK: false, Witness: "chan " + short(u.Elem()), Ref: u}

	case *types.Signature:
		return Shape{Aliasing: true, WireOK: false, Witness: "func", Ref: u}

	case *types.Pointer:
		elem := types.Unalias(u.Elem())
		s := alias(m.shapeOf(elem), "*"+short(elem), u)

		if _, nested := elem.Underlying().(*types.Pointer); nested {
			s.WireOK = false
			s.Witness = "**" + short(elem)
		}
		return s

	case *types.Struct:
		return m.structShape(u)

	case *types.Interface:
		return m.interfaceShape(u)

	case *types.Named:
		return m.namedShape(u)

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
	if inner.Witness != "" && inner.Witness != "recursive type" {
		s.Witness = hop + " -> " + inner.Witness
	} else if inner.Witness == "recursive type" {
		s.Witness = hop + " (recursive)"
	} else {
		s.Witness = hop
	}
	return s
}

func (m *Model) structShape(st *types.Struct) Shape {
	out := Shape{WireOK: true}

	aliasWitness := ""
	wireWitness := ""

	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("edf")
		fs := m.shapeOf(f.Type())

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

func (m *Model) namedShape(n *types.Named) Shape {
	path := typePath(n)

	if frameworkValueTypes[path] {
		return Shape{WireOK: true}
	}

	if path == "error" {
		if m.cfg.AcceptIfaces["error"] {
			return Shape{WireOK: true}
		}
		return Shape{Aliasing: true, Opaque: true, WireOK: true, Witness: "error"}
	}
	if reason, ok := m.cfg.AllowTypes[path]; ok {
		_ = reason
		return Shape{WireOK: true}
	}

	if strings.HasPrefix(n.Obj().Name(), "_Ctype_") {
		return Shape{Aliasing: true, Opaque: true, WireOK: false, Witness: "cgo type"}
	}

	if m.hasMarshalerPair(n) {
		s := m.shapeOf(n.Underlying())
		s.WireOK = true
		return s
	}

	if obj := n.Obj(); obj != nil && obj.Pkg() != nil && obj.Pkg() != m.pass.Pkg {
		var f ShapeFact
		if m.pass.ImportObjectFact(obj, &f) {
			return Shape{Aliasing: f.Aliasing, Opaque: f.Opaque, WireOK: f.WireOK, Witness: f.Witness}
		}
	}

	return m.shapeOf(n.Underlying())
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
