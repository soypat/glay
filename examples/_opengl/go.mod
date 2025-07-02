module glay-opengl

go 1.24.4

require (
	github.com/go-gl/gl v0.0.0-20231021071112-07e5d0ea2e71
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20250301202403-da16c1255728
	github.com/go-gl/mathgl v1.2.0
	github.com/soypat/glay v0.0.0-20250425211023-fae7b411c578
)

// Use local example for using example for debugging.
// This should be removed after project works as expected.
replace github.com/soypat/glay => ../../
