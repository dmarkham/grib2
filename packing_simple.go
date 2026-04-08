package grib2

import (
	"encoding/binary"
	"fmt"
	"math"
)

// UnpackSimple decodes data packed with Template 5.0 (simple packing).
//
// The unpacking formula per WMO spec is:
//
//	Y = (R + X * 2^E) * 10^(-D)
//
// Where:
//
//	R = reference value (float32)
//	E = binary scale factor
//	D = decimal scale factor
//	X = scaled integer value from packed bits
func UnpackSimple(data []byte, tmpl Template50, numValues uint32) ([]float64, error) {
	if numValues == 0 {
		return nil, nil
	}

	// Guard against huge allocations from malformed input.
	const maxElements = 100_000_000
	if numValues > maxElements {
		return nil, fmt.Errorf("grib2: simple packing: numValues %d exceeds maximum %d", numValues, maxElements)
	}

	bpv := int(tmpl.BitsPerValue)

	// Special case: 0 bits per value means constant field
	if bpv == 0 {
		ref := float64(tmpl.ReferenceValue)
		d := math.Pow(10, float64(-tmpl.DecimalScaleFactor))
		val := ref * d
		values := make([]float64, numValues)
		for i := range values {
			values[i] = val
		}
		return values, nil
	}

	// Check we have enough data
	expectedBits := uint64(bpv) * uint64(numValues)
	expectedBytes := (expectedBits + 7) / 8
	if uint64(len(data)) < expectedBytes {
		return nil, fmt.Errorf("grib2: simple packing: need %d bytes for %d values at %d bpv, got %d",
			expectedBytes, numValues, bpv, len(data))
	}

	s := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	d := math.Pow(10, float64(-tmpl.DecimalScaleFactor))
	ref := float64(tmpl.ReferenceValue)

	values := make([]float64, numValues)

	// Extract packed integers and apply formula
	// For aligned bit widths (8, 16, 32), use fast paths
	switch bpv {
	case 8:
		for i := uint32(0); i < numValues; i++ {
			x := float64(data[i])
			values[i] = (ref + x*s) * d
		}
	case 16:
		for i := uint32(0); i < numValues; i++ {
			x := float64(binary.BigEndian.Uint16(data[i*2 : i*2+2]))
			values[i] = (ref + x*s) * d
		}
	case 32:
		for i := uint32(0); i < numValues; i++ {
			x := float64(binary.BigEndian.Uint32(data[i*4 : i*4+4]))
			values[i] = (ref + x*s) * d
		}
	default:
		// General bit extraction for arbitrary bit widths
		for i := uint32(0); i < numValues; i++ {
			x := float64(extractBits(data, uint64(i)*uint64(bpv), bpv))
			values[i] = (ref + x*s) * d
		}
	}

	return values, nil
}

// PackSimple encodes data using Template 5.0 (simple packing).
// Returns the packed data bytes and the Template50 with computed parameters.
func PackSimple(values []float64, bitsPerValue uint8) ([]byte, Template50, error) {
	n := len(values)
	if n == 0 {
		return nil, Template50{BitsPerValue: bitsPerValue}, nil
	}

	// Find min and max
	vmin, vmax := values[0], values[0]
	for _, v := range values[1:] {
		if v < vmin {
			vmin = v
		}
		if v > vmax {
			vmax = v
		}
	}

	// Reference value is the minimum
	refValue := float32(vmin)
	ref := float64(refValue)

	// Decimal scale factor = 0 (simplest case)
	decimalScale := int16(0)

	// Calculate binary scale factor
	// We need: max_X = (vmax - ref) * 2^(-E) fits in bitsPerValue bits
	// max_X = 2^bpv - 1
	// So: 2^(-E) = max_X / (vmax - ref)
	//     -E = log2(max_X / (vmax - ref))
	//      E = -log2(max_X / (vmax - ref))
	bpv := int(bitsPerValue)
	maxPackedVal := float64(uint64(1)<<bpv - 1)

	var binaryScale int16
	rng := vmax - ref
	if rng == 0 || bpv == 0 {
		// Constant field
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
		TypeOfOriginalFieldValues: 0, // floating point
	}

	if bpv == 0 {
		return nil, tmpl, nil
	}

	// Pack values
	s := math.Pow(2, float64(-binaryScale)) // inverse of decode scale

	totalBits := uint64(bpv) * uint64(n)
	totalBytes := (totalBits + 7) / 8
	packed := make([]byte, totalBytes)

	switch bpv {
	case 8:
		for i, v := range values {
			x := uint8(math.Round((v - ref) * s))
			packed[i] = x
		}
	case 16:
		for i, v := range values {
			x := uint16(math.Round((v - ref) * s))
			binary.BigEndian.PutUint16(packed[i*2:i*2+2], x)
		}
	case 32:
		for i, v := range values {
			x := uint32(math.Round((v - ref) * s))
			binary.BigEndian.PutUint32(packed[i*4:i*4+4], x)
		}
	default:
		for i, v := range values {
			x := uint64(math.Round((v - ref) * s))
			storeBits(packed, uint64(i)*uint64(bpv), bpv, x)
		}
	}

	return packed, tmpl, nil
}

// extractBits reads an unsigned integer of width bits from data at the given bit offset.
// If the requested bits extend beyond the data slice, the out-of-bounds bits
// are treated as zero (safe for malformed input).
func extractBits(data []byte, bitOffset uint64, width int) uint64 {
	var result uint64
	dataLen := uint64(len(data))
	for i := 0; i < width; i++ {
		byteIdx := (bitOffset + uint64(i)) / 8
		if byteIdx >= dataLen {
			break
		}
		bitIdx := 7 - ((bitOffset + uint64(i)) % 8) // MSB first
		if data[byteIdx]&(1<<bitIdx) != 0 {
			result |= 1 << (width - 1 - i)
		}
	}
	return result
}

// storeBits writes an unsigned integer of width bits into data at the given bit offset.
// If the requested bit position extends beyond the data slice, out-of-bounds
// writes are silently skipped.
func storeBits(data []byte, bitOffset uint64, width int, value uint64) {
	dataLen := uint64(len(data))
	for i := 0; i < width; i++ {
		if value&(1<<(width-1-i)) != 0 {
			byteIdx := (bitOffset + uint64(i)) / 8
			if byteIdx >= dataLen {
				break
			}
			bitIdx := 7 - ((bitOffset + uint64(i)) % 8)
			data[byteIdx] |= 1 << bitIdx
		}
	}
}
