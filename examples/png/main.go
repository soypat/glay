// Minimal glay example that renders a layout to a PNG file.
// Demonstrates nesting, grow/fixed sizing, padding, child gaps,
// layout directions, and background colors — no external dependencies.
//
// Run: go run main.go
// Output: output.png
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"

	"github.com/soypat/glay"
)

const (
	canvasW = 600
	canvasH = 400
)

func main() {
	var ctx glay.Context
	ctx.MaxElementCount = 128
	if err := ctx.Initialize(glay.Config{
		Layout: glay.Dimensions{Width: canvasW, Height: canvasH},
	}); err != nil {
		log.Fatal(err)
	}

	if err := ctx.BeginLayout(); err != nil {
		log.Fatal(err)
	}

	grow := func() glay.SizingAxis { return glay.NewSizingAxis(glay.SizingGrow, 0, 0) }
	fixed := func(v float32) glay.SizingAxis { return glay.NewSizingAxis(glay.SizingFixed, v) }

	// Root container — dark background, vertical stack with padding.
	ctx.Clay(glay.ElementDeclaration{
		ID:              glay.ID("Root"),
		BackgroundColor: glay.Color{R: 30, G: 30, B: 30, A: 255},
		Layout: glay.LayoutConfig{
			LayoutDirection: glay.TopToBottom,
			Sizing:          glay.Sizing{Width: grow(), Height: grow()},
			Padding:         glay.PaddingAll(12),
			ChildGap:        8,
		},
	}, func(ctx *glay.Context) error {
		// Header row.
		ctx.Clay(glay.ElementDeclaration{
			ID:              glay.ID("Header"),
			BackgroundColor: glay.Color{R: 50, G: 50, B: 70, A: 255},
			Layout: glay.LayoutConfig{
				LayoutDirection: glay.LeftToRight,
				Sizing:          glay.Sizing{Width: grow(), Height: fixed(48)},
				Padding:         glay.PaddingAll(8),
				ChildGap:        8,
			},
		}, func(ctx *glay.Context) error {
			// Two colored boxes in the header.
			ctx.Clay(glay.ElementDeclaration{
				ID:              glay.ID("HeaderLeft"),
				BackgroundColor: glay.Color{R: 90, G: 200, B: 170, A: 255},
				Layout: glay.LayoutConfig{
					Sizing: glay.Sizing{Width: fixed(80), Height: grow()},
				},
			})
			ctx.Clay(glay.ElementDeclaration{
				ID:              glay.ID("HeaderRight"),
				BackgroundColor: glay.Color{R: 200, G: 150, B: 80, A: 255},
				Layout: glay.LayoutConfig{
					Sizing: glay.Sizing{Width: fixed(80), Height: grow()},
				},
			})
			return nil
		})

		// Content area — horizontal split: sidebar + main.
		ctx.Clay(glay.ElementDeclaration{
			ID: glay.ID("Content"),
			Layout: glay.LayoutConfig{
				LayoutDirection: glay.LeftToRight,
				Sizing:          glay.Sizing{Width: grow(), Height: grow()},
				ChildGap:        8,
			},
		}, func(ctx *glay.Context) error {
			// Sidebar — vertical stack of "menu items".
			ctx.Clay(glay.ElementDeclaration{
				ID:              glay.ID("Sidebar"),
				BackgroundColor: glay.Color{R: 45, G: 45, B: 55, A: 255},
				Layout: glay.LayoutConfig{
					LayoutDirection: glay.TopToBottom,
					Sizing:          glay.Sizing{Width: fixed(120), Height: grow()},
					Padding:         glay.PaddingAll(8),
					ChildGap:        6,
				},
			}, func(ctx *glay.Context) error {
				colors := []glay.Color{
					{R: 220, G: 80, B: 80, A: 255},
					{R: 80, G: 180, B: 220, A: 255},
					{R: 140, G: 220, B: 80, A: 255},
				}
				for i, c := range colors {
					ctx.Clay(glay.ElementDeclaration{
						ID:              glay.ID(fmt.Sprintf("MenuItem%d", i)),
						BackgroundColor: c,
						Layout: glay.LayoutConfig{
							Sizing: glay.Sizing{Width: grow(), Height: fixed(28)},
						},
					})
				}
				return nil
			})

			// Main panel.
			ctx.Clay(glay.ElementDeclaration{
				ID:              glay.ID("MainPanel"),
				BackgroundColor: glay.Color{R: 55, G: 55, B: 65, A: 255},
				Layout: glay.LayoutConfig{
					Sizing: glay.Sizing{Width: grow(), Height: grow()},
				},
			})
			return nil
		})

		// Footer.
		ctx.Clay(glay.ElementDeclaration{
			ID:              glay.ID("Footer"),
			BackgroundColor: glay.Color{R: 50, G: 50, B: 70, A: 255},
			Layout: glay.LayoutConfig{
				Sizing: glay.Sizing{Width: grow(), Height: fixed(32)},
			},
		})
		return nil
	})

	cmds, err := ctx.EndLayout()
	if err != nil {
		log.Fatal(err)
	}

	// Render to image.
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	for _, cmd := range cmds {
		switch data := cmd.RenderData.(type) {
		case glay.RectangleRenderData:
			drawRect(img, cmd.BoundingBox, data.BackgroundColor)
		}
	}

	f, err := os.Create("output.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote output.png")
}

func drawRect(img *image.RGBA, bb glay.BoundingBox, c glay.Color) {
	col := color.RGBA{R: uint8(c.R), G: uint8(c.G), B: uint8(c.B), A: uint8(c.A)}
	rect := image.Rect(int(bb.X), int(bb.Y), int(bb.X+bb.Width), int(bb.Y+bb.Height))
	draw.Draw(img, rect, &image.Uniform{col}, image.Point{}, draw.Over)
}
