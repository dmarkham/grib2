package grib2

import (
	"fmt"
	"math"
)

const missingLong = int64(math.MaxInt64)

// secondaryMissingLong distinguishes secondary from primary missing values
// when missingValueManagement == 2.
const secondaryMissingLong = int64(math.MaxInt64 - 1)

// UnpackComplexWithSubstitution is like UnpackComplex but substitutes
// the template's missing value placeholders instead of NaN.
//
// When MissingValueManagement is 1, primary missing values are replaced
// with PrimaryMissingValueSubstitute (interpreted as IEEE 754 float32 bits).
// When MissingValueManagement is 2, primary and secondary missing values
// are replaced with their respective substitutes.
//
// If MissingValueManagement is 0, this behaves identically to UnpackComplex.
func UnpackComplexWithSubstitution(data []byte, tmpl52 Template52, numValues uint32,
	orderOfSpatialDiff uint8, numOctetsExtra uint8) ([]float64, error) {

	return unpackComplexInternal(data, tmpl52, numValues, orderOfSpatialDiff, numOctetsExtra, true)
}

// UnpackComplex decodes data packed with Template 5.2 (complex packing)
// or Template 5.3 (complex packing with spatial differencing).
//
// The algorithm:
// 1. Read group reference values, group widths, and group lengths from the data stream
// 2. For each group, decode the per-value deviations and add the group reference
// 3. If spatial differencing, apply the inverse differencing operator
// 4. Apply the scaling: Y = (X * 2^E + R) * 10^(-D)
//
// Missing values (when MissingValueManagement != 0) are returned as math.NaN().
// Use UnpackComplexWithSubstitution to get template-specified substitute values instead.
func UnpackComplex(data []byte, tmpl52 Template52, numValues uint32,
	orderOfSpatialDiff uint8, numOctetsExtra uint8) ([]float64, error) {

	return unpackComplexInternal(data, tmpl52, numValues, orderOfSpatialDiff, numOctetsExtra, false)
}

