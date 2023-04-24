package {{ .Package }}

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func create{{ .Name }}() gen.RaftBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.Raft
}

func (r *{{ .Name }}) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	opts := gen.RaftOptions{}
	return opts, nil
}
func (r *{{ .Name }}) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	fmt.Println(process.Self(), "Quorum built - State:", quorum.State, "Quorum member:", quorum.Member)
	if quorum.Member == false {
		fmt.Println(process.Self(), "    since I'm not a quorum member, I won't receive any information about elected leader")
	}
	return gen.RaftStatusOK
}

func (r *{{ .Name }}) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
	if leader != nil && leader.Leader == process.Self() {
		fmt.Println(process.Self(), "I'm a leader of this quorum")
		return gen.RaftStatusOK
	}

	if leader != nil {
		fmt.Println(process.Self(), "Leader elected:", leader.Leader, "with serial", leader.Serial)
	}
	return gen.RaftStatusOK
}

func (r *{{ .Name }}) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	fmt.Printf("%s Received append request with serial %d, key %q and value %q\n", process.Self(), serial, key, value)
	return gen.RaftStatusOK
}
func (r *{{ .Name }}) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	fmt.Println(process.Self(), "Received request for serial", serial)
	return "key", "value", gen.RaftStatusOK
}

func (r *{{ .Name }}) HandleRaftInfo(process *gen.RaftProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleRaftInfo: %#v \n", message)
	return gen.ServerStatusOK
}

func (r *{{ .Name }}) HandleSerial(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	fmt.Println(process.Self(), "Received requested serial", serial, "with key", key, "and value", value)
	return gen.RaftStatusOK
}
