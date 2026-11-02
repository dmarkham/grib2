package grib2

import (
	"bytes"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestUnpackCCSDS(t *testing.T) {
	raw, err := os.ReadFile("testdata/ccsds.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template542)
	if !ok {
		t.Fatalf("not Template542: %T", field.Section5.Template)
	}

	if field.Section5.TemplateNumber != 42 {
		t.Errorf("TemplateNumber = %d, want 42", field.Section5.TemplateNumber)
	}
	if tmpl.BitsPerValue != 14 {
		t.Errorf("BitsPerValue = %d, want 14", tmpl.BitsPerValue)
	}
	if tmpl.CcsdsFlags != 12 {
		t.Errorf("CcsdsFlags = %d, want 12", tmpl.CcsdsFlags)
	}
	if tmpl.CcsdsBlockSize != 32 {
		t.Errorf("CcsdsBlockSize = %d, want 32", tmpl.CcsdsBlockSize)
	}
	if tmpl.CcsdsRsi != 128 {
		t.Errorf("CcsdsRsi = %d, want 128", tmpl.CcsdsRsi)
	}

	values, err := UnpackCCSDS(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackCCSDS: %v", err)
	}

	if len(values) != 65160 {
		t.Fatalf("got %d values, want 65160", len(values))
	}

	// Load eccodes reference values
	refValues := loadReferenceValues(t, "testdata/reference/ccsds.grib2.values")
	if len(refValues) != len(values) {
		t.Fatalf("reference has %d values, got %d", len(refValues), len(values))
	}

	maxErr := 0.0
	mismatches := 0
	for i, got := range values {
		want := refValues[i]
		diff := math.Abs(got - want)
		if diff > maxErr {
			maxErr = diff
		}
		if diff > 1e-6 {
			if mismatches < 10 {
				t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			}
			mismatches++
		}
	}
	if mismatches > 10 {
		t.Errorf("... and %d more mismatches", mismatches-10)
	}
	t.Logf("CCSDS max error vs eccodes: %.2e (%d values, %d mismatches)", maxErr, len(values), mismatches)
}

func TestPackCCSDS_RoundTrip(t *testing.T) {
	// Read and decode ccsds.grib2
	raw, err := os.ReadFile("testdata/ccsds.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	origTmpl, ok := field.Section5.Template.(Template542)
	if !ok {
		t.Fatalf("not Template542: %T", field.Section5.Template)
	}

	origValues, err := UnpackCCSDS(field.Section7.Data, origTmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackCCSDS: %v", err)
	}

	// Re-pack with same parameters
	packed, newTmpl, err := PackCCSDS(origValues, origTmpl.BitsPerValue, origTmpl.CcsdsBlockSize, origTmpl.CcsdsRsi)
	if err != nil {
		t.Fatalf("PackCCSDS: %v", err)
	}

	// Decode our re-packed data
	reValues, err := UnpackCCSDS(packed, newTmpl, uint32(len(origValues)))
	if err != nil {
		t.Fatalf("UnpackCCSDS (re-packed): %v", err)
	}

	if len(reValues) != len(origValues) {
		t.Fatalf("round-trip: got %d values, want %d", len(reValues), len(origValues))
	}

	// Values should be very close (within the quantization error of the packing)
	maxErr := 0.0
	mismatches := 0
	quantErr := math.Pow(2, float64(newTmpl.BinaryScaleFactor))
	for i, orig := range origValues {
		diff := math.Abs(orig - reValues[i])
		if diff > maxErr {
			maxErr = diff
		}
		if diff > quantErr*1.01 {
			if mismatches < 10 {
				t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, reValues[i], orig, diff)
			}
			mismatches++
		}
	}
	if mismatches > 10 {
		t.Errorf("... and %d more mismatches", mismatches-10)
	}
	t.Logf("PackCCSDS round-trip: max error = %.2e, quantization bound = %.2e (%d values, %d mismatches)",
		maxErr, quantErr, len(origValues), mismatches)
}

func TestPackCCSDS_SyntheticRoundTrip(t *testing.T) {
	// Test with synthetic data to verify encode-decode round-trip
	values := make([]float64, 1024)
	for i := range values {
		values[i] = 273.15 + float64(i)*0.1
	}

	packed, tmpl, err := PackCCSDS(values, 16, 32, 128)
	if err != nil {
		t.Fatalf("PackCCSDS: %v", err)
	}

	decoded, err := UnpackCCSDS(packed, tmpl, uint32(len(values)))
	if err != nil {
		t.Fatalf("UnpackCCSDS: %v", err)
	}

	if len(decoded) != len(values) {
		t.Fatalf("got %d values, want %d", len(decoded), len(values))
	}

	maxErr := 0.0
	quantErr := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	for i, orig := range values {
		diff := math.Abs(orig - decoded[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	t.Logf("synthetic round-trip: max error = %.2e, quantization bound = %.2e", maxErr, quantErr)
	if maxErr > quantErr*1.01 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestPackCCSDS_ConstantField(t *testing.T) {
	// Constant field: all values the same
	values := make([]float64, 256)
	for i := range values {
		values[i] = 300.0
	}

	packed, tmpl, err := PackCCSDS(values, 16, 32, 128)
	if err != nil {
		t.Fatalf("PackCCSDS: %v", err)
	}

	decoded, err := UnpackCCSDS(packed, tmpl, uint32(len(values)))
	if err != nil {
		t.Fatalf("UnpackCCSDS: %v", err)
	}

	for i, v := range decoded {
		if v != 300.0 {
			t.Errorf("value[%d] = %f, want 300.0", i, v)
		}
	}
}

func TestUnpackCCSDS_ConstantField(t *testing.T) {
	tmpl := Template542{
		Template50: Template50{
			ReferenceValue:     300.0,
			BinaryScaleFactor:  0,
			DecimalScaleFactor: 0,
			BitsPerValue:       0,
		},
		CcsdsFlags:     12,
		CcsdsBlockSize: 32,
		CcsdsRsi:       128,
	}

	values, err := UnpackCCSDS(nil, tmpl, 100)
	if err != nil {
		t.Fatalf("UnpackCCSDS: %v", err)
	}

	if len(values) != 100 {
		t.Fatalf("got %d values, want 100", len(values))
	}

	for i, v := range values {
		if v != 300.0 {
			t.Errorf("value[%d] = %f, want 300.0", i, v)
		}
	}
}

// ---------------------------------------------------------------------------
// eccodes fixtures (testdata/ccsds_*.grib2, made by testdata/ccsds_fixtures.sh)
// ---------------------------------------------------------------------------

// ccsdsLoadExpected reads one value per line ("nan" for a missing point).
func ccsdsLoadExpected(t *testing.T, path string) []float64 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read expected values: %v", err)
	}
	var out []float64
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		v, err := strconv.ParseFloat(strings.TrimSpace(line), 64)
		if err != nil {
			t.Fatalf("%s: parse %q: %v", path, line, err)
		}
		out = append(out, v)
	}
	return out
}

func ccsdsReadField(t *testing.T, path string) Field {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if len(msg.Fields) != 1 {
		t.Fatalf("%d fields, want 1", len(msg.Fields))
	}
	if msg.Fields[0].Section5.TemplateNumber != 42 {
		t.Fatalf("template 5.%d, want 5.42", msg.Fields[0].Section5.TemplateNumber)
	}
	return msg.Fields[0]
}

// ccsdsCompare checks got against eccodes' values: NaN positions must match
// and values agree to a relative 1e-14.
func ccsdsCompare(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d values, want %d", len(got), len(want))
	}
	bad := 0
	for i := range want {
		g, w := got[i], want[i]
		if math.IsNaN(w) || math.IsNaN(g) {
			if math.IsNaN(w) != math.IsNaN(g) {
				t.Errorf("value[%d] = %v, want %v", i, g, w)
				bad++
			}
		} else if math.Abs(g-w) > 1e-14*math.Abs(w) {
			t.Errorf("value[%d] = %.17g, want %.17g", i, g, w)
			bad++
		}
		if bad >= 10 {
			t.Fatalf("too many mismatches")
		}
	}
}

func TestUnpackCCSDS_EccodesFixtures(t *testing.T) {
	cases := []struct {
		file     string
		expected string // values file; "" means the test checks values itself
		flags    uint8
		bpv      uint8
		wantErr  bool
	}{
		// M7: ccsdsFlags variants of one field (grib_set -r -s ccsdsFlags=N).
		{"ccsds_base", "ccsds_base", 14, 14, false},
		{"ccsds_flags4", "ccsds_base", 4, 14, false},
		{"ccsds_flags12", "ccsds_base", 12, 14, false},
		{"ccsds_flags20", "ccsds_base", 20, 14, false},
		// SIGNED: eccodes reads libaec's sign-extended output as uint16.
		{"ccsds_flags13", "ccsds_flags13", 13, 14, false},
		{"ccsds_flags29", "ccsds_flags13", 29, 14, false},
		// PAD_RSI: the stock libaec encoder ignores it, so these streams are
		// unpadded; libaec's decoder (and ours) still drops the pad bits.
		// eccodes fails on 36 and decodes 44 to shifted values.
		{"ccsds_flags36", "", 36, 14, true},
		{"ccsds_flags44", "ccsds_flags44", 44, 14, false},
		// Block sizes and RSIs.
		{"ccsds_bs8", "ccsds_base", 14, 14, false},
		{"ccsds_bs64", "ccsds_base", 14, 14, false},
		{"ccsds_rsi1", "ccsds_base", 14, 14, false},
		{"ccsds_rsi7", "ccsds_base", 14, 14, false},
		{"ccsds_rsi4096", "ccsds_base", 14, 14, false},
		// M8: wide samples, including 3-byte (17-24) and 32-bit samples.
		{"ccsds_bpv17", "ccsds_base", 14, 17, false},
		{"ccsds_bpv20", "ccsds_base", 14, 20, false},
		{"ccsds_bpv24", "ccsds_base", 14, 24, false},
		{"ccsds_bpv25", "ccsds_base", 14, 25, false},
		{"ccsds_bpv31", "ccsds_base", 14, 31, false},
		{"ccsds_bpv32", "ccsds_base", 14, 32, false},
		{"ccsds_from_ieee", "ccsds_base", 14, 32, false},
		// L10: optimizeScaleFactor (D=1) and tiny values (E=-109).
		{"ccsds_optscale", "ccsds_optscale", 14, 14, false},
		{"ccsds_tiny", "ccsds_tiny", 14, 24, false},
		// L9: ECC-802, 2x2 grid with a negative minimum.
		{"ccsds_ecc802", "ccsds_ecc802", 14, 12, false},
		// M9: bitmaps.
		{"ccsds_bitmap", "ccsds_bitmap", 14, 16, false},
		{"ccsds_bitmap_const", "ccsds_bitmap_const", 14, 0, false},
		{"ccsds_bm6x2", "ccsds_bm6x2", 14, 12, false},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			f := ccsdsReadField(t, "testdata/"+c.file+".grib2")
			tmpl := f.Section5.Template.(Template542)
			if tmpl.CcsdsFlags != c.flags || tmpl.BitsPerValue != c.bpv {
				t.Fatalf("flags %d bpv %d, want %d %d", tmpl.CcsdsFlags, tmpl.BitsPerValue, c.flags, c.bpv)
			}
			got, err := f.Values()
			if c.wantErr {
				if err == nil {
					t.Fatalf("Values: want an error (libaec and eccodes fail on this stream)")
				}
				return
			}
			if err != nil {
				t.Fatalf("Values: %v", err)
			}
			ccsdsCompare(t, got, ccsdsLoadExpected(t, "testdata/"+c.expected+".grib2.values"))
		})
	}
}

