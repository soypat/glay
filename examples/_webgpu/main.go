package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/cogentcore/webgpu/wgpu"
	"github.com/cogentcore/webgpu/wgpuglfw"
	"github.com/go-gl/glfw/v3.3/glfw"
	kb "github.com/soypat/lefevre"
)

const (
	defaultWindowWidth  = 1024
	defaultWindowHeight = 512
	defaultText         = "Hello, world! مرحبا عالم! office résumé."
)

//go:embed shader.wgsl
var shaderSource string

type vertex struct {
	Position [2]float32
	Color    [4]float32
}

type State struct {
	instance     *wgpu.Instance
	adapter      *wgpu.Adapter
	surface      *wgpu.Surface
	device       *wgpu.Device
	queue        *wgpu.Queue
	config       *wgpu.SurfaceConfiguration
	pipeline     *wgpu.RenderPipeline
	vertexBuffer *wgpu.Buffer
	vertexCount  uint32
}

var forceFallbackAdapter = os.Getenv("WGPU_FORCE_FALLBACK_ADAPTER") == "1"

func init() {
	runtime.LockOSThread()

	switch os.Getenv("WGPU_LOG_LEVEL") {
	case "OFF":
		wgpu.SetLogLevel(wgpu.LogLevelOff)
	case "ERROR":
		wgpu.SetLogLevel(wgpu.LogLevelError)
	case "WARN":
		wgpu.SetLogLevel(wgpu.LogLevelWarn)
	case "INFO":
		wgpu.SetLogLevel(wgpu.LogLevelInfo)
	case "DEBUG":
		wgpu.SetLogLevel(wgpu.LogLevelDebug)
	case "TRACE":
		wgpu.SetLogLevel(wgpu.LogLevelTrace)
	}
}

func main() {
	fontPath := flag.String("font", "", "path to a .ttf font file")
	textArg := flag.String("text", defaultText, "text to shape and render")
	flag.Parse()

	if *fontPath == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/_webgpu -font <font.ttf> [-text \"Hello\"]")
		os.Exit(1)
	}

	fontData, err := os.ReadFile(*fontPath)
	if err != nil {
		log.Fatalf("failed to read font: %v", err)
	}

	font, err := kb.FontFromMemory(fontData, 0)
	if err != nil {
		log.Fatalf("failed to parse font: %v", err)
	}

	dir, _ := kb.GuessTextProperties(*textArg)
	cfg := kb.ShapeConfig{Font: font}
	shaped := cfg.ShapeSimple(nil, *textArg, dir)

	verts, vertexCount, err := buildGlyphVertices(shaped, font)
	if err != nil {
		log.Fatalf("failed to shape text: %v", err)
	}

	if err := glfw.Init(); err != nil {
		log.Fatalf("failed to initialize glfw: %v", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ClientAPI, glfw.NoAPI)
	window, err := glfw.CreateWindow(defaultWindowWidth, defaultWindowHeight, "Glay WebGPU + Lefevre", nil, nil)
	if err != nil {
		log.Fatalf("failed to create window: %v", err)
	}
	defer window.Destroy()

	state, err := initState(window, verts, vertexCount)
	if err != nil {
		log.Fatalf("failed to initialize WebGPU state: %v", err)
	}
	defer state.destroy()

	window.SetSizeCallback(func(w *glfw.Window, width, height int) {
		state.resize(width, height)
	})

	for !window.ShouldClose() {
		glfw.PollEvents()

		if err := state.render(); err != nil {
			log.Printf("render error: %v", err)
			if stringsContains(err.Error(), "Surface timed out") || stringsContains(err.Error(), "Surface is outdated") || stringsContains(err.Error(), "Surface was lost") {
				continue
			}
			log.Fatalf("unrecoverable render error: %v", err)
		}
	}
}

func stringsContains(s, substr string) bool {
	return strings.Index(s, substr) >= 0
}

func buildGlyphVertices(runs []kb.Run, font *kb.Font) ([]vertex, uint32, error) {
	info := font.Info()
	em := float32(info.UnitsPerEm)
	if em == 0 {
		return nil, 0, fmt.Errorf("font UnitsPerEm is zero")
	}

	totalAdvance := int32(0)
	for _, run := range runs {
		for _, g := range run.Glyphs {
			totalAdvance += g.AdvanceX
		}
	}
	if totalAdvance == 0 {
		return nil, 0, fmt.Errorf("shaped text produced no glyph advances")
	}

	fontHeight := float32(info.Ascent - info.Descent)
	if fontHeight == 0 {
		return nil, 0, fmt.Errorf("invalid font ascent/descent metrics")
	}

	xMargin := float32(0.1)
	yMargin := float32(0.15)
	scaleX := (2.0 - 2.0*xMargin) / float32(totalAdvance)
	scaleY := (2.0 - 2.0*yMargin) / fontHeight
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}

	baseline := float32(-0.4)
	y0 := baseline + float32(info.Descent)*scale
	y1 := baseline + float32(info.Ascent)*scale
	x := float32(-1.0) + xMargin

	var vertices []vertex
	runIndex := 0
	for _, run := range runs {
		for _, g := range run.Glyphs {
			glyphX := x + float32(g.OffsetX)*scale
			glyphAdvance := float32(g.AdvanceX) * scale
			if glyphAdvance < 0.005 {
				glyphAdvance = 0.005
			}

			color := [4]float32{0.22, 0.64, 0.92, 1.0}
			if g.Flags.Has(kb.GlyphFlagLigature) {
				color = [4]float32{0.98, 0.64, 0.18, 1.0}
			}
			if runIndex%2 == 1 {
				color = [4]float32{color[0] * 0.85, color[1] * 0.9, color[2] * 0.85, 1.0}
			}

			vertices = append(vertices, quad(glyphX, y0, glyphX+glyphAdvance, y1, color)...) // glyph cell
			if runIndex == 0 {
				vertices = append(vertices, quad(glyphX, y0, glyphX+0.002, y0+0.02, [4]float32{1.0, 1.0, 1.0, 1.0})...)
			}
			x += glyphAdvance
		}
		runIndex++
	}

	return vertices, uint32(len(vertices)), nil
}

