package a3005 // want `A3005.*suppression debt in this package: A1001 4 .4 by directive, tier severity error.; A1010 1`

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageSlice struct {
	Items []string
}

type Worker struct {
	act.Actor
	pid gen.PID
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// a reasoned exception is what the directive is for, and it is silent
	//argus:allow A1001 the receiver owns the slice from here on
	w.Send(w.pid, MessageSlice{})

	// No reason at all is indistinguishable from silencing the tool. The expectation
	// has to be a block comment: a line comment after the directive would become
	// part of the reason, which is exactly what the rule reads.
	/* want `\[tier3\] \[A3005\] an allow directive has no reason` */ //argus:allow A1001
	w.Send(w.pid, MessageSlice{})

	// the blanket form needs a reason too
	/* want `A3005.*an ignore directive has no reason` */ //argus:ignore
	w.Send(w.pid, MessageSlice{})

	// the placeholder the quick fix emits, left as it was
	/* want `A3005.*the reason is still the placeholder` */ //argus:allow A1001 <reason>
	w.Send(w.pid, MessageSlice{})

	// an identifier that is not a rule in this build suppresses nothing
	//argus:allow A9999 this rule does not exist // want `A3005.*A9999 is not a rule in this build`
	w.Send(w.pid, MessageSlice{})

	// two ids on one statement are legitimate, and both are checked
	//argus:allow A1004,A1010 the goroutine reads an immutable snapshot
	go func() {}()

	// one good and one unknown reports only the unknown
	//argus:allow A1004,A8888 mixed // want `A3005.*A8888 is not a rule in this build`
	go func() {}()
	return nil
}
