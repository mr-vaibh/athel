// Package asks keeps ad-hoc questions: questions and blast-radius requests asked outside a
// change (UC-01, UC-02; M2 plan G3). An ask has no lifecycle; it is a folder holding the
// question and, once answered, its investigation.md or blast-radius.md.
package asks

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/ids"
	"github.com/mr-vaibh/athel/cli/internal/lock"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// SchemaID is the schema of ask.yaml.
const SchemaID = schema.Ask

// Kinds of ask.
const (
	Question = "question"
	Blast    = "blast"
)

// Ask is one ask.yaml.
type Ask struct {
	Schema    string  `yaml:"schema" json:"schema"`
	ID        string  `yaml:"id" json:"id"`
	Kind      string  `yaml:"kind" json:"kind"`
	Question  string  `yaml:"question" json:"question"`
	CreatedAt string  `yaml:"created_at" json:"created_at"`
	Session   *string `yaml:"session" json:"session"`
}

// Store is the asks folder of one workspace.
type Store struct {
	Dir         string // .athel/asks
	Reg         *schema.Registry
	Clock       clock.Clock
	LockTimeout time.Duration
}

// DirOf is the folder of one ask; id must already be validated.
func (s Store) DirOf(id string) string { return filepath.Join(s.Dir, id) }

// New allocates the next ASK-NNN under a lock and writes its ask.yaml.
func (s Store) New(kind, question, session string) (Ask, error) {
	a := Ask{Schema: SchemaID, Kind: kind, Question: question, CreatedAt: clock.Format(s.Clock.Now())}
	if session != "" {
		a.Session = &session
	}
	if err := fsutil.EnsureDir(s.Dir); err != nil {
		return Ask{}, err
	}
	timeout := s.LockTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	l, err := lock.Acquire(filepath.Join(s.Dir, ".lock"), timeout)
	if err != nil {
		return Ask{}, err
	}
	defer func() { _ = l.Release() }() // the ask is already written when release runs
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return Ask{}, aerr.Wrap(aerr.StateIO, "list "+s.Dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	a.ID = ids.Ask.Next(names)
	b, _ := yaml.Marshal(a) // strings only: always encodes
	if err := s.Reg.ValidateYAML(SchemaID, b); err != nil {
		return Ask{}, err
	}
	if err := fsutil.EnsureDir(s.DirOf(a.ID)); err != nil {
		return Ask{}, err
	}
	return a, fsutil.WriteFileAtomic(filepath.Join(s.DirOf(a.ID), "ask.yaml"), b)
}

// Get reads one ask.
func (s Store) Get(id string) (Ask, error) {
	if _, err := ids.Ask.Parse(id); err != nil {
		return Ask{}, err
	}
	b, err := os.ReadFile(filepath.Join(s.DirOf(id), "ask.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return Ask{}, aerr.New(aerr.StateNotFound, "no ask "+id).WithNextAction("create one with athel ask new")
	}
	if err != nil {
		return Ask{}, aerr.Wrap(aerr.StateIO, "read ask "+id, err)
	}
	if err := s.Reg.ValidateYAML(SchemaID, b); err != nil {
		return Ask{}, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	var a Ask
	_ = dec.Decode(&a) // the schema has just accepted every field and type
	return a, nil
}

// AnswerFiles are the files an ask's folder may hold besides ask.yaml.
var AnswerFiles = map[string]bool{"investigation.md": true, "blast-radius.md": true}

// Write saves one answer file into an ask's folder.
func (s Store) Write(id, name string, data []byte) (string, error) {
	if !AnswerFiles[name] {
		return "", aerr.New(aerr.StateInvalid, name+" is not an answer file the CLI writes")
	}
	if _, err := s.Get(id); err != nil {
		return "", err
	}
	p := filepath.Join(s.DirOf(id), name)
	return p, fsutil.WriteFileAtomic(p, data)
}
