package gates

import (
	"fmt"
	"regexp"
	"slices"
)

// The ac-tests and contract gates are computed by the CLI (FR-BUILD-003; doc 06 contract gate),
// and a repo may override gate commands in .athel/config/gates.yaml.

// testFile matches the test file conventions of the languages Athel has defaults for, and test
// folders.
var testFile = regexp.MustCompile(`(_test\.go|\.(test|spec)\.[cm]?[jt]sx?|(^|/)test_[^/]*\.py|_test\.py)$|(^|/)(test|tests|__tests__)/`)

// IsTestFile reports whether a repo path is a test file.
func IsTestFile(p string) bool { return testFile.MatchString(p) }

// MissingACs lists the ACs that no test file's text names (FR-BUILD-003: "every AC ID appears in
// at least one test"). An ID matches only as a whole word, so AC-1 is not found in AC-10.
func MissingACs(acs []string, tests map[string]string) []string {
	var missing []string
	for _, ac := range acs {
		word := regexp.MustCompile(`\b` + regexp.QuoteMeta(ac) + `\b`)
		found := false
		for _, text := range tests {
			found = found || word.MatchString(text)
		}
		if !found {
			missing = append(missing, ac)
		}
	}
	return missing
}

// Override applies a repo's gates.yaml entries. An entry naming a default gate replaces only its
// command: its stage, pass criterion and waiver rule stay doc 06's, so an override can never
// loosen a gate, and a builtin gate has no command to replace. Any other entry adds a gate that
// runs a command and passes when it exits 0 or prints nothing.
func Override(defaults, entries []Gate) ([]Gate, error) {
	out := slices.Clone(defaults)
	for _, e := range entries {
		i := slices.IndexFunc(out, func(g Gate) bool { return g.ID == e.ID })
		if i >= 0 {
			if err := replaceable(out[i], e); err != nil {
				return nil, err
			}
			out[i].Command = e.Command
			continue
		}
		if err := addable(e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func replaceable(g, e Gate) error {
	switch {
	case g.Builtin:
		return fmt.Errorf("gate %s is computed by Athel; it has no command to replace", g.ID)
	case (e.Pass != "" && e.Pass != g.Pass) || (e.Waivable != "" && e.Waivable != g.Waivable) || (e.Stage != "" && e.Stage != g.Stage):
		return fmt.Errorf("gate %s: an override may replace only the command; its stage, pass and waivable are doc 06's", g.ID)
	case len(e.Command) == 0:
		return fmt.Errorf("gate %s: the override has no command", g.ID)
	}
	return nil
}

func addable(e Gate) error {
	switch {
	case len(e.Command) == 0:
		return fmt.Errorf("gate %s has no command", e.ID)
	case e.Pass != ExitZero && e.Pass != NoOutput:
		return fmt.Errorf("gate %s: an added gate passes on %s or %s", e.ID, ExitZero, NoOutput)
	case e.Stage != Cheap && e.Stage != Full:
		return fmt.Errorf("gate %s: stage is %s or %s", e.ID, Cheap, Full)
	case e.Waivable != Never && e.Waivable != Human:
		return fmt.Errorf("gate %s: waivable is %s or %s", e.ID, Never, Human)
	}
	return nil
}
