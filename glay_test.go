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
	err = context.BeginLayout()
	if err != nil {
		t.Fatal(err)
	}
	// backgroundColor := Color{90, 90, 90, 255}
	err = context.Clay(ElementDeclaration{
		ID:              ID("OuterContainer"),
		BackgroundColor: Color{43, 41, 51, 255},
		Layout: LayoutConfig{
			LayoutDirection: TopToBottom,
			Sizing: Sizing{
				Width:  NewSizingAxis(SizingGrow, 0, 0),
				Height: NewSizingAxis(SizingGrow, 0, 0),
			},
			Padding:  PaddingAll(16),
			ChildGap: 16,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmds, err := context.EndLayout()
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Errorf("expected 1 command, got %d", len(cmds))
	}
}