// unpackComplexInternal is the shared implementation for UnpackComplex and
// UnpackComplexWithSubstitution.
func unpackComplexInternal(data []byte, tmpl52 Template52, numValues uint32,
	orderOfSpatialDiff uint8, numOctetsExtra uint8, useSubstitution bool) ([]float64, error) {

	nGroups := int(tmpl52.NumberOfGroups)
	nVals := int(numValues)

	if nVals == 0 {
		return nil, nil
	}

	// Guard against huge allocations from malformed input.
	const maxElements = 100_000_000
	if nVals > maxElements {
		return nil, fmt.Errorf("grib2: complex packing: numValues %d exceeds maximum %d", nVals, maxElements)
	}

	// Constant field: 0 groups means all values = reference
	if nGroups == 0 {
		ref := float64(tmpl52.ReferenceValue)
		d := math.Pow(10, float64(-tmpl52.DecimalScaleFactor))
		val := ref * d
		values := make([]float64, nVals)
		for i := range values {
			values[i] = val
		}
		return values, nil
	}

	bpv := int(tmpl52.BitsPerValue)
	missingMgmt := int(tmpl52.MissingValueManagement)
	refGroupWidths := int(tmpl52.ReferenceForGroupWidths)
	nBitsGroupWidths := int(tmpl52.NumberOfBitsForGroupWidths)
	refGroupLengths := int(tmpl52.ReferenceForGroupLengths)
	lenIncrement := int(tmpl52.LengthIncrementForGroupLengths)
	trueLenLastGroup := int(tmpl52.TrueLengthOfLastGroup)
	nBitsScaledGroupLens := int(tmpl52.NumberOfBitsForScaledGroupLengths)

	// Calculate buffer pointers
	// Data layout in Section 7:
	//   [spatial diff extras (if order > 0)]
	//   [group reference values: nGroups * bitsPerValue bits]
	//   [group widths: nGroups * nBitsGroupWidths bits]
	//   [group lengths: nGroups * nBitsScaledGroupLens bits]
	//   [packed values]

	// Calculate buffer layout. Following eccodes logic:
	// buf_ref starts at data[0]
	// ref_p = numberOfGroups * bitsPerValue (+ spatial extras bits)
	// buf_width = buf_ref + ceil(ref_p / 8)
	// width_p = numberOfGroups * numberOfBitsForGroupWidths
	// buf_length = buf_width + ceil(width_p / 8)
	// length_p = numberOfGroups * nBitsScaledGroupLens
	// buf_vals = buf_length + ceil(length_p / 8)

	refP := nGroups * bpv
	if orderOfSpatialDiff > 0 {
		refP += int(orderOfSpatialDiff+1) * int(numOctetsExtra) * 8
	}

	widthByteStart := (refP + 7) / 8
	widthP := nGroups * nBitsGroupWidths

	lengthByteStart := widthByteStart + (widthP+7)/8
	lengthP := nGroups * nBitsScaledGroupLens

	valsByteStart := lengthByteStart + (lengthP+7)/8

	if widthByteStart > len(data) || lengthByteStart > len(data) || valsByteStart > len(data) {
		return nil, fmt.Errorf("grib2: complex packing: data too short: need at least %d bytes, got %d",
			valsByteStart, len(data))
	}

	bufRef := data
	bufWidth := data[widthByteStart:]
	bufLength := data[lengthByteStart:]
	bufVals := data[valsByteStart:]

	// Phase 1: Decode group parameters and per-value integers
	secVal := make([]int64, nVals)

	// Starting bit positions within each sub-buffer
	refBitPos := uint64(0)
	if orderOfSpatialDiff > 0 {
		refBitPos = uint64(int(orderOfSpatialDiff+1) * int(numOctetsExtra) * 8)
	}
	widthBitPos := uint64(0)
	lengthBitPos := uint64(0)
	valsBitPos := uint64(0)
	vcount := 0

	for i := 0; i < nGroups; i++ {
		groupRefVal := int64(0)
		if bpv > 0 {
			groupRefVal = int64(extractBits(bufRef, refBitPos, bpv))
			refBitPos += uint64(bpv)
		}

		nvalsPerGroup := int64(0)
		if nBitsScaledGroupLens > 0 {
			nvalsPerGroup = int64(extractBits(bufLength, lengthBitPos, nBitsScaledGroupLens))
			lengthBitPos += uint64(nBitsScaledGroupLens)
		}
		nvalsPerGroup = nvalsPerGroup*int64(lenIncrement) + int64(refGroupLengths)

		nbitsPerGroupVal := int(0)
		if nBitsGroupWidths > 0 {
			nbitsPerGroupVal = int(extractBits(bufWidth, widthBitPos, nBitsGroupWidths))
			widthBitPos += uint64(nBitsGroupWidths)
		}
		nbitsPerGroupVal += refGroupWidths

		// Last group uses true length
		if i == nGroups-1 {
			nvalsPerGroup = int64(trueLenLastGroup)
		}

		if int64(vcount)+nvalsPerGroup > int64(nVals) {
			return nil, fmt.Errorf("grib2: complex packing: group %d overflow: vcount=%d + nvalsPerGroup=%d > nVals=%d",
				i, vcount, nvalsPerGroup, nVals)
		}

		// Decode values within the group
		if missingMgmt == 0 {
			// No missing values
			for j := int64(0); j < nvalsPerGroup; j++ {
				deviation := int64(0)
				if nbitsPerGroupVal > 0 {
					deviation = int64(extractBits(bufVals, valsBitPos, nbitsPerGroupVal))
					valsBitPos += uint64(nbitsPerGroupVal)
				}
				secVal[vcount+int(j)] = groupRefVal + deviation
			}
		} else if missingMgmt == 1 {
			// Primary missing values
			for j := int64(0); j < nvalsPerGroup; j++ {
				if nbitsPerGroupVal == 0 {
					maxn := int64((1 << bpv) - 1)
					if groupRefVal == maxn {
						secVal[vcount+int(j)] = missingLong
					} else {
						secVal[vcount+int(j)] = groupRefVal
					}
				} else {
					temp := int64(extractBits(bufVals, valsBitPos, nbitsPerGroupVal))
					valsBitPos += uint64(nbitsPerGroupVal)
					maxn := int64((1 << nbitsPerGroupVal) - 1)
					if temp == maxn {
						secVal[vcount+int(j)] = missingLong
					} else {
						secVal[vcount+int(j)] = groupRefVal + temp
					}
				}
			}
		} else if missingMgmt == 2 {
			// Primary and secondary missing values
			for j := int64(0); j < nvalsPerGroup; j++ {
				if nbitsPerGroupVal == 0 {
					maxn := int64((1 << bpv) - 1)
					maxn2 := maxn - 1
					if groupRefVal == maxn {
						secVal[vcount+int(j)] = missingLong
					} else if groupRefVal == maxn2 {
						secVal[vcount+int(j)] = secondaryMissingLong
					} else {
						secVal[vcount+int(j)] = groupRefVal
					}
				} else {
					temp := int64(extractBits(bufVals, valsBitPos, nbitsPerGroupVal))
					valsBitPos += uint64(nbitsPerGroupVal)
					maxn := int64((1 << nbitsPerGroupVal) - 1)
					maxn2 := maxn - 1
					if temp == maxn {
						secVal[vcount+int(j)] = missingLong
					} else if temp == maxn2 {
						secVal[vcount+int(j)] = secondaryMissingLong
					} else {
						secVal[vcount+int(j)] = groupRefVal + temp
					}
				}
			}
		}

		vcount += int(nvalsPerGroup)
	}

	// Phase 2: Spatial differencing (if applicable)
	if orderOfSpatialDiff > 0 {
		noe := int(numOctetsExtra)
		bitPos := uint64(0)

		extras := make([]uint64, orderOfSpatialDiff)
		for i := 0; i < int(orderOfSpatialDiff); i++ {
			extras[i] = extractBits(bufRef, bitPos, noe*8)
			bitPos += uint64(noe * 8)
		}

		bias := readSignedBits(bufRef, bitPos, noe*8)

		postProcessSpatialDiff(secVal, nVals, int(orderOfSpatialDiff), bias, extras)
	}

	// Phase 3: Apply scaling
	binaryS := math.Pow(2, float64(tmpl52.BinaryScaleFactor))
	decimalS := math.Pow(10, float64(-tmpl52.DecimalScaleFactor))
	ref := float64(tmpl52.ReferenceValue)

	// Determine substitution values when requested
	var primarySub, secondarySub float64
	if useSubstitution && missingMgmt > 0 {
		primarySub = float64(math.Float32frombits(tmpl52.PrimaryMissingValueSubstitute))
		if missingMgmt == 2 {
			secondarySub = float64(math.Float32frombits(tmpl52.SecondaryMissingValueSubstitute))
		}
	}

	values := make([]float64, nVals)
	for i := 0; i < nVals; i++ {
		if secVal[i] == missingLong {
			if useSubstitution && missingMgmt > 0 {
				values[i] = primarySub
			} else {
				values[i] = math.NaN()
			}
		} else if secVal[i] == secondaryMissingLong {
			if useSubstitution && missingMgmt == 2 {
				values[i] = secondarySub
			} else {
				values[i] = math.NaN()
			}
		} else {
			values[i] = (float64(secVal[i])*binaryS + ref) * decimalS
		}
	}

	return values, nil
}

