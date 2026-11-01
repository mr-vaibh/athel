package cli

import "strings"

// cmdIntegrityAccept is the developer's acknowledgement of a hand edit of change.yaml (M1 plan
// §6.4). It is run in a terminal: Athel's hook denies it from a tool call (rule H), so the model
// can never launder an edit. The session is optional because a terminal has none.
func cmdIntegrityAccept(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("integrity accept")
	reason := fs.String("reason", "", "why the edit is acceptable (required)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(*reason) == "" {
		return nil, usagef("integrity accept: --reason is required")
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	c, err := s.AcceptIntegrity(pos[0], r.session, *reason)
	if err != nil {
		return nil, err
	}
	return viewOf(c, r.env.Clock.Now()), nil
}
