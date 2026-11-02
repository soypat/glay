package glay

import "testing"

func TestAPI(t *testing.T) {
	var context Context
	err := context.Initialize(Config{
		Layout: Dimensions{Width: 100, Height: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	context.BeginLayout()
	context.Clay(ID("OuterContainer"), &ElementDeclaration{
		BackgroundColor: Color{43, 41, 51, 255},
		Layout: LayoutConfig{
			LayoutDirection: TopToBottom,
			Sizing: Sizing{
				Width:  SizingAxisGrow(0, 0),
				Height: SizingAxisGrow(0, 0),
			},
			Padding:  PaddingAll(16),
			ChildGap: 16,
		},
	}, func(context *Context) {
		context.Clay(ID("child"), &ElementDeclaration{})
	})
	cmds, err := context.EndLayout()
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Errorf("expected 1 command, got %d", len(cmds))
	}
	bb, ok := context.GetElementData(ID("child"))
	if !ok {
		t.Fatal("child element not found")
	}
	if bb.X != 16 || bb.Y != 16 {
		t.Errorf("unexpected child position %+v", bb)
	}
}

func TestInitializeInvalidConfig(t *testing.T) {
	var context Context
	for _, cfg := range []Config{
		{MaxElementCount: 1},
		{MaxElementCount: 16, MaxMeasureTextCacheWordCount: 16},
		{MaxElementCount: 16, MaxMeasureTextCacheWordCount: 32*16 + 32},
	} {
		if err := context.Initialize(cfg); err != ErrInvalidConfig {
			t.Errorf("config %+v: want ErrInvalidConfig, got %v", cfg, err)
		}
	}
}

func TestOverflowRegrow(t *testing.T) {
	var c Context
	cfg := Config{Layout: Dimensions{Width: 100, Height: 100}, MaxElementCount: 8}
	if err := c.Initialize(cfg); err != nil {
		t.Fatal(err)
	}
	layout := func() error {
		c.BeginLayout()
		for i := range uint32(20) {
			c.OpenElementWithID(IDI("Box", i))
			c.ConfigureOpenElement(&ElementDeclaration{BackgroundColor: Color{A: 255}, Layout: LayoutConfig{Sizing: Sizing{Width: SizingAxisFixed(1), Height: SizingAxisFixed(1)}}})
			c.CloseElement()
		}
		_, err := c.EndLayout()
		return err
	}
	if err := layout(); err != ErrElementsCapacityExceeded {
		t.Fatalf("want ErrElementsCapacityExceeded, got %v", err)
	}
	need := c.NeededElementCount()
	if need != 21 {
		t.Fatalf("want 21 needed elements (20 boxes and root), got %d", need)
	}
	// Clay reserves one element slot, and the hash map must also hold the elements.
	cfg.MaxElementCount = need + 1
	if err := c.Initialize(cfg); err != nil {
		t.Fatal(err)
	}
	if err := layout(); err != nil {
		t.Fatalf("after growing: %v", err)
	}
	if cfg.MemoryRequirements() <= 0 {
		t.Fatal("expected positive memory requirements")
	}
}
