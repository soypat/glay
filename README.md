# glay
[![go.dev reference](https://pkg.go.dev/badge/github.com/soypat/glay)](https://pkg.go.dev/github.com/soypat/glay)
[![Go Report Card](https://goreportcard.com/badge/github.com/soypat/glay)](https://goreportcard.com/report/github.com/soypat/glay)
[![codecov](https://codecov.io/gh/soypat/glay/branch/main/graph/badge.svg)](https://codecov.io/gh/soypat/glay)
[![Go](https://github.com/soypat/glay/actions/workflows/go.yml/badge.svg)](https://github.com/soypat/glay/actions/workflows/go.yml)
[![sourcegraph](https://sourcegraph.com/github.com/soypat/glay/-/badge.svg)](https://sourcegraph.com/github.com/soypat/glay?badge)


Go module template with instructions on how to make your code importable and setting up codecov CI.

How to install package with newer versions of Go (+1.16):
```sh
go mod download github.com/soypat/glay@latest
```


## Basic usage example

```go
    var context Context
    err := context.Initialize(Config{
        Layout: Dimensions{Width: 100, Height: 100},
    })
    if err != nil {
        log.Fatalf(err)
    }
    err = context.BeginLayout()
    if err != nil {
        log.Fatalf(err)
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
```