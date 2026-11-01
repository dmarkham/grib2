package grib2

import (
	"fmt"
	"math"
	"testing"
)

// mustError runs f and fails the test if it panics or returns no error.
func mustError(t *testing.T, name string, f func() error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: panic: %v", name, r)
		}
	}()
	if err := f(); err == nil {
		t.Errorf("%s: expected an error", name)
	}
}

// TestUnpackComplex_MalformedTemplate53: a zero (or oversized) number of
// extra-descriptor octets and unsupported differencing orders are errors,
// never panics or silently undifferenced data (eccodes
// tests/grib_complex_spatial_differencing.sh: orders 3 and 4 fail with
// "Unsupported orderOfSpatialDifferencing"; DataG22OrderPacking.cc).
func TestUnpackComplex_MalformedTemplate53(t *testing.T) {
	vals := make([]float64, 300)
	for i := range vals {
		vals[i] = 250 + 20*math.Sin(float64(i)/9)
	}
	data1, t1, err := PackComplexSpatialDiff(vals, 12, 1)
	if err != nil {
		t.Fatal(err)
	}
	data2, t2, err := PackComplexSpatialDiff(vals, 12, 2)
	if err != nil {
		t.Fatal(err)
	}
	gfs := decodeAll(t, "testdata/gfs.complex.mvmu.grib2")[0].Fields[0]
	g53 := gfs.Section5.Template.(Template53)
	cases := []struct {
		name string
		data []byte
		tmpl Template53
		n    uint32
	}{
		{"order1", data1, t1, 300},
		{"order2", data2, t2, 300},
		{"gfs", gfs.Section7.Data, g53, gfs.Section5.NumberOfValues},
	}
	for _, c := range cases {
		for _, octets := range []uint8{0, 8, 9} {
			mustError(t, fmt.Sprintf("%s octets=%d", c.name, octets), func() error {
				_, err := UnpackComplex(c.data, c.tmpl.Template52, c.n, c.tmpl.OrderOfSpatialDifferencing, octets)
				return err
			})
		}
		for _, order := range []uint8{3, 4, 7, 255} {
			mustError(t, fmt.Sprintf("%s order=%d", c.name, order), func() error {
				_, err := UnpackComplex(c.data, c.tmpl.Template52, c.n, order, c.tmpl.NumberOfOctetsExtraDescriptors)
				return err
			})
		}
	}
	// Through Values(), as eccodes' grib_set -s orderOfSpatialDifferencing=3 would produce.
	f := gfs
	g := g53
	g.OrderOfSpatialDifferencing = 3
	f.Section5.Template = g
	mustError(t, "Values order 3", func() error { _, err := f.Values(); return err })
}

// TestUnpackComplex_BitWidthLimits: group widths and reference widths of 64
// bits or more are errors (eccodes tests/grib_bpv_limit.cc).
func TestUnpackComplex_BitWidthLimits(t *testing.T) {
	base := Template52{
		Template50:                        Template50{BitsPerValue: 8},
		NumberOfGroups:                    1,
		LengthIncrementForGroupLengths:    1,
		TrueLengthOfLastGroup:             4,
		NumberOfBitsForGroupWidths:        0,
		ReferenceForGroupWidths:           200,
		NumberOfBitsForScaledGroupLengths: 0,
	}
	mustError(t, "group width 200", func() error {
		_, err := UnpackComplex(make([]byte, 200), base, 4, 0, 0)
		return err
	})
	b := base
	b.ReferenceForGroupWidths = 4
	b.BitsPerValue = 64
	mustError(t, "bpv 64", func() error {
		_, err := UnpackComplex(make([]byte, 200), b, 4, 0, 0)
		return err
	})
	b.BitsPerValue = 8
	b.MissingValueManagement = 3
	mustError(t, "missing value management 3", func() error {
		_, err := UnpackComplex(make([]byte, 200), b, 4, 0, 0)
		return err
	})
}

// TestUnpack_TruncatedSection7: packed data shorter than the template
// promises is an error (eccodes tests/grib_codedValues_as_bytes.sh).
func TestUnpack_TruncatedSection7(t *testing.T) {
	// simple_16bit.grib2: bpv 16, 496 values, empty Section 7.
	f := decodeAll(t, "testdata/simple_16bit.grib2")[0].Fields[0]
	mustError(t, "simple_16bit", func() error { _, err := f.Values(); return err })

	// Complex: one group of width 8 holding 4 values over 2 bytes.
	tmpl := Template52{
		Template50:                     Template50{BitsPerValue: 8},
		NumberOfGroups:                 1,
		ReferenceForGroupWidths:        8,
		LengthIncrementForGroupLengths: 1,
		TrueLengthOfLastGroup:          4,
	}
	mustError(t, "complex short", func() error {
		_, err := UnpackComplex([]byte{0, 1, 2}, tmpl, 4, 0, 0)
		return err
	})
	// Complex: groups that hold fewer values than declared.
	mustError(t, "complex group lengths short", func() error {
		_, err := UnpackComplex(make([]byte, 16), tmpl, 9, 0, 0)
		return err
	})

	// CCSDS with half its data.
	c := decodeAll(t, "testdata/ccsds.grib2")[0].Fields[0]
	ct := c.Section5.Template.(Template542)
	mustError(t, "ccsds half", func() error {
		_, err := UnpackCCSDS(c.Section7.Data[:len(c.Section7.Data)/2], ct, c.Section5.NumberOfValues)
		return err
	})

	// PNG with a stream cut short, and with too few pixels.
	p := decodeAll(t, "testdata/upstream/png_bpv16.grib2")[0].Fields[0]
	pt := p.Section5.Template.(Template50)
	mustError(t, "png cut", func() error {
		_, err := UnpackPNG(p.Section7.Data[:len(p.Section7.Data)/2], pt, p.Section5.NumberOfValues)
		return err
	})
	mustError(t, "png too few pixels", func() error {
		_, err := UnpackPNG(p.Section7.Data, pt, p.Section5.NumberOfValues+1)
		return err
	})
	mustError(t, "png bpv>0 empty data", func() error {
		_, err := UnpackPNG(nil, pt, 10)
		return err
	})
}

