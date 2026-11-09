package facts

import (
	"slices"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/ids"
)

// Verification results.
const (
	ResultVerified   = "verified"   // every step matched: active again, verified now
	ResultUnverified = "unverified" // a source was unreachable: the fact is unchanged
	ResultDisputed   = "disputed"   // a step contradicted the fact
)

// VerifyResult is what a verification found and did.
type VerifyResult struct {
	Fact   Fact         `json:"fact"`
	Result string       `json:"result"`
	Steps  []StepResult `json:"steps"`
}

// Verify re-derives a fact with its recipe (doc 03 verification rules 2 and 3): all steps must
// match to keep it active; one mismatch makes it disputed; an unreachable source leaves it
// unchanged. With a ref, the verification is also logged as a use for a decision.
func (s *Store) Verify(id string, check func(Step, int) StepResult, ref *Ref, session string) (VerifyResult, error) {
	if _, err := ids.Fact.Parse(id); err != nil {
		return VerifyResult{}, err
	}
	if ref != nil {
		if err := ref.validate(); err != nil {
			return VerifyResult{}, err
		}
	}
	var out VerifyResult
	err := s.locked(func() error {
		recs, _, err := readRecords(s.factsPath())
		if err != nil {
			return err
		}
		f := lookup(current(recs), id)
		if f == nil {
			return aerr.New(aerr.StateNotFound, "no active fact "+id)
		}
		if f.Status == Superseded {
			return aerr.New(aerr.StateFactDisputed, "fact "+id+" has been superseded; verify its replacement")
		}
		out = VerifyResult{Fact: *f, Steps: make([]StepResult, 0, len(f.Verify))}
		for i, step := range f.Verify {
			out.Steps = append(out.Steps, check(step, i))
		}
		now := s.Clock.Now()
		if err := s.settle(&out, clock.Format(now)); err != nil {
			return err
		}
		if ref == nil {
			return nil
		}
		v, err := s.view(out.Fact, now)
		if err != nil {
			return err
		}
		return s.logAccess(access{Schema: AccessSchemaID, At: clock.Format(now), Fact: id, Change: ref.Change, Ask: ref.Ask,
			Purpose: Decision, Result: accessResult[out.Result], AgeS: v.AgeS, Session: optional(session)})
	})
	return out, err
}

var accessResult = map[string]string{ResultVerified: Verified, ResultUnverified: Unverified, ResultDisputed: RefusedDisputed}

// settle decides the result from the step outcomes and records the fact's new state.
func (s *Store) settle(out *VerifyResult, now string) error {
	outcomes := make([]string, 0, len(out.Steps))
	for _, st := range out.Steps {
		outcomes = append(outcomes, st.Outcome)
	}
	switch {
	case slices.Contains(outcomes, Mismatch):
		out.Result, out.Fact.Status = ResultDisputed, Disputed
	case slices.Contains(outcomes, Unreachable):
		out.Result = ResultUnverified
		return nil
	default:
		out.Result, out.Fact.Status, out.Fact.VerifiedAt = ResultVerified, Active, now
	}
	b, err := s.encode(SchemaID, out.Fact)
	if err != nil {
		return err
	}
	return fsutil.AppendLine(s.factsPath(), b)
}

// Dependents lists the changes whose work used a fact, from the access log, sorted. The CLI
// flags the open ones when the fact is disputed or superseded (FR-DISC-005).
func (s *Store) Dependents(id string) ([]string, error) {
	out := []string{}
	err := s.locked(func() error {
		lines, err := readLines(s.accessPath())
		if err != nil {
			return err
		}
		for _, line := range lines {
			a, err := decodeAccess(line)
			if err != nil {
				return err
			}
			if a.Fact == id && a.Change != "" && !slices.Contains(out, a.Change) {
				out = append(out, a.Change)
			}
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}
