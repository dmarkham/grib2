package grib2

import (
	"fmt"
	"math"

	"github.com/dmarkham/grib2/internal/aec"
)

// PackCCSDS encodes values using Template 5.42 (CCSDS/AEC) compression.
//
// It quantises the float64 values into unsigned integers using simple packing
// parameters (reference value, binary scale, decimal scale) and then compresses
// the integer stream with the AEC encoder (CCSDS 121.0-B-3).
//
// Returns the compressed byte stream and the populated Template542.
func PackCCSDS(values []float64, bitsPerValue uint8, blockSize uint8, rsi uint16) ([]byte, Template542, error) {
	n := len(values)
	if n == 0 {
		tmpl := Template542{
			Template50: Template50{BitsPerValue: bitsPerValue},
			CcsdsFlags:     uint8(aec.DataMSB | aec.DataPreprocess),
			CcsdsBlockSize: blockSize,
			CcsdsRsi:       rsi,
		}
		return nil, tmpl, nil
	}

	bpv := int(bitsPerValue)

	// Find min/max
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

	// Calculate binary scale factor
	maxPackedVal := float64(uint64(1)<<bpv - 1)
	var binaryScale int16
	rng := vmax - ref
	if rng == 0 || bpv == 0 {
		binaryScale = 0
	} else {
		e := math.Log2(rng / maxPackedVal)
		binaryScale = int16(math.Ceil(e))
	}

	tmpl := Template542{
		Template50: Template50{
			ReferenceValue:            refValue,
			BinaryScaleFactor:         binaryScale,
			DecimalScaleFactor:        decimalScale,
			BitsPerValue:              bitsPerValue,
			TypeOfOriginalFieldValues: 0,
		},
		CcsdsFlags:     uint8(aec.DataMSB | aec.DataPreprocess),
		CcsdsBlockSize: blockSize,
		CcsdsRsi:       rsi,
	}

	if bpv == 0 {
		return nil, tmpl, nil
	}

	// Quantise float64 values to int32 samples
	invScale := math.Pow(2, float64(-binaryScale))
	samples := make([]int32, n)
	for i, v := range values {
		x := math.Round((v - ref) * invScale)
		if x < 0 {
			x = 0
		}
		if x > maxPackedVal {
			x = maxPackedVal
		}
		samples[i] = int32(x)
	}

	// AEC encode — use flags without Data3Byte and DataMSB since we provide native int32
	flags := uint32(tmpl.CcsdsFlags)
	flags &^= aec.Data3Byte
	flags &^= aec.DataMSB

	opts := aec.Options{
		BitsPerSample: bpv,
		BlockSize:     int(blockSize),
		RSI:           int(rsi),
		Flags:         flags,
	}

	compressed, err := aec.Encode(samples, opts)
	if err != nil {
		return nil, tmpl, fmt.Errorf("grib2: CCSDS/AEC encode: %w", err)
	}

	return compressed, tmpl, nil
}

// UnpackCCSDS decodes data packed with Template 5.42 (CCSDS/AEC).
//
// The packed data in Section 7 is a CCSDS-121.0-B compressed bitstream.
// Each decoded sample is an unsigned integer. The same scaling formula as
// simple packing applies: Y = (X * 2^E + R) * 10^(-D)
func UnpackCCSDS(data []byte, tmpl Template542, numValues uint32) ([]float64, error) {
	if numValues == 0 {
		return nil, nil
	}

	// Guard against huge allocations from malformed input.
	const maxElements = 100_000_000
	if numValues > maxElements {
		return nil, fmt.Errorf("grib2: CCSDS packing: numValues %d exceeds maximum %d", numValues, maxElements)
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

	// Map GRIB2 ccsdsFlags to AEC options.
	// The GRIB2 ccsdsFlags field uses the same bit layout as libaec flags.
	// Following eccodes modify_aec_flags (ECC-1602): clear AEC_DATA_3BYTE
	// and AEC_DATA_MSB since we decode to native int32 values, not raw bytes.
	flags := uint32(tmpl.CcsdsFlags)
	flags &^= aec.Data3Byte
	flags &^= aec.DataMSB

	opts := aec.Options{
		BitsPerSample: bpv,
		BlockSize:     int(tmpl.CcsdsBlockSize),
		RSI:           int(tmpl.CcsdsRsi),
		Flags:         flags,
	}

	decoded, err := aec.Decode(data, opts)
	if err != nil {
		return nil, fmt.Errorf("grib2: CCSDS/AEC decode: %w", err)
	}

	if uint32(len(decoded)) < numValues {
		return nil, fmt.Errorf("grib2: CCSDS: decoded %d samples but need %d values", len(decoded), numValues)
	}

	// Apply the GRIB2 scaling formula: Y = (X * 2^E + R) * 10^(-D)
	binaryS := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	decimalS := math.Pow(10, float64(-tmpl.DecimalScaleFactor))
	ref := float64(tmpl.ReferenceValue)

	values := make([]float64, numValues)
	for i := uint32(0); i < numValues; i++ {
		x := float64(uint32(decoded[i]))
		values[i] = (x*binaryS + ref) * decimalS
	}

	return values, nil
}
