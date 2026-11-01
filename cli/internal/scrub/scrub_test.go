package scrub

import (
	"strings"
	"testing"
)

// Test secrets are assembled at run time so this file never contains a real-looking token.
func rep(s string, n int) string { return strings.Repeat(s, n) }

func TestScrubKnownTokens(t *testing.T) {
	tests := []struct {
		name, in, kind, secret string
	}{
		{"aws key id", "key AKIA" + rep("Q", 16) + " used", "aws-access-key-id", "AKIA" + rep("Q", 16)},
		{"github classic", "token ghp_" + rep("a1", 18), "github-token", "ghp_" + rep("a1", 18)},
		{"github fine grained", "github_pat_" + rep("Ab_9", 6), "github-token", "github_pat_" + rep("Ab_9", 6)},
		{"slack", "xoxb-" + rep("12345-", 3) + "abc", "slack-token", "xoxb-12345"},
		{"google api key", "AIza" + rep("x", 35), "google-api-key", "AIza" + rep("x", 35)},
		{"anthropic", "sk-ant-" + rep("Zq9", 10), "anthropic-key", "sk-ant-" + rep("Zq9", 10)},
		{"openai", "sk-proj-" + rep("aB3", 12), "openai-key", "sk-proj-" + rep("aB3", 12)},
		{"stripe", "sk_live_" + rep("9xZ", 8), "stripe-key", "sk_live_" + rep("9xZ", 8)},
		{"jwt", "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "jwt", "eyJhbGciOiJIUzI1NiJ9"},
		{"private key", "x\n-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA\n-----END RSA PRIVATE KEY-----\ny", "private-key", "MIIEpAIBAAKCAQEA"},
		{"truncated private key", "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk", "private-key", "b3BlbnNzaC1rZXk"},
		{"postgres url", "postgres://app:S3cretPass@db.internal:5432/x", "connection-string-password", "S3cretPass"},
		{"mongodb srv", "mongodb+srv://u:hunter2hunter2@cluster0.x.net", "connection-string-password", "hunter2hunter2"},
		{"secret env", "export DB_PASSWORD=correct-horse", "secret-env", "correct-horse"},
		{"secret env colon", "STRIPE_API_KEY: abc123def456", "secret-env", "abc123def456"},
		{"password field", `{"password": "letmein99"}`, "password", "letmein99"},
		{"high entropy near key", "client_secret = " + "Zx8Qp2Lm9Rt4Vb7Nc1Hy6Kd3Fs5Gj0Aw", "high-entropy-secret", "Zx8Qp2Lm9Rt4Vb7Nc1Hy6Kd3Fs5Gj0Aw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, fs := New().String(tt.in)
			if strings.Contains(out, tt.secret) {
				t.Errorf("secret survived: %q", out)
			}
			if !strings.Contains(out, "[REDACTED:"+tt.kind+"]") {
				t.Errorf("no %s redaction: %q (findings %v)", tt.kind, out, fs)
			}
			again, _ := New().String(out)
			if again != out {
				t.Errorf("not idempotent: %q → %q", out, again)
			}
		})
	}
}

func TestNoFalsePositives(t *testing.T) {
	clean := []string{
		"commit 9f3c2ab4e5d6f7a8b9c0d1e2f3a4b5c6d7e8f9a0 by dev",
		"sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"request id 550e8400-e29b-41d4-a716-446655440000",
		"path /home/dev/src/profile/phone_number_validation_handler.ts",
		"token_count: 1234",
		"key: user-service/src/consumers/user.ts",
		"auth: enabled",
		"password policy requires 12 characters",
		"see https://example.com/docs/api",
		"kubectl get pods -n staging",
	}
	for _, in := range clean {
		if out, fs := New().String(in); out != in || len(fs) != 0 {
			t.Errorf("false positive: %q → %q %v", in, out, fs)
		}
	}
}

func TestLiteralsAndFindings(t *testing.T) {
	s := New("configured-db-pass-123", "short", "")
	out, fs := s.String("dsn uses configured-db-pass-123 twice: configured-db-pass-123")
	if strings.Contains(out, "configured-db-pass-123") || len(fs) != 1 || fs[0].Count != 2 || fs[0].Kind != "configured-secret" {
		t.Errorf("out %q findings %v", out, fs)
	}
	if out, _ := s.String("short words stay"); out != "short words stay" {
		t.Errorf("short literal redacted: %q", out)
	}
}

func TestValueKeepsShape(t *testing.T) {
	in := map[string]any{
		"stdout":      "DB_PASSWORD=hunter22 ok",
		"stderr":      "",
		"interrupted": false,
		"lines":       []any{"x", "token ghp_" + rep("a1", 18), 3.0},
		"nested":      map[string]any{"url": "redis://u:p4ssw0rd!@h:6379"},
	}
	out, fs := New().Value(in)
	m := out.(map[string]any)
	if m["interrupted"] != false || m["stderr"] != "" || m["lines"].([]any)[2] != 3.0 {
		t.Errorf("shape changed: %v", m)
	}
	if strings.Contains(m["stdout"].(string), "hunter22") || strings.Contains(m["lines"].([]any)[1].(string), "ghp_") ||
		strings.Contains(m["nested"].(map[string]any)["url"].(string), "p4ssw0rd") {
		t.Errorf("secret survived: %v", m)
	}
	total := 0
	for _, f := range fs {
		total += f.Count
	}
	if total != 3 {
		t.Errorf("findings = %v", fs)
	}
}

func TestEntropy(t *testing.T) {
	if Entropy("") != 0 || Entropy("aaaa") != 0 || Entropy("ab") != 1 {
		t.Errorf("entropy: %v %v %v", Entropy(""), Entropy("aaaa"), Entropy("ab"))
	}
	if highEntropy(rep("ab", 20)) || !highEntropy("Zx8Qp2Lm9Rt4Vb7Nc1Hy6Kd3Fs5Gj0Aw") || highEntropy("short") {
		t.Error("highEntropy thresholds")
	}
	if out, _ := New().String("api_key = " + rep("abcd", 10)); strings.Contains(out, "REDACTED") {
		t.Errorf("low-entropy value near key redacted: %q", out)
	}
}

func FuzzScrub(f *testing.F) {
	f.Add("DB_PASSWORD=x postgres://a:b@c ghp_" + rep("a1", 18))
	f.Add("-----BEGIN PRIVATE KEY-----")
	f.Add("")
	f.Fuzz(func(t *testing.T, in string) {
		out, _ := New().String(in)
		again, _ := New().String(out)
		if again != out {
			t.Fatalf("not idempotent: %q → %q → %q", in, out, again)
		}
	})
}
