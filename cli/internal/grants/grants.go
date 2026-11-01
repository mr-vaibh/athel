// Package grants proves that a human-only transition (plan_approved, done) came from the human's
// own prompt (doc 02 §Transition rules; M1 plan §4.5, decision Q4).
//
// Only the UserPromptSubmit hook creates a grant, and only when the human's literal prompt is
// `/athel:approve CHG-NNN` or `/athel:accept CHG-NNN`. The PreToolUse hook lets
// `athel state transition … --by human` through only for the main agent, in the same session,
// for the same change and target, with an unused grant younger than TTL. The transition then
// consumes the grant, so each prompt allows exactly one transition. The book lives under
// .athel/, which the model cannot write (classify marks such writes tier X).
package grants

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/lock"
)

// TTL is how long a grant stays usable after the human's prompt.
const TTL = 10 * time.Minute

// File is the grant book inside the workspace's state directory.
const File = "grants.jsonl"

// Grant is one human approval waiting for (or already used by) its transition.
type Grant struct {
	ID       string  `json:"id"`
	Change   string  `json:"change"`
	Target   string  `json:"target"`
	Session  string  `json:"session"`
	PromptID string  `json:"prompt_id"`
	At       string  `json:"at"`
	UsedAt   *string `json:"used_at"`
}

// entry is one line of the book: a created grant, or the use of one.
type entry struct {
	Op string `json:"op"` // created | used
	Grant
}

// Book reads and writes the grant book. Dir is .athel/state.
type Book struct {
	Dir         string
	Clock       clock.Clock
	LockTimeout time.Duration
}

var promptRe = regexp.MustCompile(`^\s*/athel:(approve|accept)\s+(CHG-[0-9]{3,})\s*$`)

var targets = map[string]string{"approve": "plan_approved", "accept": "done"}

// ParsePrompt recognises the two literal grant prompts and returns the change and target state.
func ParsePrompt(prompt string) (change, target string, ok bool) {
	m := promptRe.FindStringSubmatch(prompt)
	if m == nil {
		return "", "", false
	}
	return m[2], targets[m[1]], true
}

// Create records a grant for the human's prompt.
func (b Book) Create(change, target, session, promptID string) (Grant, error) {
	g := Grant{Change: change, Target: target, Session: session, PromptID: promptID, At: clock.Format(b.Clock.Now())}
	g.ID = "GR-" + digest.Bytes([]byte(g.Change + "\n" + g.Target + "\n" + g.Session + "\n" + g.PromptID + "\n" + g.At))[7:19]
	err := b.locked(func([]Grant) error { return b.append(entry{Op: "created", Grant: g}) })
	return g, err
}

// Valid returns the newest usable grant for change → target in session, or nil.
func (b Book) Valid(change, target, session string) (*Grant, error) {
	gs, err := b.load()
	if err != nil {
		return nil, err
	}
	return b.usable(gs, change, target, session), nil
}

// Consume marks the newest usable grant used and returns it; nil when there is none.
func (b Book) Consume(change, target, session string) (*Grant, error) {
	var used *Grant
	err := b.locked(func(gs []Grant) error {
		g := b.usable(gs, change, target, session)
		if g == nil {
			return nil
		}
		at := clock.Format(b.Clock.Now())
		g.UsedAt = &at
		used = g
		return b.append(entry{Op: "used", Grant: *g})
	})
	return used, err
}

func (b Book) usable(gs []Grant, change, target, session string) *Grant {
	now := b.Clock.Now()
	for i := len(gs) - 1; i >= 0; i-- {
		g := gs[i]
		if g.UsedAt != nil || g.Change != change || g.Target != target || g.Session != session {
			continue
		}
		at, err := clock.Parse(g.At)
		if err != nil || now.Sub(at) > TTL || at.After(now) {
			continue
		}
		return &g
	}
	return nil
}

func (b Book) path() string { return filepath.Join(b.Dir, File) }

func (b Book) locked(fn func([]Grant) error) error {
	if err := fsutil.EnsureDir(b.Dir); err != nil {
		return err
	}
	fl, err := lock.Acquire(filepath.Join(b.Dir, ".grants.lock"), b.LockTimeout)
	if err != nil {
		return err
	}
	defer func() { _ = fl.Release() }()
	gs, err := b.load()
	if err != nil {
		return err
	}
	return fn(gs)
}

func (b Book) append(e entry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return aerr.Wrap(aerr.StateIO, "encode grant", err)
	}
	return fsutil.AppendLine(b.path(), line)
}

// load replays the book: created lines add grants, used lines mark them.
func (b Book) load() ([]Grant, error) {
	data, err := os.ReadFile(b.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read "+File, err)
	}
	var gs []Grant
	index := map[string]int{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, aerr.Wrap(aerr.IntegrityState, "corrupt "+File+" line "+strconv.Itoa(n), err)
		}
		switch i, seen := index[e.ID]; {
		case e.Op == "created" && !seen:
			index[e.ID] = len(gs)
			gs = append(gs, e.Grant)
		case e.Op == "used" && seen:
			gs[i].UsedAt = e.UsedAt
		default:
			return nil, aerr.New(aerr.IntegrityState, "corrupt "+File+" line "+strconv.Itoa(n)+": unexpected "+e.Op+" of "+e.ID)
		}
	}
	return gs, nil
}
