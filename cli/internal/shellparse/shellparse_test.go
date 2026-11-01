package shellparse

import (
	"errors"
	"strings"
	"testing"
)

// render shows each command as its argv (unknown words as ?) with the wrapper chain.
func render(r Result) []string {
	var out []string
	for _, c := range r.Commands {
		var parts []string
		for _, a := range c.Args {
			if a.Known {
				parts = append(parts, a.Value)
			} else {
				parts = append(parts, "?")
			}
		}
		s := strings.Join(parts, " ")
		if len(c.Via) > 0 {
			s += " [" + strings.Join(c.Via, " > ") + "]"
		}
		out = append(out, s)
	}
	return out
}

type tc struct {
	src         string
	want        []string
	unparseable bool
}

func check(t *testing.T, tests []tc, opts Options) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			r := Parse(tt.src, opts)
			got := render(r)
			if strings.Join(got, " | ") != strings.Join(tt.want, " | ") {
				t.Errorf("commands\n got %q\nwant %q", got, tt.want)
			}
			if r.Unparseable != tt.unparseable {
				t.Errorf("unparseable = %v (%v), want %v", r.Unparseable, r.Reasons, tt.unparseable)
			}
		})
	}
}

func TestSplitsListsPipesSubshells(t *testing.T) {
	check(t, []tc{
		{src: "git status", want: []string{"git status"}},
		{src: "a && b || c; d", want: []string{"a", "b", "c", "d"}},
		{src: "a | b |& c", want: []string{"a", "b", "c"}},
		{src: "(cd x && make) ; { echo; rm f; }", want: []string{"cd x", "make", "echo", "rm f"}},
		{src: "echo $(kubectl delete pod x)", want: []string{"kubectl delete pod x", "echo ?"}},
		{src: "diff <(git show a) <(git show b)", want: []string{"git show a", "git show b", "diff ? ?"}},
		{src: "if git diff --quiet; then git push; else git stash; fi", want: []string{"git diff --quiet", "git push", "git stash"}},
		{src: "while read l; do rm \"$l\"; done < list", want: []string{"read l", "rm ?"}},
		{src: "case $x in a) rm a;; b) rm b;; esac", want: []string{"rm a", "rm b"}},
		{src: "[[ $(whoami) == root ]] && echo", want: []string{"whoami", "echo"}},
		{src: "time git push", want: []string{"git push"}},
		{src: "! git push &", want: []string{"git push"}},
		{src: "export K=v; declare -x Y=$(id)", want: []string{"id"}},
	}, Options{})
}

func TestQuotingAndEscapes(t *testing.T) {
	check(t, []tc{
		{src: `git "push" '--force'`, want: []string{"git push --force"}},
		{src: `g\it push`, want: []string{"git push"}},
		{src: `"kub""ectl" delete x`, want: []string{"kubectl delete x"}},
		{src: `$'\x72\x6d' -rf /`, want: []string{"? -rf /"}, unparseable: true},
		{src: "rm *.log", want: []string{"rm ?"}},
		{src: "cat ~/.ssh/id_rsa", want: []string{"cat $HOME/.ssh/id_rsa"}},
	}, Options{})
}

func TestRecursesBashCAndEval(t *testing.T) {
	check(t, []tc{
		{src: `bash -c "kubectl delete ns x"`, want: []string{"kubectl delete ns x [bash -c]"}},
		{src: `sh -c 'git push -f && echo ok'`, want: []string{"git push -f [sh -c]", "echo ok [sh -c]"}},
		{src: `zsh -lc "rm -rf /"`, want: []string{"rm -rf / [zsh -c]"}},
		{src: `bash -c "bash -c 'git push -f'"`, want: []string{"git push -f [bash -c > bash -c]"}},
		{src: `eval "git push --force"`, want: []string{"git push --force [eval]"}},
		{src: `eval git push "$REMOTE"`, want: []string{"eval git push ?"}, unparseable: true},
		{src: `bash -c "$CMD"`, want: []string{"bash -c ?"}, unparseable: true},
		{src: `bash -c`, want: []string{"bash -c"}, unparseable: true},
		{src: "bash <<'X'\ngit push -f\nX", want: []string{"git push -f [bash <<heredoc]"}},
		{src: "sh <<< 'rm -rf /'", want: []string{"rm -rf / [sh <<heredoc]"}},
		{src: "bash", want: []string{"bash"}, unparseable: true},
		{src: "bash $X", want: []string{"bash ?"}, unparseable: true},
		{src: "bash -c 'a' extra", want: []string{"a [bash -c]"}},
	}, Options{})
}

