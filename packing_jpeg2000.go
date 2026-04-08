package grib2

import (
	"fmt"
	"math"

	"github.com/dmarkham/grib2/internal/j2k"
)

// PackJPEG2000 encodes values using Template 5.40 (JPEG2000 lossless).
//
// The scaling formula is the same as simple packing:
//
//	X = (Y * 2^E + R) * 10^(-D)
//
// where X is the original value, Y is the packed pixel value, E is the
// binary scale factor, R is the reference value, and D is the decimal
// scale factor.
//
// PackJPEG2000 chooses R as the minimum value, D=0, and E computed so
// that the quantized values fit in bitsPerValue bits. The quantized
// values are then encoded as a JPEG 2000 lossless codestream.
func PackJPEG2000(values []float64, bitsPerValue uint8) ([]byte, Template540, error) {
	n := len(values)
	if n == 0 {
		return nil, Template540{
			Template50: Template50{BitsPerValue: bitsPerValue},
			TypeOfCompression:      0,
			TargetCompressionRatio: 255,
		}, nil
	}

	bpv := int(bitsPerValue)

	// Find min and max.
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

	// Calculate binary scale factor.
	maxPackedVal := float64(uint64(1)<<bpv - 1)
	var binaryScale int16
	rng := vmax - ref
	if rng == 0 || bpv == 0 {
		binaryScale = 0
	} else {
		e := math.Log2(rng / maxPackedVal)
		binaryScale = int16(math.Ceil(e))
	}

	tmpl := Template540{
		Template50: Template50{
			ReferenceValue:            refValue,
			BinaryScaleFactor:         binaryScale,
			DecimalScaleFactor:        decimalScale,
			BitsPerValue:              bitsPerValue,
			TypeOfOriginalFieldValues: 0,
		},
		TypeOfCompression:      0,   // lossless
		TargetCompressionRatio: 255, // not applicable
	}

	if bpv == 0 {
		// Constant field: no data needed (decoder uses refValue).
		return nil, tmpl, nil
	}

	// Quantize values to integer pixel values.
	invScale := math.Pow(2, float64(-binaryScale))
	samples := make([]int32, n)
	for i, v := range values {
		samples[i] = int32(math.Round((v - ref) * invScale))
	}

	// Choose image dimensions. JPEG 2000 needs width and height.
	// Use n x 1 (single row), matching common GRIB2 practice.
	width := n
	height := 1

	// Encode the pixel values as a J2K codestream.
	encoded, err := j2k.Encode(samples, width, height, bpv)
	if err != nil {
		return nil, tmpl, fmt.Errorf("grib2: JPEG2000 encode: %w", err)
	}

	return encoded, tmpl, nil
}

// UnpackJPEG2000 decodes data packed with Template 5.40 (JPEG2000).
//
// The packed data in Section 7 is a raw JPEG2000 codestream (J2K, not JP2).
// Each pixel value is an unsigned integer. The same scaling formula as
// simple packing applies: Y = (X * 2^E + R) * 10^(-D)
func UnpackJPEG2000(data []byte, tmpl Template540, numValues uint32) ([]float64, error) {
	if numValues == 0 {
		return nil, nil
	}

	// Guard against huge allocations from malformed input.
	const maxElements = 100_000_000
	if numValues > maxElements {
		return nil, fmt.Errorf("grib2: JPEG2000 packing: numValues %d exceeds maximum %d", numValues, maxElements)
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

	// Decode the JPEG2000 codestream
	pixels, width, height, err := j2k.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("grib2: JPEG2000 decode: %w", err)
	}

	// Apply the GRIB2 scaling formula
	binaryS := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	decimalS := math.Pow(10, float64(-tmpl.DecimalScaleFactor))
	ref := float64(tmpl.ReferenceValue)

	total := width * height
	if uint32(total) < numValues {
		return nil, fmt.Errorf("grib2: JPEG2000: decoded %d pixels but need %d values", total, numValues)
	}

	values := make([]float64, numValues)
	for i := uint32(0); i < numValues; i++ {
		x := float64(pixels[i])
		values[i] = (x*binaryS + ref) * decimalS
	}

	return values, nil
}
