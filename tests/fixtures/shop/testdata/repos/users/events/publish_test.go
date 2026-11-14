package events

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestPublishUserUpdatedNeverLogsThePhone(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(nil) })
	PublishUserUpdated("u-1", "+44 20 7946 0000")
	out := buf.String()
	if !strings.Contains(out, Topic) || !strings.Contains(out, "phone_set=true") || strings.Contains(out, "7946") {
		t.Fatalf("log line %q", out)
	}
}
