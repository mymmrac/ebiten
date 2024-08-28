// Copyright 2022 The Ebitengine Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package builtinshader

import (
	"strings"
	"sync"
)

type Filter int

const (
	FilterNearest Filter = iota
	FilterLinear
)

const FilterCount = 2

type Address int

const (
	AddressUnsafe Address = iota
	AddressClampToZero
	AddressRepeat
)

const AddressCount = 3

const (
	UniformColorMBody        = "ColorMBody"
	UniformColorMTranslation = "ColorMTranslation"
)

var (
	shaders  [FilterCount][AddressCount][2][]byte
	shadersM sync.Mutex
)

type shaderOptions struct {
	Filter    Filter
	Address   Address
	UseColorM bool
}

func generateShader(options shaderOptions) []byte {
	sb := &strings.Builder{}
	sb.WriteString(`
//kage:unit pixels

package main
`)
	if options.UseColorM {
		sb.WriteString(`
var ColorMBody mat4
var ColorMTranslation vec4
`)
	}
	if options.Address == AddressRepeat {
		sb.WriteString(`
func adjustTexelForAddressRepeat(p vec2) vec2 {
	origin := imageSrc0Origin()
	size := imageSrc0Size()
	return mod(p - origin, size) + origin
}
`)
	}
	sb.WriteString(`
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
`)
	switch options.Filter {
	case FilterNearest:
		switch options.Address {
		case AddressUnsafe:
			sb.WriteString(`
	clr := imageSrc0UnsafeAt(srcPos)
`)
		case AddressClampToZero:
			sb.WriteString(`
	clr := imageSrc0At(srcPos)
`)
		case AddressRepeat:
			sb.WriteString(`
	clr := imageSrc0At(adjustTexelForAddressRepeat(srcPos))
`)
		}
	case FilterLinear:
		sb.WriteString(`
	p0 := srcPos - 1/2.0
	p1 := srcPos + 1/2.0
`)
		switch options.Address {
		case AddressUnsafe:
			sb.WriteString(`
	c0 := imageSrc0UnsafeAt(p0)
	c1 := imageSrc0UnsafeAt(vec2(p1.x, p0.y))
	c2 := imageSrc0UnsafeAt(vec2(p0.x, p1.y))
	c3 := imageSrc0UnsafeAt(p1)
`)
		case AddressClampToZero:
			sb.WriteString(`
	c0 := imageSrc0At(p0)
	c1 := imageSrc0At(vec2(p1.x, p0.y))
	c2 := imageSrc0At(vec2(p0.x, p1.y))
	c3 := imageSrc0At(p1)
`)
		case AddressRepeat:
			sb.WriteString(`
	p0 = adjustTexelForAddressRepeat(p0)
	p1 = adjustTexelForAddressRepeat(p1)

	c0 := imageSrc0At(p0)
	c1 := imageSrc0At(vec2(p1.x, p0.y))
	c2 := imageSrc0At(vec2(p0.x, p1.y))
	c3 := imageSrc0At(p1)
`)
		}
		sb.WriteString(`
	rate := fract(p1)
	clr := mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y)
`)
	}
	if options.UseColorM {
		sb.WriteString(`
	// Un-premultiply alpha.
	// When the alpha is 0, 1-sign(alpha) is 1.0, which means division does nothing.
	clr.rgb /= clr.a + (1-sign(clr.a))
	// Apply the clr matrix.
	clr = (ColorMBody * clr) + ColorMTranslation
	// Premultiply alpha
	clr.rgb *= clr.a
	// Apply the color scale.
	clr *= color
	// Clamp the output.
	clr.rgb = min(clr.rgb, clr.a)
`)
	} else {
		sb.WriteString(`
	// Apply the color scale.
	clr *= color
`)
	}
	sb.WriteString(`
	return clr
}
`)
	return []byte(sb.String())
}

// ShaderSource returns the built-in shader source based on the given parameters.
//
// The returned shader always uses a color matrix so far.
func ShaderSource(filter Filter, address Address, useColorM bool) []byte {
	shadersM.Lock()
	defer shadersM.Unlock()

	var colorM int
	if useColorM {
		colorM = 1
	}
	if s := shaders[filter][address][colorM]; s != nil {
		return s
	}

	shader := generateShader(shaderOptions{
		Filter:    filter,
		Address:   address,
		UseColorM: useColorM,
	})

	shaders[filter][address][colorM] = shader
	return shader
}

var ScreenShaderSource = []byte(`//kage:unit pixels

package main

func Fragment(dstPos vec4, srcPos vec2) vec4 {
	// Blend source colors in a square region, which size is 1/scale.
	scale := imageDstSize()/imageSrc0Size()
	pos := srcPos
	p0 := pos - 1/2.0/scale
	p1 := pos + 1/2.0/scale

	// Texels must be in the source rect, so it is not necessary to check.
	c0 := imageSrc0UnsafeAt(p0)
	c1 := imageSrc0UnsafeAt(vec2(p1.x, p0.y))
	c2 := imageSrc0UnsafeAt(vec2(p0.x, p1.y))
	c3 := imageSrc0UnsafeAt(p1)

	// p is the p1 value in one pixel assuming that the pixel's upper-left is (0, 0) and the lower-right is (1, 1).
	rate := clamp(fract(p1)*scale, 0, 1)
	return mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y)
}
`)

var ClearShaderSource = []byte(`//kage:unit pixels

package main

func Fragment() vec4 {
	return vec4(0)
}
`)

func AppendShaderSources(sources [][]byte) [][]byte {
	for filter := Filter(0); filter < FilterCount; filter++ {
		for address := Address(0); address < AddressCount; address++ {
			sources = append(sources, ShaderSource(filter, address, false), ShaderSource(filter, address, true))
		}
	}
	sources = append(sources, ScreenShaderSource, ClearShaderSource)
	return sources
}
