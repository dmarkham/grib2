package grib2

import (
	"bytes"
	"math"
	"os"
	"testing"
)

// TestValues_SimplePacking tests Field.Values() on sample.grib2
// which uses simple packing (template 5.0), no bitmap, scan mode 0.
func TestValues_SimplePacking(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]

	// Verify preconditions
	if field.Section5.TemplateNumber != 0 {
		t.Fatalf("expected template 0, got %d", field.Section5.TemplateNumber)
	}
	if field.Section6.Indicator != 255 {
		t.Fatalf("expected no bitmap (indicator 255), got %d", field.Section6.Indicator)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(values) != 496 {
		t.Fatalf("got %d values, want 496", len(values))
	}

	// Compare against eccodes reference
	refValues := loadReferenceValues(t, "testdata/reference/sample.grib2.values")
	if len(refValues) != len(values) {
		t.Fatalf("reference has %d values, got %d", len(refValues), len(values))
	}

	maxErr := 0.0
	for i, got := range values {
		want := refValues[i]
		diff := math.Abs(got - want)
		if diff > maxErr {
			maxErr = diff
		}
		if diff > 1e-6 {
			t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			if i > 10 {
				t.Fatal("too many errors, stopping")
			}
		}
	}
	t.Logf("max error vs eccodes: %.2e (across %d values)", maxErr, len(values))
}

// TestValues_MatchesUnpackSimple verifies that Field.Values() produces
// identical results to calling UnpackSimple directly (no bitmap, scan mode 0).
func TestValues_MatchesUnpackSimple(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template50)
	if !ok {
		t.Fatalf("not Template50: %T", field.Section5.Template)
	}

	direct, err := UnpackSimple(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackSimple: %v", err)
	}

	unified, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(direct) != len(unified) {
		t.Fatalf("length mismatch: direct=%d, Values=%d", len(direct), len(unified))
	}

	for i := range direct {
		if direct[i] != unified[i] {
			t.Errorf("value[%d]: direct=%.10e, Values=%.10e", i, direct[i], unified[i])
		}
	}
}

