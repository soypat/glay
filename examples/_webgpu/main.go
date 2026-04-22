package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"slices"
	"strings"
	"unsafe"

	"github.com/cogentcore/webgpu/wgpu"
	"github.com/cogentcore/webgpu/wgpuglfw"
	"github.com/go-gl/glfw/v3.3/glfw"
	kb "github.com/soypat/lefevre"
	"github.com/soypat/lefevre/raster"
)

const (
	defaultWindowWidth  = 1024
	defaultWindowHeight = 512
	defaultText         = "Hello, world! مرحبا عالم! office résumé."
	fontPixelSize       = 48
	atlasWidth          = 2048
	atlasHeight         = 256
)

//go:embed shader.wgsl
var shaderSource string

type vertex struct {
	Position [2]float32
	UV       [2]float32
	Color    [4]float32
}

type uniforms struct {
	Viewport [2]float32
	_pad     [2]float32
}

type State struct {
	instance       *wgpu.Instance
	adapter        *wgpu.Adapter
	surface        *wgpu.Surface
	device         *wgpu.Device
	queue          *wgpu.Queue
	config         *wgpu.SurfaceConfiguration
	pipeline       *wgpu.RenderPipeline
	bindGroup      *wgpu.BindGroup
	bindLayout     *wgpu.BindGroupLayout
	pipelineLayout *wgpu.PipelineLayout
	vertexBuffer   *wgpu.Buffer
	uniformBuffer  *wgpu.Buffer
	atlasTexture   *wgpu.Texture
	atlasView      *wgpu.TextureView
	sampler        *wgpu.Sampler
	vertexCount    uint32
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

	info := font.Info()
	if info.UnitsPerEm == 0 {
		log.Fatalf("font UnitsPerEm is zero")
	}
	scale := float32(fontPixelSize) / float32(info.UnitsPerEm)

	dir, _ := kb.GuessTextProperties(*textArg)
	cfg := kb.ShapeConfig{Font: font}
	shaped := cfg.ShapeSimple(nil, *textArg, dir)

	atlas := make([]byte, atlasWidth*atlasHeight)
	placements, err := bakeAtlas(font, scale, shaped, atlas)
	if err != nil {
		log.Fatalf("failed to bake atlas: %v", err)
	}

	penX := 40
	penY := fontPixelSize + int(float32(info.Ascent)*scale/2)
	if penY < fontPixelSize {
		penY = fontPixelSize
	}
	quads := raster.BuildQuads(nil, shaped, placements, penX, penY, scale)
	verts := buildVertices(quads)
	if len(verts) == 0 {
		log.Fatalf("no glyphs produced any renderable quads")
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

	state, err := initState(window, verts, atlas)
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

// bakeAtlas extracts unique glyph IDs from shaped runs, sorts them (required by
// raster.FindPackedGlyph), and rasterizes them into atlas. Returned placements
// are sorted by GlyphID and ready for raster.BuildQuads.
func bakeAtlas(font *kb.Font, scale float32, runs []kb.Run, atlas []byte) ([]raster.PackedGlyph, error) {
	seen := make(map[uint16]struct{})
	for _, run := range runs {
		for _, g := range run.Glyphs {
			seen[g.ID] = struct{}{}
		}
	}
	ids := make([]uint16, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	placements := make([]raster.PackedGlyph, len(ids))
	cfg := raster.PackConfig{Font: font, Scale: scale, Padding: 1}
	if err := cfg.BakeAtlas(&raster.ScanlineRasterizer{}, ids, atlas, atlasWidth, atlasHeight, placements); err != nil {
		return nil, err
	}
	return placements, nil
}

// buildVertices expands each textured quad into 6 vertices (two triangles).
func buildVertices(quads []raster.DrawQuad) []vertex {
	if len(quads) == 0 {
		return nil
	}
	color := [4]float32{0.95, 0.95, 0.95, 1.0}
	const aw, ah = float32(atlasWidth), float32(atlasHeight)
	verts := make([]vertex, 0, 6*len(quads))
	for _, q := range quads {
		x0 := float32(q.DstX)
		y0 := float32(q.DstY)
		x1 := float32(q.DstX + q.DstW)
		y1 := float32(q.DstY + q.DstH)
		u0 := float32(q.SrcX) / aw
		v0 := float32(q.SrcY) / ah
		u1 := float32(q.SrcX+q.SrcW) / aw
		v1 := float32(q.SrcY+q.SrcH) / ah
		verts = append(verts,
			vertex{Position: [2]float32{x0, y0}, UV: [2]float32{u0, v0}, Color: color},
			vertex{Position: [2]float32{x1, y0}, UV: [2]float32{u1, v0}, Color: color},
			vertex{Position: [2]float32{x0, y1}, UV: [2]float32{u0, v1}, Color: color},
			vertex{Position: [2]float32{x0, y1}, UV: [2]float32{u0, v1}, Color: color},
			vertex{Position: [2]float32{x1, y0}, UV: [2]float32{u1, v0}, Color: color},
			vertex{Position: [2]float32{x1, y1}, UV: [2]float32{u1, v1}, Color: color},
		)
	}
	return verts
}

func initState(window *glfw.Window, verts []vertex, atlas []byte) (*State, error) {
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

	s.atlasTexture, err = s.device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "glyph atlas",
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
		Dimension:     wgpu.TextureDimension2D,
		Size:          wgpu.Extent3D{Width: atlasWidth, Height: atlasHeight, DepthOrArrayLayers: 1},
		Format:        wgpu.TextureFormatR8Unorm,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		return nil, err
	}
	if err := s.queue.WriteTexture(
		&wgpu.ImageCopyTexture{Texture: s.atlasTexture, MipLevel: 0, Aspect: wgpu.TextureAspectAll},
		atlas,
		&wgpu.TextureDataLayout{Offset: 0, BytesPerRow: atlasWidth, RowsPerImage: atlasHeight},
		&wgpu.Extent3D{Width: atlasWidth, Height: atlasHeight, DepthOrArrayLayers: 1},
	); err != nil {
		return nil, err
	}
	s.atlasView, err = s.atlasTexture.CreateView(nil)
	if err != nil {
		return nil, err
	}

	s.sampler, err = s.device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:         "atlas sampler",
		AddressModeU:  wgpu.AddressModeClampToEdge,
		AddressModeV:  wgpu.AddressModeClampToEdge,
		AddressModeW:  wgpu.AddressModeClampToEdge,
		MagFilter:     wgpu.FilterModeLinear,
		MinFilter:     wgpu.FilterModeLinear,
		MipmapFilter:  wgpu.MipmapFilterModeNearest,
		LodMinClamp:   0,
		LodMaxClamp:   1,
		MaxAnisotropy: 1,
	})
	if err != nil {
		return nil, err
	}

	u := uniforms{Viewport: [2]float32{float32(width), float32(height)}}
	s.uniformBuffer, err = s.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label:    "uniforms",
		Contents: uniformBytes(&u),
		Usage:    wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, err
	}

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

	s.bindLayout, err = s.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "text bind group layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageVertex | wgpu.ShaderStageFragment,
				Buffer: wgpu.BufferBindingLayout{
					Type: wgpu.BufferBindingTypeUniform,
				},
			},
			{
				Binding:    1,
				Visibility: wgpu.ShaderStageFragment,
				Texture: wgpu.TextureBindingLayout{
					SampleType:    wgpu.TextureSampleTypeFloat,
					ViewDimension: wgpu.TextureViewDimension2D,
				},
			},
			{
				Binding:    2,
				Visibility: wgpu.ShaderStageFragment,
				Sampler: wgpu.SamplerBindingLayout{
					Type: wgpu.SamplerBindingTypeFiltering,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	s.bindGroup, err = s.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "text bind group",
		Layout: s.bindLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: s.uniformBuffer, Offset: 0, Size: uint64(unsafe.Sizeof(uniforms{}))},
			{Binding: 1, TextureView: s.atlasView},
			{Binding: 2, Sampler: s.sampler},
		},
	})
	if err != nil {
		return nil, err
	}

	s.pipelineLayout, err = s.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "text pipeline layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{s.bindLayout},
	})
	if err != nil {
		return nil, err
	}

	s.pipeline, err = s.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "text pipeline",
		Layout: s.pipelineLayout,
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs_main",
			Buffers: []wgpu.VertexBufferLayout{
				{
					ArrayStride: uint64(unsafe.Sizeof(vertex{})),
					StepMode:    wgpu.VertexStepModeVertex,
					Attributes: []wgpu.VertexAttribute{
						{Format: wgpu.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0},
						{Format: wgpu.VertexFormatFloat32x2, Offset: 8, ShaderLocation: 1},
						{Format: wgpu.VertexFormatFloat32x4, Offset: 16, ShaderLocation: 2},
					},
				},
			},
		},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets: []wgpu.ColorTargetState{{
				Format:    s.config.Format,
				Blend:     &wgpu.BlendStateAlphaBlending,
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
		Label:    "glyph vertices",
		Contents: wgpu.ToBytes(verts),
		Usage:    wgpu.BufferUsageVertex,
	})
	if err != nil {
		return nil, err
	}
	s.vertexCount = uint32(len(verts))

	return s, nil
}

