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
	if err := ctx.Initialize(glay.Config{
		Layout:          glay.Dimensions{Width: canvasW, Height: canvasH},
		MaxElementCount: 128,
	}); err != nil {
		log.Fatal(err)
	}

	ctx.BeginLayout()

	grow := glay.SizingAxisGrow(0, 0)
	fixed := glay.SizingAxisFixed

	// Root container — dark background, vertical stack with padding.
	ctx.Clay(glay.ID("Root"), &glay.ElementDeclaration{
		BackgroundColor: glay.Color{R: 30, G: 30, B: 30, A: 255},
		Layout: glay.LayoutConfig{
			LayoutDirection: glay.TopToBottom,
			Sizing:          glay.Sizing{Width: grow, Height: grow},
			Padding:         glay.PaddingAll(12),
			ChildGap:        8,
		},
	}, func(ctx *glay.Context) {
		// Header row.
		ctx.Clay(glay.ID("Header"), &glay.ElementDeclaration{
			BackgroundColor: glay.Color{R: 50, G: 50, B: 70, A: 255},
			Layout: glay.LayoutConfig{
				LayoutDirection: glay.LeftToRight,
				Sizing:          glay.Sizing{Width: grow, Height: fixed(48)},
				Padding:         glay.PaddingAll(8),
				ChildGap:        8,
			},
		}, func(ctx *glay.Context) {
			// Two colored boxes in the header.
			ctx.Clay(glay.ID("HeaderLeft"), &glay.ElementDeclaration{
				BackgroundColor: glay.Color{R: 90, G: 200, B: 170, A: 255},
				Layout:          glay.LayoutConfig{Sizing: glay.Sizing{Width: fixed(80), Height: grow}},
			})
			ctx.Clay(glay.ID("HeaderRight"), &glay.ElementDeclaration{
				BackgroundColor: glay.Color{R: 200, G: 150, B: 80, A: 255},
				Layout:          glay.LayoutConfig{Sizing: glay.Sizing{Width: fixed(80), Height: grow}},
			})
		})

		// Content area — horizontal split: sidebar + main.
		ctx.Clay(glay.ID("Content"), &glay.ElementDeclaration{
			Layout: glay.LayoutConfig{
				LayoutDirection: glay.LeftToRight,
				Sizing:          glay.Sizing{Width: grow, Height: grow},
				ChildGap:        8,
			},
		}, func(ctx *glay.Context) {
			// Sidebar — vertical stack of "menu items".
			ctx.Clay(glay.ID("Sidebar"), &glay.ElementDeclaration{
				BackgroundColor: glay.Color{R: 45, G: 45, B: 55, A: 255},
				Layout: glay.LayoutConfig{
					LayoutDirection: glay.TopToBottom,
					Sizing:          glay.Sizing{Width: fixed(120), Height: grow},
					Padding:         glay.PaddingAll(8),
					ChildGap:        6,
				},
			}, func(ctx *glay.Context) {
				colors := []glay.Color{
					{R: 220, G: 80, B: 80, A: 255},
					{R: 80, G: 180, B: 220, A: 255},
					{R: 140, G: 220, B: 80, A: 255},
				}
				for i, c := range colors {
					// IDI derives IDs from a label and index without formatting strings.
					ctx.Clay(glay.IDI("MenuItem", uint32(i)), &glay.ElementDeclaration{
						BackgroundColor: c,
						Layout:          glay.LayoutConfig{Sizing: glay.Sizing{Width: grow, Height: fixed(28)}},
					})
				}
			})

			// Main panel.
			ctx.Clay(glay.ID("MainPanel"), &glay.ElementDeclaration{
				BackgroundColor: glay.Color{R: 55, G: 55, B: 65, A: 255},
				Layout:          glay.LayoutConfig{Sizing: glay.Sizing{Width: grow, Height: grow}},
			})
		})

		// Footer with border.
		ctx.Clay(glay.ID("Footer"), &glay.ElementDeclaration{
			BackgroundColor: glay.Color{R: 50, G: 50, B: 70, A: 255},
			Layout: glay.LayoutConfig{
				LayoutDirection: glay.LeftToRight,
				Sizing:          glay.Sizing{Width: grow, Height: fixed(32)},
				Padding:         glay.PaddingAll(4),
				ChildGap:        8,
			},
			Border: glay.BorderElementConfig{
				Color: glay.Color{R: 200, G: 200, B: 50, A: 255},
				Width: glay.BorderWidth{Left: 2, Right: 2, Top: 2, Bottom: 2, BetweenChildren: 2},
			},
		}, func(ctx *glay.Context) {
			ctx.Clay(glay.ID("FooterLeft"), &glay.ElementDeclaration{
				BackgroundColor: glay.Color{R: 80, G: 80, B: 120, A: 255},
				Layout:          glay.LayoutConfig{Sizing: glay.Sizing{Width: grow, Height: grow}},
			})
			ctx.Clay(glay.ID("FooterRight"), &glay.ElementDeclaration{
				BackgroundColor: glay.Color{R: 120, G: 80, B: 80, A: 255},
				Layout:          glay.LayoutConfig{Sizing: glay.Sizing{Width: grow, Height: grow}},
			})
		})
	})

	cmds, err := ctx.EndLayout()
	if err != nil {
		log.Fatal(err)
	}

	// Render to image.
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	for _, cmd := range cmds {
		bb := cmd.BoundingBox
		data := &cmd.RenderData
		switch cmd.CommandType {
		case glay.RenderCommandTypeRectangle:
			drawRect(img, bb, data.Color)
		case glay.RenderCommandTypeBorder:
			c := data.Color
			w := data.BorderWidth
			// Top
			drawRect(img, glay.BoundingBox{Vector2: glay.Vector2{X: bb.X, Y: bb.Y}, Dimensions: glay.Dimensions{Width: bb.Width, Height: float32(w.Top)}}, c)
			// Bottom
			drawRect(img, glay.BoundingBox{Vector2: glay.Vector2{X: bb.X, Y: bb.Y + bb.Height - float32(w.Bottom)}, Dimensions: glay.Dimensions{Width: bb.Width, Height: float32(w.Bottom)}}, c)
			// Left
			drawRect(img, glay.BoundingBox{Vector2: glay.Vector2{X: bb.X, Y: bb.Y}, Dimensions: glay.Dimensions{Width: float32(w.Left), Height: bb.Height}}, c)
			// Right
			drawRect(img, glay.BoundingBox{Vector2: glay.Vector2{X: bb.X + bb.Width - float32(w.Right), Y: bb.Y}, Dimensions: glay.Dimensions{Width: float32(w.Right), Height: bb.Height}}, c)
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
