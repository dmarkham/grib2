package grib2

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// encoderCase packs values with one encoder and decodes them back.
type encoderCase struct {
	name   string
	maxBPV int
	pack   func(v []float64, bpv uint8) (Template50, func() ([]float64, error), error)
}

func encoders() []encoderCase {
	return []encoderCase{
		{"simple", 63, func(v []float64, bpv uint8) (Template50, func() ([]float64, error), error) {
			data, tmpl, err := PackSimple(v, bpv)
			return tmpl, func() ([]float64, error) { return UnpackSimple(data, tmpl, uint32(len(v))) }, err
		}},
		{"png", 16, func(v []float64, bpv uint8) (Template50, func() ([]float64, error), error) {
			data, tmpl, err := PackPNG(v, bpv, len(v), 1)
			return tmpl, func() ([]float64, error) { return UnpackPNG(data, tmpl, uint32(len(v))) }, err
		}},
		{"complex", 23, func(v []float64, bpv uint8) (Template50, func() ([]float64, error), error) {
			data, tmpl, err := PackComplex(v, bpv)
			return tmpl.Template50, func() ([]float64, error) { return UnpackComplex(data, tmpl, uint32(len(v)), 0, 0) }, err
		}},
		{"spatial1", 23, func(v []float64, bpv uint8) (Template50, func() ([]float64, error), error) {
			data, tmpl, err := PackComplexSpatialDiff(v, bpv, 1)
			return tmpl.Template50, func() ([]float64, error) {
				return UnpackComplex(data, tmpl.Template52, uint32(len(v)), 1, tmpl.NumberOfOctetsExtraDescriptors)
			}, err
		}},
		{"spatial2", 23, func(v []float64, bpv uint8) (Template50, func() ([]float64, error), error) {
			data, tmpl, err := PackComplexSpatialDiff(v, bpv, 2)
			return tmpl.Template50, func() ([]float64, error) {
				return UnpackComplex(data, tmpl.Template52, uint32(len(v)), 2, tmpl.NumberOfOctetsExtraDescriptors)
			}, err
		}},
	}
}

// awkwardFields are inputs whose minimum is not a float32, so a reference
// of float32(min) would lie above the minimum (eccodes unit_tests.cc
// test_grib_nearest_smaller_ieeefloat, tests/grib_ecc-1431.cc).
func awkwardFields() map[string][]float64 {
	ramp := func(start, step float64, n int) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = start + float64(i)*step
		}
		return v
	}
	return map[string][]float64{
		"audit":      {279, 279.96, 278.5, 300.25, 270.1, 285.3},
		"ramp100.3":  ramp(100.3, 0.001, 200),
		"tiny-range": {280.00002, 280.0004, 280.0011},
		"0.1":        ramp(0.1, 0.37, 50),
		"1.1":        ramp(1.1, 0.013, 50),
		"7.85":       ramp(7.85, 1.5, 50),
		"-0.3":       ramp(-0.3, 0.07, 50),
		"large":      ramp(7000000300, 13, 40),
		"negative":   ramp(-273.15, 3.3, 100),
	}
}

// TestEncoders_ReferenceNotAboveMinimum sweeps bits per value for every
// encoder: the reference value must not exceed the minimum and the error
// must stay within half a quantisation step (eccodes tests/grib_bitsPerValue.sh,
// grib_ecc-1431.cc).
func TestEncoders_ReferenceNotAboveMinimum(t *testing.T) {
	for _, enc := range encoders() {
		for name, vals := range awkwardFields() {
			vmin := vals[0]
			for _, v := range vals {
				vmin = math.Min(vmin, v)
			}
			for bpv := 1; bpv <= enc.maxBPV; bpv++ {
				tmpl, unpack, err := enc.pack(vals, uint8(bpv))
				if err != nil {
					t.Fatalf("%s/%s bpv %d: %v", enc.name, name, bpv, err)
				}
				if float64(tmpl.ReferenceValue) > vmin {
					t.Fatalf("%s/%s bpv %d: reference %v above minimum %v", enc.name, name, bpv, tmpl.ReferenceValue, vmin)
				}
				got, err := unpack()
				if err != nil {
					t.Fatalf("%s/%s bpv %d: decode: %v", enc.name, name, bpv, err)
				}
				step := math.Ldexp(1, int(tmpl.BinaryScaleFactor))
				tol := 0.5*step*(1+1e-9) + 1e-9*math.Abs(vmin)
				for i := range vals {
					if d := math.Abs(got[i] - vals[i]); d > tol {
						t.Fatalf("%s/%s bpv %d: value %d: |%v - %v| = %v > %v (E=%d)",
							enc.name, name, bpv, i, got[i], vals[i], d, tol, tmpl.BinaryScaleFactor)
					}
				}
			}
		}
	}
}

