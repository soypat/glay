package main

import (
	"fmt"
	"log"
	"log/slog"
	"runtime"
	"sort"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/soypat/glay"
)

const (
	width  = 800
	height = 600
)

func init() {
	// GLFW event handling must run on the main OS thread
	runtime.LockOSThread()
}

func main() {
	if err := glfw.Init(); err != nil {
		log.Fatalf("failed to initialize glfw: %v", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.Resizable, glfw.True)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCompatProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(width, height, "Glay UI Example", nil, nil)
	if err != nil {
		log.Fatalf("failed to create window: %v", err)
	}
	window.MakeContextCurrent()

	// Initialize OpenGL
	if err := gl.Init(); err != nil {
		log.Fatalf("failed to initialize OpenGL: %v", err)
	}
	// Initialize the UI renderer
	renderer, err := NewUIRenderer(width, height)
	if err != nil {
		log.Fatalf("failed to initialize UI renderer: %v", err)
	}
	defer renderer.Cleanup()

	fmt.Println("OpenGL version", gl.GoStr(gl.GetString(gl.VERSION)))

	// Set up Glay context
	var context glay.Context
	context.MaxElementCount = 100
	err = context.Initialize(glay.Config{
		Layout: glay.Dimensions{Width: width, Height: height},
	})
	if err != nil {
		log.Fatalf("failed to initialize Glay context: %v", err)
	}

	// Main rendering loop
	for !window.ShouldClose() {
		// Poll for and process events
		glfw.PollEvents()

		// Clear the screen
		gl.ClearColor(0.2, 0.2, 0.2, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		// Add this code to your main.go, right before context.BeginLayout()

		// Begin Glay layout
		err = context.BeginLayout()
		if err != nil {
			slog.Warn("BeginLayout error: %v", err)
		}

		// Main container
		shared := &glay.SharedElementConfig{
			BackgroundColor: glay.Color{50, 50, 50, 0},
		}
		context.Clay(glay.ElementDeclaration{
			ID:              glay.ID("MainContainer"),
			BackgroundColor: glay.Color{50, 50, 50, 128},
			Layout: glay.LayoutConfig{
				LayoutDirection: glay.TopToBottom,
				Sizing: glay.Sizing{
					Width:  glay.NewSizingAxis(glay.SizingGrow, 0, 0),
					Height: glay.NewSizingAxis(glay.SizingGrow, 0, 0),
				},
				Padding:  glay.PaddingAll(16),
				ChildGap: 16,
			},
			UserData: shared,
			Floating: glay.FloatingElementConfig{
				Zindex: 0, // Lower Z-index for parent
			},
		}, func(context *glay.Context) error {
			err = declButton(context, "Button1", glay.Color{255, 120, 200, 255}, 200, 50)
			if err != nil {
				return err
			}
			err = declButton(context, "Button2", glay.Color{100, 120, 200, 255}, 200, 50)
			if err != nil {
				return err
			}
			return nil
		})

		// Get render commands from Glay
		renderCommands, err := context.EndLayout()
		if err != nil {
			log.Fatalf("EndLayout error: %v", err)
		}

		// Process render commands using the modern renderer
		//slog.Info("render commands", renderCommands)
		renderer.ProcessRenderCommands(renderCommands)

		// Swap buffers
		window.SwapBuffers()

	}
}

func declButton(context *glay.Context, IDName string, color glay.Color, width, height float32) error {
	return context.Clay(glay.ElementDeclaration{
		ID:              glay.ID(IDName),
		BackgroundColor: color,
		Layout: glay.LayoutConfig{
			Sizing: glay.Sizing{
				Width:  glay.NewSizingAxis(glay.SizingFixed, width),
				Height: glay.NewSizingAxis(glay.SizingFixed, height),
			},
		},
		Floating: glay.FloatingElementConfig{
			Zindex: 1, // Higher Z-index for child
		},
	})
}

// Shader sources for rendering UI elements
const (
	vertexShaderSource = `
		#version 410
		layout (location = 0) in vec2 position;
		layout (location = 1) in vec4 color;
		out vec4 vertexColor;
		uniform mat4 projection;
		void main() {
			gl_Position = projection * vec4(position, 0.0, 1.0);
			vertexColor = color;
		}
	`

	fragmentShaderSource = `
		#version 410
		in vec4 vertexColor;
		out vec4 fragColor;
		void main() {
			fragColor = vertexColor;
		}
	`
)

// UIRenderer manages OpenGL resources for UI rendering
type UIRenderer struct {
	program           uint32
	vao, vbo, ebo     uint32
	projectionUniform int32
	screenWidth       float32
	screenHeight      float32
}

// NewUIRenderer initializes and returns a new UI renderer
func NewUIRenderer(screenWidth, screenHeight int) (*UIRenderer, error) {
	renderer := &UIRenderer{
		screenWidth:  float32(screenWidth),
		screenHeight: float32(screenHeight),
	}

	// Compile shaders
	vertexShader, err := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		return nil, fmt.Errorf("failed to compile vertex shader: %v", err)
	}

	fragmentShader, err := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		return nil, fmt.Errorf("failed to compile fragment shader: %v", err)
	}

	// Link program
	program := gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(log))
		return nil, fmt.Errorf("failed to link program: %v", log)
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)

	renderer.program = program

	// Get uniform locations
	renderer.projectionUniform = gl.GetUniformLocation(program, gl.Str("projection\x00"))

	// Create VAO, VBO, and EBO
	var vao, vbo, ebo uint32
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &vbo)
	gl.GenBuffers(1, &ebo)

	renderer.vao = vao
	renderer.vbo = vbo
	renderer.ebo = ebo

	// Set up VAO and vertex attributes
	gl.BindVertexArray(vao)

	// Set up element indices for drawing quads as triangles
	indices := []uint32{0, 1, 2, 2, 3, 0} // Quad as two triangles
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, 6*4, gl.Ptr(indices), gl.STATIC_DRAW)

	// Configure vertex attributes
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)

	// Position attribute
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 6*4, gl.PtrOffset(0))

	// Color attribute
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, 4, gl.FLOAT, false, 6*4, gl.PtrOffset(2*4))

	// Unbind VAO
	gl.BindVertexArray(0)

	return renderer, nil
}

