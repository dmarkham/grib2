package grib2

import (
	"fmt"
	"math"

	"github.com/dmarkham/grib2/internal/aec"
)

// ccsdsMaxBitsPerValue is the widest sample libaec (and so CCSDS packing)
// can code.
const ccsdsMaxBitsPerValue = 32

// PackCCSDS encodes values using Template 5.42 (CCSDS/AEC) compression.
//
// It quantises the float64 values into unsigned integers using simple packing
// parameters (reference value, binary scale, decimal scale 0) and then
// compresses the integer stream with the AEC encoder (CCSDS 121.0-B-3).
//
// The reference value is the largest float32 that is <= the minimum, so every
// quantised sample is >= 0 and the round-trip error is at most half a
// quantisation step (2^E / 2).
//
// Like eccodes (ECC-1387), a constant field is written with BitsPerValue 0
// and an empty payload whatever bitsPerValue is requested. It decodes to the
// reference value, which is the largest float32 <= the constant.
//
// It returns an error for NaN, ±Inf, values whose magnitude exceeds the
// float32 range, bitsPerValue > 32, bitsPerValue 0 on a field that is not
// constant, and block sizes or RSIs that libaec rejects (block size 8, 16,
// 32 or 64; RSI 1..4096).
//
// Returns the compressed byte stream and the populated Template542.
func PackCCSDS(values []float64, bitsPerValue uint8, blockSize uint8, rsi uint16) ([]byte, Template542, error) {
	tmpl := Template542{
		Template50:     Template50{BitsPerValue: bitsPerValue},
		CcsdsFlags:     uint8(aec.DataMSB | aec.DataPreprocess),
		CcsdsBlockSize: blockSize,
		CcsdsRsi:       rsi,
	}
	if bitsPerValue > ccsdsMaxBitsPerValue {
		return nil, tmpl, fmt.Errorf("grib2: CCSDS packing: invalid number of bits per value %d (max %d)", bitsPerValue, ccsdsMaxBitsPerValue)
	}

	n := len(values)
	if n == 0 {
		return nil, tmpl, nil
	}

	vmin, vmax, err := ccsdsMinMax(values)
	if err != nil {
		return nil, tmpl, err
	}

	refValue := float32AtOrBelow(vmin)
	ref := float64(refValue)
	tmpl.ReferenceValue = refValue

	if vmin == vmax {
		tmpl.BitsPerValue = 0
		return nil, tmpl, nil
	}

	bpv := int(bitsPerValue)
	if bpv == 0 {
		return nil, tmpl, fmt.Errorf("grib2: CCSDS packing: bitsPerValue 0 cannot represent a non-constant field (min %g, max %g)", vmin, vmax)
	}

	maxPackedVal := float64(uint64(1)<<bpv - 1)
	binaryScale := ccsdsBinaryScale(vmax-ref, maxPackedVal)
	tmpl.BinaryScaleFactor = binaryScale

	if err := ccsdsCheckConfig(blockSize, rsi); err != nil {
		return nil, tmpl, err
	}

	// Quantise. v-ref lies in [0, vmax-ref] and ccsdsBinaryScale guarantees
	// round((vmax-ref)*2^-E) <= maxPackedVal, so no clamping is needed. The
	// sample is passed as its uint32 bit pattern, which matters at 32 bits.
	invScale := math.Ldexp(1, -int(binaryScale))
	samples := make([]int32, n)
	for i, v := range values {
		x := math.Round((v - ref) * invScale)
		samples[i] = int32(uint32(x))
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

// ccsdsMinMax returns the minimum and maximum, rejecting values that cannot be
// packed: NaN, ±Inf and magnitudes beyond the float32 range.
func ccsdsMinMax(values []float64) (vmin, vmax float64, err error) {
	vmin, vmax = math.Inf(1), math.Inf(-1)
	for i, v := range values {
		if math.IsNaN(v) {
			return 0, 0, fmt.Errorf("grib2: CCSDS packing: value %d is NaN", i)
		}
		if math.Abs(v) > math.MaxFloat32 {
			return 0, 0, fmt.Errorf("grib2: CCSDS packing: value %d = %g is outside the float32 range", i, v)
		}
		if v < vmin {
			vmin = v
		}
		if v > vmax {
			vmax = v
		}
	}
	return vmin, vmax, nil
}

// float32AtOrBelow returns the largest float32 that is <= v (v must be within
// the float32 range). float32(v) rounds to nearest, which is above v about
// half of the time; a reference value above the minimum would make the
// smallest samples negative.
func float32AtOrBelow(v float64) float32 {
	f := float32(v)
	if float64(f) > v {
		f = math.Nextafter32(f, float32(math.Inf(-1)))
	}
	return f
}

// ccsdsBinaryScale returns the smallest E such that round(rng*2^-E) fits in
// maxPackedVal (rng > 0).
func ccsdsBinaryScale(rng, maxPackedVal float64) int16 {
	// Log2 of each operand separately: rng/maxPackedVal can underflow to 0.
	e := int(math.Ceil(math.Log2(rng) - math.Log2(maxPackedVal)))
	fits := func(e int) bool { return math.Round(math.Ldexp(rng, -e)) <= maxPackedVal }
	for !fits(e) {
		e++
	}
	for fits(e - 1) {
		e--
	}
	return int16(e)
}

// ccsdsCheckConfig applies libaec's encoder limits up front so the error
// names the GRIB parameters.
func ccsdsCheckConfig(blockSize uint8, rsi uint16) error {
	switch blockSize {
	case 8, 16, 32, 64:
	default:
		return fmt.Errorf("grib2: CCSDS packing: block size %d must be 8, 16, 32 or 64", blockSize)
	}
	if rsi == 0 || rsi > 4096 {
		return fmt.Errorf("grib2: CCSDS packing: RSI %d must be 1..4096", rsi)
	}
	return nil
}

// UnpackCCSDS decodes data packed with Template 5.42 (CCSDS/AEC).
//
// The packed data in Section 7 is a CCSDS-121.0-B compressed bitstream.
// Each decoded sample is an unsigned integer. The same scaling formula as
// simple packing applies: Y = (X * 2^E + R) * 10^(-D)
//
// The conventions follow eccodes (DataCcsdsPacking.cc):
//   - BitsPerValue 0 is a constant field and every value is R. The decimal
//     scale factor is not applied (eccodes and g2clib agree on this).
//   - BitsPerValue above 32 is an error.
//   - With the SIGNED flag, a sample is read back as the unsigned integer
//     eccodes reads from libaec's output buffer: (bpv+7)/8 bytes (3 rounds up
//     to 4) with the preprocessor, the raw bpv-bit pattern without it.
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
	if bpv > ccsdsMaxBitsPerValue {
		return nil, fmt.Errorf("grib2: CCSDS packing: invalid number of bits per value %d (max %d)", bpv, ccsdsMaxBitsPerValue)
	}

	// Constant field: every value is the reference value.
	if bpv == 0 {
		ref := float64(tmpl.ReferenceValue)
		values := make([]float64, numValues)
		for i := range values {
			values[i] = ref
		}
		return values, nil
	}

	// Apply the GRIB2 scaling formula: Y = (X * 2^E + R) * 10^(-D)
	binaryS := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	decimalS := math.Pow(10, float64(-tmpl.DecimalScaleFactor))
	ref := float64(tmpl.ReferenceValue)

	// An empty payload with bpv > 0 decodes as all-zero samples, as eccodes'
	// zero-initialised output buffer does.
	if len(data) == 0 {
		values := make([]float64, numValues)
		for i := range values {
			values[i] = ref * decimalS
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

	// Unsigned samples are already in [0, 2^bpv-1]. Signed ones come back
	// sign-extended; reproduce eccodes' unsigned read of libaec's output.
	mask := ^uint32(0)
	if flags&aec.DataSigned != 0 {
		width := bpv
		if flags&aec.DataPreprocess != 0 {
			nbytes := (bpv + 7) / 8
			if nbytes == 3 {
				nbytes = 4
			}
			width = 8 * nbytes
		}
		if width < 32 {
			mask = uint32(1)<<width - 1
		}
	}

	values := make([]float64, numValues)
	for i := uint32(0); i < numValues; i++ {
		x := float64(uint32(decoded[i]) & mask)
		values[i] = (x*binaryS + ref) * decimalS
	}

	return values, nil
}
