// Package systest is the shop's system check (M5-plan §5.9, A1): it builds the Go services from
// the folders it is given, starts them together on local ports, wired to each other, and runs the
// cross-service flows against them. M5's merge-order rehearsal and its eval grader run it after
// each merge step, so "every intermediate state works" is checked by real requests.
package systest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// Services start in this order: each may call the ones before it.
var Services = []string{"users", "orders", "gateway"}

// Flow is one request through the system and what it must answer.
type Flow struct {
	Name     string `yaml:"name" json:"name"`
	Service  string `yaml:"service" json:"service"`
	Method   string `yaml:"method" json:"method"`
	Path     string `yaml:"path" json:"path"`
	Status   int    `yaml:"status" json:"status"`
	Contains string `yaml:"contains" json:"contains"`
}

// Result is one flow's outcome; Got says what came back when it failed.
type Result struct {
	Flow string `json:"flow"`
	OK   bool   `json:"ok"`
	Got  string `json:"got,omitempty"`
}

// LoadFlows reads a flows file: {flows: [...]}.
func LoadFlows(path string) ([]Flow, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the caller names its flows file
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var doc struct {
		Flows []Flow `yaml:"flows"`
	}
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc.Flows, nil
}

// Timeouts bound each phase, so a hung service fails the check instead of the caller.
var (
	startTimeout = 10 * time.Second
	flowTimeout  = 5 * time.Second
)

// Run builds and starts every service named in dirs (service name to source folder) and runs
// flows against them. A flow that fails is a Result with OK false; an error means the check
// itself could not run (a build failed, a service never came up, a flow names no running
// service).
func Run(ctx context.Context, dirs map[string]string, flows []Flow) ([]Result, error) {
	bin, err := os.MkdirTemp("", "shopsys-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(bin) }()
	sys := &system{ports: map[string]int{}}
	defer sys.stop()
	for _, name := range Services {
		if dir, ok := dirs[name]; ok {
			if err := sys.start(ctx, name, dir, filepath.Join(bin, name)); err != nil {
				return nil, err
			}
		}
	}
	results := make([]Result, 0, len(flows))
	for _, f := range flows {
		r, err := sys.run(ctx, f)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// Failed lists the flows that did not pass.
func Failed(results []Result) []string {
	var out []string
	for _, r := range results {
		if !r.OK {
			out = append(out, r.Flow+": "+r.Got)
		}
	}
	return out
}

type system struct {
	ports map[string]int
	procs []*exec.Cmd
}

// env wires a service to the others already started, through the settings the seeded mains read.
func (s *system) env(port int) []string {
	env := append(os.Environ(), fmt.Sprintf("ADDR=127.0.0.1:%d", port))
	if p, ok := s.ports["users"]; ok {
		users := fmt.Sprintf("http://127.0.0.1:%d", p)
		env = append(env, "USERS_URL="+users, "ROUTE_USERS="+users)
	}
	return env
}

func (s *system) start(ctx context.Context, name, dir, bin string) error {
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, ".") //nolint:gosec // go build of a fixture service
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	logs := &lockedBuffer{}
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env, cmd.Stdout, cmd.Stderr = s.env(port), logs, logs
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	s.procs = append(s.procs, cmd)
	s.ports[name] = port
	if err := waitListening(port); err != nil {
		return fmt.Errorf("%s did not start: %w: %s", name, err, strings.TrimSpace(logs.String()))
	}
	return nil
}

func (s *system) stop() {
	for _, p := range s.procs {
		_ = p.Process.Kill()
		_ = p.Wait()
	}
}

func (s *system) run(ctx context.Context, f Flow) (Result, error) {
	port, ok := s.ports[f.Service]
	if !ok {
		return Result{}, fmt.Errorf("flow %q calls %s, which is not running", f.Name, f.Service)
	}
	ctx, cancel := context.WithTimeout(ctx, flowTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, f.Method, fmt.Sprintf("http://127.0.0.1:%d%s", port, f.Path), nil)
	if err != nil {
		return Result{}, fmt.Errorf("flow %q: %w", f.Name, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Flow: f.Name, Got: err.Error()}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10)) // a short read still shows what came back
	got := strings.TrimSpace(fmt.Sprintf("%d %s", resp.StatusCode, body))
	if resp.StatusCode != f.Status || !strings.Contains(string(body), f.Contains) {
		return Result{Flow: f.Name, Got: got}, nil
	}
	return Result{Flow: f.Name, OK: true}, nil
}

// lockedBuffer collects a service's output; exec copies into it from its own goroutine while the
// service runs, and start reads it when the service fails to come up.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("no TCP port")
	}
	return addr.Port, nil
}

func waitListening(port int) error {
	deadline := time.Now().Add(startTimeout)
	for {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			return c.Close()
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}
