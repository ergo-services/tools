package {{ .Package }}

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)
func create{{ .Name }}Worker() gen.PoolWorkerBehavior {
	return &{{ .Name }}Worker{}
}

type {{ .Name }}Worker struct {
	gen.PoolWorker
}

func (w *{{ .Name }}Worker) InitPoolWorker(process *gen.PoolWorkerProcess, args ...etf.Term) error {
	fmt.Println("   started pool worker: ", process.Self())
	return nil
}

func (w *{{ .Name }}Worker) HandleWorkerCall(process *gen.PoolWorkerProcess, message etf.Term) etf.Term {
	fmt.Printf("[%s] received Call request: %v\n", process.Self(), message)
	return "pong"
}

func (w *{{ .Name }}Worker) HandleWorkerCast(process *gen.PoolWorkerProcess, message etf.Term) {
	fmt.Printf("[%s] received Cast message: %v\n", process.Self(), message)
}

func (w *{{ .Name }}Worker) HandleWorkerInfo(process *gen.PoolWorkerProcess, message etf.Term) {
	fmt.Printf("[%s] received Info message: %v\n", process.Self(), message)
}
