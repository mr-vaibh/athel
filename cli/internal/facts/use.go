package facts

import (
	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/ids"
)

// Purposes of a fact use (doc 03 verification rule 1).
const (
	Decision    = "decision"    // drives a decision or a write: must be fresh or re-verified
	Orientation = "orientation" // shown with its age only
)

// Results recorded in the access log.
const (
	InWindow        = "in_window"
	Verified        = "verified"
	Unverified      = "unverified"
	RefusedStale    = "refused_stale"
	RefusedDisputed = "refused_disputed"
)

// Ref names what a fact is used for: exactly one change or one ad-hoc question.
type Ref struct {
	Change string
	Ask    string
}

func (r Ref) validate() error {
	switch {
	case (r.Change == "") == (r.Ask == ""):
		return aerr.New(aerr.StateInvalid, "a fact use names exactly one change or one ad-hoc question")
	case r.Change != "":
		_, err := ids.ParseChange(r.Change)
		return err
	default:
		_, err := ids.Ask.Parse(r.Ask)
		return err
	}
}

// UseResult is the fact as used and how the use was recorded.
type UseResult struct {
	View
	Result string `json:"result"`
}

// access is one fact-access@1 record.
type access struct {
	Schema  string  `json:"schema"`
	At      string  `json:"at"`
	Fact    string  `json:"fact"`
	Change  string  `json:"change,omitempty"`
	Ask     string  `json:"ask,omitempty"`
	Purpose string  `json:"purpose"`
	Result  string  `json:"result"`
	AgeS    int64   `json:"age_s"`
	Session *string `json:"session"`
}

// Use is the only way a fact is read for work, so the access log is complete (FR-DISC-004). A
// decision use needs an active fact within its window; anything else is refused, and the
// refusal is logged too. An orientation use is always allowed and shows the fact's age.
func (s *Store) Use(id, purpose string, ref Ref, session string) (UseResult, error) {
	if _, err := ids.Fact.Parse(id); err != nil {
		return UseResult{}, err
	}
	if purpose != Decision && purpose != Orientation {
		return UseResult{}, aerr.New(aerr.StateInvalid, "purpose must be decision or orientation, not "+purpose)
	}
	if err := ref.validate(); err != nil {
		return UseResult{}, err
	}
	var out UseResult
	var refusal error
	err := s.locked(func() error {
		recs, _, err := readRecords(s.factsPath())
		if err != nil {
			return err
		}
		f := lookup(current(recs), id)
		if f == nil {
			return aerr.New(aerr.StateNotFound, "no active fact "+id).WithNextAction("investigate the subject again")
		}
		now := s.Clock.Now()
		v, err := s.view(*f, now)
		if err != nil {
			return err
		}
		out.View = v
		out.Result, refusal = judge(v, purpose)
		return s.logAccess(access{
			Schema: AccessSchemaID, At: clock.Format(now), Fact: id, Change: ref.Change, Ask: ref.Ask,
			Purpose: purpose, Result: out.Result, AgeS: v.AgeS, Session: optional(session),
		})
	})
	if err != nil {
		return UseResult{}, err
	}
	return out, refusal
}

// judge applies doc 03 verification rule 1 to one use.
func judge(v View, purpose string) (string, error) {
	switch {
	case v.Fresh:
		return InWindow, nil
	case purpose == Orientation:
		return Unverified, nil
	case v.Status != Active:
		return RefusedDisputed, aerr.New(aerr.StateFactDisputed, "fact "+v.ID+" is "+v.Status+" and cannot drive a decision").
			WithNextAction("investigate the subject again; a new verified fact supersedes this one")
	default:
		return RefusedStale, aerr.New(aerr.StateFactStale, "fact "+v.ID+" was verified outside its "+v.FreshnessClass+" window").
			WithNextAction("re-verify it with its recipe (athel fact verify " + v.ID + ") before using it for a decision")
	}
}

func (s *Store) logAccess(a access) error {
	b, err := s.encode(AccessSchemaID, a)
	if err != nil {
		return err
	}
	return fsutil.AppendLine(s.accessPath(), b)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
