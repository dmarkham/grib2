package grib2

import (
	"bytes"
	"math"
	"os"
	"testing"
)

func TestUnpackComplex_ConstantField(t *testing.T) {
	// complex.grib2 is a constant field: all values = 273.15
	raw, err := os.ReadFile("testdata/complex.grib2")
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
		t.Fatalf("not Template52: %T", field.Section5.Template)
	}

	values, err := UnpackComplex(field.Section7.Data, tmpl, field.Section5.NumberOfValues, 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(values) != 496 {
		t.Fatalf("got %d values, want 496", len(values))
	}

	// Load reference
	refValues := loadReferenceValues(t, "testdata/reference/complex.grib2.values")
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
		if diff > 1e-4 {
			t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			if i > 10 {
				t.Fatal("too many errors")
			}
		}
	}
	t.Logf("constant field max error vs eccodes: %.2e (across %d values)", maxErr, len(values))
}

func TestUnpackComplex_SpatialDiff(t *testing.T) {
	// grid_complex_spatial_differencing.grib2: template 5.3, order=2, 32872 values, 1028 groups
	raw, err := os.ReadFile("testdata/grid_complex_spatial_differencing.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template53)
	if !ok {
		t.Fatalf("not Template53: %T (got %T)", field.Section5.Template, field.Section5.Template)
	}

	// Verify parsed template values match grib_dump
	if tmpl.BitsPerValue != 14 {
		t.Errorf("BitsPerValue = %d, want 14", tmpl.BitsPerValue)
	}
	if tmpl.BinaryScaleFactor != -2 {
		t.Errorf("BinaryScaleFactor = %d, want -2", tmpl.BinaryScaleFactor)
	}
	if tmpl.NumberOfGroups != 1028 {
		t.Errorf("NumberOfGroups = %d, want 1028", tmpl.NumberOfGroups)
	}
	if tmpl.OrderOfSpatialDifferencing != 2 {
		t.Errorf("OrderOfSpatialDifferencing = %d, want 2", tmpl.OrderOfSpatialDifferencing)
	}
	if tmpl.NumberOfOctetsExtraDescriptors != 2 {
		t.Errorf("NumberOfOctetsExtraDescriptors = %d, want 2", tmpl.NumberOfOctetsExtraDescriptors)
	}

	values, err := UnpackComplex(field.Section7.Data, tmpl.Template52, field.Section5.NumberOfValues,
		tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(values) != 32872 {
		t.Fatalf("got %d values, want 32872", len(values))
	}

	// Load reference and compare
	refValues := loadReferenceValues(t, "testdata/reference/grid_complex_spatial_differencing.grib2.values")

	// This file has a bitmap — the reference only includes non-missing values
	// Let me count non-NaN values
	nonMissing := 0
	for _, v := range values {
		if !math.IsNaN(v) {
			nonMissing++
		}
	}

	// The reference file may include all values (with coordinates)
	// or only non-missing values. Let's check the count.
	t.Logf("decoded %d values, %d non-missing, reference has %d entries",
		len(values), nonMissing, len(refValues))

	// Compare against reference (eccodes grib_get_data skips missing values for missing bitmap,
	// but includes all values for non-missing ones — it depends on the bitmap indicator)
	if len(refValues) == len(values) {
		maxErr := 0.0
		mismatches := 0
		for i, got := range values {
			want := refValues[i]
			if math.IsNaN(got) {
				continue
			}
			diff := math.Abs(got - want)
			if diff > maxErr {
				maxErr = diff
			}
			if diff > 1e-3 {
				if mismatches < 10 {
					t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
				}
				mismatches++
			}
		}
		if mismatches > 10 {
			t.Errorf("... and %d more mismatches", mismatches-10)
		}
		t.Logf("spatial diff max error vs eccodes: %.2e (across %d values)", maxErr, len(values))
	} else {
		t.Logf("reference count %d != decoded count %d — comparing non-missing values only", len(refValues), len(values))
		// Compare just the non-missing values in order
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
		t.Logf("spatial diff max error vs eccodes: %.2e (across %d non-missing values)", maxErr, j)
	}
}

func TestUnpackComplex_GFS(t *testing.T) {
	// gfs.complex.mvmu.grib2: template 5.3, order=2, 1038240 values, 14699 groups
	// This file has primary missing values (missingValueManagement=1)
	raw, err := os.ReadFile("testdata/gfs.complex.mvmu.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template53)
	if !ok {
		t.Fatalf("not Template53: %T", field.Section5.Template)
	}

	if tmpl.MissingValueManagement != 1 {
		t.Errorf("MissingValueManagement = %d, want 1", tmpl.MissingValueManagement)
	}
	if tmpl.NumberOfGroups != 14699 {
		t.Errorf("NumberOfGroups = %d, want 14699", tmpl.NumberOfGroups)
	}

	values, err := UnpackComplex(field.Section7.Data, tmpl.Template52, field.Section5.NumberOfValues,
		tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(values) != 1038240 {
		t.Fatalf("got %d values, want 1038240", len(values))
	}

	// Count missing values
	nMissing := 0
	for _, v := range values {
		if math.IsNaN(v) {
			nMissing++
		}
	}
	t.Logf("GFS: %d total values, %d missing, %d non-missing", len(values), nMissing, len(values)-nMissing)

	// Load reference and compare non-missing values
	refValues := loadReferenceValues(t, "testdata/reference/gfs.complex.mvmu.grib2.values")

	// grib_get_data omits missing values, so refValues count = non-missing count
	nonMissing := len(values) - nMissing
	if len(refValues) != nonMissing {
		t.Logf("reference has %d values, we have %d non-missing (expected match if grib_get_data omits missing)",
			len(refValues), nonMissing)
	}

	// Compare non-missing values in order against reference
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
	t.Logf("GFS max error vs eccodes: %.2e (across %d non-missing values)", maxErr, j)
}

func TestMissingValueSubstitution_GFS(t *testing.T) {
	// Test that UnpackComplexWithSubstitution replaces NaN with the template's
	// PrimaryMissingValueSubstitute for gfs.complex.mvmu.grib2 which has
	// missingValueManagement=1.
	raw, err := os.ReadFile("testdata/gfs.complex.mvmu.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template53)
	if !ok {
		t.Fatalf("not Template53: %T", field.Section5.Template)
	}

	if tmpl.MissingValueManagement != 1 {
		t.Fatalf("MissingValueManagement = %d, want 1", tmpl.MissingValueManagement)
	}

	// Compute the expected substitute value from the template
	expectedSub := float64(math.Float32frombits(tmpl.PrimaryMissingValueSubstitute))
	t.Logf("PrimaryMissingValueSubstitute bits = 0x%08x, float = %g",
		tmpl.PrimaryMissingValueSubstitute, expectedSub)

	// Decode with substitution
	valuesSub, err := UnpackComplexWithSubstitution(field.Section7.Data, tmpl.Template52,
		field.Section5.NumberOfValues, tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplexWithSubstitution: %v", err)
	}

	// Decode without substitution (NaN)
	valuesNaN, err := UnpackComplex(field.Section7.Data, tmpl.Template52,
		field.Section5.NumberOfValues, tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	if len(valuesSub) != len(valuesNaN) {
		t.Fatalf("length mismatch: sub=%d, nan=%d", len(valuesSub), len(valuesNaN))
	}

	// Verify: every NaN in the original should be replaced with the substitute value,
	// and non-NaN values should be identical.
	nSubstituted := 0
	nNaN := 0
	for i := range valuesNaN {
		if math.IsNaN(valuesNaN[i]) {
			nNaN++
			if math.IsNaN(valuesSub[i]) {
				t.Errorf("value[%d]: expected substitute %g, got NaN", i, expectedSub)
			} else if valuesSub[i] != expectedSub {
				t.Errorf("value[%d]: expected substitute %g, got %g", i, expectedSub, valuesSub[i])
			} else {
				nSubstituted++
			}
		} else {
			if valuesNaN[i] != valuesSub[i] {
				t.Errorf("value[%d]: non-missing mismatch: nan=%g, sub=%g", i, valuesNaN[i], valuesSub[i])
			}
		}
	}

	if nNaN == 0 {
		t.Fatal("expected some missing values in this file")
	}

	t.Logf("MissingValueSubstitution: %d NaN values replaced with %g (%d total values)",
		nSubstituted, expectedSub, len(valuesNaN))
}

func TestMissingValueSubstitution_NoMissing(t *testing.T) {
	// Test that UnpackComplexWithSubstitution works identically to UnpackComplex
	// when there are no missing values (missingValueManagement=0).
	raw, err := os.ReadFile("testdata/complex.grib2")
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
		t.Fatalf("not Template52: %T", field.Section5.Template)
	}

	valuesNaN, err := UnpackComplex(field.Section7.Data, tmpl, field.Section5.NumberOfValues, 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplex: %v", err)
	}

	valuesSub, err := UnpackComplexWithSubstitution(field.Section7.Data, tmpl, field.Section5.NumberOfValues, 0, 0)
	if err != nil {
		t.Fatalf("UnpackComplexWithSubstitution: %v", err)
	}

	if len(valuesNaN) != len(valuesSub) {
		t.Fatalf("length mismatch: %d vs %d", len(valuesNaN), len(valuesSub))
	}

	for i := range valuesNaN {
		if valuesNaN[i] != valuesSub[i] {
			t.Errorf("value[%d]: %g != %g", i, valuesNaN[i], valuesSub[i])
		}
	}
	t.Logf("NoMissing: %d values identical between UnpackComplex and UnpackComplexWithSubstitution", len(valuesNaN))
}
