package clayref

import (
	"math/rand"
	"testing"
	"time"
)

func TestCompareDetects(t *testing.T) {
	bin := Build(t)
	s, err := NewStream(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Timeout = time.Second
	rng := rand.New(rand.NewSource(2))
	buf := make([]byte, 512)
	var cmds, texts, frames, errs, queries, found int
	for range 500 {
		rng.Read(buf[:rng.Intn(len(buf))])
		req := Gen(buf)
		want, err := s.Run(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := RunGlay(req)
		if err != nil {
			t.Fatal(err)
		}
		for fi := range want.Frames {
			frames++
			errs += len(want.Frames[fi].Errors)
			cmds += len(want.Frames[fi].Commands)
			for _, c := range want.Frames[fi].Commands {
				if c.Type == "text" {
					texts++
				}
			}
			for _, q := range want.Frames[fi].Query {
				queries++
				if q.Found {
					found++
				}
			}
			if len(got.Frames[fi].Commands) > 0 {
				got.Frames[fi].Commands[0].BB[0] += 1
				if Compare(want, got, 1) == "" {
					t.Fatal("perturbed response compared equal")
				}
				got.Frames[fi].Commands[0].BB[0] -= 1
			}
		}
	}
	t.Logf("frames=%d commands=%d text=%d errors=%d queries=%d found=%d", frames, cmds, texts, errs, queries, found)
}