// TestEncoders_SingleValueAndTinyGrids: n = 1, 2, 3, 7 and the ECC-802
// values (eccodes tests/grib_jpeg.sh) pack and decode.
func TestEncoders_SingleValueAndTinyGrids(t *testing.T) {
	inputs := [][]float64{{7000000300}, {3.5}, {2.1, 3.4}, {2.1, 3.4, 8.9}, {-0.01, 98.99, 5, 7}, {1, 2, 3, 4, 5, 6, 7.5}}
	for _, enc := range encoders() {
		for _, vals := range inputs {
			tmpl, unpack, err := enc.pack(vals, 12)
			if err != nil {
				t.Fatalf("%s %v: %v", enc.name, vals, err)
			}
			got, err := unpack()
			if err != nil {
				t.Fatalf("%s %v: decode: %v", enc.name, vals, err)
			}
			step := math.Ldexp(1, int(tmpl.BinaryScaleFactor))
			for i := range vals {
				if math.Abs(got[i]-vals[i]) > 0.5*step+1e-6*math.Abs(vals[i]) {
					t.Errorf("%s %v: got %v", enc.name, vals, got)
					break
				}
			}
		}
	}
}

// TestEncoders_RejectInvalidInput: infinities, values beyond float32 range,
// NaN where it cannot be represented, and 0 bits per value on varying data
// are errors (eccodes unit_tests.cc: INFINITY gives GRIB_OUT_OF_RANGE, bpv 0
// gives GRIB_ENCODING_ERROR).
func TestEncoders_RejectInvalidInput(t *testing.T) {
	bad := map[string][]float64{
		"+Inf":  {1, 2, math.Inf(1), 4},
		"-Inf":  {math.Inf(-1), 2},
		"1e39":  {1, 1e39},
		"-1e39": {-1e39, 1},
	}
	for _, enc := range encoders() {
		for name, vals := range bad {
			if _, _, err := enc.pack(vals, 12); err == nil {
				t.Errorf("%s: %s accepted", enc.name, name)
			}
		}
		if _, _, err := enc.pack([]float64{1, 2, 3}, 0); err == nil {
			t.Errorf("%s: bpv 0 on varying data accepted", enc.name)
		}
		if _, _, err := enc.pack([]float64{1, 2, 3}, uint8(enc.maxBPV+1)); err == nil && enc.maxBPV < 23 {
			t.Errorf("%s: bpv %d accepted", enc.name, enc.maxBPV+1)
		}
	}
	for _, enc := range encoders()[:2] { // simple and PNG cannot represent NaN
		for _, vals := range [][]float64{{math.NaN(), 1, 2}, {1, math.NaN(), 2}} {
			if _, _, err := enc.pack(vals, 12); err == nil {
				t.Errorf("%s: NaN accepted", enc.name)
			}
		}
	}
	if _, _, err := PackSimple([]float64{1, 2}, 64); err == nil {
		t.Error("PackSimple bpv 64 accepted")
	}
	if _, _, err := PackPNG([]float64{1, 2, 3}, 8, 1, 1); err == nil {
		t.Error("PackPNG with an image too small accepted")
	}
	for bpv := 17; bpv <= 32; bpv++ {
		if _, _, err := PackPNG([]float64{1, 2, 3}, uint8(bpv), 3, 1); err == nil || !strings.Contains(err.Error(), "bits per value") {
			t.Errorf("PackPNG bpv %d: err = %v", bpv, err)
		}
	}
}