// TestUnpackCCSDS_ConstantWithDecimalScale is H3: a bpv=0 field with D=2
// decodes to R (5.5, as grib_get_data prints), not R*10^-D (0.055).
func TestUnpackCCSDS_ConstantWithDecimalScale(t *testing.T) {
	f := ccsdsReadField(t, "testdata/ccsds_const_d2.grib2")
	tmpl := f.Section5.Template.(Template542)
	if tmpl.BitsPerValue != 0 || tmpl.DecimalScaleFactor != 2 {
		t.Fatalf("bpv %d D %d, want 0 2", tmpl.BitsPerValue, tmpl.DecimalScaleFactor)
	}
	got, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 256 {
		t.Fatalf("%d values", len(got))
	}
	for i, v := range got {
		if v != 5.5 {
			t.Fatalf("value[%d] = %v, want 5.5", i, v)
		}
	}

	// Unit table: R is returned unscaled for any D.
	for _, d := range []int16{3, 2, 1, 0, -2} {
		tm := Template542{Template50: Template50{ReferenceValue: 273, DecimalScaleFactor: d}}
		vals, err := UnpackCCSDS(nil, tm, 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range vals {
			if v != 273 {
				t.Errorf("D=%d: got %v, want 273", d, v)
			}
		}
	}
}

// TestUnpackCCSDS_AllMissing is M9: an all-missing field has no coded values.
func TestUnpackCCSDS_AllMissing(t *testing.T) {
	f := ccsdsReadField(t, "testdata/ccsds_allmissing.grib2")
	if f.Section5.NumberOfValues != 0 {
		t.Fatalf("NumberOfValues = %d", f.Section5.NumberOfValues)
	}
	got, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 256 {
		t.Fatalf("%d values, want 256", len(got))
	}
	for i, v := range got {
		if !math.IsNaN(v) {
			t.Fatalf("value[%d] = %v, want NaN", i, v)
		}
	}
}

// TestUnpackCCSDS_BitsPerValueOver32 is M8: libaec cannot code more than 32
// bits per sample.
func TestUnpackCCSDS_BitsPerValueOver32(t *testing.T) {
	tm := Template542{Template50: Template50{BitsPerValue: 33}, CcsdsFlags: 12, CcsdsBlockSize: 32, CcsdsRsi: 128}
	if _, err := UnpackCCSDS([]byte{1, 2, 3, 4}, tm, 4); err == nil {
		t.Fatal("want error for bpv 33")
	}
}

// ---------------------------------------------------------------------------
// PackCCSDS
// ---------------------------------------------------------------------------

// ccsdsRoundTrip packs and unpacks, checking R <= min and the error bound.
func ccsdsRoundTrip(t *testing.T, values []float64, bpv uint8, bs uint8, rsi uint16) Template542 {
	t.Helper()
	packed, tmpl, err := PackCCSDS(values, bpv, bs, rsi)
	if err != nil {
		t.Fatalf("PackCCSDS(bpv %d): %v", bpv, err)
	}
	vmin, vmax := values[0], values[0]
	for _, v := range values {
		vmin = math.Min(vmin, v)
		vmax = math.Max(vmax, v)
	}
	if float64(tmpl.ReferenceValue) > vmin {
		t.Fatalf("bpv %d: reference %.17g > min %.17g", bpv, tmpl.ReferenceValue, vmin)
	}
	dec, err := UnpackCCSDS(packed, tmpl, uint32(len(values)))
	if err != nil {
		t.Fatalf("UnpackCCSDS(bpv %d): %v", bpv, err)
	}
	// Half a step, plus float64 rounding of x*2^E+R, plus (for a constant
	// field) the gap between the value and the float32 below it.
	bound := 0.5*math.Ldexp(1, int(tmpl.BinaryScaleFactor)) + 4*ulp64(math.Max(math.Abs(vmin), math.Abs(vmax)))
	if tmpl.BitsPerValue == 0 {
		bound = vmin - float64(tmpl.ReferenceValue)
	}
	for i, v := range values {
		if d := math.Abs(dec[i] - v); d > bound {
			t.Fatalf("bpv %d (E=%d): value[%d] = %.17g, want %.17g (error %.3g > bound %.3g)",
				bpv, tmpl.BinaryScaleFactor, i, dec[i], v, d, bound)
		}
	}
	return tmpl
}

func ulp64(x float64) float64 { return math.Nextafter(x, math.Inf(1)) - x }

// TestPackCCSDS_ReferenceNotAboveMin is H2: float32(min) rounds above min
// about half of the time, which made the smallest samples negative (clamped
// to 0, an error of many steps).
func TestPackCCSDS_ReferenceNotAboveMin(t *testing.T) {
	sets := map[string][]float64{
		"mixed":  {279, 279.96, 278.5, 300.25, 270.1, 285.3},
		"0.1":    {0.1, 0.7, 3.3},
		"1.1":    {1.1, 1.2, 9.9},
		"7.85":   {7.85, 7.9, 12},
		"-0.3":   {-0.3, 0.3, 0.9},
		"tight":  {280.00002, 280.0004, 280.0011},
		"single": {7000000300},
	}
	ramp := make([]float64, 1000)
	for i := range ramp {
		ramp[i] = 100.3 + float64(i)/1000
	}
	sets["100.3+i/1000"] = ramp
	for name, vals := range sets {
		t.Run(name, func(t *testing.T) {
			for bpv := uint8(1); bpv <= 32; bpv++ {
				ccsdsRoundTrip(t, vals, bpv, 32, 128)
			}
		})
	}
}

// TestPackCCSDS_SeededRanges ports ECC-1431: random ranges at several sizes.
func TestPackCCSDS_SeededRanges(t *testing.T) {
	r := rand.New(rand.NewSource(1431))
	sizes := []int{1, 10, 100, 10000}
	for _, n := range sizes {
		for trial := 0; trial < 8; trial++ {
			lo := (r.Float64() - 0.5) * math.Pow(10, float64(r.Intn(12)-4))
			span := math.Pow(10, float64(r.Intn(10)-5))
			vals := make([]float64, n)
			for i := range vals {
				vals[i] = lo + span*r.Float64()
			}
			for _, bpv := range []uint8{8, 12, 16, 23, 24, 31, 32} {
				ccsdsRoundTrip(t, vals, bpv, 32, 128)
			}
		}
	}
}

// TestPackCCSDS_HighBitsPerValue is M8: at 32 bits, samples above 2^31 used to
// overflow int32 (max error 72 on a 100..300 ramp); 33 bits must be rejected.
func TestPackCCSDS_HighBitsPerValue(t *testing.T) {
	vals := make([]float64, 4096)
	for i := range vals {
		vals[i] = 100 + 200*float64(i)/4095
	}
	for bpv := uint8(1); bpv <= 32; bpv++ {
		ccsdsRoundTrip(t, vals, bpv, 32, 128)
	}
	if _, _, err := PackCCSDS(vals, 33, 32, 128); err == nil {
		t.Fatal("bpv 33: want error")
	}
}

// TestPackCCSDS_RejectsInvalid is M4.
func TestPackCCSDS_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		vals []float64
		bpv  uint8
	}{
		{"+Inf", []float64{1, 2, math.Inf(1), 4}, 16},
		{"-Inf", []float64{1, 2, math.Inf(-1), 4}, 16},
		{"leading NaN", []float64{math.NaN(), 2, 3}, 16},
		{"middle NaN", []float64{1, math.NaN(), 3}, 16},
		{"1e39", []float64{1, 1e39}, 16},
		{"-1e39", []float64{-1e39, 1}, 16},
		{"bpv0 varying", []float64{1, 2, 3}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := PackCCSDS(c.vals, c.bpv, 32, 128); err == nil {
				t.Fatal("want error")
			}
		})
	}
	// The float32 limits themselves are accepted.
	ccsdsRoundTrip(t, []float64{-math.MaxFloat32, 0, math.MaxFloat32}, 24, 32, 128)
}

