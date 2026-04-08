package grib2

import (
	"fmt"
	"math"
)

// UnpackRunLength decodes data packed with Template 5.200 (run-length packing
// with level values).
//
// The algorithm follows the eccodes DataRunLengthPacking implementation:
//   - Each compressed value <= maxLevelValue is a level index (0 = missing).
//   - Values > maxLevelValue encode a run-length count in a variable-base
//     representation where range = (2^bitsPerValue - 1) - maxLevelValue.
//
// The missingValue is used for level index 0.
func UnpackRunLength(data []byte, tmpl Template5200, numValues uint32, missingValue float64) ([]float64, error) {
	if numValues == 0 {
		return nil, nil
	}

	// Guard against huge allocations from malformed input.
	const maxElements = 100_000_000
	if numValues > maxElements {
		return nil, fmt.Errorf("grib2: run-length packing: numValues %d exceeds maximum %d", numValues, maxElements)
	}

	bpv := int(tmpl.BitsPerValue)
	maxLevel := int64(tmpl.MaxLevelValue)
	numLevels := int64(tmpl.NumberOfLevelValues)
	dsf := int(tmpl.DecimalScaleFactor)

	// Number of compressed values in the data stream
	numCompressed := int64(len(data)*8) / int64(bpv)
	if numCompressed == 0 || maxLevel == 0 {
		// All missing
		values := make([]float64, numValues)
		for i := range values {
			values[i] = missingValue
		}
		return values, nil
	}

	rng := (int64(1) << bpv) - 1 - maxLevel
	if maxLevel <= 0 || numLevels <= 0 || maxLevel > numLevels || rng <= 0 {
		return nil, fmt.Errorf("grib2: run-length packing: invalid parameters: maxLevelValue=%d, numberOfLevelValues=%d, range=%d",
			maxLevel, numLevels, rng)
	}

	// Handle decimal scale factor (eccodes sign-magnitude for values > 127)
	scaledDSF := dsf
	if scaledDSF > 127 {
		scaledDSF = -(scaledDSF - 128)
	}
	levelScaleFactor := math.Pow(10, float64(-scaledDSF))

	// Build levels table: levels[0] = missing, levels[1..n] = levelValues[0..n-1] * scaleFactor
	levels := make([]float64, numLevels+1)
	levels[0] = missingValue
	for i := int64(0); i < numLevels; i++ {
		levels[i+1] = float64(tmpl.LevelValues[i]) * levelScaleFactor
	}

	// Extract all compressed values from the bit stream
	compressed := make([]int64, numCompressed)
	for i := int64(0); i < numCompressed; i++ {
		compressed[i] = int64(extractBits(data, uint64(i)*uint64(bpv), bpv))
	}

	// Decode run-length encoded values
	values := make([]float64, numValues)
	j := uint32(0) // output index
	i := int64(0)  // compressed input index

	for i < numCompressed {
		if compressed[i] > maxLevel {
			return nil, fmt.Errorf("grib2: run-length packing: compressed_values[%d]=%d exceeds maxLevelValue=%d at start of run",
				i, compressed[i], maxLevel)
		}
		v := compressed[i]
		i++

		// Bounds-check v against the levels slice.
		if v < 0 || v >= int64(len(levels)) {
			return nil, fmt.Errorf("grib2: run-length packing: level index %d out of range [0, %d)", v, len(levels))
		}

		// Decode run-length: subsequent values > maxLevel encode the repeat count
		// in a variable-base representation.
		n := int64(1)
		factor := int64(1)
		for i < numCompressed && compressed[i] > maxLevel {
			delta := compressed[i] - maxLevel - 1
			// Guard against int64 overflow in factor multiplication.
			if factor > 0 && delta > (math.MaxInt64-n)/factor {
				return nil, fmt.Errorf("grib2: run-length packing: run-length overflow at compressed index %d", i)
			}
			n += factor * delta
			if rng != 0 && factor > math.MaxInt64/rng {
				return nil, fmt.Errorf("grib2: run-length packing: factor overflow at compressed index %d", i)
			}
			factor *= rng
			i++
		}

		if int64(j)+n > int64(numValues) {
			return nil, fmt.Errorf("grib2: run-length packing: run of %d at output position %d exceeds numValues %d",
				n, j, numValues)
		}

		level := levels[v]
		for k := int64(0); k < n; k++ {
			values[j] = level
			j++
		}
	}

	if j != numValues {
		return nil, fmt.Errorf("grib2: run-length packing: decoded %d values, expected %d", j, numValues)
	}

	return values, nil
}