// TestEncoders_ConstantCollapses: constant input is packed with 0 bits per
// value and no data, decoding to exactly the value (eccodes ECC-1387, ECC-317).
func TestEncoders_ConstantCollapses(t *testing.T) {
	vals := []float64{273.15, 273.15, 273.15, 273.15}
	for _, enc := range encoders() {
		tmpl, unpack, err := enc.pack(vals, 16)
		if err != nil {
			t.Fatalf("%s: %v", enc.name, err)
		}
		got, err := unpack()
		if err != nil {
			t.Fatalf("%s: %v", enc.name, err)
		}
		for i := range got {
			if got[i] != float64(float32AtOrBelow(273.15)) && math.Abs(got[i]-273.15) > 1e-5 {
				t.Fatalf("%s: got %v", enc.name, got)
			}
		}
		if enc.name == "simple" || enc.name == "png" {
			if tmpl.BitsPerValue != 0 {
				t.Errorf("%s: constant packed with %d bits", enc.name, tmpl.BitsPerValue)
			}
		}
	}
}

// TestPackComplex_MissingValues: NaN round-trips as NaN through complex
// packing, at the start, the end, every 100th point and everywhere
// (eccodes tests/grib_complex_spatial_differencing.sh, grib_2nd_order_numValues.cc).
func TestPackComplex_MissingValues(t *testing.T) {
	n := 1000
	patterns := map[string]func(i int) bool{
		"all":       func(int) bool { return true },
		"first":     func(i int) bool { return i == 0 },
		"first two": func(i int) bool { return i < 2 },
		"last":      func(i int) bool { return i == n-1 },
		"every100":  func(i int) bool { return i%100 == 0 },
	}
	for _, enc := range encoders()[2:] {
		for name, missing := range patterns {
			vals := make([]float64, n)
			for i := range vals {
				vals[i] = 250 + 30*math.Sin(float64(i)/37)
				if missing(i) {
					vals[i] = math.NaN()
				}
			}
			_, unpack, err := enc.pack(vals, 16)
			if err != nil {
				t.Fatalf("%s/%s: %v", enc.name, name, err)
			}
			got, err := unpack()
			if err != nil {
				t.Fatalf("%s/%s: decode: %v", enc.name, name, err)
			}
			for i := range vals {
				if math.IsNaN(vals[i]) != math.IsNaN(got[i]) {
					t.Fatalf("%s/%s: point %d: got %v, want %v", enc.name, name, i, got[i], vals[i])
				}
				if !math.IsNaN(vals[i]) && math.Abs(got[i]-vals[i]) > 1e-3 {
					t.Fatalf("%s/%s: point %d: got %v, want %v", enc.name, name, i, got[i], vals[i])
				}
			}
		}
	}
}

// TestPackSimple_BitsPerValueLimit mirrors eccodes tests/grib_bpv_limit.cc:
// every bits per value below 64 round-trips regular_latlon_surface, 64 and
// above are errors for both packing and unpacking.
func TestPackSimple_BitsPerValueLimit(t *testing.T) {
	f := decodeAll(t, "testdata/regular_latlon_surface.grib2")[0].Fields[0]
	vals, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	for bpv := 1; bpv < 256; bpv++ {
		data, tmpl, err := PackSimple(vals, uint8(bpv))
		if bpv >= 64 {
			if err == nil {
				t.Errorf("PackSimple bpv %d: expected error", bpv)
			}
			if _, err := UnpackSimple(make([]byte, 8*len(vals)), Template50{BitsPerValue: uint8(bpv)}, uint32(len(vals))); err == nil {
				t.Errorf("UnpackSimple bpv %d: expected error", bpv)
			}
			continue
		}
		if err != nil {
			t.Fatalf("bpv %d: %v", bpv, err)
		}
		got, err := UnpackSimple(data, tmpl, uint32(len(vals)))
		if err != nil {
			t.Fatal(err)
		}
		step := math.Ldexp(1, int(tmpl.BinaryScaleFactor))
		for _, i := range []int{0, 1} {
			if math.Abs(got[i]-vals[i]) > 0.5*step*(1+1e-9)+1e-12 {
				t.Errorf("bpv %d: values[%d] = %v, want %v", bpv, i, got[i], vals[i])
			}
		}
	}
	if vals[0] != 279 || vals[1] != 279.9609375 {
		t.Errorf("fixture values %v %v, upstream says 279 and 279.9609375", vals[0], vals[1])
	}
}