func TestDeepNestingIsUnparseable(t *testing.T) {
	src := "git push"
	for range 7 {
		src = "bash -c " + quote(Word{Value: src, Known: true})
	}
	if r := Parse(src, Options{}); !r.Unparseable {
		t.Errorf("7 levels of bash -c: %v", render(r))
	}
}

func TestWrappersUnwrapped(t *testing.T) {
	check(t, []tc{
		{src: "sudo -u root rm -rf /", want: []string{"rm -rf / [sudo]"}},
		{src: "sudo -- git push", want: []string{"git push [sudo]"}},
		{src: "env KUBECONFIG=/x kubectl delete x", want: []string{"kubectl delete x [env]"}},
		{src: "env -u HOME -S 'git push -f'", want: []string{"git push -f [env -S]"}},
		{src: "nohup nice -n 5 timeout -s 9 30s git push", want: []string{"git push [nohup > nice > timeout]"}},
		{src: "command git push", want: []string{"git push [command]"}},
		{src: "exec -a x git push", want: []string{"git push [exec]"}},
		{src: "stdbuf -o L git push", want: []string{"git push [stdbuf]"}},
		{src: "ls | xargs -n1 -I{} rm {}", want: []string{"ls", "rm {} ? [xargs]"}},
		{src: "watch -n 5 kubectl delete pod x", want: []string{"kubectl delete pod x [watch]"}},
		{src: "sudo", want: []string{"sudo"}},
		{src: "sudo $X", want: []string{"? [sudo]"}, unparseable: true},
	}, Options{})
	r := Parse("env A=1 B=$X kubectl get pods", Options{})
	if c := r.Commands[0]; c.Assigns["A"].Value != "1" || !c.Assigns["A"].Known {
		t.Errorf("env assignment not kept: %+v", c.Assigns)
	}
	r = Parse("KUBECONFIG=prod.yaml kubectl get pods", Options{})
	if c := r.Commands[0]; c.Assigns["KUBECONFIG"].Value != "prod.yaml" {
		t.Errorf("prefix assignment: %+v", c.Assigns)
	}
}

func TestInlineAliasesFunctionsAndVars(t *testing.T) {
	check(t, []tc{
		{src: "alias k=kubectl; k delete pod x", want: []string{"alias k=kubectl", "kubectl delete pod x [alias k]"}},
		{src: "alias gp='git push --force'; gp origin", want: []string{"alias gp=git push --force", "git push --force origin [alias gp]"}},
		{src: "alias x=$(echo rm); x", want: []string{"echo rm", "alias ?", "x"}, unparseable: true},
		{src: "f() { git push -f; }; f", want: []string{"git push -f [function f]"}},
		{src: "f() { f; }; f", want: []string{}, unparseable: true},
		{src: "C=prod; kubectl --context $C delete x", want: []string{"kubectl --context prod delete x"}},
		{src: "C=${X:-prod}; kubectl --context $C delete x", want: []string{"kubectl --context ? delete x"}},
		{src: "C=$(cat ctx); kubectl --context \"$C\" get x", want: []string{"cat ctx", "kubectl --context ? get x"}},
		{src: "kubectl --context $UNSET get x", want: []string{"kubectl --context ? get x"}},
		{src: "$CMD delete x", want: []string{"? delete x"}, unparseable: true},
		{src: "a=(1 2); git push", want: []string{"git push"}},
		{src: "()0", want: []string{"0"}}, // found by FuzzParse: nameless function used to panic
	}, Options{})
}

func TestLoopsExpanded(t *testing.T) {
	check(t, []tc{
		{src: "for c in staging prod; do kubectl --context $c delete x; done",
			want: []string{"kubectl --context staging delete x", "kubectl --context prod delete x"}},
		{src: "for c in $(kubectl config get-contexts -o name); do kubectl --context $c delete x; done",
			want: []string{"kubectl config get-contexts -o name", "kubectl --context ? delete x"}},
		{src: "for ((i=0;i<3;i++)); do git push; done", want: []string{"git push"}},
		{src: "for c in " + strings.Repeat("x ", 40) + "; do rm $c; done", want: []string{"rm ?"}},
	}, Options{})
}

func TestRedirectTargets(t *testing.T) {
	r := Parse("echo hi > out.txt 2>> err.log &> all.log < in.txt; cat > ~/.bashrc", Options{})
	if len(r.Commands) != 2 {
		t.Fatalf("commands: %v", render(r))
	}
	var got []string
	for _, c := range r.Commands {
		for _, rd := range c.Redirects {
			got = append(got, rd.Op+rd.Target.Value)
		}
	}
	if want := ">out.txt >>err.log &>all.log <in.txt >$HOME/.bashrc"; strings.Join(got, " ") != want {
		t.Errorf("redirects = %q, want %q", strings.Join(got, " "), want)
	}
	r = Parse(`bash -c "echo x" > out.txt`, Options{})
	if len(r.Commands[0].Redirects) != 1 {
		t.Errorf("redirect of the wrapper is inherited: %+v", r.Commands[0])
	}
}