// readSignedBits reads a sign-magnitude integer from a bit stream.
// Bit 0 = sign (1=negative), remaining bits = magnitude.
func readSignedBits(data []byte, bitOffset uint64, width int) int64 {
	raw := extractBits(data, bitOffset, width)
	signBit := uint64(1) << (width - 1)
	if raw&signBit != 0 {
		return -int64(raw & (signBit - 1))
	}
	return int64(raw)
}

// postProcessSpatialDiff undoes spatial differencing.
// For order 1: cumulative sum starting from extras[0]
// For order 2: second-order cumulative using extras[0] and extras[1]
// isMissing returns true if the value is a primary or secondary missing sentinel.
func isMissing(v int64) bool {
	return v == missingLong || v == secondaryMissingLong
}

func postProcessSpatialDiff(vals []int64, n int, order int, bias int64, extras []uint64) {
	j := 0

	if order == 1 {
		last := int64(extras[0])
		// Skip leading missing values, then place first extra
		for j < n {
			if isMissing(vals[j]) {
				j++
			} else {
				vals[j] = int64(extras[0])
				j++
				break
			}
		}
		// Cumulative sum
		for j < n {
			if isMissing(vals[j]) {
				j++
			} else {
				vals[j] += last + bias
				last = vals[j]
				j++
			}
		}
	} else if order == 2 {
		penultimate := int64(extras[0])
		last := int64(extras[1])
		_ = penultimate // used below

		// Skip leading missing, place extras[0]
		for j < n {
			if isMissing(vals[j]) {
				j++
			} else {
				vals[j] = int64(extras[0])
				j++
				break
			}
		}
		// Skip leading missing, place extras[1]
		for j < n {
			if isMissing(vals[j]) {
				j++
			} else {
				vals[j] = int64(extras[1])
				j++
				break
			}
		}
		// Second order: val[j] = val[j] + bias + 2*last - penultimate
		penultimate = int64(extras[0])
		last = int64(extras[1])
		for ; j < n; j++ {
			if !isMissing(vals[j]) {
				vals[j] = vals[j] + bias + last + last - penultimate
				penultimate = last
				last = vals[j]
			}
		}
	}
}
