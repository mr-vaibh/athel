package cli

import (
	"io"
	"path/filepath"

	"github.com/mr-vaibh/athel/cli/internal/blast"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/facts"
)

type blastScored struct {
	Risk       string        `json:"risk"`
	RulesFired []blast.Fired `json:"rules_fired"`
	Path       string        `json:"path,omitempty"`
}

// cmdBlastScore reports the risk of a blast@1 report without saving it (FR-BLAST-004).
func cmdBlastScore(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("blast score"), args, 1)
	if err != nil {
		return nil, err
	}
	rep, err := r.scoredBlast(pos[0])
	if err != nil {
		return nil, err
	}
	return blastScored{Risk: *rep.Risk, RulesFired: rep.RulesFired}, nil
}

// cmdBlastSave scores a report and writes blast-radius.md into the change or ask folder. The
// risk and the rules fired are always the CLI's own, whatever the input says.
func cmdBlastSave(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("blast save")
	var ref facts.Ref
	fs.StringVar(&ref.Change, "change", "", "the change the report is for")
	fs.StringVar(&ref.Ask, "ask", "", "the ad-hoc request the report is for")
	summary := fs.String("summary-text", "", "a short summary for the reader")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	if (ref.Change == "") == (ref.Ask == "") {
		return nil, usagef("blast save: name exactly one of --change or --ask")
	}
	rep, err := r.scoredBlast(pos[0])
	if err != nil {
		return nil, err
	}
	if _, err := r.storeFor(); err != nil {
		return nil, err
	}
	if rep.Change != ref.Change || rep.Ask != ref.Ask {
		return nil, aerr.New(aerr.StateInvalid, "the report is for "+rep.Change+rep.Ask+", not "+ref.Change+ref.Ask)
	}
	md := blast.Render(rep, *summary)
	out := blastScored{Risk: *rep.Risk, RulesFired: rep.RulesFired}
	if ref.Ask != "" {
		out.Path, err = r.askStore().Write(ref.Ask, "blast-radius.md", md)
		return out, err
	}
	if _, err := r.store.SaveChangeFile(ref.Change, r.session, "blast-radius.md", md); err != nil {
		return nil, err
	}
	out.Path = filepath.Join(r.store.WS.ChangeDir(ref.Change), "blast-radius.md")
	return out, nil
}

// scoredBlast reads a report (FILE, or stdin for "-") and scores it with the effective blast.yaml
// (the workspace's project layer when there is one).
func (r *runCtx) scoredBlast(file string) (*blast.Report, error) {
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(r.env.Stdin)
	} else {
		data, err = readFile(file)
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read blast radius report "+file, err)
	}
	rep, err := blast.Decode(r.registry(), data)
	if err != nil {
		return nil, err
	}
	paths, err := r.configPaths()
	if err != nil {
		return nil, err
	}
	rules, err := blast.LoadRules(paths, r.registry())
	if err != nil {
		return nil, err
	}
	level, fired := blast.Score(rep, rules)
	rep.Risk, rep.RulesFired = &level, fired
	return rep, nil
}
