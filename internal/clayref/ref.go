package clayref

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// ClayCommit is the upstream clay commit glay is checked against.
// go generate downloads its clay.h into [ClayDir].
const ClayCommit = "e6cc36941ab2af5d81107617039d6f527a1c660b"

//go:generate go run fetchclay.go

// sourceDir returns the directory of this package, which holds csrc/clayref.c.
func sourceDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

// ClayDir returns the directory containing clay.h: $CLAY_DIR or <repo>/local/clay.
func ClayDir() string {
	if dir := os.Getenv("CLAY_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(sourceDir(), "..", "..", "local", "clay")
}

var buildOnce struct {
	sync.Once
	path string
	err  error
}

// Build compiles the reference harness, caching the binary in the OS temp directory
// keyed by the hash of its sources so parallel fuzz workers share one build.
// Build skips the test if no C compiler or clay.h is available.
func Build(tb testing.TB) string {
	tb.Helper()
	buildOnce.Do(func() {
		buildOnce.path, buildOnce.err = build()
	})
	if errors.Is(buildOnce.err, errSkip) {
		tb.Skip(buildOnce.err)
	} else if buildOnce.err != nil {
		tb.Fatal(buildOnce.err)
	}
	return buildOnce.path
}

var errSkip = errors.New("clayref: reference harness unavailable")

func build() (string, error) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		return "", fmt.Errorf("%w: no C compiler: %v", errSkip, err)
	}
	src := filepath.Join(sourceDir(), "csrc", "clayref.c")
	hdr := filepath.Join(ClayDir(), "clay.h")
	h := sha256.New()
	for _, f := range []string{src, hdr} {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("%w: %v (run go generate ./internal/clayref or set CLAY_DIR)", errSkip, err)
		}
		h.Write(b)
	}
	bin := filepath.Join(os.TempDir(), "glay-clayref-"+hex.EncodeToString(h.Sum(nil))[:16])
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	tmp := fmt.Sprintf("%s.%d.tmp", bin, os.Getpid())
	cmd := exec.Command(cc, "-std=c99", "-O1", "-I"+ClayDir(), "-o", tmp, src, "-lm")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("clayref: compiling harness: %v\n%s", err, out)
	}
	if err := os.Rename(tmp, bin); err != nil {
		return "", err
	}
	return bin, nil
}

// ErrTimeout is returned by [Stream.Run] when the harness does not respond within [Stream.Timeout].
// The harness process is killed and the Stream is no longer usable.
var ErrTimeout = errors.New("clayref: harness timed out")

// Stream is a running reference harness in -stream mode. Not safe for concurrent use.
type Stream struct {
	// Timeout bounds how long Run waits for a response. Zero means no timeout.
	Timeout time.Duration
	cmd     *exec.Cmd
	in      io.WriteCloser
	out     *bufio.Reader
}

// NewStream starts the reference harness binary built by [Build].
func NewStream(bin string) (*Stream, error) {
	cmd := exec.Command(bin, "-stream")
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Stream{cmd: cmd, in: in, out: bufio.NewReaderSize(out, 1<<20)}, nil
}

// Run sends req to the harness and decodes its response.
func (s *Stream) Run(req *Request) (*Response, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if _, err := s.in.Write(b); err != nil {
		return nil, fmt.Errorf("clayref: writing request (harness crashed?): %w", err)
	}
	line, err := s.readLine()
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("clayref: decoding response: %w\n%s", err, line)
	}
	if resp.Fatal != "" {
		return &resp, errors.New("clayref: harness: " + resp.Fatal)
	}
	return &resp, nil
}

func (s *Stream) readLine() ([]byte, error) {
	type result struct {
		line []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := s.out.ReadBytes('\n')
		done <- result{line, err}
	}()
	var timeout <-chan time.Time
	if s.Timeout > 0 {
		timer := time.NewTimer(s.Timeout)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case r := <-done:
		if r.err != nil {
			return nil, fmt.Errorf("clayref: reading response (harness crashed?): %w", r.err)
		}
		return r.line, nil
	case <-timeout:
		s.cmd.Process.Kill()
		return nil, ErrTimeout
	}
}

// Close stops the harness.
func (s *Stream) Close() error {
	s.in.Close()
	return s.cmd.Wait()
}
