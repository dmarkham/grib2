package grib2

import (
	"fmt"
	"math"
)

// Values unpacks the data in this field and returns the final grid of values.
//
// It performs three steps:
//  1. Dispatch to the correct Unpack* function based on Section 5 TemplateNumber
//  2. Apply the Section 6 bitmap (if indicator == 0), expanding the packed
//     values into a full grid with NaN for missing positions
//  3. Apply scanning mode reordering (if Section 3 Template 3.0 ScanningMode != 0)
func (f *Field) Values() ([]float64, error) {
	values, err := f.unpackRaw()
	if err != nil {
		return nil, err
	}

	// Apply bitmap if present
	if f.Section6.HasBitmap() {
		values, err = applyBitmap(values, f.Section6.Bitmap, f.Section3.NumberOfDataPoints)
		if err != nil {
			return nil, fmt.Errorf("grib2: applying bitmap: %w", err)
		}
	}

	// Apply scanning mode reordering if needed
	scanMode, ni, nj := gridScanParams(f.Section3.Template)
	if scanMode != 0 {
		values = applyScanningMode(values, scanMode, ni, nj)
	}

	return values, nil
}

// unpackRaw dispatches to the correct Unpack* function based on the
// data representation template number in Section 5.
func (f *Field) unpackRaw() ([]float64, error) {
	switch f.Section5.TemplateNumber {
	case 0: // Simple packing
		tmpl, ok := f.Section5.Template.(Template50)
		if !ok {
			return nil, fmt.Errorf("grib2: template 5.0: expected Template50, got %T", f.Section5.Template)
		}
		return UnpackSimple(f.Section7.Data, tmpl, f.Section5.NumberOfValues)

	case 2: // Complex packing
		tmpl, ok := f.Section5.Template.(Template52)
		if !ok {
			return nil, fmt.Errorf("grib2: template 5.2: expected Template52, got %T", f.Section5.Template)
		}
		return UnpackComplex(f.Section7.Data, tmpl, f.Section5.NumberOfValues, 0, 0)

	case 3: // Complex packing with spatial differencing
		tmpl, ok := f.Section5.Template.(Template53)
		if !ok {
			return nil, fmt.Errorf("grib2: template 5.3: expected Template53, got %T", f.Section5.Template)
		}
		return UnpackComplex(f.Section7.Data, tmpl.Template52, f.Section5.NumberOfValues,
			tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)

	case 4: // IEEE floating point
		tmpl, ok := f.Section5.Template.(Template54)
		if !ok {
			return nil, fmt.Errorf("grib2: template 5.4: expected Template54, got %T", f.Section5.Template)
		}
		return UnpackIEEE(f.Section7.Data, tmpl, f.Section5.NumberOfValues)

	case 40: // JPEG2000
		tmpl, ok := f.Section5.Template.(Template540)
		if !ok {
			return nil, fmt.Errorf("grib2: template 5.40: expected Template540, got %T", f.Section5.Template)
		}
		return UnpackJPEG2000(f.Section7.Data, tmpl, f.Section5.NumberOfValues)

	case 41: // PNG
		tmpl, ok := f.Section5.Template.(Template50)
		if !ok {
			return nil, fmt.Errorf("grib2: template 5.41: expected Template50, got %T", f.Section5.Template)
		}
		return UnpackPNG(f.Section7.Data, tmpl, f.Section5.NumberOfValues)

	case 42: // CCSDS
		tmpl, ok := f.Section5.Template.(Template542)
		if !ok {
			return nil, fmt.Errorf("grib2: template 5.42: expected Template542, got %T", f.Section5.Template)
		}
		return UnpackCCSDS(f.Section7.Data, tmpl, f.Section5.NumberOfValues)

	case 200: // Run-length packing with level values
		tmpl, ok := f.Section5.Template.(Template5200)
		if !ok {
			return nil, fmt.Errorf("grib2: template 5.200: expected Template5200, got %T", f.Section5.Template)
		}
		return UnpackRunLength(f.Section7.Data, tmpl, f.Section5.NumberOfValues, math.NaN())

	default:
		return nil, fmt.Errorf("grib2: unsupported data representation template %d", f.Section5.TemplateNumber)
	}
}

