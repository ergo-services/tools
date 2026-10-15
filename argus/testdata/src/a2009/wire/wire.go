// Package wire is a dependency that registers its types through an exported helper,
// the way a library hands its wire types to whoever runs the node.
package wire

import "ergo.services/ergo/gen"

type MessageFrame struct{ Seq int }

func Register(node gen.Node) error {
	return node.Network().RegisterTypes([]any{MessageFrame{}})
}

// Describe touches the network without registering anything.
func Describe(node gen.Node) string {
	_ = node.Network()
	return "wire"
}
