package state

// FlagFact records on an open change that a fact it used was disputed or superseded (doc 03
// FR-DISC-005), so the change's owner re-checks what was built on it. A finished or cancelled
// change is left alone, and an unresolved flag for the same fact and kind is not repeated. It
// reports whether the change now carries a new flag.
func (s *Store) FlagFact(id string, f Flag) (bool, error) {
	added := false
	_, err := s.Update(id, func(c *Change) error {
		if c.State == Done || c.State == Cancelled {
			return nil
		}
		for _, old := range c.Flags {
			if old.Fact == f.Fact && old.Kind == f.Kind && old.ResolvedAt == nil {
				return nil
			}
		}
		c.Flags, added = append(c.Flags, f), true
		return nil
	})
	return added, err
}
