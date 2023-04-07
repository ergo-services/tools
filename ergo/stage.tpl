package {{ .Package }}

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func create{{ .Name }}() gen.StageBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.Stage
}

// InitStage
func (s *{{ .Name }}) InitStage(process *gen.StageProcess, args ...etf.Term) (gen.StageOptions, error) {
	//
	// You should specify Dispatcher if this process is intended to be used as a producer. There are
	// 3 different types of dispatchers:
	//
	// - gen.CreateStageDispatcherDemand() - creates a dispatcher that sends batches
	//   to the highest demand. This is the default dispatcher used
	//   by Stage. In order to avoid greedy consumers, it is recommended
	//   that all consumers have exactly the same maximum demand.
	//
	// - gen.CreateStageDispatcherBroadcast() creates a dispatcher that accumulates
	//   demand from all consumers before broadcasting events to all of them.
	//   This dispatcher guarantees that events are dispatched to
	//   all consumers without exceeding the demand of any given consumer.
	//   The demand is only sent upstream once all consumers ask for data.
	//
	// - gen.CreateStageDispatcherPartition creates a dispatcher that sends
	//   events according to partitions. Number of partitions 'n' must be > 0.
	//   'hash' should return number within range [0,n). Value outside of this range
	//   is discarding event. If 'hash' is nil the random partition will be used on every event.
	//
	// options := gen.StageOptions{
	//    Dispatcher: gen.CreateStageDispatcherDemand(),
	// }

	// As a consumer you need to create a subscription
	// opts := gen.StageSubscribeOptions{}
	// process.Subscribe(gen.ProcessID{Name: "producer", Node: "node@localhost"}, opts)

	return gen.StageOptions{}, nil
}

//
// Because Saga process can act as a producer and consumer at once, you should use callback methods
// for your implementation accordingly
//

//
// Consumer callbacks
//

// HandleEvents this callback is invoked on a consumer stage.
func (s *{{ .Name }}) HandleEvents(process *gen.StageProcess, subscription gen.StageSubscription, events etf.List) gen.StageStatus {
	return gen.StageStatusOK
}

// HandleCanceled
// Invoked when a consumer is no longer subscribed to a producer (invoked on a consumer stage)
// Termination this stage depends on a cancel mode for the given subscription. For the cancel mode
// gen.StageCancelPermanent - this stage will be terminated right after this callback invoking.
// For the cancel mode gen.StageCancelTransient - it depends on a reason of subscription canceling.
// Cancel mode gen.StageCancelTemporary keeps this stage alive whether the reason could be.
func (s *{{ .Name }}) HandleCanceled(process *gen.StageProcess, subscription gen.StageSubscription, reason string) gen.StageStatus {
	return gen.StageStatusOK
}

// HandleSubscribed this callback is invoked as a confirmation for the subscription request
// Returning false means that demand must be sent to producers explicitly using Ask method.
// Returning true means the stage implementation will take care of automatically sending.
func (s *{{ .Name }}) HandleSubscribed(process *gen.StageProcess, subscription gen.StageSubscription, opts gen.StageSubscribeOptions) (bool, gen.StageStatus) {
	return false, gen.StageStatusOK
}

//
// Producer callbacks
//

// HandleDemand this callback is invoked on a producer stage
// The producer that implements this callback must either store the demand, or return the amount of requested events.
func (s *{{ .Name }}) HandleDemand(process *gen.StageProcess, subscription gen.StageSubscription, count uint) (etf.List, gen.StageStatus) {
	var list etf.List
	return list, gen.StageStatusOK
}

// HandleSubscribe This callback is invoked on a producer stage.
func (s *{{ .Name }}) HandleSubscribe(process *gen.StageProcess, subscription gen.StageSubscription, options gen.StageSubscribeOptions) gen.StageStatus {
	return gen.StageStatusOK
}

// HandleCancel
// Invoked when a consumer is no longer subscribed to a producer (invoked on a producer stage)
// The cancelReason will be a {Cancel: "cancel", Reason: _} if the reason for cancellation
// was a gen.Stage.Cancel call. Any other value means the cancellation reason was
// due to an EXIT.
func (s *{{ .Name }}) HandleCancel(process *gen.StageProcess, subscription gen.StageSubscription, reason string) gen.StageStatus {
	return gen.StageStatusOK
}


//
// Optional gen.Server's callbacks
//

// HandleStageCall this callback is invoked on gen.ServerProcess.Call(...).
func (s *{{ .Name }}) HandleStageCall(process *gen.StageProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return nil, gen.ServerStatusOK
}

// HandleStageCast this callback is invoked on gen.ServerProcess.Cast(...).
func (s *{{ .Name }}) HandleStageCast(process *gen.StageProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleStageInfo this callback is invoked on Process.Send(...).
func (s *{{ .Name }}) HandleStageInfo(process *gen.StageProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleStageDirect this callback is invoked on Process.Direct(...).
func (s *{{ .Name }}) HandleStageDirect(process *gen.StageProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	return nil, gen.DirectStatusOK
}

// HandleStageTerminate this callback is invoked on a termination process
func (s *{{ .Name }}) HandleStageTerminate(process *gen.StageProcess, reason string) {

}
