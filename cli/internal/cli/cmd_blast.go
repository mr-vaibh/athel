package cli

import (
	"io"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	"github.com/mr-vaibh/athel/cli/internal/blast"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/facts"
	"github.com/mr-vaibh/athel/cli/internal/grants"
	"github.com/mr-vaibh/athel/cli/internal/schema"
	"github.com/mr-vaibh/athel/cli/internal/state"
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
	if ref.Change != "" && rep.Mode == "planned" {
		if err := r.checkPlannedReport(rep); err != nil {
			return nil, err
		}
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

type signoffFile struct {
	Schema       string `yaml:"schema" json:"schema"`
	Change       string `yaml:"change" json:"change"`
	ReportSHA256 string `yaml:"report_sha256" json:"report_sha256"`
	Risk         string `yaml:"risk" json:"risk"`
	By           string `yaml:"by" json:"by"`
	At           string `yaml:"at" json:"at"`
	Grant        string `yaml:"grant" json:"grant"`
}

// cmdBlastSignoff records the developer's sign-off of the change's blast radius report, bound to
// its hash, only with their own /athel:signoff grant (doc 05: CRITICAL needs sign-off on the
// report itself in addition to plan approval; ADR 019's mechanism).
func cmdBlastSignoff(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("blast signoff"), args, 1)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	md, err := r.readChangeFile(s, pos[0], "blast-radius.md")
	if err != nil {
		return nil, err
	}
	risk, err := r.blastRisk(s, pos[0])
	if err != nil {
		return nil, err
	}
	g, err := s.Grants().Consume(pos[0], grants.TargetSignoff, session)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, aerr.New(aerr.PolicyHumanOrigin, "signing off "+pos[0]+"'s blast radius needs the developer's own prompt /athel:signoff "+pos[0]+" in this session").
			WithNextAction("ask the developer to read blast-radius.md and type /athel:signoff " + pos[0])
	}
	out := signoffFile{Schema: schema.BlastSignoff, Change: pos[0], ReportSHA256: digest.Bytes(md), Risk: risk, By: state.ByHuman,
		At: clock.Format(r.env.Clock.Now()), Grant: g.ID}
	b, _ := yaml.Marshal(out) // strings only: always encodes
	if err := r.registry().ValidateYAML(schema.BlastSignoff, b); err != nil {
		return nil, err
	}
	if _, err := s.AuditLog().Append(audit.Record{Kind: audit.KindGrantUsed, Session: audit.Str(session), Change: audit.Str(pos[0]),
		To: audit.Str(grants.TargetSignoff), By: audit.Str(state.ByHuman), Reason: audit.Str("grant " + g.ID + " for blast-radius.md " + out.ReportSHA256), Result: audit.ResultOK}); err != nil {
		return nil, err
	}
	if _, err := s.SaveChangeFile(pos[0], session, "blast-signoff.yaml", b); err != nil {
		return nil, err
	}
	return out, nil
}

// signedOff reports whether the current blast radius report carries the developer's sign-off.
func (r *runCtx) signedOff(s *state.Store, id string) (bool, error) {
	b, err := r.readChangeFile(s, id, "blast-signoff.yaml")
	if aerr.HasCode(err, aerr.StateNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := r.registry().ValidateYAML(schema.BlastSignoff, b); err != nil {
		return false, err
	}
	var so signoffFile
	_ = yaml.Unmarshal(b, &so) // validated above
	md, err := r.readChangeFile(s, id, "blast-radius.md")
	if err != nil {
		return false, err
	}
	return so.ReportSHA256 == digest.Bytes(md), nil
}

// checkPlannedReport applies the planned report's spec links (CheckPlanned) on a change.
func (r *runCtx) checkPlannedReport(rep *blast.Report) error {
	specs, err := r.loadSpecs(r.store, rep.Change)
	if err != nil {
		return err
	}
	var reqs []string
	for _, sp := range specs {
		for _, q := range sp.Requirements {
			reqs = append(reqs, q.ID)
		}
	}
	if p := blast.CheckPlanned(rep, reqs); len(p) > 0 {
		return aerr.New(aerr.StateInvalid, "planned blast radius rejected: "+strings.Join(p, "; ")).
			WithNextAction("link each changed item to its requirement and list what was ruled out, then save again")
	}
	return nil
}
