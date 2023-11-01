package registrar

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factoryControl() gen.ProcessBehavior {
	return &Control{}
}

type Control struct {
	act.Actor
}

func (c *Control) Init(args ...any) error {
	options := gen.ProcessOptions{
		LinkParent: true,
	}
	registrarArgs := RegistrarArgs{
		Token: "123",
		Port:  4499,
	}
	c.SpawnRegister(NameRegistrar, factoryRegistrar, options, registrarArgs)
	return nil
}

func (c *Control) HandleMessage(from gen.PID, message any) error {
	c.Log().Info("got unknown message from %s: %#v", from, message)
	return nil
}

func (c *Control) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	c.Log().Info("got request from %s with reference %s", from, ref)
	return gen.Atom("pong"), nil
}

func (c *Control) Terminate(reason error) {
	c.Log().Info("terminated with reason: %s", reason)
}

func (c *Control) HandleMessageName(name gen.Atom, from gen.PID, message any) error {
	return nil
}
