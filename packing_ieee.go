package grib2

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Template54 is Data Representation Template 5.4:
// Grid point data — IEEE floating point.
//
//	12     Precision (Table 5.7):
//	         1 = IEEE 32-bit (4 bytes per value)
//	         2 = IEEE 64-bit (8 bytes per value)
type Template54 struct {
	Precision uint8 // Table 5.7
}

func (Template54) TemplateNumber() uint16 { return 4 }

// UnpackIEEE decodes data packed with Template 5.4 (IEEE floating point).
func UnpackIEEE(data []byte, tmpl Template54, numValues uint32) ([]float64, error) {
	if numValues == 0 {
		return nil, nil
	}

	// Guard against huge allocations from malformed input.
	const maxElements = 100_000_000
	if numValues > maxElements {
		return nil, fmt.Errorf("grib2: IEEE packing: numValues %d exceeds maximum %d", numValues, maxElements)
	}

	values := make([]float64, numValues)

	switch tmpl.Precision {
	case 1: // IEEE 32-bit
		bytesPerVal := uint64(4)
		if uint64(len(data)) < uint64(numValues)*bytesPerVal {
			return nil, fmt.Errorf("grib2: IEEE packing: need %d bytes for %d float32 values, got %d",
				uint64(numValues)*bytesPerVal, numValues, len(data))
		}
		for i := uint32(0); i < numValues; i++ {
			bits := binary.BigEndian.Uint32(data[i*4 : i*4+4])
			values[i] = float64(math.Float32frombits(bits))
		}
	case 2: // IEEE 64-bit
		bytesPerVal := uint64(8)
		if uint64(len(data)) < uint64(numValues)*bytesPerVal {
			return nil, fmt.Errorf("grib2: IEEE packing: need %d bytes for %d float64 values, got %d",
				uint64(numValues)*bytesPerVal, numValues, len(data))
		}
		for i := uint32(0); i < numValues; i++ {
			bits := binary.BigEndian.Uint64(data[i*8 : i*8+8])
			values[i] = math.Float64frombits(bits)
		}
	default:
		return nil, fmt.Errorf("grib2: IEEE packing: unsupported precision %d", tmpl.Precision)
	}

	return values, nil
}

// PackIEEE encodes data using Template 5.4 (IEEE floating point).
func PackIEEE(values []float64, precision uint8) ([]byte, Template54, error) {
	tmpl := Template54{Precision: precision}
	n := len(values)
	if n == 0 {
		return nil, tmpl, nil
	}

	switch precision {
	case 1: // IEEE 32-bit
		data := make([]byte, n*4)
		for i, v := range values {
			binary.BigEndian.PutUint32(data[i*4:i*4+4], math.Float32bits(float32(v)))
		}
		return data, tmpl, nil
	case 2: // IEEE 64-bit
		data := make([]byte, n*8)
		for i, v := range values {
			binary.BigEndian.PutUint64(data[i*8:i*8+8], math.Float64bits(v))
		}
		return data, tmpl, nil
	default:
		return nil, tmpl, fmt.Errorf("grib2: IEEE packing: unsupported precision %d", precision)
	}
}