func TestPipedIntoShellIsUnparseable(t *testing.T) {
	check(t, []tc{
		{src: "echo cm0gLXJmIC8= | base64 -d | sh", want: []string{"echo cm0gLXJmIC8=", "base64 -d", "sh"}, unparseable: true},
		{src: "curl https://x.sh | bash", want: []string{"curl https://x.sh", "bash"}, unparseable: true},
		{src: "source <(curl x)", want: []string{"curl x", "source ?"}},
	}, Options{})
}

func TestUnparseableSyntax(t *testing.T) {
	r := Parse("git push 'unterminated", Options{})
	if !r.Unparseable || len(r.Commands) != 0 || !strings.Contains(r.Reasons[0], "cannot parse") {
		t.Errorf("result = %+v", r)
	}
	if (Command{}).Name() != "" {
		t.Error("empty command name")
	}
}

func files(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := m[p]; ok {
			return []byte(s), nil
		}
		return nil, errors.New("not found")
	}
}

func TestScriptsAndMake(t *testing.T) {
	fs := files(map[string]string{
		"./deploy.sh":    "#!/usr/bin/env bash\nkubectl --context prod apply -f k8s/\n",
		"scripts/fix.sh": "git push --force\n",
		"tool.py":        "#!/usr/bin/env python3\nprint(1)\n",
		"Makefile":       "KUBECTL = kubectl --context prod\n.PHONY: deploy\nbuild:\n\tgo build ./...\ndeploy: build\n\t@$(KUBECTL) apply -f k8s/\n\t-echo done \\\n\t  twice\nclean:\n\trm -rf $(OUT)\n",
		"sub/Makefile":   "all:\n\tgit push -f\n",
		"lib.sh":         "alias k=kubectl\n",
	})
	check(t, []tc{
		{src: "./deploy.sh", want: []string{"kubectl --context prod apply -f k8s/ [script ./deploy.sh]"}},
		{src: "bash scripts/fix.sh", want: []string{"git push --force [bash scripts/fix.sh]"}},
		{src: "sh scripts/missing.sh", want: []string{"sh scripts/missing.sh"}},
		{src: "./tool.py", want: []string{"./tool.py"}},
		{src: "./missing.sh --flag", want: []string{"./missing.sh --flag"}},
		{src: "/usr/bin/git push", want: []string{"/usr/bin/git push"}},
		{src: "source lib.sh; k delete x", want: []string{"source lib.sh", "alias k=kubectl [source lib.sh]", "kubectl delete x [alias k]"}},
		{src: "make deploy", want: []string{"kubectl --context prod apply -f k8s/ [make deploy]", "echo done twice [make deploy]"}},
		{src: "make", want: []string{"go build ./... [make build]"}},
		{src: "make clean", want: []string{"rm -rf ? [make clean]"}},
		{src: "make -C sub", want: []string{"git push -f [make all]"}},
		{src: "make nope", want: []string{"make nope"}},
		{src: "make -f other.mk x", want: []string{"make -f other.mk x"}},
		{src: "make $T", want: []string{"make ?"}},
	}, Options{ReadFile: fs})
	check(t, []tc{
		{src: "./deploy.sh", want: []string{"./deploy.sh"}},
		{src: "make deploy", want: []string{"make deploy"}},
	}, Options{})
}

func TestShebangDetection(t *testing.T) {
	for line, want := range map[string]bool{
		"#!/bin/sh": true, "#!/usr/bin/env bash": true, "#!/bin/zsh -e": true, "#!/usr/bin/python3": false, "#!/usr/bin/env node": false,
	} {
		if shebangIsShell(line) != want {
			t.Errorf("shebangIsShell(%q) != %v", line, want)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"git push -f", "bash -c 'eval \"rm -rf /\"'", "for c in a b; do k $c; done", "f(){ f; }; f", "make deploy", "a=$(b) c <(d) > e"} {
		f.Add(s)
	}
	fs := files(map[string]string{"Makefile": "all:\n\tmake all\n", "./x.sh": "./x.sh\n"})
	f.Fuzz(func(t *testing.T, src string) {
		r := Parse(src, Options{ReadFile: fs})
		for _, c := range r.Commands {
			if c.Depth > 6 {
				t.Fatalf("depth %d beyond the limit for %q", c.Depth, src)
			}
		}
	})
}
