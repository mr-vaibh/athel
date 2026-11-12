package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	prdpkg "github.com/mr-vaibh/athel/cli/internal/prd"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// loadPRDIndex reads a version's index; version "" means the current one.
func (r *runCtx) loadPRDIndex(s *state.Store, id, version string) (*prdpkg.Index, *state.Change, error) {
	c, err := s.Get(id)
	if err != nil {
		return nil, nil, err
	}
	if version == "" {
		version = c.PRD.Current
	}
	b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(c.ID), "prd", version+".index.yaml")) //nolint:gosec // a change's own PRD index
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, aerr.New(aerr.StateNotFound, c.ID+" has no PRD index for "+version).
			WithNextAction("run athel prd index " + c.ID)
	}
	if err != nil {
		return nil, nil, aerr.Wrap(aerr.StateIO, "read the PRD index", err)
	}
	x, err := prdpkg.DecodeIndex(r.registry(), b)
	return x, c, err
}

// cmdPRDIndex makes sure the current PRD version has its index. Changes created before M3 have
// none; an existing index is returned as it is, never renumbered (IDs are stable).
func cmdPRDIndex(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("prd index"), args, 1)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	x, _, err := r.loadPRDIndex(s, pos[0], "")
	if err == nil {
		return x, nil
	}
	if !aerr.HasCode(err, aerr.StateNotFound) {
		return nil, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	text, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(c.ID), "prd", c.PRD.Current+".md"))
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read the PRD", err)
	}
	built := prdpkg.Build(c.ID, c.PRD.Current, digest.Bytes(text), text, nil)
	if _, err := s.SaveChangeFile(c.ID, session, "prd/"+c.PRD.Current+".index.yaml", prdpkg.Encode(built)); err != nil {
		return nil, err
	}
	return built, nil
}

func cmdPRDShow(r *runCtx, args []string) (any, error) {
	fl := r.newFlags("prd show")
	version := fl.String("version", "", "PRD version (default: the current one)")
	pos, err := parse(fl, args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	x, _, err := r.loadPRDIndex(s, pos[0], *version)
	return x, err
}

type prdScan struct {
	Version   string           `json:"version"`
	Ambiguous []prdpkg.Finding `json:"ambiguous"`
	Limits    []prdpkg.Finding `json:"limits"`
}

// cmdPRDScan lists vague words and numeric limits for the architect's review (FR-SPEC-002).
func cmdPRDScan(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("prd scan"), args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	x, _, err := r.loadPRDIndex(s, pos[0], "")
	if err != nil {
		return nil, err
	}
	amb, lim := prdpkg.Scan(*x)
	return prdScan{Version: x.Version, Ambiguous: amb, Limits: lim}, nil
}

type prdAdded struct {
	Version string      `json:"version"`
	Diff    prdpkg.Diff `json:"diff"`
}

// cmdPRDAddVersion stores a new PRD version whose unchanged text keeps its IDs, and reports
// what changed (doc 05 edge case "PRD changes mid-build").
func cmdPRDAddVersion(r *runCtx, args []string) (any, error) {
	fl := r.newFlags("prd add-version")
	file := fl.String("prd-file", "", "the new PRD from this file")
	text := fl.String("prd-text", "", "the new PRD as text")
	stdin := fl.Bool("prd-stdin", false, "read the new PRD from stdin")
	pos, err := parse(fl, args, 1)
	if err != nil {
		return nil, err
	}
	body, source, err := readPRD(r, *file, *text, *stdin)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	prev, c, err := r.loadPRDIndex(s, pos[0], "")
	if err != nil {
		return nil, err
	}
	next := state.NextPRDVersion(c)
	built := prdpkg.Build(c.ID, next, digest.Bytes(body), body, prev)
	if _, err := s.AddPRDVersion(c.ID, session, next, body, source, prdpkg.Encode(built)); err != nil {
		return nil, err
	}
	return prdAdded{Version: next, Diff: prdpkg.Compare(*prev, built)}, nil
}

func cmdPRDDiff(r *runCtx, args []string) (any, error) {
	fl := r.newFlags("prd diff")
	from := fl.String("from", "", "older version (required)")
	to := fl.String("to", "", "newer version (default: the current one)")
	pos, err := parse(fl, args, 1)
	if err != nil {
		return nil, err
	}
	if *from == "" {
		return nil, usagef("prd diff: --from is required")
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	a, _, err := r.loadPRDIndex(s, pos[0], *from)
	if err != nil {
		return nil, err
	}
	b, _, err := r.loadPRDIndex(s, pos[0], *to)
	if err != nil {
		return nil, err
	}
	return prdpkg.Compare(*a, *b), nil
}

type reviewSaved struct {
	Path    string   `json:"path"`
	Missing []string `json:"missing"`
}

// cmdPRDReviewSave stores the architect's review of the current PRD version, after the schema and
// every reference check pass (FR-SPEC-001/002). The block comes on stdin ("-") or from a file.
func cmdPRDReviewSave(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("prd review save"), args, 2)
	if err != nil {
		return nil, err
	}
	data, err := r.readBlock(pos[1])
	if err != nil {
		return nil, err
	}
	rev, err := prdpkg.DecodeReview(r.registry(), data)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	x, c, err := r.loadPRDIndex(s, pos[0], "")
	if err != nil {
		return nil, err
	}
	if p := prdpkg.CheckReview(rev, *x); len(p) > 0 {
		return nil, aerr.New(aerr.StateInvalid, "PRD review rejected: "+strings.Join(p, "; ")).
			WithNextAction("fix the review block and save again")
	}
	name := "prd/review-" + x.Version + ".md"
	if _, err := s.SaveChangeFile(c.ID, session, name, prdpkg.RenderReview(rev, data)); err != nil {
		return nil, err
	}
	missing := rev.Missing()
	if missing == nil {
		missing = []string{}
	}
	return reviewSaved{Path: filepath.Join(s.WS.ChangeDir(c.ID), name), Missing: missing}, nil
}

// readBlock reads a YAML block from stdin ("-") or a file.
func (r *runCtx) readBlock(from string) ([]byte, error) {
	var data []byte
	var err error
	if from == "-" {
		data, err = io.ReadAll(r.env.Stdin)
	} else {
		data, err = readFile(from)
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read "+from, err)
	}
	return data, nil
}
