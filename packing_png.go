package grib2

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

// UnpackPNG decodes data packed with Template 5.41 (PNG packing).
//
// The packed data in Section 7 is a complete PNG image where each pixel
// value represents a scaled integer. The same formula as simple packing
// applies: Y = (X * 2^E + R) * 10^(-D)
func UnpackPNG(data []byte, tmpl Template50, numValues uint32) ([]float64, error) {
	if numValues == 0 {
		return nil, nil
	}

	// Guard against huge allocations from malformed input.
	const maxElements = 100_000_000
	if numValues > maxElements {
		return nil, fmt.Errorf("grib2: PNG packing: numValues %d exceeds maximum %d", numValues, maxElements)
	}

	bpv := int(tmpl.BitsPerValue)

	// Constant field
	if bpv == 0 || len(data) == 0 {
		ref := float64(tmpl.ReferenceValue)
		d := math.Pow(10, float64(-tmpl.DecimalScaleFactor))
		val := ref * d
		values := make([]float64, numValues)
		for i := range values {
			values[i] = val
		}
		return values, nil
	}

	// Decode the PNG image
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("grib2: PNG decode: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	binaryS := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	decimalS := math.Pow(10, float64(-tmpl.DecimalScaleFactor))
	ref := float64(tmpl.ReferenceValue)

	// Round bitsPerValue up to nearest byte boundary for extraction
	bits8 := ((bpv + 7) / 8) * 8

	values := make([]float64, 0, numValues)

	for j := 0; j < height; j++ {
		for k := 0; k < width; k++ {
			if uint32(len(values)) >= numValues {
				break
			}
			pixel := img.At(bounds.Min.X+k, bounds.Min.Y+j)
			x := pixelToUint(pixel, bits8)
			values = append(values, (float64(x)*binaryS+ref)*decimalS)
		}
	}

	return values, nil
}

// pixelToUint extracts the unsigned integer value from a pixel.
// PNG in GRIB2 uses grayscale with 8 or 16 bits per channel.
func pixelToUint(p color.Color, bits int) uint64 {
	switch bits {
	case 8:
		// 8-bit grayscale
		r, _, _, _ := p.RGBA()
		return uint64(r >> 8)
	case 16:
		// 16-bit grayscale
		r, _, _, _ := p.RGBA()
		return uint64(r)
	case 24:
		// RGB: 3 channels of 8 bits each
		r, g, b, _ := p.RGBA()
		return (uint64(r>>8) << 16) | (uint64(g>>8) << 8) | uint64(b>>8)
	case 32:
		// RGBA: 4 channels of 8 bits each
		r, g, b, a := p.RGBA()
		return (uint64(r>>8) << 24) | (uint64(g>>8) << 16) | (uint64(b>>8) << 8) | uint64(a>>8)
	default:
		// Fall back to grayscale
		r, _, _, _ := p.RGBA()
		if bits >= 16 {
			return uint64(r)
		}
		return uint64(r >> (16 - bits))
	}
}

// PackPNG encodes data using Template 5.41 (PNG packing).
// It packs the values into a grayscale PNG image.
func PackPNG(values []float64, bitsPerValue uint8, width, height int) ([]byte, Template50, error) {
	n := len(values)
	if n == 0 {
		return nil, Template50{BitsPerValue: bitsPerValue}, nil
	}

	// Find min/max for scaling parameters
	vmin, vmax := values[0], values[0]
	for _, v := range values[1:] {
		if v < vmin {
			vmin = v
		}
		if v > vmax {
			vmax = v
		}
	}

	refValue := float32(vmin)
	ref := float64(refValue)
	decimalScale := int16(0)
	bpv := int(bitsPerValue)
	maxPackedVal := float64(uint64(1)<<bpv - 1)

	var binaryScale int16
	rng := vmax - ref
	if rng == 0 || bpv == 0 {
		binaryScale = 0
	} else {
		e := math.Log2(rng / maxPackedVal)
		binaryScale = int16(math.Ceil(e))
	}

	tmpl := Template50{
		ReferenceValue:            refValue,
		BinaryScaleFactor:         binaryScale,
		DecimalScaleFactor:        decimalScale,
		BitsPerValue:              bitsPerValue,
		TypeOfOriginalFieldValues: 0,
	}

	if bpv == 0 {
		return nil, tmpl, nil
	}

	invScale := math.Pow(2, float64(-binaryScale))
	bits8 := ((bpv + 7) / 8) * 8

	// Create the PNG image
	var img image.Image
	idx := 0
	switch {
	case bits8 <= 8:
		gray := image.NewGray(image.Rect(0, 0, width, height))
		for j := 0; j < height; j++ {
			for k := 0; k < width; k++ {
				if idx < n {
					x := uint8(math.Round((values[idx] - ref) * invScale))
					gray.SetGray(k, j, color.Gray{Y: x})
					idx++
				}
			}
		}
		img = gray
	case bits8 <= 16:
		gray16 := image.NewGray16(image.Rect(0, 0, width, height))
		for j := 0; j < height; j++ {
			for k := 0; k < width; k++ {
				if idx < n {
					x := uint16(math.Round((values[idx] - ref) * invScale))
					gray16.SetGray16(k, j, color.Gray16{Y: x})
					idx++
				}
			}
		}
		img = gray16
	default:
		return nil, tmpl, fmt.Errorf("grib2: PNG packing: unsupported bits per value %d", bpv)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, tmpl, fmt.Errorf("grib2: PNG encode: %w", err)
	}

	return buf.Bytes(), tmpl, nil
}
