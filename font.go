package gltext

import (
	"errors"
	"fmt"
	"code.google.com/p/freetype-go/freetype"
	"code.google.com/p/freetype-go/freetype/truetype"
	"github.com/go-gl/glh"
	"github.com/jimarnold/gl"
	"image"
	"io/ioutil"
	"log"
	"reflect"
)

type Font struct {
	program        gl.Program
	vs, fs         gl.Shader
	positionAttrib gl.AttribLocation
	colorUniform   gl.UniformLocation
	offsetUniform  gl.UniformLocation
	vao            gl.VertexArray
	vbo            gl.Buffer
	offsets        []float32
	color          []float32
}

type Vector4 [4]float32

func NewFont(fontPath string, scale int32, dpi float64, width, height float32) *Font {
	font := loadFont(fontPath)
	coords, texture, offsets := generateAtlas(font, scale, dpi, width, height)
	program := createProgram()

	vao := gl.GenVertexArray()
	vao.Bind()

	vbo := gl.GenBuffer()
	vbo.Bind(gl.ARRAY_BUFFER)
	gl.BufferData(gl.ARRAY_BUFFER, int(reflect.TypeOf(coords[0]).Size())*len(coords), coords, gl.STATIC_DRAW)

	positionAttrib := program.GetAttribLocation("position")
	positionAttrib.AttribPointer(4, gl.FLOAT, false, 0, nil)
	positionAttrib.EnableArray()
	vbo.Unbind(gl.ARRAY_BUFFER)

	textureUniform := program.GetUniformLocation("tex")
	offsetUniform := program.GetUniformLocation("offset")
	colorUniform := program.GetUniformLocation("color")

	gl.ActiveTexture(gl.TEXTURE0)
	tex := gl.GenTexture()
	tex.Bind(gl.TEXTURE_2D)
	textureUniform.Uniform1i(0)

	/* We require 1 byte alignment when uploading texture data */
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	/* Clamping to edges is important to prevent artifacts when scaling */
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	/* Linear filtering usually looks best for text */
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, texture.Bounds().Dx(), texture.Bounds().Dy(), 0, gl.RGBA, gl.UNSIGNED_BYTE, texture.Pix)

	vao.Unbind()

	return &Font {
		program:program,
		vao:vao,
		vbo:vbo,
		positionAttrib:positionAttrib,
		offsetUniform:offsetUniform,
		colorUniform:colorUniform,
		offsets:offsets,
		color:[]float32{1,1,1,1}}
}

func loadFont(fontPath string) *truetype.Font {
	b, err := ioutil.ReadFile(fontPath)
	if err != nil {
		log.Fatal(err)
		return nil
	}
	font, err := freetype.ParseFont(b)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	return font
}

func generateAtlas(font *truetype.Font, scale int32, dpi float64, width, height float32) ([]Vector4, *image.RGBA, []float32) {
	var low rune = 32
	var high rune = 127
	glyphCount := int32(high-low+1)
	offsets := make([]float32, glyphCount)

	bounds := font.Bounds(scale)
	gw := float32(bounds.XMax - bounds.XMin)
	gh := float32(bounds.YMax - bounds.YMin)
	imageWidth := glh.Pow2(uint32(gw * float32(glyphCount)))
	imageHeight := glh.Pow2(uint32(gh))
	imageBounds := image.Rect(0, 0, int(imageWidth), int(imageHeight))
	sx := float32(2) / width
	sy := float32(2) / height
	w := gw * sx
	h := gh * sy
	img := image.NewRGBA(imageBounds)
	c := freetype.NewContext()
	c.SetDst(img)
	c.SetClip(img.Bounds())
	c.SetSrc(image.White)
	c.SetDPI(dpi)
	c.SetFontSize(float64(scale))
	c.SetFont(font)

	var gi int32
	var gx, gy float32
	verts := make([]Vector4, 0)
	texWidth := float32(img.Bounds().Dx())
	texHeight := float32(img.Bounds().Dy())

	for ch := low; ch <= high; ch++ {
		index := font.Index(ch)
		metric := font.HMetric(scale, index)

		//the offset is used when drawing a string of glyphs - we will advance a glyph's quad by the width of all previous glyphs in the string
		offsets[gi] = float32(metric.AdvanceWidth) * sx

		//draw the glyph into the atlas at the correct location
		pt := freetype.Pt(int(gx), int(gy)+int(c.PointToFix32(float64(scale))>>8))
		c.DrawString(string(ch), pt)

		tx1 := gx / texWidth
		ty1 := gy / texHeight
		tx2 := (gx + gw) / texWidth
		ty2 := (gy + gh) / texHeight
		
		//the x,y coordinates are the same for each quad; only the texture coordinates (stored in z,w) change.
		//an optimization would be to only store texture coords, but I haven't figured that out yet
		verts = append(verts, Vector4{-1, 1, tx1, ty1},
		Vector4{-1 + (w), 1, tx2, ty1},
		Vector4{-1, 1 - (h), tx1, ty2},
		Vector4{-1 + (w), 1 - (h), tx2, ty2})

		gx += gw
		gi++
	}
	return verts, img, offsets
}

func (this *Font) Printf(x, y float32, fs string, argv ...interface{}) {
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	this.program.Use()
	this.vao.Bind()

	this.colorUniform.Uniform4fv(1, this.color)
	totalOffset := float32(0)

	s := fmt.Sprintf(fs, argv...)

	for _, ch := range s {
		index := int(ch-32)
		offset := this.offsets[index]
		this.offsetUniform.Uniform2f(x + totalOffset, y)
		gl.DrawArrays(gl.TRIANGLE_STRIP, index * 4, 4)
		totalOffset += offset
	}
	this.vao.Unbind()
	this.program.Unuse()
	gl.Disable(gl.BLEND)
}

func (this *Font) Delete() {
	this.vs.Delete()
	this.fs.Delete()
	this.program.Delete()
	this.vbo.Delete()
	this.vao.Delete()
}

func createProgram() gl.Program {
	vs,err := NewShader(gl.VERTEX_SHADER,`#version 150
    in vec4 position;
    out vec2 texpos;
    uniform vec2 offset;
    void main() {
        gl_Position = vec4(position.xy + offset, 0, 1);
		texpos = position.zw;
    }`)

	if err != nil {
		log.Printf("gltext: Error in vertex shader\n")
		log.Println(err)
	}

	fs,err := NewShader(gl.FRAGMENT_SHADER,
	`#version 150
    in vec2 texpos;
    uniform sampler2D tex;
    uniform vec4 color;
    out vec4  fragColor;
    void main(void) {
        fragColor = texture(tex, texpos) * color;
    }`)

	if err != nil {
		log.Printf("gltext: Error in fragment shader\n")
		log.Println(err)
	}

	return NewProgram(vs, fs)
}

func NewProgram(vs, fs gl.Shader) gl.Program {
	program := gl.CreateProgram()

	program.AttachShader(vs)
	program.AttachShader(fs)
	program.Link()
	link_ok := program.Get(gl.LINK_STATUS)
	if link_ok == 0 {
		log.Printf("gltext: Error linking shader program")
	}

	return program
}

func NewShader(shaderType gl.GLenum, source string) (gl.Shader,error) {
	s := gl.CreateShader(shaderType)
	s.Source(source)
	s.Compile()
	compile_ok := s.Get(gl.COMPILE_STATUS)
	if compile_ok == 0 {
		return gl.Shader(0),errors.New(s.GetInfoLog())
	}
	return s, nil
}

