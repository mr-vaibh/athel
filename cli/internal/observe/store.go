package observe

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// ScoutAgentType is the agent_type Claude Code reports for Athel's scout subagent (plugin
// subagents are named <plugin>:<agent>). Only the scout's calls are captured.
const ScoutAgentType = "athel:scout"

// SchemaID is the schema of one observation file.
const SchemaID = schema.Observation

// DefaultTTL is how long a captured call is kept: long enough for the facts built from it to be
// checked, short enough that raw output does not pile up.
const DefaultTTL = 7 * 24 * time.Hour

// Captured sizes: enough to judge a recipe step, small enough to keep the hook fast.
const (
	MaxInput  = 4 * 1024
	MaxOutput = 256 * 1024
)

// idRe is the tool_use_id shape; it is also the file name, so it must never hold a separator.
var idRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,200}$`)

// Call is one completed tool call, with input and output already scrubbed of secrets.
type Call struct {
	ToolUseID, Session, AgentType, Tool string
	Input, Output                       []byte
	Flags                               []Flag
}

// Observation is one observation@1 file.
type Observation struct {
	Schema       string `json:"schema"`
	ToolUseID    string `json:"tool_use_id"`
	Session      string `json:"session"`
	AgentType    string `json:"agent_type"`
	Tool         string `json:"tool"`
	Input        string `json:"input"`
	Output       string `json:"output"`
	OutputSHA256 string `json:"output_sha256"`
	Truncated    bool   `json:"truncated"`
	Suspicious   []Flag `json:"suspicious"`
	At           string `json:"at"`
	ExpiresAt    string `json:"expires_at"`
}

// Store keeps observations in one folder, one file per tool call. Files are written once and
// atomically, so no lock is needed.
type Store struct {
	Dir   string
	Reg   *schema.Registry
	Clock clock.Clock
	TTL   time.Duration
}

func (s *Store) path(id string) (string, error) {
	if !idRe.MatchString(id) {
		return "", aerr.New(aerr.StateInvalid, "invalid tool_use_id "+id)
	}
	return filepath.Join(s.Dir, id+".json"), nil
}

// Save captures one call. The hash covers the whole scrubbed output even when the stored copy is cut.
func (s *Store) Save(c Call) (Observation, error) {
	p, err := s.path(c.ToolUseID)
	if err != nil {
		return Observation{}, err
	}
	now := s.Clock.Now()
	flags := c.Flags
	if flags == nil {
		flags = []Flag{}
	}
	o := Observation{
		Schema: SchemaID, ToolUseID: c.ToolUseID, Session: c.Session, AgentType: c.AgentType, Tool: c.Tool,
		Input: cut(c.Input, MaxInput), Output: cut(c.Output, MaxOutput), OutputSHA256: digest.Bytes(c.Output),
		Truncated: len(c.Output) > MaxOutput, Suspicious: flags, At: clock.Format(now), ExpiresAt: clock.Format(now.Add(s.TTL)),
	}
	if err := s.Reg.Validate(SchemaID, o); err != nil {
		return Observation{}, err
	}
	if err := fsutil.EnsureDir(s.Dir); err != nil {
		return Observation{}, err
	}
	b, _ := json.Marshal(o) // only strings, booleans and lists of them: always encodes
	return o, fsutil.WriteFileAtomic(p, b)
}

// Get returns a captured call that has not expired.
func (s *Store) Get(id string) (Observation, error) {
	p, err := s.path(id)
	if err != nil {
		return Observation{}, err
	}
	o, expires, err := read(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Observation{}, aerr.New(aerr.StateNotFound, "no captured output for tool call "+id)
	}
	if err != nil {
		return Observation{}, err
	}
	if s.Clock.Now().After(expires) {
		return Observation{}, aerr.New(aerr.StateNotFound, "the captured output of tool call "+id+" has expired")
	}
	return o, nil
}

// Prune deletes expired observations and returns how many it deleted. Other files are left alone.
func (s *Store) Prune() (int, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, aerr.Wrap(aerr.StateIO, "list "+s.Dir, err)
	}
	n := 0
	now := s.Clock.Now()
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), ".json"); !ok || !idRe.MatchString(id) {
			continue
		}
		p := filepath.Join(s.Dir, e.Name())
		_, expires, err := read(p)
		if err != nil {
			return n, err
		}
		if !now.After(expires) {
			continue
		}
		if err := os.Remove(p); err != nil {
			return n, aerr.Wrap(aerr.StateIO, "delete "+p, err)
		}
		n++
	}
	return n, nil
}

func read(p string) (Observation, time.Time, error) {
	data, err := os.ReadFile(p) //nolint:gosec // p is built from a validated tool_use_id inside the observations folder
	if errors.Is(err, fs.ErrNotExist) {
		return Observation{}, time.Time{}, err
	}
	if err != nil {
		return Observation{}, time.Time{}, aerr.Wrap(aerr.StateIO, "read "+p, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var o Observation
	if err := dec.Decode(&o); err != nil || o.Schema != SchemaID {
		return Observation{}, time.Time{}, aerr.Wrap(aerr.StateSchema, p+" is not an "+SchemaID+" file", err)
	}
	expires, err := clock.Parse(o.ExpiresAt)
	if err != nil {
		return Observation{}, time.Time{}, aerr.Wrap(aerr.StateSchema, p+" has an invalid expires_at", err)
	}
	return o, expires, nil
}

// cut keeps at most n bytes of b as valid UTF-8 text.
func cut(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return strings.ToValidUTF8(string(b[:n]), "")
}
