// Package shapes exercises the shape computation over recursive type graphs without
// asserting any diagnostic.
package shapes

// Tree recurses through a wire field, so registration refuses it.
type Tree struct {
	Name     string
	Children []Tree
}

// Linked recurses only through a field EDF skips, so the cycle never reaches the wire.
type Linked struct {
	Name   string
	Parent *Linked `edf:"-"`
}

// Holder carries Chain by value and Chain points back at Holder. Which of the two a
// traversal enters first must not change what either one is.
type Chain struct{ holder *Holder }

type Holder struct {
	Chain Chain
	N     int
}

// Flat reaches the cycle without being part of it.
type Flat struct {
	Tree Tree
	ID   int64
}

// Plain has no reference anywhere.
type Plain struct {
	ID   int64
	Name string
}
