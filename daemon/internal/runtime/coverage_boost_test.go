package runtime

import (
	"bufio"
	"context"
	"io"
	"runtime"
	"strings"
	"testing"
)

func TestExecCommandRunnerRunInteractive(t *testing.T) {
	r := ExecCommandRunner{}
	ctx := context.Background()

	var err error
	if runtime.GOOS == "windows" {
		err = r.RunInteractive(ctx, io.Discard, io.Discard, "cmd.exe", "/C", "echo hi")
	} else {
		err = r.RunInteractive(ctx, io.Discard, io.Discard, "/bin/sh", "-lc", "printf hi")
	}
	if err != nil {
		t.Fatalf("RunInteractive returned error: %v", err)
	}
}

func TestBufioStdinReaderReadLine(t *testing.T) {
	reader := &BufioStdinReader{reader: bufio.NewReader(strings.NewReader("yes\n"))}
	got, err := reader.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine error: %v", err)
	}
	if got != "yes" {
		t.Fatalf("got=%q want=%q", got, "yes")
	}
}
