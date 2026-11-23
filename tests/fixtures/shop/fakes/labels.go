package fakes

import (
	"errors"
	"fmt"
	"slices"
)

// Labels are per repo, as on GitHub: a PR can carry only a label its repo has (M5-plan G6).

func (s Shim) loadLabels() (map[string][]string, error) {
	labels := map[string][]string{}
	return labels, s.load(labelsFile, &labels)
}

// knownLabels checks that name, when given, is a label of repo, as gh does on pr create.
func (s Shim) knownLabels(repo, name string) ([]prLabel, error) {
	if name == "" {
		return nil, nil
	}
	labels, err := s.loadLabels()
	if err != nil {
		return nil, err
	}
	if !slices.Contains(labels[repo], name) {
		return nil, fmt.Errorf("could not add label: '%s' not found", name)
	}
	return []prLabel{{Name: name}}, nil
}

func (s Shim) labelList(repo string) error {
	labels, err := s.loadLabels()
	if err != nil {
		return err
	}
	out := []prLabel{}
	for _, l := range labels[repo] {
		out = append(out, prLabel{Name: l})
	}
	return s.json(out)
}

func (s Shim) labelCreate(a args) error {
	if s.State == "" {
		return errors.New("the shop fixture has no label store: set SHOP_GH_STATE to a folder")
	}
	repo, name := a.flags["--repo"], a.pos[2]
	labels, err := s.loadLabels()
	if err != nil {
		return err
	}
	if slices.Contains(labels[repo], name) {
		if a.flags["--force"] == "true" {
			return nil
		}
		return fmt.Errorf("label with name %q already exists; use `--force` to update its color and description", name)
	}
	labels[repo] = append(labels[repo], name)
	return s.save(labelsFile, labels)
}
