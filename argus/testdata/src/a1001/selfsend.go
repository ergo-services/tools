package a1001

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// SelfSender keeps writing into live, so any receiver holding it races with this
// actor. Except the one receiver that is this actor: its own mailbox is handled by
// the same goroutine, one message after another.
type SelfSender struct {
	act.Actor
	live map[string]string
	peer gen.PID
}

func (s *SelfSender) record(key string, value string) { s.live[key] = value }

func (s *SelfSender) HandleMessage(from gen.PID, message any) error {
	live := s.live

	// a timer to itself, by pid, by name, and through a local bound to its pid
	s.SendAfter(s.PID(), MessageMap{Meta: live}, time.Second)
	s.Send(s.Name(), MessageMap{Meta: live})
	self := s.PID()
	s.Send(self, MessageMap{Meta: live})

	// an alias it created is its own mailbox too
	alias, _ := s.CreateAlias()
	s.Send(alias, MessageMap{Meta: live})

	// the same payload to another process is the race
	s.Send(s.peer, MessageMap{Meta: live}) // want `\[tier1\] \[A1001\] a1001.MessageMap .*s.live, mutated in place`

	// a target that is only sometimes this process is not a self send
	to := s.peer
	if len(live) > 1 {
		to = s.PID()
	}
	s.Send(to, MessageMap{Meta: live}) // want `\[tier1\] \[A1001\] a1001.MessageMap`
	return nil
}

// A helper on the actor runs on the same goroutine as the callback that calls it.
func (s *SelfSender) retry() {
	live := s.live
	s.SendAfter(s.PID(), MessageMap{Meta: live}, time.Second)
}

// Another process's pid in a field is not this process, whatever the field is named.
func (s *SelfSender) notify(other *SelfSender) {
	live := s.live
	s.Send(other.PID(), MessageMap{Meta: live}) // want `\[tier1\] \[A1001\] a1001.MessageMap`
}