// TestPackCCSDS_BadConfig: block sizes and RSIs libaec rejects.
func TestPackCCSDS_BadConfig(t *testing.T) {
	vals := []float64{1, 2, 3, 4}
	for _, c := range []struct {
		bs  uint8
		rsi uint16
	}{{0, 128}, {12, 128}, {128, 128}, {32, 0}, {32, 4097}} {
		if _, _, err := PackCCSDS(vals, 16, c.bs, c.rsi); err == nil {
			t.Errorf("bs %d rsi %d: want error", c.bs, c.rsi)
		}
	}
}

// TestPackCCSDS_ConstantCollapses is L8 (ECC-1387): a constant field is
// written with bpv 0 and no payload, whatever bpv was requested.
func TestPackCCSDS_ConstantCollapses(t *testing.T) {
	for _, c := range []float64{6, 0.1, -273.15, 0} {
		packed, tmpl, err := PackCCSDS([]float64{c, c, c}, 16, 32, 128)
		if err != nil {
			t.Fatal(err)
		}
		if tmpl.BitsPerValue != 0 || len(packed) != 0 || tmpl.BinaryScaleFactor != 0 {
			t.Fatalf("%v: bpv %d, %d bytes, E %d; want bpv 0, no payload, E 0", c, tmpl.BitsPerValue, len(packed), tmpl.BinaryScaleFactor)
		}
		if want := float32AtOrBelow(c); tmpl.ReferenceValue != want {
			t.Fatalf("%v: reference %v, want %v", c, tmpl.ReferenceValue, want)
		}
		ccsdsRoundTrip(t, []float64{c, c, c}, 16, 32, 128)
	}
	// bpv 0 is fine on a constant field.
	if _, _, err := PackCCSDS([]float64{6, 6}, 0, 32, 128); err != nil {
		t.Fatal(err)
	}
}

