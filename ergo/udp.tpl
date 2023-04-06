package {{ .Package }}

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func create{{ .Name }}() gen.UDPBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.UDP
}

//
// Mandatory callbacks
//

// InitUDP invoked on starting UDP server
func (us *{{ .Name }}) InitUDP(process *gen.UDPProcess, args ...etf.Term) (gen.UDPOptions, error) {
	return gen.UDPOptions{
		Host:    "localhost",
		Port:    uint16(12345),
		Handler: create{{ .Name }}Handler, // udp_handler.go
	}, nil
}

//
// Optional gen.Server's callbacks
//

// HandleUDPCall this callback is invoked on ServerProcess.Call(...).
func (us *{{ .Name }}) HandleUDPCall(process *gen.UDPProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return nil, gen.ServerStatusOK
}

// HandleUDPCast this callback is invoked on ServerProcess.Cast(...).
func (us *{{ .Name }}) HandleUDPCast(process *gen.UDPProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleUDPInfo this callback is invoked on Process.Send(...).
func (us *{{ .Name }}) HandleUDPInfo(process *gen.UDPProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleUDPTerminate this callback invoked on a process termination
func (us *{{ .Name }}) HandleUDPTerminate(process *gen.UDPProcess, reason string) {

}