func quad(x0, y0, x1, y1 float32, color [4]float32) []vertex {
	return []vertex{
		{Position: [2]float32{x0, y0}, Color: color},
		{Position: [2]float32{x1, y0}, Color: color},
		{Position: [2]float32{x0, y1}, Color: color},
		{Position: [2]float32{x0, y1}, Color: color},
		{Position: [2]float32{x1, y0}, Color: color},
		{Position: [2]float32{x1, y1}, Color: color},
	}
}

func initState(window *glfw.Window, verts []vertex, vertexCount uint32) (*State, error) {
	s := &State{}
	s.instance = wgpu.CreateInstance(nil)

	s.surface = s.instance.CreateSurface(wgpuglfw.GetSurfaceDescriptor(window))

	var err error
	s.adapter, err = s.instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		ForceFallbackAdapter: forceFallbackAdapter,
		CompatibleSurface:    s.surface,
	})
	if err != nil {
		return nil, err
	}

	s.device, err = s.adapter.RequestDevice(nil)
	if err != nil {
		return nil, err
	}
	s.queue = s.device.GetQueue()

	caps := s.surface.GetCapabilities(s.adapter)
	width, height := window.GetSize()
	s.config = &wgpu.SurfaceConfiguration{
		Usage:       wgpu.TextureUsageRenderAttachment,
		Format:      caps.Formats[0],
		Width:       uint32(width),
		Height:      uint32(height),
		PresentMode: wgpu.PresentModeFifo,
		AlphaMode:   caps.AlphaModes[0],
	}
	s.surface.Configure(s.adapter, s.device, s.config)

	shader, err := s.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "text shader",
		WGSLDescriptor: &wgpu.ShaderModuleWGSLDescriptor{
			Code: shaderSource,
		},
	})
	if err != nil {
		return nil, err
	}
	defer shader.Release()

	s.pipeline, err = s.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "text pipeline",
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs_main",
			Buffers: []wgpu.VertexBufferLayout{
				{
					ArrayStride: 6 * 4,
					StepMode:    wgpu.VertexStepModeVertex,
					Attributes: []wgpu.VertexAttribute{
						{Format: wgpu.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0},
						{Format: wgpu.VertexFormatFloat32x4, Offset: 8, ShaderLocation: 1},
					},
				},
			},
		},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets: []wgpu.ColorTargetState{{
				Format:    s.config.Format,
				Blend:     &wgpu.BlendStateReplace,
				WriteMask: wgpu.ColorWriteMaskAll,
			}},
		},
		Primitive: wgpu.PrimitiveState{
			Topology:         wgpu.PrimitiveTopologyTriangleList,
			FrontFace:        wgpu.FrontFaceCCW,
			CullMode:         wgpu.CullModeNone,
			StripIndexFormat: wgpu.IndexFormatUndefined,
		},
		Multisample: wgpu.MultisampleState{Count: 1, Mask: 0xFFFFFFFF, AlphaToCoverageEnabled: false},
	})
	if err != nil {
		return nil, err
	}

	s.vertexBuffer, err = s.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label:    "glyph quad vertices",
		Contents: wgpu.ToBytes(verts),
		Usage:    wgpu.BufferUsageVertex,
	})
	if err != nil {
		return nil, err
	}
	s.vertexCount = vertexCount

	return s, nil
}

func (s *State) resize(width, height int) {
	if width > 0 && height > 0 {
		s.config.Width = uint32(width)
		s.config.Height = uint32(height)
		s.surface.Configure(s.adapter, s.device, s.config)
	}
}

func (s *State) render() error {
	nextTexture, err := s.surface.GetCurrentTexture()
	if err != nil {
		return err
	}
	view, err := nextTexture.CreateView(nil)
	if err != nil {
		return err
	}
	defer view.Release()

	encoder, err := s.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "command encoder"})
	if err != nil {
		return err
	}
	defer encoder.Release()

	renderPass := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     wgpu.LoadOpClear,
			StoreOp:    wgpu.StoreOpStore,
			ClearValue: wgpu.Color{R: 0.08, G: 0.1, B: 0.14, A: 1.0},
		}},
	})
	renderPass.SetPipeline(s.pipeline)
	renderPass.SetVertexBuffer(0, s.vertexBuffer, 0, wgpu.WholeSize)
	renderPass.Draw(s.vertexCount, 1, 0, 0)
	renderPass.End()
	renderPass.Release()

	cmdBuffer, err := encoder.Finish(nil)
	if err != nil {
		return err
	}
	defer cmdBuffer.Release()

	s.queue.Submit(cmdBuffer)
	s.surface.Present()
	return nil
}

func (s *State) destroy() {
	if s.vertexBuffer != nil {
		s.vertexBuffer.Release()
		s.vertexBuffer = nil
	}
	if s.pipeline != nil {
		s.pipeline.Release()
		s.pipeline = nil
	}
	if s.config != nil {
		s.config = nil
	}
	if s.queue != nil {
		s.queue.Release()
		s.queue = nil
	}
	if s.device != nil {
		s.device.Release()
		s.device = nil
	}
	if s.surface != nil {
		s.surface.Release()
		s.surface = nil
	}
	if s.adapter != nil {
		s.adapter.Release()
		s.adapter = nil
	}
	if s.instance != nil {
		s.instance.Release()
		s.instance = nil
	}
}
