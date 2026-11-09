package state

import (
	"os"
	"regexp"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

var sealHash = regexp.MustCompile(`(?m)^  hash: sha256:[0-9a-f]{64}$`)

// toV2 turns a current change file into the sealed change@2 file M1 would have written: the
// schema line names version 2 and the seal covers those bytes.
func toV2(t *testing.T, current string) string {
	t.Helper()
	v2 := strings.Replace(current, "schema: "+SchemaID, "schema: "+SchemaV2, 1)
	h, err := ContentHash([]byte(v2))
	if err != nil {
		t.Fatal(err)
	}
	return sealHash.ReplaceAllString(v2, "  hash: "+h)
}

func TestMigrateChange2To3(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "sealed v2")
	b, _ := os.ReadFile(s.WS.ChangeFile(c.ID))
	if err := os.WriteFile(s.WS.ChangeFile(c.ID), []byte(toV2(t, string(b))), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := s.Get(c.ID)
	if err != nil || read.Schema != SchemaID || read.Tampered() {
		t.Fatalf("sealed v2 read: %+v %v", read, err)
	}
	if ids, err := s.Migrate(false); err != nil || strings.Join(ids, ",") != c.ID {
		t.Fatalf("migrate = %v %v", ids, err)
	}
	after, _ := os.ReadFile(s.WS.ChangeFile(c.ID))
	if !strings.Contains(string(after), "schema: "+SchemaID) || IsLegacy(after) {
		t.Errorf("not upgraded:\n%s", after)
	}
	if read, err := s.Get(c.ID); err != nil || read.Tampered() {
		t.Errorf("upgraded file: %v tampered=%v", err, read.Tampered())
	}
}

func TestChangeV2StillChecked(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	b, _ := os.ReadFile(s.WS.ChangeFile(c.ID))
	v2 := toV2(t, string(b))
	edited := strings.Replace(v2, "title: x", "title: edited by hand", 1)
	if err := os.WriteFile(s.WS.ChangeFile(c.ID), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if read, err := s.Get(c.ID); err != nil || !read.Tampered() {
		t.Errorf("hand edit of a v2 file not detected: %v", err)
	}
	if _, err := s.Migrate(false); !aerr.HasCode(err, aerr.IntegrityState) {
		t.Errorf("migrating a tampered file: %v, want E-INTEGRITY-STATE", err)
	}
	withFlag := strings.Replace(v2, "flags: []", "flags:\n  - {kind: fact_disputed, fact: F-0001, replacement: null, at: \"2026-11-09T10:00:00.000Z\", reason: y, resolved_at: null}", 1)
	if err := os.WriteFile(s.WS.ChangeFile(c.ID), []byte(withFlag), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(c.ID); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("v2 with flags: %v, want E-STATE-SCHEMA", err)
	}
}

func TestChangeFlagsRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	repl := "F-0002"
	flag := Flag{Kind: FlagFactSuperseded, Fact: "F-0001", Replacement: &repl, At: "2026-11-09T10:00:00.000Z", Reason: "consumer moved", ResolvedAt: nil}
	if _, err := s.Update(c.ID, func(c *Change) error { c.Flags = append(c.Flags, flag); return nil }); err != nil {
		t.Fatal(err)
	}
	read, err := s.Get(c.ID)
	if err != nil || len(read.Flags) != 1 || *read.Flags[0].Replacement != repl || read.Flags[0].ResolvedAt != nil {
		t.Fatalf("flags = %+v %v", read.Flags, err)
	}
	bad := Flag{Kind: "fact_forgotten", Fact: "F-0001", At: "2026-11-09T10:00:00.000Z", Reason: "x"}
	if _, err := s.Update(c.ID, func(c *Change) error { c.Flags = []Flag{bad}; return nil }); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid flag kind: %v, want E-STATE-SCHEMA", err)
	}
}
