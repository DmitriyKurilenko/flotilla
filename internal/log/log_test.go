package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{})
	logger.Info("hello", "k", "v")

	out := buf.String()
	if !strings.Contains(out, "msg=hello") {
		t.Errorf("text output should contain msg=hello, got %q", out)
	}
	if !strings.Contains(out, "k=v") {
		t.Errorf("text output should contain k=v, got %q", out)
	}
}

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{JSON: true})
	logger.Info("hello", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("JSON output should be valid JSON: %v\n%q", err, buf.String())
	}
	if rec["msg"] != "hello" {
		t.Errorf("expected msg=hello, got %v", rec["msg"])
	}
	if rec["k"] != "v" {
		t.Errorf("expected k=v, got %v", rec["k"])
	}
}

func TestNew_QuietSuppressesInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{Quiet: true})
	logger.Info("should be suppressed")
	logger.Error("should appear")

	out := buf.String()
	if strings.Contains(out, "should be suppressed") {
		t.Errorf("quiet mode should suppress info, got %q", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Errorf("quiet mode should still emit error, got %q", out)
	}
}
