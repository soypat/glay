# glay
[![go.dev reference](https://pkg.go.dev/badge/github.com/soypat/glay)](https://pkg.go.dev/github.com/soypat/glay)
[![Go Report Card](https://goreportcard.com/badge/github.com/soypat/glay)](https://goreportcard.com/report/github.com/soypat/glay)
[![codecov](https://codecov.io/gh/soypat/glay/branch/main/graph/badge.svg)](https://codecov.io/gh/soypat/glay)
[![Go](https://github.com/soypat/glay/actions/workflows/go.yml/badge.svg)](https://github.com/soypat/glay/actions/workflows/go.yml)
[![sourcegraph](https://sourcegraph.com/github.com/soypat/glay/-/badge.svg)](https://sourcegraph.com/github.com/soypat/glay?badge)


A Go line-by-line port of the fascinating [Clay UI](https://github.com/nicbarker/clay) layout library.

- **Static allocation**: all memory is allocated once by `Initialize`. Laying out a frame does not allocate.
- **Faithful**: glay is differentially tested and fuzzed against the upstream C implementation
  (clay 0.14, commit `e6cc369`), see [internal/clayref](./internal/clayref).

## Basic usage example

```go
var ctx glay.Context
err := ctx.Initialize(glay.Config{
    Layout:          glay.Dimensions{Width: 640, Height: 480},
    MaxElementCount: 1024,
    MeasureText:     measureText, // Required for text elements.
})
if err != nil {
    log.Fatal(err)
}

ctx.BeginLayout()
ctx.Clay(glay.ID("OuterContainer"), &glay.ElementDeclaration{
    BackgroundColor: glay.Color{R: 43, G: 41, B: 51, A: 255},
    Layout: glay.LayoutConfig{
        LayoutDirection: glay.TopToBottom,
        Sizing:          glay.Sizing{Width: glay.SizingAxisGrow(0, 0), Height: glay.SizingAxisGrow(0, 0)},
        Padding:         glay.PaddingAll(16),
        ChildGap:        16,
    },
}, func(ctx *glay.Context) {
    for i := range uint32(3) {
        ctx.Clay(glay.IDI("Item", i), &glay.ElementDeclaration{
            BackgroundColor: glay.Color{R: 90, G: 200, B: 170, A: 255},
            Layout:          glay.LayoutConfig{Sizing: glay.Sizing{Width: glay.SizingAxisGrow(0, 0), Height: glay.SizingAxisFixed(32)}},
        })
    }
    ctx.Text("Hello glay", glay.TextElementConfig{FontSize: 16, TextColor: glay.Color{R: 255, G: 255, B: 255, A: 255}})
})
cmds, err := ctx.EndLayout()
if err != nil {
    log.Println(err) // Commands remain valid unless err is glay.ErrElementsCapacityExceeded.
}
for _, cmd := range cmds {
    // Draw cmd according to cmd.CommandType.
}
```

`Clay` takes closures for convenience, which may allocate. For allocation free layouts
use the raw API that the `CLAY()` macro expands to in C:

```go
ctx.OpenElementWithID(glay.ID("Panel")) // Or ctx.OpenElement() for an anonymous element.
ctx.ConfigureOpenElement(&decl)
ctx.OpenTextElement("label", textConfig)
ctx.CloseElement()
```

## Differences with clay

- Errors are reported through `Config.ErrorHandler` like in clay, and `EndLayout` also returns the first error of the frame.
- `Initialize` validates its configuration and returns `ErrInvalidConfig` instead of crashing
  (clay requires `32 <= MaxMeasureTextCacheWordCount <= 32*MaxElementCount`).
- Clay loops forever in its compression and grow passes when an iteration makes no progress
  (i.e. with negative sizes). glay stops iterating, which produces the same result whenever clay terminates.
- Clay reads past the end of its arrays in some capacity overflow cases. glay never reads
  out of bounds, instead reporting `ErrInternal` like clay does for its other bounds checks.
- The debug view, transitions and overlay colors are not implemented.