// TestExtractStoreBits_AllWidths round-trips every width 1..64 at every bit
// offset 0..7 (eccodes tests/bits.cc).
func TestExtractStoreBits_AllWidths(t *testing.T) {
	for w := 1; w <= 64; w++ {
		for off := 0; off < 8; off++ {
			buf := make([]byte, 20)
			v := uint64(0xA5C3_96F0_1E2D_4B87)
			if w < 64 {
				v &= 1<<w - 1
			}
			storeBits(buf, uint64(off), w, v)
			if got := extractBits(buf, uint64(off), w); got != v {
				t.Fatalf("width %d offset %d: got %#x, want %#x", w, off, got, v)
			}
		}
	}
}

// TestEccodes_DecodesOurEncoders writes regular_latlon_surface.grib2 back
// with each of our encoders and checks eccodes decodes the same values and
// counts (eccodes tests/grib_packing_order.sh, grib_2nd_order_numValues.cc).
func TestEccodes_DecodesOurEncoders(t *testing.T) {
	if !eccodesAvailable(t) {
		t.Skip("eccodes not available")
	}
	gcsd := decodeAll(t, "testdata/grid_complex_spatial_differencing.grib2")[0]
	rll := decodeAll(t, "testdata/regular_latlon_surface.grib2")[0]
	type variant struct {
		name string
		msg  *Message
		set  func(f *Field, vals []float64) error
	}
	withNaN := func(m *Message) *Message { return m }
	variants := []variant{}
	for _, bpv := range []uint8{7, 16, 24} {
		bpv := bpv
		variants = append(variants, variant{fmt.Sprintf("simple%d", bpv), rll, func(f *Field, v []float64) error {
			data, tmpl, err := PackSimple(v, bpv)
			f.Section5 = Section5{TemplateNumber: 0, NumberOfValues: uint32(len(v)), Template: tmpl}
			f.Section7.Data = data
			return err
		}})
	}
	for _, m := range []*Message{rll, withNaN(gcsd)} {
		m := m
		variants = append(variants,
			variant{"complex", m, func(f *Field, v []float64) error {
				data, tmpl, err := PackComplex(v, 16)
				f.Section5 = Section5{TemplateNumber: 2, NumberOfValues: uint32(len(v)), Template: tmpl}
				f.Section7.Data = data
				return err
			}},
			variant{"spatial1", m, func(f *Field, v []float64) error {
				data, tmpl, err := PackComplexSpatialDiff(v, 16, 1)
				f.Section5 = Section5{TemplateNumber: 3, NumberOfValues: uint32(len(v)), Template: tmpl}
				f.Section7.Data = data
				return err
			}},
			variant{"spatial2", m, func(f *Field, v []float64) error {
				data, tmpl, err := PackComplexSpatialDiff(v, 16, 2)
				f.Section5 = Section5{TemplateNumber: 3, NumberOfValues: uint32(len(v)), Template: tmpl}
				f.Section7.Data = data
				return err
			}},
		)
	}
	variants = append(variants,
		variant{"ieee32", rll, func(f *Field, v []float64) error {
			data, tmpl, err := PackIEEE(v, 1)
			f.Section5 = Section5{TemplateNumber: 4, NumberOfValues: uint32(len(v)), Template: tmpl}
			f.Section7.Data = data
			return err
		}},
		variant{"ieee64", rll, func(f *Field, v []float64) error {
			data, tmpl, err := PackIEEE(v, 2)
			f.Section5 = Section5{TemplateNumber: 4, NumberOfValues: uint32(len(v)), Template: tmpl}
			f.Section7.Data = data
			return err
		}},
		variant{"ccsds16", rll, func(f *Field, v []float64) error {
			data, tmpl, err := PackCCSDS(v, 16, 32, 128)
			f.Section5 = Section5{TemplateNumber: 42, NumberOfValues: uint32(len(v)), Template: tmpl}
			f.Section7.Data = data
			return err
		}},
		variant{"ccsds24", withNaN(gcsd), func(f *Field, v []float64) error {
			data, tmpl, err := PackCCSDS(v, 24, 32, 128)
			f.Section5 = Section5{TemplateNumber: 42, NumberOfValues: uint32(len(v)), Template: tmpl}
			f.Section7.Data = data
			return err
		}},
		variant{"png", rll, func(f *Field, v []float64) error {
			tmpl30 := f.Section3.Template.(Template30)
			data, tmpl, err := PackPNG(v, 16, int(tmpl30.Ni), int(tmpl30.Nj))
			f.Section5 = Section5{TemplateNumber: 41, NumberOfValues: uint32(len(v)), Template: tmpl}
			f.Section7.Data = data
			return err
		}},
	)
	for i, vt := range variants {
		t.Run(fmt.Sprintf("%d-%s", i, vt.name), func(t *testing.T) {
			src := vt.msg
			f := src.Fields[0]
			full, err := f.Values()
			if err != nil {
				t.Fatal(err)
			}
			// Values packed in Section 7 are the bitmap-present ones, in data
			// order; this fixture has scanning mode 0 so canonical == data order.
			var packed []float64
			for _, v := range full {
				if !math.IsNaN(v) {
					packed = append(packed, v)
				}
			}
			if vt.name == "complex" || strings.HasPrefix(vt.name, "spatial") {
				if f.Section6.HasBitmap() {
					// Exercise complex missing-value management instead of the bitmap.
					packed = full
					f.Section6 = Section6{Indicator: 255}
				}
			}
			if err := vt.set(&f, packed); err != nil {
				t.Fatal(err)
			}
			msg := *src
			msg.Fields = []Field{f}
			var buf bytes.Buffer
			if err := WriteMessage(&buf, &msg); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "out.grib2")
			if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			ours, err := decodeAll(t, path)[0].Fields[0].Values()
			if err != nil {
				t.Fatal(err)
			}
			_, _, eVals := eccodesPoints(t, path, 1)
			if len(eVals) != len(ours) {
				t.Fatalf("eccodes %d values, ours %d", len(eVals), len(ours))
			}
			for k := range ours {
				if math.IsNaN(ours[k]) != math.IsNaN(eVals[k]) || (!math.IsNaN(ours[k]) && math.Abs(ours[k]-eVals[k]) > 1e-9*math.Max(1, math.Abs(ours[k]))) {
					t.Fatalf("point %d: ours %v, eccodes %v", k, ours[k], eVals[k])
				}
			}
			nanCount := 0
			for _, v := range ours {
				if math.IsNaN(v) {
					nanCount++
				}
			}
			if miss := eccodesInts(t, path, "numberOfMissing"); miss[0] != int64(nanCount) {
				t.Errorf("eccodes numberOfMissing %d, ours %d", miss[0], nanCount)
			}
			if strings.HasPrefix(vt.name, "ieee") {
				width := 4
				if vt.name == "ieee64" {
					width = 8
				}
				if got := eccodesInts(t, path, "section7Length")[0]; got != int64(5+width*len(packed)) {
					t.Errorf("section7Length %d, want %d", got, 5+width*len(packed))
				}
			}
			_ = strconv.Itoa
		})
	}
}

// TestPackSimple_SampleStatsAcrossBitsPerValue mirrors eccodes
// tests/grib_bitsPerValue.sh: re-packing sample.grib2 at 17..58 bits keeps
// min, max and mean to three decimals.
func TestPackSimple_SampleStatsAcrossBitsPerValue(t *testing.T) {
	vals, err := decodeAll(t, "testdata/sample.grib2")[0].Fields[0].Values()
	if err != nil {
		t.Fatal(err)
	}
	_, mn0, mx0, mean0 := fieldStats(vals)
	for bpv := 17; bpv <= 58; bpv++ {
		data, tm, err := PackSimple(vals, uint8(bpv))
		if err != nil {
			t.Fatal(err)
		}
		got, err := UnpackSimple(data, tm, uint32(len(vals)))
		if err != nil {
			t.Fatal(err)
		}
		_, mn, mx, mean := fieldStats(got)
		r := func(x float64) string { return strconv.FormatFloat(x, 'f', 3, 64) }
		if r(mn) != r(mn0) || r(mx) != r(mx0) || r(mean) != r(mean0) {
			t.Errorf("bpv %d: %s %s %s, want %s %s %s", bpv, r(mn), r(mx), r(mean), r(mn0), r(mx0), r(mean0))
		}
	}
}
