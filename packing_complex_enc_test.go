package grib2

import (
	"bytes"
	"math"
	"math/rand"
	"os"
	"testing"
)

func TestPackComplex_RoundTrip_Constant(t *testing.T) {
	// All values identical: should produce a single group with width 0.
	n := 500
	values := make([]float64, n)
	for i := range values {
		values[i] = 273.15
	}

	packed, tmpl, err := PackComplex(values, 16)
	if err != nil {
		t.Fatalf("PackComplex: %v", err)
	}

	decoded, err := UnpackComplex(packed, tmpl, uint32(n), 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(decoded) != n {
		t.Fatalf("got %d values, want %d", len(decoded), n)
	}

	t.Logf("constant field: %d groups, bpv=%d", tmpl.NumberOfGroups, tmpl.BitsPerValue)
}

func TestPackComplex_RoundTrip_Varying(t *testing.T) {
	// Linearly varying values.
	n := 10000
	values := make([]float64, n)
	rng := rand.New(rand.NewSource(42))
	for i := range values {
		values[i] = 200.0 + 100.0*rng.Float64()
	}

	packed, tmpl, err := PackComplex(values, 16)
	if err != nil {
		t.Fatalf("PackComplex: %v", err)
	}

	decoded, err := UnpackComplex(packed, tmpl, uint32(n), 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(decoded) != n {
		t.Fatalf("got %d values, want %d", len(decoded), n)
	}

	maxErr := 0.0
	for i, got := range decoded {
		diff := math.Abs(got - values[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	t.Logf("varying field: %d groups, bpv=%d, maxErr=%.6e, quantBound=%.6e",
		tmpl.NumberOfGroups, tmpl.BitsPerValue, maxErr, quantErr)

	if maxErr > quantErr*1.1 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestPackComplex_RoundTrip_SmallValues(t *testing.T) {
	// Small identical values.
	n := 100
	values := make([]float64, n)
	for i := range values {
		values[i] = 0.001
	}

	packed, tmpl, err := PackComplex(values, 8)
	if err != nil {
		t.Fatalf("PackComplex: %v", err)
	}

	decoded, err := UnpackComplex(packed, tmpl, uint32(n), 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	maxErr := 0.0
	for i, got := range decoded {
		diff := math.Abs(got - values[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	t.Logf("small values: %d groups, bpv=%d, maxErr=%.6e, quantBound=%.6e",
		tmpl.NumberOfGroups, tmpl.BitsPerValue, maxErr, quantErr)
}

func TestPackComplex_RoundTrip_LargeArray(t *testing.T) {
	// Large array similar to real GRIB2 data.
	n := 32872
	values := make([]float64, n)
	rng := rand.New(rand.NewSource(99))
	for i := range values {
		values[i] = 250.0 + 50.0*rng.Float64()
	}

	packed, tmpl, err := PackComplex(values, 16)
	if err != nil {
		t.Fatalf("PackComplex: %v", err)
	}

	decoded, err := UnpackComplex(packed, tmpl, uint32(n), 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(decoded) != n {
		t.Fatalf("got %d values, want %d", len(decoded), n)
	}

	maxErr := 0.0
	for i, got := range decoded {
		diff := math.Abs(got - values[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	t.Logf("large array: %d values, %d groups, bpv=%d, maxErr=%.6e, quantBound=%.6e",
		n, tmpl.NumberOfGroups, tmpl.BitsPerValue, maxErr, quantErr)

	if maxErr > quantErr*1.1 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestPackComplexSpatialDiff_RoundTrip_Order1(t *testing.T) {
	n := 1000
	values := make([]float64, n)
	rng := rand.New(rand.NewSource(7))
	for i := range values {
		values[i] = 280.0 + 20.0*rng.Float64()
	}

	packed, tmpl, err := PackComplexSpatialDiff(values, 8, 1)
	if err != nil {
		t.Fatalf("PackComplexSpatialDiff: %v", err)
	}

	decoded, err := UnpackComplex(packed, tmpl.Template52, uint32(n),
		tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(decoded) != n {
		t.Fatalf("got %d values, want %d", len(decoded), n)
	}

	maxErr := 0.0
	for i, got := range decoded {
		diff := math.Abs(got - values[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	t.Logf("spatial diff order 1: %d values, %d groups, bpv=%d, nOctExtra=%d, maxErr=%.6e, quantBound=%.6e",
		n, tmpl.NumberOfGroups, tmpl.BitsPerValue, tmpl.NumberOfOctetsExtraDescriptors, maxErr, quantErr)

	if maxErr > quantErr*1.1 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestPackComplexSpatialDiff_RoundTrip_Order2(t *testing.T) {
	n := 1000
	values := make([]float64, n)
	rng := rand.New(rand.NewSource(13))
	for i := range values {
		values[i] = 280.0 + 20.0*rng.Float64()
	}

	packed, tmpl, err := PackComplexSpatialDiff(values, 8, 2)
	if err != nil {
		t.Fatalf("PackComplexSpatialDiff: %v", err)
	}

	decoded, err := UnpackComplex(packed, tmpl.Template52, uint32(n),
		tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(decoded) != n {
		t.Fatalf("got %d values, want %d", len(decoded), n)
	}

	maxErr := 0.0
	for i, got := range decoded {
		diff := math.Abs(got - values[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	t.Logf("spatial diff order 2: %d values, %d groups, bpv=%d, nOctExtra=%d, maxErr=%.6e, quantBound=%.6e",
		n, tmpl.NumberOfGroups, tmpl.BitsPerValue, tmpl.NumberOfOctetsExtraDescriptors, maxErr, quantErr)

	if maxErr > quantErr*1.1 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestPackComplexSpatialDiff_RoundTrip_LargeOrder2(t *testing.T) {
	// Large array with spatial differencing order 2.
	n := 32872
	values := make([]float64, n)
	rng := rand.New(rand.NewSource(55))
	for i := range values {
		values[i] = 280.0 + 20.0*rng.Float64()
	}

	packed, tmpl, err := PackComplexSpatialDiff(values, 8, 2)
	if err != nil {
		t.Fatalf("PackComplexSpatialDiff: %v", err)
	}

	decoded, err := UnpackComplex(packed, tmpl.Template52, uint32(n),
		tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(decoded) != n {
		t.Fatalf("got %d values, want %d", len(decoded), n)
	}

	maxErr := 0.0
	for i, got := range decoded {
		diff := math.Abs(got - values[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	t.Logf("large order 2: %d values, %d groups, bpv=%d, nOctExtra=%d, maxErr=%.6e, quantBound=%.6e",
		n, tmpl.NumberOfGroups, tmpl.BitsPerValue, tmpl.NumberOfOctetsExtraDescriptors, maxErr, quantErr)

	if maxErr > quantErr*1.1 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestPackComplex_SampleGrib2RoundTrip(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template52)
	if !ok {
		t.Skipf("not Template52: %T", field.Section5.Template)
	}

	origValues, err := UnpackComplex(field.Section7.Data, tmpl, field.Section5.NumberOfValues, 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	// Re-encode
	packed, newTmpl, err := PackComplex(origValues, 16)
	if err != nil {
		t.Fatalf("PackComplex: %v", err)
	}

	// Decode re-encoded data
	reValues, err := UnpackComplex(packed, newTmpl, uint32(len(origValues)), 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex re-packed: %v", err)
	}

	if len(reValues) != len(origValues) {
		t.Fatalf("got %d values, want %d", len(reValues), len(origValues))
	}

	maxErr := 0.0
	for i, got := range reValues {
		diff := math.Abs(got - origValues[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(newTmpl.BinaryScaleFactor))
	t.Logf("sample.grib2 round-trip: %d values, maxErr=%.6e, quantBound=%.6e",
		len(origValues), maxErr, quantErr)

	if maxErr > quantErr*1.1 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestPackComplex_SpatialDiffGrib2RoundTrip(t *testing.T) {
	// Read a real spatial-differencing GRIB2, decode, re-encode with spatial diff
	raw, err := os.ReadFile("testdata/grid_complex_spatial_differencing.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	origTmpl, ok := field.Section5.Template.(Template53)
	if !ok {
		t.Fatalf("not Template53: %T", field.Section5.Template)
	}

	origValues, err := UnpackComplex(field.Section7.Data, origTmpl.Template52, field.Section5.NumberOfValues,
		origTmpl.OrderOfSpatialDifferencing, origTmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	// Re-encode with spatial differencing order 2
	packed, newTmpl, err := PackComplexSpatialDiff(origValues, 14, 2)
	if err != nil {
		t.Fatalf("PackComplexSpatialDiff: %v", err)
	}

	// Decode re-encoded data
	reValues, err := UnpackComplex(packed, newTmpl.Template52, uint32(len(origValues)),
		newTmpl.OrderOfSpatialDifferencing, newTmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplex re-packed: %v", err)
	}

	if len(reValues) != len(origValues) {
		t.Fatalf("got %d values, want %d", len(reValues), len(origValues))
	}

	maxErr := 0.0
	mismatches := 0
	for i, got := range reValues {
		want := origValues[i]
		if math.IsNaN(got) && math.IsNaN(want) {
			continue
		}
		if math.IsNaN(got) != math.IsNaN(want) {
			if mismatches < 5 {
				t.Errorf("value[%d]: NaN mismatch: got NaN=%v, want NaN=%v", i, math.IsNaN(got), math.IsNaN(want))
			}
			mismatches++
			continue
		}
		diff := math.Abs(got - want)
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(newTmpl.BinaryScaleFactor))
	t.Logf("spatial diff round-trip: %d values, maxErr=%.6e, quantBound=%.6e, mismatches=%d",
		len(origValues), maxErr, quantErr, mismatches)

	if maxErr > quantErr*1.1 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestPackComplex_GFSRoundTrip(t *testing.T) {
	// Test with GFS data that has missing values
	raw, err := os.ReadFile("testdata/gfs.complex.mvmu.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]

	// The GFS file may use Template52 or Template53 (with spatial differencing).
	var tmpl52 Template52
	var spatialOrder uint8
	var numOctetsExtra uint8

	switch tt := field.Section5.Template.(type) {
	case Template52:
		tmpl52 = tt
	case Template53:
		tmpl52 = tt.Template52
		spatialOrder = tt.OrderOfSpatialDifferencing
		numOctetsExtra = tt.NumberOfOctetsExtraDescriptors
	default:
		t.Fatalf("unexpected template type: %T", field.Section5.Template)
	}

	origValues, err := UnpackComplex(field.Section7.Data, tmpl52, field.Section5.NumberOfValues,
		spatialOrder, numOctetsExtra)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	nMissing := 0
	for _, v := range origValues {
		if math.IsNaN(v) {
			nMissing++
		}
	}
	t.Logf("GFS original: %d values, %d missing", len(origValues), nMissing)

	// Re-encode with complex packing (no spatial differencing)
	packed, newTmpl, err := PackComplex(origValues, tmpl52.BitsPerValue)
	if err != nil {
		t.Fatalf("PackComplex: %v", err)
	}

	// Decode re-encoded data
	reValues, err := UnpackComplex(packed, newTmpl, uint32(len(origValues)), 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex re-packed: %v", err)
	}

	if len(reValues) != len(origValues) {
		t.Fatalf("got %d values, want %d", len(reValues), len(origValues))
	}

	maxErr := 0.0
	nonMissing := 0
	for i, got := range reValues {
		want := origValues[i]
		if math.IsNaN(got) && math.IsNaN(want) {
			continue
		}
		if math.IsNaN(got) != math.IsNaN(want) {
			t.Errorf("value[%d]: NaN mismatch", i)
			continue
		}
		nonMissing++
		diff := math.Abs(got - want)
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(newTmpl.BinaryScaleFactor))
	t.Logf("GFS round-trip: %d non-missing compared, maxErr=%.6e, quantBound=%.6e",
		nonMissing, maxErr, quantErr)

	if maxErr > quantErr*1.1 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}
