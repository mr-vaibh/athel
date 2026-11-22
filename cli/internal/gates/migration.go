package gates

import (
	"regexp"
	"strings"
)

// Doc 06's migration gate: a migration runs up and down on a disposable database, and the
// online-safety checks pass. Never waivable.

// Migration is the gate's pass criterion.
const Migration = "migration"

var migrationFile = regexp.MustCompile(`(^|/)(migrations?|db/migrate)/[^/]+\.sql$`)

// IsMigration reports whether a repo path is a SQL migration (the same folders M3's blast
// radius counts as migrations).
func IsMigration(p string) bool { return migrationFile.MatchString(p) }

// DownFor is the down migration that undoes an up migration named *.up.sql, or "" for a
// migration with no up/down pair.
func DownFor(up string) string {
	if base, ok := strings.CutSuffix(up, ".up.sql"); ok {
		return base + ".down.sql"
	}
	return ""
}

var (
	sqlComments = regexp.MustCompile(`(?s)--[^\n]*|/\*.*?\*/`)
	unsafeSQL   = []struct {
		re, unless *regexp.Regexp
		why        string
	}{
		{regexp.MustCompile(`(?i)\bADD\s+(COLUMN\s+)?\S+\s+[^,]*\bNOT\s+NULL\b`), regexp.MustCompile(`(?i)\bDEFAULT\b`),
			"adds a NOT NULL column without a default: it fails on existing rows or rewrites the table"},
		{regexp.MustCompile(`(?i)\bCREATE\s+(UNIQUE\s+)?INDEX\b`), regexp.MustCompile(`(?i)\bCONCURRENTLY\b`),
			"builds an index without CONCURRENTLY: writes to the table are locked until it finishes"},
		{regexp.MustCompile(`(?i)\bALTER\s+COLUMN\s+\S+\s+(SET\s+DATA\s+)?TYPE\b`), nil,
			"changes a column type: the table is rewritten under an exclusive lock"},
		{regexp.MustCompile(`(?i)\bRENAME\b`), nil,
			"renames: readers of the old name break; add the new name, move readers, then drop (expand and contract)"},
		{regexp.MustCompile(`(?i)\bFOREIGN\s+KEY\b`), regexp.MustCompile(`(?i)\bNOT\s+VALID\b`),
			"adds a foreign key that is validated under lock; add it NOT VALID, then VALIDATE CONSTRAINT"},
	}
	drops = regexp.MustCompile(`(?i)\bDROP\s+(TABLE|COLUMN)\b`)
)

// OnlineSafety lists the statements that are not safe to run against a live database. Drops are
// allowed only in a contract task, which runs after every consumer has moved (doc 05).
func OnlineSafety(sql string, contract bool) []string {
	var out []string
	for _, stmt := range strings.Split(sqlComments.ReplaceAllString(sql, ""), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		for _, u := range unsafeSQL {
			if u.re.MatchString(stmt) && (u.unless == nil || !u.unless.MatchString(stmt)) {
				out = append(out, u.why+": "+oneLine(stmt))
			}
		}
		if drops.MatchString(stmt) && !contract {
			out = append(out, "drops data; only a contract task, after every consumer moved, may drop: "+oneLine(stmt))
		}
	}
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
