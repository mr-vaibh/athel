package cli

import (
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// cmdAuditVerify checks the whole hash chain (FR-SAFE-011, M1 exit criterion 3). A broken chain
// is an E-INTEGRITY error (exit 17) naming the file, line and reason.
func cmdAuditVerify(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("audit verify"), args, 0); err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	rep, err := s.AuditLog().Verify()
	if err != nil {
		return nil, err
	}
	if !rep.OK {
		return nil, aerr.New(aerr.IntegrityAudit, "audit chain broken at "+rep.Break.File+" line "+itoa(rep.Break.Line)+": "+rep.Break.Reason).
			WithNextAction("stop all work on this workspace; a human must review the audit log")
	}
	return rep, nil
}

func cmdAuditTail(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("audit tail")
	n := fs.Int("n", 20, "number of records")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	if *n < 1 {
		return nil, usagef("audit tail: -n must be at least 1")
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	return s.AuditLog().Tail(*n)
}
