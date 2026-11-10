package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/facts"
	"github.com/mr-vaibh/athel/cli/internal/observe"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// factStore returns the workspace's knowledge cache, with the freshness windows of the
// effective config. It loads the change store too, which flagging dependent changes uses.
func (r *runCtx) factStore() (*facts.Store, error) {
	st, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	set := r.cfg.Settings()
	return &facts.Store{Dir: st.WS.KnowledgeDir(), Reg: r.registry(), Clock: r.env.Clock,
		Windows: set.FactWindows, ArchiveAfter: set.FactArchiveAfter}, nil
}

type factAdded struct {
	Fact    facts.Fact `json:"fact"`
	Created bool       `json:"created"`
	Flagged []string   `json:"flagged"` // open changes that used the fact this one supersedes
}

// cmdFactAdd stores a fact draft read from FILE, or from stdin when FILE is "-".
func cmdFactAdd(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("fact add"), args, 1)
	if err != nil {
		return nil, err
	}
	var data []byte
	if pos[0] == "-" {
		data, err = io.ReadAll(r.env.Stdin)
	} else {
		data, err = readFile(pos[0])
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read fact draft "+pos[0], err)
	}
	d, err := facts.DecodeDraft(data)
	if err != nil {
		return nil, err
	}
	s, err := r.factStore()
	if err != nil {
		return nil, err
	}
	f, created, err := s.Add(d)
	if err != nil {
		return nil, err
	}
	out := factAdded{Fact: f, Created: created, Flagged: []string{}}
	if created && f.Supersedes != nil {
		out.Flagged, err = r.flagDependents(s, state.Flag{Kind: state.FlagFactSuperseded, Fact: *f.Supersedes,
			Replacement: &f.ID, Reason: "superseded by " + f.ID})
	}
	return out, err
}

// flagDependents flags every open change that used the fact (FR-DISC-005) and returns the
// changes that got a new flag. A change that no longer exists is skipped.
func (r *runCtx) flagDependents(s *facts.Store, flag state.Flag) ([]string, error) {
	deps, err := s.Dependents(flag.Fact)
	if err != nil {
		return []string{}, err
	}
	flag.At = clock.Format(r.env.Clock.Now())
	flagged := []string{}
	for _, id := range deps {
		added, err := r.store.FlagFact(id, flag)
		if aerr.HasCode(err, aerr.StateNotFound) {
			continue
		}
		if err != nil {
			return flagged, err
		}
		if added {
			flagged = append(flagged, id)
		}
	}
	return flagged, nil
}

// observedFlag collects --observed STEP=TOOL_USE_ID: the scout's tool call that ran recipe step
// STEP, whose captured output the CLI judges.
type observedFlag map[int]string

func (o observedFlag) String() string { return fmt.Sprint(map[int]string(o)) }

func (o observedFlag) Set(v string) error {
	step, id, ok := strings.Cut(v, "=")
	n, err := strconv.Atoi(step)
	if !ok || err != nil || n < 0 || id == "" {
		return fmt.Errorf("want STEP=TOOL_USE_ID, got %q", v)
	}
	o[n] = id
	return nil
}

type factVerified struct {
	facts.VerifyResult
	Flagged []string `json:"flagged"` // open changes that used the fact, when it is disputed
}

// cmdFactVerify re-derives a fact with its recipe (doc 03 verification rules 2 and 3).
func cmdFactVerify(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("fact verify")
	obs := observedFlag{}
	fs.Var(obs, "observed", "STEP=TOOL_USE_ID: the scout's call that ran a recipe step (repeatable)")
	var ref facts.Ref
	fs.StringVar(&ref.Change, "change", "", "log the verification as a decision use for this change")
	fs.StringVar(&ref.Ask, "ask", "", "log the verification as a decision use for this ad-hoc question")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.factStore()
	if err != nil {
		return nil, err
	}
	recipe, err := r.recipe(obs)
	if err != nil {
		return nil, err
	}
	var refp *facts.Ref
	if ref.Change != "" || ref.Ask != "" {
		refp = &ref
	}
	res, err := s.Verify(pos[0], recipe.Check, refp, r.session)
	if err != nil {
		return nil, err
	}
	out := factVerified{VerifyResult: res, Flagged: []string{}}
	if res.Result == facts.ResultDisputed {
		out.Flagged, err = r.flagDependents(s, state.Flag{Kind: state.FlagFactDisputed, Fact: res.Fact.ID, Reason: mismatches(res.Steps)})
	}
	return out, err
}

