package a3006

// Prose that already says what the marker would say, which is the migration path this
// rule exists to shorten.
//
// MessageProse carries a handle that is same node only, so it must never be registered
// for the wire.
type MessageProse struct { // want `\[tier3\] \[A3006\] the doc comment on MessageProse already says "same node only"`
	Handle chan int
}

// A grouped declaration carries its comment on the spec rather than on the group.
type (
	// MessageGrouped is local only.
	MessageGrouped struct { // want `A3006.*the doc comment on MessageGrouped already says "local only"`
		Handle chan int
	}

	// MessageOrdinary says nothing about scope.
	MessageOrdinary struct {
		ID int64
	}
)

// The marker is already there, so there is nothing to convert.
//
// MessageDone is same node only.
//
//argus:message local
type MessageDone struct {
	Handle chan int
}

// A comment that does not talk about scope at all is not prior art.
//
// MessageDocumented explains what it carries and why, at length, without ever saying
// where it may travel.
type MessageDocumented struct {
	ID int64
}
