package clayref

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestHarnessRobust checks the reference harness survives generated requests.
func TestHarnessRobust(t *testing.T) {
	bin := Build(t)
	s, err := NewStream(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	s.Timeout = time.Second
	rng := rand.New(rand.NewSource(1))
	buf := make([]byte, 512)
	var cmds, errs, timeouts int
	for i := range 2000 {
		rng.Read(buf[:rng.Intn(len(buf))])
		req := Gen(buf)
		resp, err := s.Run(req)
		if errors.Is(err, ErrTimeout) {
			// Upstream clay livelocks on some layouts with negative content boxes.
			timeouts++
			if dir := os.Getenv("CLAYREF_DUMP"); dir != "" {
				b, _ := json.Marshal(req)
				os.WriteFile(filepath.Join(dir, fmt.Sprintf("timeout-%d.json", i)), b, 0o644)
			}
			s, err = NewStream(bin)
			if err != nil {
				t.Fatal(err)
			}
			s.Timeout = time.Second
			continue
		}
		if err != nil {
			b, _ := json.Marshal(req)
			t.Fatalf("request %d: %v\n%s", i, err, b)
		}
		if len(resp.Frames) != len(req.Frames) {
			t.Fatalf("request %d: want %d frames, got %d", i, len(req.Frames), len(resp.Frames))
		}
		for _, f := range resp.Frames {
			cmds += len(f.Commands)
			errs += len(f.Errors)
		}
	}
	t.Logf("%d commands, %d errors, %d upstream timeouts", cmds, errs, timeouts)
}

func TestGenDeterministic(t *testing.T) {
	data := []byte("some fuzz input bytes that drive the generator")
	a, _ := json.Marshal(Gen(data))
	b, _ := json.Marshal(Gen(data))
	if string(a) != string(b) {
		t.Fatal("Gen is not deterministic")
	}
}