func mismatches(steps []facts.StepResult) string {
	var parts []string
	for _, st := range steps {
		if st.Outcome == facts.Mismatch {
			parts = append(parts, fmt.Sprintf("step %d %s %s: %s", st.Step, st.Tool, st.Action, st.Detail))
		}
	}
	return "verification contradicted the fact: " + strings.Join(parts, "; ")
}

// recipe wires the recipe checker to this workspace: captured scout calls, the policy's tool
// registry to tell which tool a captured call used, and the repo index for git steps. It runs
// after factStore, which loaded the workspace.
func (r *runCtx) recipe(obs observedFlag) (facts.Recipe, error) {
	pol, err := r.policySet()
	if err != nil {
		return facts.Recipe{}, err
	}
	resolve, err := r.repoResolver()
	if err != nil {
		return facts.Recipe{}, err
	}
	store := &observe.Store{Dir: filepath.Join(r.store.WS.KnowledgeDir(), "observations"), Reg: r.registry(), Clock: r.env.Clock, TTL: observe.DefaultTTL}
	return facts.Recipe{
		Repo: resolve, Observation: store.Get, Observed: obs,
		ToolsOf: func(tool string, input []byte) []string {
			res := classify.Classify(classify.Call{Tool: tool, Input: json.RawMessage(input)}, pol, classify.Options{ReadFile: noFiles})
			var out []string
			for _, a := range res.Actions {
				if a.ToolID != "" {
					out = append(out, a.ToolID)
				}
			}
			return out
		},
	}, nil
}

// noFiles stops the classifier from reading scripts while it names the tools of a captured call.
func noFiles(string) ([]byte, error) { return nil, errors.New("not read while verifying") }

type factList struct {
	Facts []facts.View `json:"facts"`
}

func cmdFactFind(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("fact find")
	var q facts.Query
	fs.StringVar(&q.SubjectKind, "subject-kind", "", "subject kind, e.g. service or event")
	fs.StringVar(&q.Subject, "subject", "", "subject name")
	fs.StringVar(&q.Predicate, "predicate", "", "predicate, e.g. consumed_by")
	fs.StringVar(&q.Object, "object", "", "object name")
	fs.StringVar(&q.Env, "env", "", "environment, or any")
	fs.BoolVar(&q.All, "all", false, "include superseded facts")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	s, err := r.factStore()
	if err != nil {
		return nil, err
	}
	v, err := s.Find(q)
	if err != nil {
		return nil, err
	}
	return factList{Facts: v}, nil
}

type factShown struct {
	Fact     facts.Fact `json:"fact"`
	Archived bool       `json:"archived"`
}

func cmdFactShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("fact show"), args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.factStore()
	if err != nil {
		return nil, err
	}
	f, archived, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	return factShown{Fact: f, Archived: archived}, nil
}

// cmdFactUse reads a fact for work and logs the use (FR-DISC-004). A refused decision use is
// logged and reported as an error that names the fact and the step that makes it usable.
func cmdFactUse(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("fact use")
	purpose := fs.String("purpose", "", "decision or orientation")
	var ref facts.Ref
	fs.StringVar(&ref.Change, "change", "", "the change the fact is used for")
	fs.StringVar(&ref.Ask, "ask", "", "the ad-hoc question the fact is used for")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.factStore()
	if err != nil {
		return nil, err
	}
	return s.Use(pos[0], *purpose, ref, r.session)
}

type factArchived struct {
	DryRun   bool     `json:"dry_run"`
	Archived []string `json:"archived"`
}

func cmdFactArchive(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("fact archive")
	dry := fs.Bool("dry-run", false, "list the facts that would be archived")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	s, err := r.factStore()
	if err != nil {
		return nil, err
	}
	ids, err := s.Archive(*dry)
	if err != nil {
		return nil, err
	}
	return factArchived{DryRun: *dry, Archived: ids}, nil
}
