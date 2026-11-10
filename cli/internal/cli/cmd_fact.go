package cli

import (
	"io"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/facts"
)

// factStore returns the workspace's knowledge cache, with the freshness windows of the
// effective config.
func (r *runCtx) factStore() (*facts.Store, error) {
	ws, err := r.workspaceDir()
	if err != nil {
		return nil, err
	}
	cfg, err := r.config()
	if err != nil {
		return nil, err
	}
	set := cfg.Settings()
	return &facts.Store{Dir: ws.KnowledgeDir(), Reg: r.registry(), Clock: r.env.Clock,
		Windows: set.FactWindows, ArchiveAfter: set.FactArchiveAfter}, nil
}

type factAdded struct {
	Fact    facts.Fact `json:"fact"`
	Created bool       `json:"created"`
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
	return factAdded{Fact: f, Created: created}, nil
}

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
