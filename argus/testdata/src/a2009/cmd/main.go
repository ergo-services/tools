// Command cmd is main for A2009: the one function that can fill the package level
// table before the node exists, and that only has a node after StartNode.
package main

import (
	"a2009/wire"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
)

type MessageBoot struct{ ID int }

func startNode() gen.Node { return nil }

func main() {
	// the package table can be filled before the node starts
	edf.RegisterTypeOf(MessageBoot{}) // want `\[tier3\] \[A2009\] edf.RegisterTypeOf is deprecated`
	registerBoot()

	// a node exists only after StartNode, so whatever registers through it is late
	node := startNode()
	node.Network().RegisterType(MessageBoot{}) // want `\[tier2\] \[A2009\] RegisterType in function main runs after a peer may already have connected`
	wire.Register(node)                        // want `\[tier2\] \[A2009\] Register, called from function main, registers types through Network\(\).RegisterTypes after a peer may already have connected`
}

func registerBoot() { // want registerBoot:"registers through edf.RegisterTypeOf"
	edf.RegisterTypeOf(MessageBoot{}) // want `\[tier3\] \[A2009\] edf.RegisterTypeOf is deprecated`
}
