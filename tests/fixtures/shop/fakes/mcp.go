package fakes

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Server is a fake MCP server speaking JSON-RPC 2.0 over stdio, one message per line. Name
// picks its tools: grafana, atlassian, or shopmetrics (a server no built-in entry knows, for
// the FR-DISC-008 contract test).
type Server struct {
	Name string
	Data *Data
}

type rpcRequest struct {
	ID     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Params json.RawMessage  `json:"params"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	run         func(args map[string]string) any
}

// Serve answers requests from in until it ends.
func (s Server) Serve(in io.Reader, out io.Writer) error {
	tools := s.tools()
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req rpcRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32700, "message": "parse error"}}); err != nil {
				return err
			}
			continue
		}
		if req.ID == nil {
			continue // a notification, such as notifications/initialized
		}
		if err := enc.Encode(s.answer(req, tools)); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s Server) answer(req rpcRequest, tools map[string]tool) map[string]any {
	resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p) // absent params keep the default
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-06-18"
		}
		resp["result"] = map[string]any{"protocolVersion": p.ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{"name": "shopmcp-" + s.Name, "version": "0.0.0"}}
	case "ping":
		resp["result"] = map[string]any{}
	case "tools/list":
		list := make([]tool, 0, len(tools))
		for _, n := range keys(tools) {
			list = append(list, tools[n])
		}
		resp["result"] = map[string]any{"tools": list}
	case "tools/call":
		resp["result"] = call(req.Params, tools)
	default:
		resp["error"] = map[string]any{"code": -32601, "message": "method not found: " + req.Method}
	}
	return resp
}

func call(params json.RawMessage, tools map[string]tool) map[string]any {
	var p struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return toolResult("invalid arguments: "+err.Error(), true)
	}
	t, ok := tools[p.Name]
	if !ok {
		return toolResult("unknown tool "+p.Name, true)
	}
	b, _ := json.Marshal(t.run(p.Arguments)) // fixture values: always encode
	return toolResult(string(b), false)
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isError}
}

func schemaOf(props ...string) map[string]any {
	p := map[string]any{}
	for _, name := range props {
		p[name] = map[string]any{"type": "string"}
	}
	return map[string]any{"type": "object", "properties": p}
}

func (s Server) tools() map[string]tool {
	var list []tool
	switch s.Name {
	case "grafana":
		list = []tool{
			{Name: "query_prometheus", Description: "Run a PromQL query over the lookback window", InputSchema: schemaOf("expr"), run: s.prometheus},
			{Name: "search_tempo_traces", Description: "Search traces", InputSchema: schemaOf("query"), run: s.traces},
			{Name: "query_loki_logs", Description: "Query logs with LogQL", InputSchema: schemaOf("logql"), run: s.loki},
			{Name: "alerting_rules_read", Description: "List alert rules and their state", InputSchema: schemaOf(), run: func(map[string]string) any { return s.Data.Grafana.Alerts }},
			{Name: "list_incidents", Description: "List open incidents", InputSchema: schemaOf(), run: func(map[string]string) any { return []any{} }},
		}
	case "atlassian":
		list = []tool{
			{Name: "searchJiraIssuesUsingJql", Description: "Search Jira issues with JQL", InputSchema: schemaOf("jql"), run: s.jql},
			{Name: "getJiraIssue", Description: "Read one issue", InputSchema: schemaOf("issueIdOrKey"), run: s.issue},
			{Name: "listJiraIssueComments", Description: "Read an issue's comments", InputSchema: schemaOf("issueIdOrKey"), run: s.comments},
		}
	case "shopmetrics":
		list = []tool{{Name: "query", Description: "Series of one metric", InputSchema: schemaOf("metric"), run: s.metric}}
	}
	out := map[string]tool{}
	for _, t := range list {
		out[t.Name] = t
	}
	return out
}

var matcher = regexp.MustCompile(`(\w+)="([^"]*)"`)

// prometheus returns the series whose metric appears in the expression and whose labels match
// every equality matcher in it, as rates per minute over 7 days.
func (s Server) prometheus(args map[string]string) any {
	expr := args["expr"]
	out := []Series{}
	for _, ser := range s.Data.Grafana.Series {
		if !strings.Contains(expr, ser.Metric) {
			continue
		}
		match := true
		for _, m := range matcher.FindAllStringSubmatch(expr, -1) {
			match = match && ser.Labels[m[1]] == m[2]
		}
		if match {
			out = append(out, ser)
		}
	}
	return map[string]any{"window": "7d", "series": out}
}

func (s Server) metric(args map[string]string) any {
	out := []Series{}
	for _, ser := range s.Data.Grafana.Series {
		if ser.Metric == args["metric"] {
			out = append(out, ser)
		}
	}
	return out
}

func (s Server) traces(map[string]string) any {
	return map[string]any{"sampling": s.Data.Grafana.Traces.Sampling, "note": fmt.Sprintf("traces are sampled at %g", s.Data.Grafana.Traces.Sampling),
		"traces": s.Data.Grafana.Traces.Spans}
}

var appMatcher = regexp.MustCompile(`app="([^"]*)"`)

func (s Server) loki(args map[string]string) any {
	app := ""
	if m := appMatcher.FindStringSubmatch(args["logql"]); m != nil {
		app = m[1]
	}
	lines := []string{}
	for _, l := range s.Data.Grafana.Logs {
		if app == "" || l.App == app {
			lines = append(lines, l.Line)
		}
	}
	return lines
}

func (s Server) jql(args map[string]string) any {
	out := []Issue{}
	for _, i := range s.Data.Jira.Issues {
		if strings.Contains(args["jql"], i.Component) || strings.Contains(args["jql"], i.Key) {
			out = append(out, i)
		}
	}
	return map[string]any{"issues": out}
}

func (s Server) find(key string) *Issue {
	for i := range s.Data.Jira.Issues {
		if s.Data.Jira.Issues[i].Key == key {
			return &s.Data.Jira.Issues[i]
		}
	}
	return nil
}

func (s Server) issue(args map[string]string) any {
	if i := s.find(args["issueIdOrKey"]); i != nil {
		return i
	}
	return map[string]string{"error": "no issue " + args["issueIdOrKey"]}
}

func (s Server) comments(args map[string]string) any {
	if i := s.find(args["issueIdOrKey"]); i != nil {
		return map[string]any{"comments": i.Comments}
	}
	return map[string]any{"comments": []string{}}
}
