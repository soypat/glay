package clayref

import (
	"encoding/json"
	"errors"
	"flag"
	"math/rand"
	"testing"
	"time"
)

var flagDiffN = flag.Int("diffn", 500, "number of random requests run by TestDiffRandom")

// diff runs req through both implementations, reporting any differences.
// It returns false if the reference harness timed out (upstream livelock), in which case s is restarted.
func diff(t testing.TB, bin string, s **Stream, req *Request) bool {
	t.Helper()
	want, err := (*s).Run(req)
	if errors.Is(err, ErrTimeout) {
		(*s).Close()
		*s, err = NewStream(bin)
		if err != nil {
			t.Fatal(err)
		}
		(*s).Timeout = time.Second
		return false
	} else if err != nil {
		b, _ := json.Marshal(req)
		t.Fatalf("reference: %v\n%s", err, b)
	}
	got, err := RunGlay(req)
	if err != nil {
		t.Fatal(err)
	}
	if d := Compare(want, got, 8); d != "" {
		b, _ := json.Marshal(req)
		t.Fatalf("glay differs from clay:\n%s\nrequest:\n%s", d, b)
	}
	return true
}

func TestDiffRandom(t *testing.T) {
	bin := Build(t)
	s, err := NewStream(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	s.Timeout = time.Second
	rng := rand.New(rand.NewSource(2))
	buf := make([]byte, 512)
	for range *flagDiffN {
		rng.Read(buf[:rng.Intn(len(buf))])
		diff(t, bin, &s, Gen(buf))
	}
}

func FuzzClayRef(f *testing.F) {
	bin := Build(f)
	rng := rand.New(rand.NewSource(3))
	for range 16 {
		buf := make([]byte, rng.Intn(512))
		rng.Read(buf)
		f.Add(buf)
	}
	s, err := NewStream(bin)
	if err != nil {
		f.Fatal(err)
	}
	s.Timeout = time.Second
	f.Fuzz(func(t *testing.T, data []byte) {
		if !diff(t, bin, &s, Gen(data)) {
			t.Skip("reference harness timed out (upstream clay livelock)")
		}
	})
}