func uniformBytes(u *uniforms) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(u)), unsafe.Sizeof(*u))
}

func (s *State) resize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	s.config.Width = uint32(width)
	s.config.Height = uint32(height)
	s.surface.Configure(s.adapter, s.device, s.config)
	u := uniforms{Viewport: [2]float32{float32(width), float32(height)}}
	_ = s.queue.WriteBuffer(s.uniformBuffer, 0, uniformBytes(&u))
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
	renderPass.SetBindGroup(0, s.bindGroup, nil)
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
	if s.uniformBuffer != nil {
		s.uniformBuffer.Release()
		s.uniformBuffer = nil
	}
	if s.bindGroup != nil {
		s.bindGroup.Release()
		s.bindGroup = nil
	}
	if s.bindLayout != nil {
		s.bindLayout.Release()
		s.bindLayout = nil
	}
	if s.pipelineLayout != nil {
		s.pipelineLayout.Release()
		s.pipelineLayout = nil
	}
	if s.pipeline != nil {
		s.pipeline.Release()
		s.pipeline = nil
	}
	if s.sampler != nil {
		s.sampler.Release()
		s.sampler = nil
	}
	if s.atlasView != nil {
		s.atlasView.Release()
		s.atlasView = nil
	}
	if s.atlasTexture != nil {
		s.atlasTexture.Release()
		s.atlasTexture = nil
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
