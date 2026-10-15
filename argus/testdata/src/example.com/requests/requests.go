// Package requests holds request helpers that callbacks in other packages reach. The
// timeout of the request is part of what has to travel: a helper that asks for thirty
// seconds and one that asks for one are different verdicts for the caller's budget.
// The path has a dot because a dotless one reads as the standard library, which
// exports no blocking facts.
package requests

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// Slow asks for more than any default budget.
func Slow(a *act.Actor, to gen.PID) error {
	_, err := a.CallWithTimeout(to, "seed", 30)
	return err
}

// Brief stays well inside one.
func Brief(a *act.Actor, to gen.PID) error {
	_, err := a.CallWithTimeout(to, "seed", 1)
	return err
}

// Both makes a short request and a default one, so its budget is the default's.
func Both(a *act.Actor, to gen.PID) error {
	if _, err := a.CallWithTimeout(to, "seed", 2); err != nil {
		return err
	}
	_, err := a.Call(to, "seed")
	return err
}

// Later only stores the request: calling it waits for nothing.
func Later(a *act.Actor, to gen.PID) func() {
	return func() { a.Call(to, "seed") }
}
