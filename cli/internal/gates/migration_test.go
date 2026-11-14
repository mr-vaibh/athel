package gates

import (
	"strings"
	"testing"
)

func TestIsMigration(t *testing.T) {
	for p, want := range map[string]bool{
		"migrations/002_phone.up.sql": true, "db/migrate/003.sql": true, "svc/migration/1.sql": true,
		"migrations/README.md": false, "src/migrations.go": false, "sql/schema.sql": false,
	} {
		if IsMigration(p) != want {
			t.Errorf("%s: %v", p, !want)
		}
	}
}

func TestOnlineSafety(t *testing.T) {
	tests := []struct {
		sql      string
		contract bool
		want     string
	}{
		{"ALTER TABLE users ADD COLUMN phone text;", false, ""},
		{"alter table users add column phone text not null;", false, "NOT NULL column without a default"},
		{"ALTER TABLE users ADD COLUMN n int NOT NULL DEFAULT 0;", false, ""},
		{"CREATE INDEX idx ON users (phone);", false, "without CONCURRENTLY"},
		{"CREATE UNIQUE INDEX CONCURRENTLY idx ON users (phone);", false, ""},
		{"ALTER TABLE users ALTER COLUMN phone TYPE varchar(20);", false, "changes a column type"},
		{"ALTER TABLE users RENAME COLUMN phone TO phone_number;", false, "renames"},
		{"ALTER TABLE users DROP COLUMN legacy;", false, "drops"},
		{"ALTER TABLE users DROP COLUMN legacy;", true, ""},
		{"DROP TABLE old_profiles;", false, "drops"},
		{"ALTER TABLE orders ADD CONSTRAINT fk FOREIGN KEY (user_id) REFERENCES users (id);", false, "NOT VALID"},
		{"ALTER TABLE orders ADD CONSTRAINT fk FOREIGN KEY (user_id) REFERENCES users (id) NOT VALID;", false, ""},
		{"-- CREATE INDEX idx ON users (phone);\nSELECT 1; /* DROP TABLE x; */", false, ""},
	}
	for _, tt := range tests {
		got := strings.Join(OnlineSafety(tt.sql, tt.contract), "; ")
		if (tt.want == "") != (got == "") || !strings.Contains(got, tt.want) {
			t.Errorf("%q (contract %v): %q, want %q", tt.sql, tt.contract, got, tt.want)
		}
	}
}

func TestDownFor(t *testing.T) {
	for up, want := range map[string]string{
		"migrations/002_phone.up.sql": "migrations/002_phone.down.sql",
		"migrations/002_phone.sql":    "",
		"db/migrate/3.UP.sql":         "",
	} {
		if got := DownFor(up); got != want {
			t.Errorf("%s: %q, want %q", up, got, want)
		}
	}
}
