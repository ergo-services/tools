// Package exposure exercises which part of a type a receiver shares with the sender
// once allowed types and self-guarding pointees are left out, without asserting any
// diagnostic.
package exposure

import "sync"

type Guarded struct {
	mu   sync.Mutex
	data map[string]int
}

// The first reference is allowlisted, the second is not.
type RawThenItems struct {
	Raw   []byte
	Items []int
}

// The first reference guards itself, the second does not.
type GuardThenItems struct {
	Cache *Guarded
	Items []int
}

// Every reference is accepted.
type OnlyAccepted struct {
	Raw   []byte
	Cache *Guarded
	When  int64
}

// A value struct is walked into, so the witness names the path down to the part.
type Nested struct {
	ID    int64
	Inner RawThenItems
}

// A guarded struct held by value: the copy's mutex guards nothing the original
// holds, so its map is shared like any other.
type GuardedValue struct {
	Cache Guarded
}

type Values struct {
	ID   int64
	Name string
}
