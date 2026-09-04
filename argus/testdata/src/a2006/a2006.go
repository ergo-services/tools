package a2006

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessageTick struct{ N int }

const tickPeriod = 5 * time.Second

type Ticker struct {
	act.Actor
	pid gen.PID
}

func (t *Ticker) HandleMessage(from gen.PID, message any) error {
	// a real period is the point of the API
	t.SendEvery(t.pid, MessageTick{}, time.Second)
	t.SendEvery(t.pid, MessageTick{}, tickPeriod)

	// zero never arms, so the work silently never happens
	t.SendEvery(t.pid, MessageTick{}, 0) // want `\[tier2\] \[A2006\] SendEvery with a period of zero or less`

	// negative is the same defect written differently
	t.SendEvery(t.pid, MessageTick{}, -time.Second) // want `A2006.*SendEvery with a period of zero or less`

	// the priority variant carries the period one slot further along
	t.SendWithPriorityEvery(t.pid, MessageTick{}, gen.MessagePriorityHigh, 0) // want `A2006.*SendWithPriorityEvery with a period of zero or less`

	// a one shot delay is a different method and is not a period at all
	t.SendAfter(t.pid, MessageTick{}, 0)
	return nil
}

func (t *Ticker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// a raw ticker in a callback schedules off the mailbox
	ticker := time.NewTicker(time.Second) // want `A2006.*time.NewTicker inside HandleCall`
	defer ticker.Stop()

	// so does a bare delay
	<-time.After(time.Millisecond) // want `A2006.*time.After inside HandleCall`

	// bounding a receive with a timeout arm is the documented pattern
	select {
	case <-ticker.C:
	case <-time.After(time.Second):
	default:
	}
	return nil, nil
}

// a period computed at runtime is undecidable, so the rule stays quiet
func (t *Ticker) HandleEvent(event gen.MessageEvent) error {
	period := time.Duration(t.pid.ID) * time.Second
	t.SendEvery(t.pid, MessageTick{}, period)
	return nil
}

// a recorded exception is silent
func (t *Ticker) Terminate(reason error) {
	//argus:allow A2006 the deadline is enforced by the caller
	<-time.After(time.Millisecond)
}
