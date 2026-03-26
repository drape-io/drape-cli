package output

import (
	"bytes"
	"os"
	"testing"
)

func TestInfoWritesToStdout(t *testing.T) {
	defer Reset()
	var buf bytes.Buffer
	Stdout = &buf

	Info("hello %s", "world")

	if got := buf.String(); got != "hello world\n" {
		t.Errorf("Info() wrote %q, want %q", got, "hello world\n")
	}
}

func TestErrorWritesToStderr(t *testing.T) {
	defer Reset()
	var buf bytes.Buffer
	Stderr = &buf

	Error("bad %s", "thing")

	want := "Error: bad thing\n"
	if got := buf.String(); got != want {
		t.Errorf("Error() wrote %q, want %q", got, want)
	}
}

func TestVerboseOnlyWhenEnabled(t *testing.T) {
	defer Reset()
	var buf bytes.Buffer
	Stderr = &buf

	SetVerbose(false)
	Verbose("should not appear")
	if buf.Len() != 0 {
		t.Errorf("Verbose() wrote when disabled: %q", buf.String())
	}

	SetVerbose(true)
	Verbose("visible")
	want := "[verbose] visible\n"
	if got := buf.String(); got != want {
		t.Errorf("Verbose() wrote %q, want %q", got, want)
	}
}

func TestQuietSuppressesInfoAndVerbose(t *testing.T) {
	defer Reset()
	var stdoutBuf, stderrBuf bytes.Buffer
	Stdout = &stdoutBuf
	Stderr = &stderrBuf
	SetQuiet(true)
	SetVerbose(true)

	Info("should not appear")
	Verbose("should not appear")

	if stdoutBuf.Len() != 0 {
		t.Errorf("Info() wrote in quiet mode: %q", stdoutBuf.String())
	}
	if stderrBuf.Len() != 0 {
		t.Errorf("Verbose() wrote in quiet mode: %q", stderrBuf.String())
	}
}

func TestQuietPreservesErrors(t *testing.T) {
	defer Reset()
	var stderrBuf bytes.Buffer
	Stderr = &stderrBuf
	SetQuiet(true)

	Error("something broke")

	want := "Error: something broke\n"
	if got := stderrBuf.String(); got != want {
		t.Errorf("Error() in quiet mode wrote %q, want %q", got, want)
	}
}

func TestResetRestoresDefaults(t *testing.T) {
	Stdout = os.Stderr // intentionally wrong
	Stderr = os.Stdout // intentionally wrong
	SetVerbose(true)
	SetQuiet(true)

	Reset()

	if Stdout != os.Stdout {
		t.Error("Reset() did not restore Stdout")
	}
	if Stderr != os.Stderr {
		t.Error("Reset() did not restore Stderr")
	}

	// After reset, quiet should be false — Info should write
	var buf bytes.Buffer
	Stdout = &buf
	defer Reset()

	Info("after reset")
	if buf.Len() == 0 {
		t.Error("Info() produced no output after Reset(), expected quiet=false")
	}
}
