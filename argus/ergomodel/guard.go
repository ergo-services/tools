package ergomodel

import "go/types"

type Guardedness int

const (
	GuardUnknown Guardedness = iota
	GuardGuarded
	GuardUnguarded
)

func (m *Model) Guard(t types.Type) Guardedness {
	if t == nil {
		return GuardUnknown
	}
	base := deref(t)

	if isSyncMap(base) {
		return GuardGuarded
	}
	named, ok := base.(*types.Named)
	if ok == false {
		return GuardUnknown
	}
	st, ok := named.Underlying().(*types.Struct)
	if ok == false {
		return GuardUnknown
	}

	hasMutex := false
	exposedUnguarded := false
	mutableFields := 0
	atomicFields := 0

	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		ft := types.Unalias(f.Type())

		if isMutex(ft) {
			hasMutex = true
			continue
		}
		if isAtomic(ft) || isSyncMap(ft) {
			atomicFields++
			continue
		}
		s := m.shapeOf(ft)
		if s.Aliasing == false {
			continue
		}
		mutableFields++

		if f.Exported() {
			exposedUnguarded = true
		}
	}

	switch {
	case mutableFields == 0 && atomicFields > 0:
		return GuardGuarded
	case hasMutex && exposedUnguarded:
		return GuardUnguarded
	case hasMutex:
		return GuardGuarded
	case mutableFields > 0:
		return GuardUnguarded
	}
	return GuardUnknown
}

func isMutex(t types.Type) bool {
	named, ok := deref(t).(*types.Named)
	if ok == false {
		return false
	}
	if named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "sync" {
		return false
	}
	switch named.Obj().Name() {
	case "Mutex", "RWMutex":
		return true
	}
	return false
}

func isSyncMap(t types.Type) bool {
	named, ok := deref(t).(*types.Named)
	if ok == false {
		return false
	}
	pkg := named.Obj().Pkg()
	return pkg != nil && pkg.Path() == "sync" && named.Obj().Name() == "Map"
}

func isAtomic(t types.Type) bool {
	named, ok := deref(t).(*types.Named)
	if ok == false {
		return false
	}
	pkg := named.Obj().Pkg()
	return pkg != nil && pkg.Path() == "sync/atomic"
}
