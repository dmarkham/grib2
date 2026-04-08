package grib2

import (
	"bytes"
	"math"
	"os"
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
			ReferenceValue:    300.0,
			BinaryScaleFactor: 0,
			DecimalScaleFactor: 0,
			BitsPerValue:      0,
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