// applyBitmap expands packed values using the Section 6 bitmap.
//
// When a bitmap is present, the packed data contains only values for grid
// points where the bitmap bit is 1. This function expands those values into
// a full grid array of size totalPoints, inserting NaN where the bitmap
// bit is 0.
func applyBitmap(packedValues []float64, bitmap []byte, totalPoints uint32) ([]float64, error) {
	expanded := make([]float64, totalPoints)
	valueIdx := 0

	for i := uint32(0); i < totalPoints; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8) // MSB first

		if byteIdx >= uint32(len(bitmap)) {
			// Bitmap too short; fill remaining with NaN
			for j := i; j < totalPoints; j++ {
				expanded[j] = math.NaN()
			}
			break
		}

		if bitmap[byteIdx]&(1<<bitIdx) != 0 {
			// Bit is set: value exists
			if valueIdx >= len(packedValues) {
				return nil, fmt.Errorf("bitmap expects more values than packed data contains (%d values exhausted at grid point %d)", len(packedValues), i)
			}
			expanded[i] = packedValues[valueIdx]
			valueIdx++
		} else {
			// Bit is clear: missing value
			expanded[i] = math.NaN()
		}
	}

	return expanded, nil
}

// applyScanningMode reorders grid values based on the Flag Table 3.4
// scanning mode byte.
//
// Scanning mode bits (from MSB):
//
//	Bit 1 (0x80): i direction: 0 = +i (west to east), 1 = -i (east to west)
//	Bit 2 (0x40): j direction: 0 = -j (north to south), 1 = +j (south to north)
//	Bit 3 (0x20): adjacency:   0 = consecutive in i, 1 = consecutive in j
//	Bit 4 (0x10): row direction: 0 = all rows same direction, 1 = alternating
//
// The canonical output order is scan mode 0: +i, -j, consecutive in i,
// non-alternating (i.e., west-to-east, north-to-south, row-major).
func applyScanningMode(values []float64, scanMode uint8, ni, nj int) []float64 {
	if ni <= 0 || nj <= 0 || len(values) == 0 {
		return values
	}

	n := ni * nj
	if n > len(values) {
		// Grid dimensions don't match data length; skip reordering
		return values
	}

	iNeg := scanMode&0x80 != 0 // bit 1: -i direction
	jPos := scanMode&0x40 != 0 // bit 2: +j direction
	jConsec := scanMode&0x20 != 0 // bit 3: consecutive in j
	alternating := scanMode&0x10 != 0 // bit 4: boustrophedonic

	out := make([]float64, n)

	for srcIdx := 0; srcIdx < n; srcIdx++ {
		// Determine the source row and column in the input scanning order
		var row, col int
		if jConsec {
			// Data is column-major: consecutive points are in j direction
			col = srcIdx / nj
			row = srcIdx % nj
		} else {
			// Data is row-major: consecutive points are in i direction
			row = srcIdx / ni
			col = srcIdx % ni
		}

		// Handle alternating (boustrophedonic) rows
		if alternating {
			if jConsec {
				// Columns alternate direction
				if col%2 == 1 {
					row = nj - 1 - row
				}
			} else {
				// Rows alternate direction
				if row%2 == 1 {
					col = ni - 1 - col
				}
			}
		}

		// Reverse i direction if needed
		if iNeg {
			col = ni - 1 - col
		}

		// Reverse j direction if needed (canonical is -j = north to south)
		if jPos {
			row = nj - 1 - row
		}

		// Place in canonical row-major order: row * ni + col
		dstIdx := row*ni + col
		if dstIdx >= 0 && dstIdx < n {
			out[dstIdx] = values[srcIdx]
		}
	}

	// Preserve any values beyond ni*nj (shouldn't happen, but be safe)
	if len(values) > n {
		out = append(out, values[n:]...)
	}

	return out
}

// gridScanParams extracts scanning mode and grid dimensions from
// whichever Section 3 grid definition template is present.
// Returns (0, 0, 0) for unrecognized template types.
func gridScanParams(tmpl GridDefinitionTemplate) (scanMode uint8, ni, nj int) {
	switch t := tmpl.(type) {
	case Template30:
		return t.ScanningMode, int(t.Ni), int(t.Nj)
	case Template31:
		return t.ScanningMode, int(t.Ni), int(t.Nj)
	case Template310:
		return t.ScanningMode, int(t.Ni), int(t.Nj)
	case Template320:
		return t.ScanningMode, int(t.Nx), int(t.Ny)
	case Template330:
		return t.ScanningMode, int(t.Nx), int(t.Ny)
	case Template340:
		return t.ScanningMode, int(t.Ni), int(t.Nj)
	case Template390:
		return t.ScanningMode, int(t.Nx), int(t.Ny)
	default:
		return 0, 0, 0
	}
}