// ProcessRenderCommands draws UI elements using modern OpenGL techniques
func (ren *UIRenderer) ProcessRenderCommands(commands []glay.RenderCommand) {
	if len(commands) == 0 {
		return
	}
	// Sort commands by z-index to ensure correct drawing order
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Zindex < commands[j].Zindex
	})

	// Ensure viewport is correctly set
	gl.Viewport(0, 0, int32(ren.screenWidth), int32(ren.screenHeight))

	// Use our shader program
	gl.UseProgram(ren.program)

	// Set up orthographic projection matrix
	projection := mgl32.Ortho(0, ren.screenWidth, ren.screenHeight, 0, -1, 1)
	gl.UniformMatrix4fv(ren.projectionUniform, 1, false, &projection[0])

	// Bind VAO
	gl.BindVertexArray(ren.vao)

	// Enable blending for transparency
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// Process each UI element
	for i, cmd := range commands {
		// Extract coordinates from bounding box
		x := cmd.BoundingBox.X
		y := cmd.BoundingBox.Y
		w := cmd.BoundingBox.Width
		h := cmd.BoundingBox.Height

		// Extract color from render data
		var r, g, b, a float32 = 1.0, 1.0, 1.0, 1.0 // Default white

		switch data := cmd.RenderData.(type) {
		case glay.RectangleRenderData:
			r = float32(data.BackgroundColor.R) / 255.0
			g = float32(data.BackgroundColor.G) / 255.0
			b = float32(data.BackgroundColor.B) / 255.0
			a = float32(data.BackgroundColor.A) / 255.0
		case glay.Color:
			r = float32(data.R) / 255.0
			g = float32(data.G) / 255.0
			b = float32(data.B) / 255.0
			a = float32(data.A) / 255.0
		default:
			log.Printf("Unknown render data type: %T for command %d", cmd.RenderData, i)
		}

		// Create vertex data for quad with positions and colors
		vertices := []float32{
			// Positions      // Colors
			x, y, r, g, b, a, // Top left
			x + w, y, r, g, b, a, // Top right
			x + w, y + h, r, g, b, a, // Bottom right
			x, y + h, r, g, b, a, // Bottom left
		}

		// Update buffer data
		gl.BindBuffer(gl.ARRAY_BUFFER, ren.vbo)
		gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.DYNAMIC_DRAW)

		// Draw the quad
		gl.DrawElements(gl.TRIANGLES, 6, gl.UNSIGNED_INT, gl.PtrOffset(0))
	}

	// Clean up
	gl.Disable(gl.BLEND)
	gl.BindVertexArray(0)
	gl.UseProgram(0)
}

// Cleanup releases OpenGL resources
func (r *UIRenderer) Cleanup() {
	gl.DeleteProgram(r.program)
	gl.DeleteVertexArrays(1, &r.vao)
	gl.DeleteBuffers(1, &r.vbo)
	gl.DeleteBuffers(1, &r.ebo)
}

// Helper function to compile a shader
func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)

	csources, free := gl.Strs(source + "\x00")
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))
		return 0, fmt.Errorf("failed to compile shader: %v", log)
	}

	return shader, nil
}
