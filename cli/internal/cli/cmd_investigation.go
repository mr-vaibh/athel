package cli

import (
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/asks"
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

// cmdInvestigationTemplate prints a valid claim block listing every allowed value, for the scout
// to fill in.
func cmdInvestigationTemplate(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("investigation template"), args, 0); err != nil {
		return nil, err
	}
	return rawOutput(strings.TrimRight(investigation.Template, "\n")), nil
}

// cmdInvestigationValidate reports what save would decide, without writing anything.
func cmdInvestigationValidate(r *runCtx, args []string) (any, error) {
	in, err := r.checkInvestigation("investigation validate", args)
	if err != nil {
		return nil, err
	}
	return investigationChecked{Valid: in.report.Err() == nil, Report: in.report}, nil
}

// cmdInvestigationSave writes investigation.md into the change folder (under the change lock)
// or the ask folder after every check passes.
func cmdInvestigationSave(r *runCtx, args []string) (any, error) {
	in, err := r.checkInvestigation("investigation save", args)
	if err != nil {
		return nil, err
	}
	if err := in.report.Err(); err != nil {
		return nil, err
	}
	md := investigation.Render(in.inv, in.summary)
	if in.inv.Ask != "" {
		p, err := r.askStore().Write(in.inv.Ask, "investigation.md", md)
		if err != nil {
			return nil, err
		}
		return investigationSaved{Path: p, Report: in.report}, nil
	}
	if _, err := r.store.SaveChangeFile(in.inv.Change, r.session, "investigation.md", md); err != nil {
		return nil, err
	}
	return investigationSaved{Path: filepath.Join(r.store.WS.ChangeDir(in.inv.Change), "investigation.md"), Report: in.report}, nil
}

// checkInvestigation reads the claim block (FILE, or stdin for "-") and the summary, and runs
// every check against this workspace: the change or ask, the fact access log and the captured
// calls.
func (r *runCtx) checkInvestigation(name string, args []string) (investigationInput, error) {
	fs := r.newFlags(name)
	var ref facts.Ref
	fs.StringVar(&ref.Change, "change", "", "the change the answer is for")
	fs.StringVar(&ref.Ask, "ask", "", "the ad-hoc question the answer is for")
	summaryFile := fs.String("summary", "", "file with the Markdown summary for the reader (at most 2,000 words)")
	summaryText := fs.String("summary-text", "", "the summary itself, for callers that may not write files (the scout)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return investigationInput{}, err
	}
	if (ref.Change == "") == (ref.Ask == "") {
		return investigationInput{}, usagef("%s: name exactly one of --change or --ask", name)
	}
	if *summaryFile != "" && *summaryText != "" {
		return investigationInput{}, usagef("%s: give --summary or --summary-text, not both", name)
	}
	if err := state.CheckSession(r.session); err != nil {
		return investigationInput{}, err
	}
	data, summary, err := r.investigationFiles(pos[0], *summaryFile)
	if err != nil {
		return investigationInput{}, err
	}
	if *summaryText != "" {
		summary = *summaryText
	}
	inv, err := investigation.Decode(r.registry(), data)
	if err != nil {
		return investigationInput{}, err
	}
	fstore, err := r.factStore()
	if err != nil {
		return investigationInput{}, err
	}
	since, err := r.investigationStart(ref)
	if err != nil {
		return investigationInput{}, err
	}
	ctx, err := r.investigationContext(fstore, ref, since)
	if err != nil {
		return investigationInput{}, err
	}
	return investigationInput{inv: inv, summary: summary, report: investigation.Check(inv, ctx, summary)}, nil
}

// investigationStart is when the work behind the answer began: the ask's creation, or the
// change's running step (else the change's creation). Suspicious calls since then must be listed.
func (r *runCtx) investigationStart(ref facts.Ref) (time.Time, error) {
	text := ""
	if ref.Ask != "" {
		a, err := r.askStore().Get(ref.Ask)
		if err != nil {
			return time.Time{}, err
		}
		text = a.CreatedAt
	} else {
		c, err := r.store.Get(ref.Change)
		if err != nil {
			return time.Time{}, err
		}
		text = c.CreatedAt
		if c.Step != nil && c.Step.Status == state.StepRunning {
			text = c.Step.StartedAt
		}
	}
	since, _ := clock.Parse(text) // ask@1 and change@3 guarantee the timestamp format
	return since, nil
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

// investigationContext gathers what the checks compare the answer with.
func (r *runCtx) investigationContext(fstore *facts.Store, ref facts.Ref, since time.Time) (investigation.Context, error) {
	used, err := fstore.UsedFor(ref)
	if err != nil {
		return investigation.Context{}, err
	}
	obs := &observe.Store{Dir: filepath.Join(r.store.WS.KnowledgeDir(), "observations"), Reg: r.registry(), Clock: r.env.Clock, TTL: observe.DefaultTTL}
	flagged, err := obs.Flagged(r.session, since)
	if err != nil {
		return investigation.Context{}, err
	}
	return investigation.Context{
		Change: ref.Change, Ask: ref.Ask,
		Accessed: func(f string) bool { return used[f] },
		FactStatus: func(f string) (string, bool) {
			got, archived, err := fstore.Get(f)
			return got.Status, err == nil && !archived
		},
		Flagged: flagged,
	}, nil
}

// askStore is the workspace's ad-hoc questions; it runs after factStore loaded the workspace.
func (r *runCtx) askStore() asks.Store {
	return asks.Store{Dir: filepath.Join(r.store.WS.Dir(), "asks"), Reg: r.registry(), Clock: r.env.Clock}
}
