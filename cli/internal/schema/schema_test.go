package schema

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const validChange = `
schema: athel/change@1
id: CHG-001
title: Customers can update phone number
state: draft
blocked_from: null
created_by: dev@example.com
created_at: "2026-11-01T10:00:00.000Z"
updated_at: "2026-11-01T10:00:00.000Z"
prd:
  current: v1
  versions:
    - {version: v1, path: prd/v1.md, sha256: "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", source: file, at: "2026-11-01T10:00:00.000Z"}
step: null
step_attempts: {}
stop_blocks: {count: 0}
checkpoints: []
budget: {max_cost_usd: 40, max_hours: 8, max_tool_calls: 2000, used_cost_usd: 0, used_minutes: 0, used_tool_calls: 0, status: ok, events: []}
lock: null
approvals: {plan: null}
waivers: []
repos: []
change_set: null
parent: null
flags: []
history: []
`

func TestValidChangeAccepted(t *testing.T) {
	if err := NewRegistry().ValidateYAML(Change, []byte(validChange)); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidChangeErrorsHavePaths(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		wantLoc string
	}{
		{"unknown state", "state: draft", "state: shipping", "/state"},
		{"bad id", "id: CHG-001", "id: CHG-1", "/id"},
		{"unknown field", "flags: []", "flags: []\nsurprise: 1", "/"},
		{"reserved field filled", "waivers: []", "waivers: [x]", "/waivers"},
		{"blocked without origin", "state: draft", "state: blocked", "/blocked_from"},
		{"origin while not blocked", "blocked_from: null", "blocked_from: draft", "/blocked_from"},
		{"bad timestamp", `created_at: "2026-11-01T10:00:00.000Z"`, `created_at: "2026-11-01 10:00"`, "/created_at"},
		{"negative budget", "used_cost_usd: 0", "used_cost_usd: -1", "/budget/used_cost_usd"},
		{"missing required", "lock: null\n", "", "/"},
	}
	r := NewRegistry()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := strings.Replace(validChange, tt.from, tt.to, 1)
			if doc == validChange {
				t.Fatalf("test setup: %q not found", tt.from)
			}
			err := r.ValidateYAML(Change, []byte(doc))
			if !aerr.HasCode(err, aerr.StateSchema) {
				t.Fatalf("err = %v, want E-STATE-SCHEMA", err)
			}
			var p ViolationErrors
			if !errors.As(err, &p) {
				t.Fatalf("error has no ViolationErrors list: %v", err)
			}
			found := false
			for _, line := range p {
				if strings.HasPrefix(line, tt.wantLoc+":") {
					found = true
				}
			}
			if !found {
				t.Errorf("no problem at %s in %v", tt.wantLoc, p)
			}
		})
	}
}

func TestUnknownSchemaID(t *testing.T) {
	r := NewRegistry()
	for _, id := range []string{"athel/nope@1", "athel/change@9", "change@1", "athel/Change@1", ""} {
		if err := r.Validate(id, map[string]any{}); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("Validate(%q) err = %v, want E-STATE-SCHEMA", id, err)
		}
	}
}

func TestAllEmbeddedSchemasCompile(t *testing.T) {
	r := NewRegistry()
	ids, err := r.IDs()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{Change, Config, HookEvent} {
		found := false
		for _, id := range ids {
			found = found || id == want
		}
		if !found {
			t.Errorf("schema %s not embedded (have %v)", want, ids)
		}
	}
	for _, id := range ids {
		if _, err := r.schema(id); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	if _, err := r.schema(Change); err != nil {
		t.Errorf("cached lookup: %v", err)
	}
}

func TestBrokenSchemaFiles(t *testing.T) {
	r := newRegistry(fstest.MapFS{
		"schemas/notjson@1.json": {Data: []byte("{")},
		"schemas/badtype@1.json": {Data: []byte(`{"type": 5}`)},
		"schemas/badref@1.json":  {Data: []byte(`{"$ref": "#/$defs/missing"}`)},
		"schemas/badid@1.json":   {Data: []byte(`{"$id": 7}`)},
	})
	for _, id := range []string{"athel/notjson@1", "athel/badtype@1", "athel/badref@1", "athel/badid@1"} {
		if err := r.Validate(id, map[string]any{}); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: err = %v, want E-STATE-SCHEMA", id, err)
		}
	}
	if _, err := newRegistry(fstest.MapFS{}).IDs(); err != nil {
		t.Errorf("IDs on empty FS: %v", err)
	}
}

func TestDecodeYAML(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"mapping", "a: 1", false},
		{"syntax error", "a: [1", true},
		{"two documents", "a: 1\n---\nb: 2", true},
		{"duplicate keys", "a: 1\na: 2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeYAML([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRejectsNonJSONValues(t *testing.T) {
	r := NewRegistry()
	doc, err := DecodeYAML([]byte("1: value\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(Config, doc); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("non-string key: err = %v, want E-STATE-SCHEMA", err)
	}
	if err := r.Validate(Config, map[string]any{"schema": "athel/config@1", "t": time.Now()}); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("unknown key: err = %v", err)
	}
	if err := r.ValidateYAML(Config, []byte("a: [")); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("bad yaml: err = %v", err)
	}
}

func TestDetectID(t *testing.T) {
	tests := []struct {
		doc     any
		want    string
		wantErr bool
	}{
		{map[string]any{"schema": "athel/change@1"}, "athel/change@1", false},
		{map[string]any{"schema": 3}, "", true},
		{map[string]any{}, "", true},
		{[]any{1}, "", true},
	}
	for _, tt := range tests {
		got, err := DetectID(tt.doc)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("DetectID(%v) = %q, %v", tt.doc, got, err)
		}
	}
}
