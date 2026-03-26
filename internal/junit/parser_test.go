package junit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata", name)
}

func TestParse_BasicTestSuites(t *testing.T) {
	data, err := os.ReadFile(testdataPath("basic.xml"))
	if err != nil {
		t.Fatal(err)
	}

	cases, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(cases) != 4 {
		t.Fatalf("expected 4 test cases, got %d", len(cases))
	}

	// test_add - passed
	assertCase(t, cases[0], "test_add", StatusPassed, "")
	// test_subtract - passed
	assertCase(t, cases[1], "test_subtract", StatusPassed, "")
	// test_divide - failed
	assertCase(t, cases[2], "test_divide", StatusFailed, "expected 2 but got 3")
	// test_multiply - skipped
	assertCase(t, cases[3], "test_multiply", StatusSkipped, "not implemented yet")

	// Check duration conversion (0.010s = 10ms)
	if cases[0].DurationMs != 10 {
		t.Errorf("expected DurationMs=10, got %d", cases[0].DurationMs)
	}

	// Check suite name
	if cases[0].Suite != "com.example.TestMath" {
		t.Errorf("expected Suite=%q, got %q", "com.example.TestMath", cases[0].Suite)
	}
}

func TestParse_SingleTestSuite(t *testing.T) {
	data, err := os.ReadFile(testdataPath("single_suite.xml"))
	if err != nil {
		t.Fatal(err)
	}

	cases, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(cases) != 3 {
		t.Fatalf("expected 3 test cases, got %d", len(cases))
	}

	assertCase(t, cases[0], "test_login", StatusPassed, "")
	assertCase(t, cases[1], "test_logout", StatusPassed, "")
	assertCase(t, cases[2], "test_register", StatusError, "connection refused")

	// Check file attribute
	if cases[0].File != "tests/test_auth.py" {
		t.Errorf("expected File=%q, got %q", "tests/test_auth.py", cases[0].File)
	}
}

func TestParse_InvalidXML(t *testing.T) {
	_, err := Parse([]byte("not xml at all"))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestParse_EmptyTestSuites(t *testing.T) {
	_, err := Parse([]byte(`<?xml version="1.0"?><testsuites></testsuites>`))
	if err == nil {
		t.Error("expected error for empty testsuites")
	}
}

func TestSummarize(t *testing.T) {
	cases := []TestCase{
		{Status: StatusPassed},
		{Status: StatusPassed},
		{Status: StatusFailed},
		{Status: StatusSkipped},
		{Status: StatusError},
	}

	s := Summarize(cases)
	if s.Total != 5 {
		t.Errorf("Total: got %d, want 5", s.Total)
	}
	if s.Passed != 2 {
		t.Errorf("Passed: got %d, want 2", s.Passed)
	}
	if s.Failed != 1 {
		t.Errorf("Failed: got %d, want 1", s.Failed)
	}
	if s.Skipped != 1 {
		t.Errorf("Skipped: got %d, want 1", s.Skipped)
	}
	if s.Errored != 1 {
		t.Errorf("Errored: got %d, want 1", s.Errored)
	}
}

func assertCase(t *testing.T, tc TestCase, name string, status Status, errMsg string) {
	t.Helper()
	if tc.Name != name {
		t.Errorf("expected Name=%q, got %q", name, tc.Name)
	}
	if tc.Status != status {
		t.Errorf("%s: expected Status=%q, got %q", name, status, tc.Status)
	}
	if errMsg != "" && tc.ErrorMsg != errMsg {
		t.Errorf("%s: expected ErrorMsg=%q, got %q", name, errMsg, tc.ErrorMsg)
	}
}
