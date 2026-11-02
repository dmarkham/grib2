package grib2

import (
	"fmt"
	"math"
)

// simpleScale holds the scaling an encoder chose for Y = R + X*2^E (D = 0).
type simpleScale struct {
	ref      float32 // R: the largest float32 not above the minimum
	e        int16   // E: the smallest binary scale that fits the range
	constant bool    // all (non-missing) values equal R
	maxX     float64 // largest packed integer, 2^bpv - 1
	allNaN   bool    // every value is missing (only when NaN is allowed)
}

// chooseSimpleScale validates values for an encoder and picks R and E.
// Values must be finite and within float32 range (eccodes unit_tests.cc
// returns GRIB_OUT_OF_RANGE for infinity); NaN is accepted only when
// allowNaN (complex packing encodes it as missing). bpv must be at most
// maxBPV, and 0 only for constant input (eccodes GRIB_ENCODING_ERROR).
func chooseSimpleScale(name string, values []float64, bpv, maxBPV int, allowNaN bool) (simpleScale, error) {
	if bpv > maxBPV {
		return simpleScale{}, fmt.Errorf("grib2: %s: invalid bits per value %d (maximum %d)", name, bpv, maxBPV)
	}
	vmin, vmax := math.Inf(1), math.Inf(-1)
	for i, v := range values {
		switch {
		case math.IsNaN(v):
			if !allowNaN {
				return simpleScale{}, fmt.Errorf("grib2: %s: value %d is NaN", name, i)
			}
			continue
		case math.IsInf(v, 0) || math.Abs(v) > math.MaxFloat32:
			return simpleScale{}, fmt.Errorf("grib2: %s: value %d (%v) is out of range", name, i, v)
		}
		vmin = math.Min(vmin, v)
		vmax = math.Max(vmax, v)
	}
	if vmin > vmax {
		return simpleScale{allNaN: true, constant: true}, nil
	}
	s := simpleScale{ref: float32AtOrBelow(vmin)}
	rng := vmax - float64(s.ref)
	if vmax == vmin {
		s.constant = true
		return s, nil
	}
	if bpv == 0 {
		return simpleScale{}, fmt.Errorf("grib2: %s: 0 bits per value on data that is not constant", name)
	}
	s.maxX = math.Ldexp(1, bpv) - 1
	s.e = ccsdsBinaryScale(rng, s.maxX)
	return s, nil
}

// packed returns the integer X for v, clamped to [0, 2^bpv-1].
func (s simpleScale) packed(v float64) uint64 {
	x := math.Round(math.Ldexp(v-float64(s.ref), -int(s.e)))
	return uint64(math.Max(0, math.Min(x, s.maxX)))
}
