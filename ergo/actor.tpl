package {{ .Package }}

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func create{{ .Name }}() gen.ServerBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.Server
}

// Init invoked on a start this process.
func (s *{{ .Name }}) Init(process *gen.ServerProcess, args ...etf.Term) error {
	fmt.Printf("Init process: %s with args %v \n", process.Self(), args)
	return nil
}

//
// Methods below are optional, so you can remove those that aren't be used
//

// HandleInfo invoked if this process received message sent with Process.Send(...).
func (s *{{ .Name }}) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleInfo: %#v \n", message)
	return gen.ServerStatusOK
}

// HandleCast invoked if this process received message sent with ServerProcess.Cast(...).
// Return ServerStatusStop to stop server with "normal" reason. Use ServerStatus(error)
// for the custom reason
func (s *{{ .Name }}) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleCast: %#v \n", message)
	return gen.ServerStatusOK
}

// HandleCall invoked if this process got sync request using ServerProcess.Call(...)
func (s *{{ .Name }}) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	fmt.Printf("HandleCall: %#v \n", message)
	return nil, gen.ServerStatusOK
}

// HandleDirect invoked on a direct request made with Process.Direct(...)
func (s *{{ .Name }}) HandleDirect(process *gen.ServerProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	fmt.Printf("HandleDirect: %#v \n", message)
	return nil, nil
}

// Terminate invoked on a termination process. ServerProcess.State is not locked during this callback.
func (s *{{ .Name }}) Terminate(process *gen.ServerProcess, reason string) {
	fmt.Printf("Terminated: %s with reason %s", process.Self(), reason)
}
