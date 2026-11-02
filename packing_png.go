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

	// Constant field: the reference value itself, without the decimal
	// scale factor (eccodes DataPngPacking.cc).
	if bpv == 0 {
		val := float64(tmpl.ReferenceValue)
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

	n := int(numValues)
	if width*height < n {
		return nil, fmt.Errorf("grib2: PNG packing: image is %dx%d, need %d values", width, height, n)
	}
	values := make([]float64, n)
	if err := pngSamples(img, bpv, func(i int, x uint64) bool {
		if i >= n {
			return false
		}
		values[i] = (float64(x)*binaryS + ref) * decimalS
		return true
	}); err != nil {
		return nil, err
	}
	return values, nil
}

// pngSamples calls put with each pixel's raw sample, row by row. Samples
// are read from the decoded pixel bytes: going through color.Color would
// premultiply RGBA pixels by their alpha byte, which in GRIB2 is the low
// byte of a 32-bit sample (eccodes DataPngPacking.cc).
func pngSamples(img image.Image, bpv int, put func(i int, x uint64) bool) error {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	depth := ((bpv + 7) / 8) * 8
	i := 0
	switch im := img.(type) {
	case *image.Gray:
		if depth != 8 {
			return fmt.Errorf("grib2: PNG packing: 8-bit grayscale image for %d bits per value", bpv)
		}
		for y := 0; y < h; y++ {
			for _, p := range im.Pix[y*im.Stride : y*im.Stride+w] {
				if !put(i, uint64(p)) {
					return nil
				}
				i++
			}
		}
	case *image.Gray16:
		if depth != 16 {
			return fmt.Errorf("grib2: PNG packing: 16-bit grayscale image for %d bits per value", bpv)
		}
		for y := 0; y < h; y++ {
			row := im.Pix[y*im.Stride : y*im.Stride+2*w]
			for k := 0; k < w; k++ {
				if !put(i, uint64(row[2*k])<<8|uint64(row[2*k+1])) {
					return nil
				}
				i++
			}
		}
	case *image.RGBA, *image.NRGBA:
		// 24-bit samples are RGB (Go decodes them with an opaque alpha),
		// 32-bit samples are RGBA.
		var pix []byte
		var stride int
		if rgba, ok := im.(*image.RGBA); ok {
			pix, stride = rgba.Pix, rgba.Stride
		} else {
			nrgba := im.(*image.NRGBA)
			pix, stride = nrgba.Pix, nrgba.Stride
		}
		if depth != 24 && depth != 32 {
			return fmt.Errorf("grib2: PNG packing: colour image for %d bits per value", bpv)
		}
		for y := 0; y < h; y++ {
			for k := 0; k < w; k++ {
				p := pix[y*stride+4*k : y*stride+4*k+4]
				x := uint64(p[0])<<16 | uint64(p[1])<<8 | uint64(p[2])
				if depth == 32 {
					x = x<<8 | uint64(p[3])
				}
				if !put(i, x) {
					return nil
				}
				i++
			}
		}
	default:
		return fmt.Errorf("grib2: PNG packing: unsupported image type %T for %d bits per value", img, bpv)
	}
	return nil
}

// PackPNG encodes data using Template 5.41 (PNG packing).
// It packs the values into a grayscale PNG image.
func PackPNG(values []float64, bitsPerValue uint8, width, height int) ([]byte, Template50, error) {
	n := len(values)
	if n == 0 {
		return nil, Template50{BitsPerValue: bitsPerValue}, nil
	}

	sc, err := chooseSimpleScale("PNG packing", values, int(bitsPerValue), 16, false)
	if err != nil {
		return nil, Template50{}, err
	}
	if width <= 0 || height <= 0 || width*height < n {
		return nil, Template50{}, fmt.Errorf("grib2: PNG packing: %dx%d image cannot hold %d values", width, height, n)
	}
	tmpl := Template50{
		ReferenceValue:            sc.ref,
		BinaryScaleFactor:         sc.e,
		BitsPerValue:              bitsPerValue,
		TypeOfOriginalFieldValues: 0,
	}
	if sc.constant {
		tmpl.BitsPerValue = 0
		return nil, tmpl, nil
	}
	bpv := int(bitsPerValue)
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
					x := uint8(sc.packed(values[idx]))
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
					x := uint16(sc.packed(values[idx]))
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
