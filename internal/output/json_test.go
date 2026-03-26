package output

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestJSONWritesToRealStdout(t *testing.T) {
	// Redirect os.Stdout to a pipe so we can capture output
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	// Also redirect the output.Stdout var to prove JSON bypasses it
	oldOutputStdout := Stdout
	Stdout = os.Stderr // simulate --json mode
	defer func() { Stdout = oldOutputStdout }()

	input := map[string]string{"key": "value"}
	if err := JSON(input); err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	_ = w.Close()

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	got := string(buf[:n])

	// Should be valid JSON
	var parsed map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &parsed); err != nil {
		t.Fatalf("JSON() output is not valid JSON: %v\nGot: %s", err, got)
	}

	if parsed["key"] != "value" {
		t.Errorf("JSON() output mismatch: got %v", parsed)
	}
}

func TestJSONIndented(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	type nested struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	if err := JSON(nested{Name: "test", Count: 42}); err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	_ = w.Close()

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	got := string(buf[:n])

	// Verify indentation
	if !strings.Contains(got, "  \"name\"") {
		t.Errorf("JSON() output not indented:\n%s", got)
	}
}

func TestJSONNil(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	if err := JSON(nil); err != nil {
		t.Fatalf("JSON(nil) error: %v", err)
	}

	_ = w.Close()

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	got := strings.TrimSpace(string(buf[:n]))

	if got != "null" {
		t.Errorf("JSON(nil) = %q, want %q", got, "null")
	}
}
