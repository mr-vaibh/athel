package state

import (
	"github.com/mr-vaibh/athel/cli/internal/audit"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// volatile fields change on every heartbeat or save and are left out of the state hash, so a
// lock refresh is not mistaken for an edit.
var volatile = []string{"lock", "updated_at", "integrity"}

// ContentHash is the sha256 of a change.yaml document without its volatile fields (M1 plan
// §6.4), computed on the parsed YAML so the CLI's write and a later read hash the same thing.
// A different hash on load means someone edited the file outside the CLI (doc 02 §Security
// baseline).
func ContentHash(data []byte) (string, error) {
	doc, err := schema.DecodeYAML(data)
	if err != nil {
		return "", err
	}
	m, ok := doc.(map[string]any)
	if !ok {
		return "", aerr.New(aerr.StateSchema, "change.yaml is not a mapping")
	}
	for _, k := range volatile {
		delete(m, k)
	}
	return digest.CanonicalJSON(m)
}

// seal records the state hash of data (c encoded) in c; the store calls it before every write.
func seal(c *Change, data []byte) error {
	h, err := ContentHash(data)
	if err != nil {
		return err
	}
	c.Integrity, c.tampered = &Integrity{Hash: h, At: c.UpdatedAt}, false
	return nil
}

// verify sets c.tampered when the stored hash is missing or differs. change@1 files predate the
// hash and are sealed by `athel migrate` or their next write. A change@2 file is checked like a
// current one: its seal covers its own bytes, whatever the schema version.
func verify(c *Change, data []byte, unsealed bool) error {
	if unsealed {
		return nil
	}
	h, err := ContentHash(data)
	if err != nil {
		return err
	}
	c.tampered = c.Integrity == nil || c.Integrity.Hash != h
	return nil
}

// Tampered reports whether change.yaml was edited outside the CLI since its last write.
func (c *Change) Tampered() bool { return c.tampered }

func tamperedError(c *Change) error {
	return aerr.New(aerr.IntegrityState, "change.yaml of "+c.ID+" was edited outside the athel CLI; it is frozen until the developer reviews it").
		WithNextAction("review the file, then run `athel integrity accept " + c.ID + " --reason \"...\"` in a terminal")
}

// AcceptIntegrity is the developer's acknowledgement of a hand edit: it re-seals the change with
// its current content and logs the old and new hashes and the reason (M1 decision Q10).
func (s *Store) AcceptIntegrity(id, session, reason string) (*Change, error) {
	if reason == "" {
		return nil, aerr.New(aerr.StateInvalid, "integrity accept needs --reason")
	}
	var before string
	c, err := s.update(id, true, func(c *Change) error {
		if !c.tampered {
			return aerr.New(aerr.StateInvalid, c.ID+" has no unaccepted edit")
		}
		if c.Integrity != nil {
			before = c.Integrity.Hash
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	_, err = s.AuditLog().Append(audit.Record{Kind: audit.KindIntegrityAccept, Change: audit.Str(c.ID), Session: audit.Str(session),
		From: audit.Str(before), To: audit.Str(c.Integrity.Hash), By: audit.Str(ByHuman), Reason: audit.Str(reason), Result: audit.ResultOK})
	return c, err
}
