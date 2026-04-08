package grib2

import (
	"bytes"
	"math"
	"os"
	"testing"
)

func TestUnpackPNG_ConstantField(t *testing.T) {
	// png.grib2 is a constant field (bitsPerValue=0)
	raw, err := os.ReadFile("testdata/png.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
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

	if field.Section5.TemplateNumber != 41 {
		t.Errorf("TemplateNumber = %d, want 41", field.Section5.TemplateNumber)
	}

	values, err := UnpackPNG(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackPNG: %v", err)
	}

	if len(values) != 496 {
		t.Fatalf("got %d values, want 496", len(values))
	}

	// Load reference
	refValues := loadReferenceValues(t, "testdata/reference/png.grib2.values")
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
			t.Errorf("value[%d] = %.10e, want %.10e", i, got, want)
			if i > 10 {
				t.Fatal("too many errors")
			}
		}
	}
	t.Logf("PNG constant field max error: %.2e (%d values)", maxErr, len(values))
}

func TestPackPNG_RoundTrip(t *testing.T) {
	// Use our simple-packed sample values as input
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	origTmpl, _ := field.Section5.Template.(Template50)
	origValues, err := UnpackSimple(field.Section7.Data, origTmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackSimple: %v", err)
	}

	// Pack to PNG with 16 bits per value
	packed, tmpl, err := PackPNG(origValues, 16, 16, 31)
	if err != nil {
		t.Fatalf("PackPNG: %v", err)
	}

	// Unpack PNG
	reValues, err := UnpackPNG(packed, tmpl, uint32(len(origValues)))
	if err != nil {
		t.Fatalf("UnpackPNG: %v", err)
	}

	if len(reValues) != len(origValues) {
		t.Fatalf("round-trip: got %d values, want %d", len(reValues), len(origValues))
	}

	// Check values are close (within quantization error)
	maxErr := 0.0
	for i, orig := range origValues {
		diff := math.Abs(orig - reValues[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	quantErr := math.Pow(2, float64(tmpl.BinaryScaleFactor))
	t.Logf("PNG round-trip max error: %.6e, quantization bound: %.6e", maxErr, quantErr)
	if maxErr > quantErr*1.01 {
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}
