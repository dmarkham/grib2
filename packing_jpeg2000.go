package grib2

import (
	"fmt"
	"math"

	"github.com/dmarkham/grib2/internal/j2k"
)

// maxJPEG2000BitsPerValue is the largest bitsPerValue PackJPEG2000 accepts.
// JPEG 2000 allows 38-bit components and OpenJPEG (and this package) decode
// 31, but Jasper, the JPEG 2000 library of wgrib2, g2clib and the default
// eccodes build, silently returns wrong values for 31-bit streams (seen with
// Jasper 4.2.9 through grib_get_data: all 496 values of a 31-bit field were
// wrong, while 29 and 30 bits are exact), so the encoder stops at 30.
const maxJPEG2000BitsPerValue = 30

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
// PackJPEG2000 chooses D=0, R as the largest float32 that does not exceed
// the minimum value, and E so that the quantized values fit in
// bitsPerValue bits (1..30). Every value then decodes within half a
// quantization step (2^E / 2) of the original. The quantized values are
// encoded as a JPEG 2000 lossless codestream.
//
// A constant field is always written with BitsPerValue 0 and no data,
// whatever bitsPerValue was requested (as eccodes does); it decodes to R,
// the value rounded to float32.
//
// Errors are returned for NaN or infinite values, values outside the
// float32 range, bitsPerValue 0 with non-constant data, bitsPerValue above
// 30, and fields whose samples the JPEG 2000 coder cannot represent
// exactly (possible only at 29 bits and above).
func PackJPEG2000(values []float64, bitsPerValue uint8) ([]byte, Template540, error) {
	tmpl := Template540{
		Template50: Template50{
			BitsPerValue:              bitsPerValue,
			TypeOfOriginalFieldValues: 0,
		},
		TypeOfCompression:      0,   // lossless
		TargetCompressionRatio: 255, // not applicable
	}
	n := len(values)
	if n == 0 {
		return nil, tmpl, nil
	}
	bpv := int(bitsPerValue)
	if bpv > maxJPEG2000BitsPerValue {
		return nil, tmpl, fmt.Errorf("grib2: JPEG2000 packing: bitsPerValue %d exceeds the maximum of %d", bpv, maxJPEG2000BitsPerValue)
	}

	vmin, vmax := math.Inf(1), math.Inf(-1)
	for i, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, tmpl, fmt.Errorf("grib2: JPEG2000 packing: value[%d] is %v", i, v)
		}
		if math.Abs(v) > math.MaxFloat32 {
			return nil, tmpl, fmt.Errorf("grib2: JPEG2000 packing: value[%d] = %g is outside the float32 range", i, v)
		}
		vmin = math.Min(vmin, v)
		vmax = math.Max(vmax, v)
	}

	if vmin == vmax {
		// Constant field: no data; decoders return R.
		tmpl.ReferenceValue = float32(vmin)
		tmpl.BitsPerValue = 0
		return nil, tmpl, nil
	}
	if bpv == 0 {
		return nil, tmpl, fmt.Errorf("grib2: JPEG2000 packing: bitsPerValue 0 cannot represent non-constant data (range %g..%g)", vmin, vmax)
	}

	refValue := jpeg2000ReferenceBelow(vmin)
	ref := float64(refValue)

	// Binary scale factor: smallest E with round((vmax-R)/2^E) <= 2^bpv-1.
	maxPacked := float64(uint64(1)<<uint(bpv) - 1)
	rng := vmax - ref
	binaryScale := int(math.Ceil(math.Log2(rng / maxPacked)))
	for binaryScale > -32767 && math.Round(math.Ldexp(rng, -(binaryScale-1))) <= maxPacked {
		binaryScale--
	}
	for math.Round(math.Ldexp(rng, -binaryScale)) > maxPacked {
		binaryScale++
	}
	if binaryScale < -32767 || binaryScale > 32767 {
		return nil, tmpl, fmt.Errorf("grib2: JPEG2000 packing: binary scale factor %d out of range", binaryScale)
	}

	tmpl.ReferenceValue = refValue
	tmpl.BinaryScaleFactor = int16(binaryScale)
	tmpl.DecimalScaleFactor = 0

	samples := make([]int32, n)
	for i, v := range values {
		q := math.Round(math.Ldexp(v-ref, -binaryScale))
		if q < 0 {
			q = 0
		} else if q > maxPacked {
			q = maxPacked
		}
		samples[i] = int32(q)
	}

	// A single n x 1 row, matching common GRIB2 practice.
	encoded, err := j2k.Encode(samples, n, 1, bpv)
	if err != nil {
		return nil, tmpl, fmt.Errorf("grib2: JPEG2000 encode: %w", err)
	}
	return encoded, tmpl, nil
}

// jpeg2000ReferenceBelow returns the largest float32 that is <= v. The
// reference value must not exceed the field minimum: float32(v) rounds to
// nearest, which is above v for about half of all inputs, and the
// packed integers are unsigned.
func jpeg2000ReferenceBelow(v float64) float32 {
	r := float32(v)
	if float64(r) > v {
		r = math.Nextafter32(r, float32(math.Inf(-1)))
	}
	return r
}

// UnpackJPEG2000 decodes data packed with Template 5.40 (JPEG2000).
//
// The packed data in Section 7 is a raw JPEG2000 codestream (J2K, not JP2).
// Each pixel value is an unsigned integer. The same scaling formula as
// simple packing applies: Y = (X * 2^E + R) * 10^(-D)
//
// A field with BitsPerValue 0 is constant and every value is R itself,
// without the decimal scaling, as in eccodes (DataJpeg2000Packing) and
// g2clib.
//
// Only lossless (reversible 5/3) codestreams are supported; a lossy
// irreversible (9/7) stream returns an error.
func UnpackJPEG2000(data []byte, tmpl Template540, numValues uint32) ([]float64, error) {
	if numValues == 0 {
		return nil, nil
	}

	// Guard against huge allocations from malformed input.
	const maxElements = 100_000_000
	if numValues > maxElements {
		return nil, fmt.Errorf("grib2: JPEG2000 packing: numValues %d exceeds maximum %d", numValues, maxElements)
	}

	// Constant field.
	if tmpl.BitsPerValue == 0 {
		values := make([]float64, numValues)
		ref := float64(tmpl.ReferenceValue)
		for i := range values {
			values[i] = ref
		}
		return values, nil
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("grib2: JPEG2000: empty codestream with bitsPerValue %d", tmpl.BitsPerValue)
	}

	// Decode the JPEG2000 codestream
	pixels, width, height, err := j2k.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("grib2: JPEG2000 decode: %w", err)
	}

	total := width * height
	if uint32(total) < numValues {
		return nil, fmt.Errorf("grib2: JPEG2000: decoded %d pixels but need %d values", total, numValues)
	}

	// Apply the GRIB2 scaling formula
	binaryS := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	decimalS := math.Pow(10, float64(-tmpl.DecimalScaleFactor))
	ref := float64(tmpl.ReferenceValue)

	values := make([]float64, numValues)
	for i := uint32(0); i < numValues; i++ {
		x := float64(pixels[i])
		values[i] = (x*binaryS + ref) * decimalS
	}

	return values, nil
}