// TestPackCCSDS_TinyGrids is L9 (ECC-802).
func TestPackCCSDS_TinyGrids(t *testing.T) {
	ecc802 := []float64{-0.01, 11.99, 56.11, 98.99}
	tmpl := ccsdsRoundTrip(t, ecc802, 12, 32, 128)
	if float64(tmpl.ReferenceValue) > -0.01 {
		t.Fatalf("reference %v > -0.01", tmpl.ReferenceValue)
	}
	for _, n := range []int{1, 2, 3, 7} {
		vals := make([]float64, n)
		for i := range vals {
			vals[i] = -0.01 + 14.143*float64(i)
		}
		for _, bpv := range []uint8{1, 8, 12, 24, 32} {
			for _, bs := range []uint8{8, 16, 32, 64} {
				ccsdsRoundTrip(t, vals, bpv, bs, 1)
			}
		}
	}
}

// TestPackCCSDS_ExtremeScales is L10: values near the ends of the float32
// range, at sizes around block and RSI boundaries.
func TestPackCCSDS_ExtremeScales(t *testing.T) {
	ranges := [][2]float64{{1e-38, 1e-37}, {3.4e37, 3.4e38}, {-3.4e38, -3.4e37}}
	for _, rg := range ranges {
		for _, n := range []int{1, 10, 31, 33, 4097} {
			vals := make([]float64, n)
			for i := range vals {
				vals[i] = rg[0] + (rg[1]-rg[0])*float64(i)/math.Max(1, float64(n-1))
			}
			for _, bpv := range []uint8{16, 24, 32} {
				ccsdsRoundTrip(t, vals, bpv, 32, 128)
			}
		}
	}
}

// TestPackCCSDS_FiveBlockConstantRun is H21 at the PackCCSDS level: a run of
// exactly five constant blocks used to be coded as ROS and UnpackCCSDS
// returned "aec: data error".
func TestPackCCSDS_FiveBlockConstantRun(t *testing.T) {
	vals := make([]float64, 1024)
	for i := range vals {
		vals[i] = 250 + 30*math.Sin(float64(i)/17) + float64(i%7)*0.3
	}
	for i := 32; i < 192; i++ {
		vals[i] = vals[31]
	}
	ccsdsRoundTrip(t, vals, 16, 32, 128)
}
