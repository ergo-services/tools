package {{ .Package }}

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type {{ .Name }} struct {
	gen.Server
}
func (s *{{ .Name }}) Init(process *gen.ServerProcess, args ...etf.Term) error {
	fmt.Printf("Init process: %s with args %v \n", process.Self(), args)
	return nil
}

func (s *{{ .Name }}) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleInfo: %#v \n", message)
	return gen.ServerStatusOK
}

func (s *{{ .Name }}) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleCast: %#v \n", message)
	return gen.ServerStatusOK
}

func (s *{{ .Name }}) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	fmt.Printf("HandleCall: %#v \n", message)
	return nil, gen.ServerStatusOK
}

func (s *{{ .Name }}) Terminate(process *gen.ServerProcess, reason string) {
	fmt.Printf("Terminated: %s with reason %s", process.Self(), reason)
}
