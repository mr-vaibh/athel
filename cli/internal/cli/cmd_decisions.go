package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/decisions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// editDecisions runs fn on the change's decisions.yaml under the change lock and stores the
// result (doc 04: change files are written only through the CLI).
func (r *runCtx) editDecisions(id string, fn func(f *decisions.File) error) error {
	s, session, err := r.mutating()
	if err != nil {
		return err
	}
	_, err = s.UpdateChangeFile(id, session, "decisions.yaml", func(old []byte) ([]byte, error) {
		f, err := decisions.Decode(r.registry(), id, old)
		if err != nil {
			return nil, err
		}
		if err := fn(f); err != nil {
			return nil, err
		}
		return f.Encode(r.registry())
	})
	return err
}

// readDecisions reads a change's decisions.yaml without a lock (read-only commands).
func (r *runCtx) readDecisions(id string) (*decisions.File, error) {
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	c, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(c.ID), "decisions.yaml"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, aerr.Wrap(aerr.StateIO, "read decisions.yaml", err)
	}
	return decisions.Decode(r.registry(), c.ID, b)
}

// decodeStrict reads one YAML document into v, refusing unknown fields.
func decodeStrict(data []byte, v any, what string) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return aerr.Wrap(aerr.StateSchema, "decode "+what, err)
	}
	return nil
}

// cmdDecideAdd raises a decision card (FR-DEC-001/002). The card comes on stdin ("-") or from a
// file as a draft: a card that sets id, kind, presented_at, answer or feedback is refused, since
// the CLI owns those and only the human answers.
func cmdDecideAdd(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("decide add"), args, 2)
	if err != nil {
		return nil, err
	}
	data, err := r.readBlock(pos[1])
	if err != nil {
		return nil, err
	}
	var c decisions.Draft
	if err := decodeStrict(data, &c, "card"); err != nil {
		return nil, err
	}
	var added *decisions.Card
	err = r.editDecisions(pos[0], func(f *decisions.File) error {
		added, err = f.Add(c)
		return err
	})
	return added, err
}

// cmdDecideRecord records a choice Athel made alone, with its reason (doc 05 §Decide alone).
func cmdDecideRecord(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("decide record"), args, 2)
	if err != nil {
		return nil, err
	}
	data, err := r.readBlock(pos[1])
	if err != nil {
		return nil, err
	}
	var d decisions.DecidedDraft
	if err := decodeStrict(data, &d, "decision"); err != nil {
		return nil, err
	}
	var rec *decisions.Decided
	err = r.editDecisions(pos[0], func(f *decisions.File) error {
		rec = f.Record(d, clock.Format(r.env.Clock.Now()))
		return nil
	})
	return rec, err
}

type decisionList struct {
	Decisions []decisions.Entry `json:"decisions"`
}

func cmdDecideList(r *runCtx, args []string) (any, error) {
	fl := r.newFlags("decide list")
	open := fl.Bool("open", false, "only unanswered cards")
	step := fl.String("step", "", "only this step's cards")
	pos, err := parse(fl, args, 1)
	if err != nil {
		return nil, err
	}
	f, err := r.readDecisions(pos[0])
	if err != nil {
		return nil, err
	}
	out := decisionList{Decisions: []decisions.Entry{}}
	for _, e := range f.Decisions {
		keep := !*open || (e.Card != nil && e.Card.Answer == nil)
		keep = keep && (*step == "" || (e.Card != nil && e.Card.Step == *step) || (e.Decided != nil && e.Decided.Step == *step))
		if keep {
			out.Decisions = append(out.Decisions, e)
		}
	}
	return out, nil
}

func cmdDecideShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("decide show"), args, 2)
	if err != nil {
		return nil, err
	}
	f, err := r.readDecisions(pos[0])
	if err != nil {
		return nil, err
	}
	for _, e := range f.Decisions {
		if e.ID() == pos[1] {
			return e, nil
		}
	}
	return nil, aerr.New(aerr.StateNotFound, pos[0]+" has no decision "+pos[1])
}

type cardBatch struct {
	Step  string            `json:"step"`
	Cards []*decisions.Card `json:"cards"`
}

// cmdDecidePresent returns a step's open cards as one batch and marks them presented
// (FR-DEC-003: one interruption per step unless a new blocker appears).
func cmdDecidePresent(r *runCtx, args []string) (any, error) {
	fl := r.newFlags("decide present")
	step := fl.String("step", "", "the step whose cards to present (required)")
	pos, err := parse(fl, args, 1)
	if err != nil {
		return nil, err
	}
	if *step == "" {
		return nil, usagef("decide present: --step is required")
	}
	out := cardBatch{Step: *step, Cards: []*decisions.Card{}}
	err = r.editDecisions(pos[0], func(f *decisions.File) error {
		batch, err := f.Present(*step, clock.Format(r.env.Clock.Now()))
		if batch != nil {
			out.Cards = batch
		}
		return err
	})
	return out, err
}
