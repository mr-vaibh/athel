package cli

import (
	"io"
	"path/filepath"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/facts"
	"github.com/mr-vaibh/athel/cli/internal/investigation"
	"github.com/mr-vaibh/athel/cli/internal/observe"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

type investigationInput struct {
	inv     *investigation.Investigation
	summary string
	report  investigation.Report
}

type investigationChecked struct {
	Valid  bool                 `json:"valid"`
	Report investigation.Report `json:"report"`
}

type investigationSaved struct {
	Path   string               `json:"path"`
	Report investigation.Report `json:"report"`
}

// cmdInvestigationValidate reports what save would decide, without writing anything.
func cmdInvestigationValidate(r *runCtx, args []string) (any, error) {
	in, err := r.checkInvestigation("investigation validate", args)
	if err != nil {
		return nil, err
	}
	return investigationChecked{Valid: in.report.Err() == nil, Report: in.report}, nil
}

// cmdInvestigationSave writes investigation.md into the change folder after every check passes.
func cmdInvestigationSave(r *runCtx, args []string) (any, error) {
	in, err := r.checkInvestigation("investigation save", args)
	if err != nil {
		return nil, err
	}
	if err := in.report.Err(); err != nil {
		return nil, err
	}
	if _, err := r.store.SaveChangeFile(in.inv.Change, r.session, "investigation.md", investigation.Render(in.inv, in.summary)); err != nil {
		return nil, err
	}
	return investigationSaved{Path: filepath.Join(r.store.WS.ChangeDir(in.inv.Change), "investigation.md"), Report: in.report}, nil
}

// checkInvestigation reads the claim block (FILE, or stdin for "-") and the summary, and runs
// every check against this workspace: the change, the fact access log and the captured calls.
func (r *runCtx) checkInvestigation(name string, args []string) (investigationInput, error) {
	fs := r.newFlags(name)
	change := fs.String("change", "", "the change the answer is for (required)")
	summaryFile := fs.String("summary", "", "Markdown summary for the reader (at most 2,000 words)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return investigationInput{}, err
	}
	if err := state.CheckSession(r.session); err != nil {
		return investigationInput{}, err
	}
	data, summary, err := r.investigationFiles(pos[0], *summaryFile)
	if err != nil {
		return investigationInput{}, err
	}
	inv, err := investigation.Decode(r.registry(), data)
	if err != nil {
		return investigationInput{}, err
	}
	fstore, err := r.factStore()
	if err != nil {
		return investigationInput{}, err
	}
	c, err := r.store.Get(*change)
	if err != nil {
		return investigationInput{}, err
	}
	ctx, err := r.investigationContext(fstore, c)
	if err != nil {
		return investigationInput{}, err
	}
	return investigationInput{inv: inv, summary: summary, report: investigation.Check(inv, ctx, summary)}, nil
}

func (r *runCtx) investigationFiles(file, summaryFile string) ([]byte, string, error) {
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(r.env.Stdin)
	} else {
		data, err = readFile(file)
	}
	if err != nil {
		return nil, "", aerr.Wrap(aerr.StateIO, "read claim block "+file, err)
	}
	if summaryFile == "" {
		return data, "", nil
	}
	summary, err := readFile(summaryFile)
	if err != nil {
		return nil, "", aerr.Wrap(aerr.StateIO, "read summary "+summaryFile, err)
	}
	return data, string(summary), nil
}

// investigationContext gathers what the checks compare the answer with. Suspicious calls are
// those of this session since the change's current step started.
func (r *runCtx) investigationContext(fstore *facts.Store, c *state.Change) (investigation.Context, error) {
	used, err := fstore.UsedFor(facts.Ref{Change: c.ID})
	if err != nil {
		return investigation.Context{}, err
	}
	sinceText := c.CreatedAt
	if c.Step != nil && c.Step.Status == state.StepRunning {
		sinceText = c.Step.StartedAt
	}
	since, _ := clock.Parse(sinceText) // change@3 guarantees the timestamp format
	obs := &observe.Store{Dir: filepath.Join(r.store.WS.KnowledgeDir(), "observations"), Reg: r.registry(), Clock: r.env.Clock, TTL: observe.DefaultTTL}
	flagged, err := obs.Flagged(r.session, since)
	if err != nil {
		return investigation.Context{}, err
	}
	return investigation.Context{
		Change:   c.ID,
		Accessed: func(f string) bool { return used[f] },
		FactStatus: func(f string) (string, bool) {
			got, archived, err := fstore.Get(f)
			return got.Status, err == nil && !archived
		},
		Flagged: flagged,
	}, nil
}