// TestUnpackRunLength_Malformed: bpv 0 and a short level-value list are
// errors, not a divide by zero or an index out of range (eccodes
// DataRunLengthPacking.cc).
func TestUnpackRunLength_Malformed(t *testing.T) {
	mustError(t, "bpv 0", func() error {
		_, err := UnpackRunLength([]byte{1, 2, 3}, Template5200{BitsPerValue: 0, MaxLevelValue: 3, NumberOfLevelValues: 3, LevelValues: []uint16{1, 2, 3}}, 3, math.NaN())
		return err
	})
	mustError(t, "short level values", func() error {
		_, err := UnpackRunLength([]byte{1, 2, 3}, Template5200{BitsPerValue: 8, MaxLevelValue: 3, NumberOfLevelValues: 3, LevelValues: []uint16{1}}, 3, math.NaN())
		return err
	})
}

// TestUnpackComplex_MissingValueManagement2 exercises primary and secondary
// missing values at several group widths (eccodes unit_tests.cc
// test_grib_is_all_bits_one, DataG22OrderPacking.cc).
func TestUnpackComplex_MissingValueManagement2(t *testing.T) {
	for _, w := range []int{1, 7, 13, 31} {
		max := uint64(1)<<w - 1
		// One group of width w with values 0, 2^w-1 (primary), 2^w-2 (secondary), 1.
		raw := []uint64{0, max, max - 1, 1}
		if w == 1 {
			raw = []uint64{0, max, max, 0} // with one bit, 2^w-2 is 0
		}
		data := make([]byte, 2+(4*w+7)/8)
		// Group reference (8 bits) = 10, then values.
		storeBits(data, 0, 8, 10)
		off := uint64(8)
		for _, x := range raw {
			storeBits(data, off, w, x)
			off += uint64(w)
		}
		for _, mvm := range []uint8{1, 2} {
			tmpl := Template52{
				Template50:                      Template50{ReferenceValue: 100, BitsPerValue: 8},
				MissingValueManagement:          mvm,
				NumberOfGroups:                  1,
				ReferenceForGroupWidths:         uint8(w),
				LengthIncrementForGroupLengths:  1,
				TrueLengthOfLastGroup:           4,
				PrimaryMissingValueSubstitute:   math.Float32bits(-1),
				SecondaryMissingValueSubstitute: math.Float32bits(-2),
			}
			got, err := UnpackComplex(data, tmpl, 4, 0, 0)
			if err != nil {
				t.Fatalf("w=%d mvm=%d: %v", w, mvm, err)
			}
			// With one bit and management 2, 0 is the secondary missing value.
			if w == 1 && mvm == 2 {
				if !math.IsNaN(got[0]) || !math.IsNaN(got[1]) {
					t.Errorf("w=1 mvm=2: %v", got)
				}
				continue
			}
			if got[0] != 110 || !math.IsNaN(got[1]) {
				t.Errorf("w=%d mvm=%d: %v", w, mvm, got)
			}
			if w > 1 {
				if mvm == 2 && !math.IsNaN(got[2]) {
					t.Errorf("w=%d mvm=2: secondary missing decoded as %v", w, got[2])
				}
				if mvm == 1 && got[2] != 110+float64(max-1) {
					t.Errorf("w=%d mvm=1: %v", w, got[2])
				}
			}
			sub, err := UnpackComplexWithSubstitution(data, tmpl, 4, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if sub[1] != -1 || (mvm == 2 && w > 1 && sub[2] != -2) {
				t.Errorf("w=%d mvm=%d substitution: %v", w, mvm, sub)
			}
		}
	}
	// Width-0 groups whose reference is all ones (primary) or all ones minus one (secondary).
	for _, ref := range []uint64{255, 254} {
		data := []byte{byte(ref)}
		tmpl := Template52{
			Template50:                     Template50{BitsPerValue: 8},
			MissingValueManagement:         2,
			NumberOfGroups:                 1,
			LengthIncrementForGroupLengths: 1,
			TrueLengthOfLastGroup:          3,
		}
		got, err := UnpackComplex(data, tmpl, 3, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range got {
			if !math.IsNaN(v) {
				t.Errorf("width-0 ref %d: %v", ref, got)
			}
		}
	}
}

// TestReadSignedBits pins the sign-magnitude reader used for the overall
// minimum of spatial differencing.
func TestReadSignedBits(t *testing.T) {
	for _, w := range []int{8, 16, 24, 32} {
		buf := make([]byte, 8)
		storeBits(buf, 0, w, uint64(1)<<(w-1)|5)
		if got := readSignedBits(buf, 0, w); got != -5 {
			t.Errorf("width %d: -5 read as %d", w, got)
		}
		buf = make([]byte, 8)
		storeBits(buf, 0, w, 7)
		if got := readSignedBits(buf, 0, w); got != 7 {
			t.Errorf("width %d: 7 read as %d", w, got)
		}
	}
}
