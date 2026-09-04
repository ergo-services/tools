// Package edf is a stand-in for the framework's EDF package. Only the package
// level registration entry points are present, which are the ones A2009 reports on.
package edf

import "ergo.services/ergo/gen"

func RegisterType(v any) error { return nil }

func RegisterTypeOf(v any) error { return nil }

func RegisterTypesOf(types []any) error { return nil }

func RegisterError(e error) error { return nil }

func RegisterAtom(a gen.Atom) error { return nil }