// TestValues_ComplexSpatialDiffWithBitmap tests Field.Values() on
// grid_complex_spatial_differencing.grib2 which uses template 5.3
// and has a Section 6 bitmap (indicator == 0).
func TestValues_ComplexSpatialDiffWithBitmap(t *testing.T) {
	raw, err := os.ReadFile("testdata/grid_complex_spatial_differencing.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]

	// Verify preconditions
	if field.Section5.TemplateNumber != 3 {
		t.Fatalf("expected template 3, got %d", field.Section5.TemplateNumber)
	}
	if field.Section6.Indicator != 0 {
		t.Fatalf("expected bitmap (indicator 0), got %d", field.Section6.Indicator)
	}
	if field.Section3.NumberOfDataPoints != 83441 {
		t.Fatalf("expected 83441 grid points, got %d", field.Section3.NumberOfDataPoints)
	}
	if field.Section5.NumberOfValues != 32872 {
		t.Fatalf("expected 32872 packed values, got %d", field.Section5.NumberOfValues)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	// After bitmap expansion, we should have totalPoints values
	if uint32(len(values)) != field.Section3.NumberOfDataPoints {
		t.Fatalf("got %d values, want %d (total grid points)", len(values), field.Section3.NumberOfDataPoints)
	}

	// Count non-NaN values — should equal NumberOfValues from section 5
	nonMissing := 0
	for _, v := range values {
		if !math.IsNaN(v) {
			nonMissing++
		}
	}
	if nonMissing != int(field.Section5.NumberOfValues) {
		t.Errorf("non-missing count = %d, want %d", nonMissing, field.Section5.NumberOfValues)
	}

	// Compare non-missing values against eccodes reference.
	// grib_get_data only outputs non-missing values for files with bitmaps.
	refValues := loadReferenceValues(t, "testdata/reference/grid_complex_spatial_differencing.grib2.values")
	t.Logf("reference has %d values, we have %d non-missing out of %d total",
		len(refValues), nonMissing, len(values))

	j := 0
	maxErr := 0.0
	mismatches := 0
	for _, got := range values {
		if math.IsNaN(got) {
			continue
		}
		if j >= len(refValues) {
			break
		}
		diff := math.Abs(got - refValues[j])
		if diff > maxErr {
			maxErr = diff
		}
		if diff > 1e-3 {
			if mismatches < 10 {
				t.Errorf("non-missing[%d] = %.10e, want %.10e (diff = %.10e)", j, got, refValues[j], diff)
			}
			mismatches++
		}
		j++
	}
	if mismatches > 10 {
		t.Errorf("... and %d more mismatches", mismatches-10)
	}
	t.Logf("bitmap expansion: max error vs eccodes: %.2e (across %d non-missing values)", maxErr, j)
}

// TestValues_JPEG2000 tests Field.Values() dispatch for JPEG2000 (template 5.40).
func TestValues_JPEG2000(t *testing.T) {
	raw, err := os.ReadFile("testdata/jpeg.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section5.TemplateNumber != 40 {
		t.Fatalf("expected template 40, got %d", field.Section5.TemplateNumber)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(values) == 0 {
		t.Fatal("got 0 values")
	}

	// Compare against eccodes reference
	refValues := loadReferenceValues(t, "testdata/reference/jpeg.grib2.values")
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
		if diff > 1e-4 {
			if mismatches < 10 {
				t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			}
			mismatches++
		}
	}
	if mismatches > 0 {
		t.Errorf("total mismatches: %d", mismatches)
	}
	t.Logf("JPEG2000: max error vs eccodes: %.2e (across %d values)", maxErr, len(values))
}

// TestValues_PNG tests Field.Values() dispatch for PNG (template 5.41).
func TestValues_PNG(t *testing.T) {
	raw, err := os.ReadFile("testdata/png.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section5.TemplateNumber != 41 {
		t.Fatalf("expected template 41, got %d", field.Section5.TemplateNumber)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(values) == 0 {
		t.Fatal("got 0 values")
	}

	// Compare against eccodes reference
	refValues := loadReferenceValues(t, "testdata/reference/png.grib2.values")
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
		if diff > 1e-4 {
			if mismatches < 10 {
				t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			}
			mismatches++
		}
	}
	if mismatches > 0 {
		t.Errorf("total mismatches: %d", mismatches)
	}
	t.Logf("PNG: max error vs eccodes: %.2e (across %d values)", maxErr, len(values))
}

// TestValues_IEEE tests Field.Values() dispatch for IEEE (template 5.4).
func TestValues_IEEE(t *testing.T) {
	raw, err := os.ReadFile("testdata/ieee.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section5.TemplateNumber != 4 {
		t.Fatalf("expected template 4, got %d", field.Section5.TemplateNumber)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(values) == 0 {
		t.Fatal("got 0 values")
	}

	// Compare against eccodes reference
	refValues := loadReferenceValues(t, "testdata/reference/ieee.grib2.values")
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
		if diff > 1e-4 {
			if mismatches < 10 {
				t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			}
			mismatches++
		}
	}
	if mismatches > 0 {
		t.Errorf("total mismatches: %d", mismatches)
	}
	t.Logf("IEEE: max error vs eccodes: %.2e (across %d values)", maxErr, len(values))
}

// TestValues_CCSDS tests Field.Values() dispatch for CCSDS (template 5.42).
func TestValues_CCSDS(t *testing.T) {
	raw, err := os.ReadFile("testdata/ccsds.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section5.TemplateNumber != 42 {
		t.Fatalf("expected template 42, got %d", field.Section5.TemplateNumber)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(values) == 0 {
		t.Fatal("got 0 values")
	}

	// Compare against eccodes reference
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
		if diff > 1e-4 {
			if mismatches < 10 {
				t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			}
			mismatches++
		}
	}
	if mismatches > 0 {
		t.Errorf("total mismatches: %d", mismatches)
	}
	t.Logf("CCSDS: max error vs eccodes: %.2e (across %d values)", maxErr, len(values))
}

// TestValues_ComplexPacking tests Field.Values() dispatch for complex packing
// (template 5.2) without spatial differencing.
func TestValues_ComplexPacking(t *testing.T) {
	raw, err := os.ReadFile("testdata/complex.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section5.TemplateNumber != 2 {
		t.Fatalf("expected template 2, got %d", field.Section5.TemplateNumber)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(values) == 0 {
		t.Fatal("got 0 values")
	}

	// Compare against eccodes reference
	refValues := loadReferenceValues(t, "testdata/reference/complex.grib2.values")
	if len(refValues) != len(values) {
		t.Fatalf("reference has %d values, got %d", len(refValues), len(values))
	}

	maxErr := 0.0
	for i, got := range values {
		want := refValues[i]
		if math.IsNaN(got) {
			continue
		}
		diff := math.Abs(got - want)
		if diff > maxErr {
			maxErr = diff
		}
		if diff > 1e-4 {
			t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			if i > 10 {
				t.Fatal("too many errors")
			}
		}
	}
	t.Logf("Complex: max error vs eccodes: %.2e (across %d values)", maxErr, len(values))
}

// TestValues_UnsupportedTemplate verifies that Values() returns an error
// for an unsupported template number.
func TestValues_UnsupportedTemplate(t *testing.T) {
	f := Field{
		Section5: Section5{
			TemplateNumber: 9999,
			Template: &RawDataRepTemplate{
				Number: 9999,
				Data:   []byte{0x00},
			},
		},
	}

	_, err := f.Values()
	if err == nil {
		t.Fatal("expected error for unsupported template, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// TestApplyBitmap tests bitmap expansion directly.
func TestApplyBitmap(t *testing.T) {
	// Bitmap: 1010 1100 = 0xAC
	// 8 grid points, 4 have data
	bitmap := []byte{0xAC} // bits: 1 0 1 0 1 1 0 0
	packed := []float64{10.0, 20.0, 30.0, 40.0}
	totalPoints := uint32(8)

	expanded, err := applyBitmap(packed, bitmap, totalPoints)
	if err != nil {
		t.Fatalf("applyBitmap: %v", err)
	}

	if len(expanded) != 8 {
		t.Fatalf("got %d values, want 8", len(expanded))
	}

	// Expected: 10, NaN, 20, NaN, 30, 40, NaN, NaN
	want := []float64{10, math.NaN(), 20, math.NaN(), 30, 40, math.NaN(), math.NaN()}
	for i, got := range expanded {
		if math.IsNaN(want[i]) {
			if !math.IsNaN(got) {
				t.Errorf("value[%d] = %f, want NaN", i, got)
			}
		} else {
			if got != want[i] {
				t.Errorf("value[%d] = %f, want %f", i, got, want[i])
			}
		}
	}
}

// TestGridScanParams_Template390 verifies that gridScanParams correctly
// extracts scanning mode and grid dimensions from Template390 (BUG-2 regression).
// Before the fix, Template390 was not handled in gridScanParams, so
// Fields with space-view grids would silently skip scanning mode reordering.
func TestGridScanParams_Template390(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     GridDefinitionTemplate
		wantScan uint8
		wantNi   int
		wantNj   int
	}{
		{
			name: "Template390_ScanMode0",
			tmpl: Template390{
				Nx:           5424,
				Ny:           5424,
				ScanningMode: 0,
			},
			wantScan: 0,
			wantNi:   5424,
			wantNj:   5424,
		},
		{
			name: "Template390_ScanMode64",
			tmpl: Template390{
				Nx:           1000,
				Ny:           2000,
				ScanningMode: 64, // +j direction
			},
			wantScan: 64,
			wantNi:   1000,
			wantNj:   2000,
		},
		{
			name: "Template30_for_comparison",
			tmpl: Template30{
				Ni:           16,
				Nj:           31,
				ScanningMode: 0,
			},
			wantScan: 0,
			wantNi:   16,
			wantNj:   31,
		},
		{
			name: "Template330_Lambert",
			tmpl: Template330{
				Nx:           349,
				Ny:           277,
				ScanningMode: 64,
			},
			wantScan: 64,
			wantNi:   349,
			wantNj:   277,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanMode, ni, nj := gridScanParams(tt.tmpl)
			if scanMode != tt.wantScan {
				t.Errorf("scanMode = %d, want %d", scanMode, tt.wantScan)
			}
			if ni != tt.wantNi {
				t.Errorf("ni = %d, want %d", ni, tt.wantNi)
			}
			if nj != tt.wantNj {
				t.Errorf("nj = %d, want %d", nj, tt.wantNj)
			}
		})
	}
}

// TestGridScanParams_Template390_Integration verifies that Values() on a
// space_view.grib2 file correctly dispatches through gridScanParams for Template390.
func TestGridScanParams_Template390_Integration(t *testing.T) {
	raw, err := os.ReadFile("testdata/space_view.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section3.Template.(Template390)
	if !ok {
		t.Fatalf("Template type = %T, want Template390", field.Section3.Template)
	}

	// Verify gridScanParams works for this template
	scanMode, ni, nj := gridScanParams(tmpl)
	if scanMode != tmpl.ScanningMode {
		t.Errorf("gridScanParams scanMode = %d, want %d", scanMode, tmpl.ScanningMode)
	}
	if ni != int(tmpl.Nx) {
		t.Errorf("gridScanParams ni = %d, want %d", ni, tmpl.Nx)
	}
	if nj != int(tmpl.Ny) {
		t.Errorf("gridScanParams nj = %d, want %d", nj, tmpl.Ny)
	}

	// Values() should not error
	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(values) == 0 {
		t.Fatal("got 0 values")
	}
	t.Logf("Template390 Values(): %d values, scanMode=%d, Nx=%d, Ny=%d", len(values), scanMode, ni, nj)
}

// TestApplyScanningMode_Mode0 verifies that mode 0 is a no-op.
func TestApplyScanningMode_Mode0(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6}
	result := applyScanningMode(values, 0, 3, 2)
	for i := range values {
		if result[i] != values[i] {
			t.Errorf("value[%d] = %f, want %f (mode 0 should be identity)", i, result[i], values[i])
		}
	}
}

// TestApplyScanningMode_JPos verifies +j (south to north) reordering.
func TestApplyScanningMode_JPos(t *testing.T) {
	// 3 columns (ni=3), 2 rows (nj=2)
	// Input scanning: +i, +j (S to N), row-major
	// Row 0 in input = southernmost row
	// Row 1 in input = northernmost row
	// After reordering to canonical (-j = N to S):
	//   canonical row 0 = input row 1
	//   canonical row 1 = input row 0
	input := []float64{1, 2, 3, 4, 5, 6}
	result := applyScanningMode(input, 0x40, 3, 2)
	want := []float64{4, 5, 6, 1, 2, 3}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("value[%d] = %f, want %f", i, result[i], want[i])
		}
	}
}

// TestApplyScanningMode_INeg verifies -i (east to west) reordering.
func TestApplyScanningMode_INeg(t *testing.T) {
	// Input scanning: -i (E to W), -j (N to S), row-major
	// Each row is reversed compared to canonical
	input := []float64{3, 2, 1, 6, 5, 4}
	result := applyScanningMode(input, 0x80, 3, 2)
	want := []float64{1, 2, 3, 4, 5, 6}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("value[%d] = %f, want %f", i, result[i], want[i])
		}
	}
}
