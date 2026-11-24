// Command systest is the shop's system check: it builds the users, orders and gateway services
// from the folders it is given, starts them together on local ports, wired to each other, and
// runs the cross-service flows in a JSON file against them.
//
//	go run ./systest --flows systest/flows.json --users DIR --orders DIR --gateway DIR
//
// It prints one JSON object with every flow's result. Exit 0 when every flow passes, 1 when one
// fails, 2 when the check could not run.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
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
)

// services start in this order: each may call the ones before it.
var services = []string{"users", "orders", "gateway"}

// flow is one request through the system and what it must answer.
type flow struct {
	Name     string `json:"name"`
	Service  string `json:"service"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Status   int    `json:"status"`
	Contains string `json:"contains"`
}

// result is one flow's outcome; Got says what came back when it failed.
type result struct {
	Flow string `json:"flow"`
	OK   bool   `json:"ok"`
	Got  string `json:"got,omitempty"`
}

var (
	startTimeout = 15 * time.Second
	flowTimeout  = 5 * time.Second
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(argv []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("systest", flag.ContinueOnError)
	fs.SetOutput(errOut)
	flowsFile := fs.String("flows", "", "the flows file (JSON)")
	dirs := map[string]*string{}
	for _, name := range services {
		dirs[name] = fs.String(name, "", "the "+name+" service folder")
	}
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	flows, err := loadFlows(*flowsFile)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "error: "+err.Error())
		return 2
	}
	given := map[string]string{}
	for name, dir := range dirs {
		if *dir != "" {
			given[name] = *dir
		}
	}
	results, err := check(context.Background(), given, flows)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "error: "+err.Error())
		return 2
	}
	ok := true
	for _, r := range results {
		ok = ok && r.OK
	}
	_ = json.NewEncoder(out).Encode(map[string]any{"ok": ok, "results": results}) // plain values always encode
	if !ok {
		return 1
	}
	return 0
}

func loadFlows(path string) ([]flow, error) {
	if path == "" {
		return nil, errors.New("--flows is required")
	}
	b, err := os.ReadFile(path) //nolint:gosec // the caller names its flows file
	if err != nil {
		return nil, err
	}
	var doc struct {
		Flows []flow `json:"flows"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc.Flows, nil
}

// check builds and starts every service in dirs and runs the flows. A flow that fails is a
// result with OK false; an error means the check itself could not run.
func check(ctx context.Context, dirs map[string]string, flows []flow) ([]result, error) {
	bin, err := os.MkdirTemp("", "systest-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(bin) }()
	sys := &system{ports: map[string]int{}}
	defer sys.stop()
	for _, name := range services {
		if dir, ok := dirs[name]; ok {
			if err := sys.start(ctx, name, dir, filepath.Join(bin, name)); err != nil {
				return nil, err
			}
		}
	}
	results := make([]result, 0, len(flows))
	for _, f := range flows {
		r, err := sys.run(ctx, f)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

type system struct {
	ports map[string]int
	procs []*exec.Cmd
}

// env wires a service to the users service when it runs, through the settings the mains read.
func (s *system) env(port int) []string {
	env := append(os.Environ(), fmt.Sprintf("ADDR=127.0.0.1:%d", port))
	if p, ok := s.ports["users"]; ok {
		users := fmt.Sprintf("http://127.0.0.1:%d", p)
		env = append(env, "USERS_URL="+users, "ROUTE_USERS="+users)
	}
	return env
}

func (s *system) start(ctx context.Context, name, dir, bin string) error {
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, ".") //nolint:gosec // go build of a shop service
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

func (s *system) run(ctx context.Context, f flow) (result, error) {
	port, ok := s.ports[f.Service]
	if !ok {
		return result{}, fmt.Errorf("flow %q calls %s, which is not running", f.Name, f.Service)
	}
	ctx, cancel := context.WithTimeout(ctx, flowTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, f.Method, fmt.Sprintf("http://127.0.0.1:%d%s", port, f.Path), nil)
	if err != nil {
		return result{}, fmt.Errorf("flow %q: %w", f.Name, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result{Flow: f.Name, Got: err.Error()}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10)) // a short read still shows what came back
	if resp.StatusCode != f.Status || !strings.Contains(string(body), f.Contains) {
		return result{Flow: f.Name, Got: strings.TrimSpace(fmt.Sprintf("%d %s", resp.StatusCode, body))}, nil
	}
	return result{Flow: f.Name, OK: true}, nil
}

// lockedBuffer collects a service's output; exec copies into it from its own goroutine.
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
