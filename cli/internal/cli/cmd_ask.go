package cli

import "github.com/mr-vaibh/athel/cli/internal/asks"

// cmdAskNew records an ad-hoc question (UC-01) or blast-radius request (UC-02) outside a change.
func cmdAskNew(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("ask new")
	question := fs.String("question", "", "the question, or the proposed change for --kind blast (required)")
	kind := fs.String("kind", asks.Question, "question | blast")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	if *question == "" {
		return nil, usagef("ask new: --question is required")
	}
	if _, err := r.storeFor(); err != nil {
		return nil, err
	}
	return r.askStore().New(*kind, *question, r.session)
}

func cmdAskShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("ask show"), args, 1)
	if err != nil {
		return nil, err
	}
	if _, err := r.storeFor(); err != nil {
		return nil, err
	}
	return r.askStore().Get(pos[0])
}
