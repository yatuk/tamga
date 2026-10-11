package store

import (
	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
)

// EventFindingHasher returns the function the event bus puts every event
// through when it is published (events.Bus.SetPrepare). With
// data.hash_findings on, it replaces the match of every finding on the
// event by its keyed digest and reduces the finding's metadata to what
// describes the detection (see hashFindingMatches), and marks the event.
//
// It is done here, once, and not in each subscriber: everything that keeps
// a finding or sends it on takes it from the bus (the request log, by
// either way to it; the ring of recent events behind the console, the
// exports and the event detail when there is no database; the live feed;
// the NATS envelope, which the stream keeps for a week; the analyzer), so
// none of them ever holds the match. The request is decided, and masked or
// stripped, before its event is built, from the scanners' positions in the
// text: nothing on that side reads an event.
//
// The policy is read when the event is published. An event published with
// the option off is hashed by the request log if the option is on when its
// row is written, as before.
func EventFindingHasher(log zerolog.Logger, getPolicy func() *policy.Policy) func(events.Event) events.Event {
	return func(e events.Event) events.Event {
		if getPolicy == nil || e.FindingsHashed {
			return e
		}
		pol := getPolicy()
		if pol == nil || pol.Data == nil || !pol.Data.HashFindings {
			return e
		}
		before := UnkeyedMatches()
		e = events.MapFindings(e, hashFindingMatches)
		e.FindingsHashed = true
		if UnkeyedMatches() != before {
			LogUnkeyedMatches(log)
		}
		return e
	}
}
