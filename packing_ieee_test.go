package grib2

import (
	"bytes"
	"math"
	"os"
	"testing"
)

func TestUnpackIEEE_File(t *testing.T) {
	raw, err := os.ReadFile("testdata/ieee.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template54)
	if !ok {
		t.Fatalf("not Template54: %T", field.Section5.Template)
	}

	if field.Section5.TemplateNumber != 4 {
		t.Errorf("TemplateNumber = %d, want 4", field.Section5.TemplateNumber)
	}
	if tmpl.Precision != 1 {
		t.Errorf("Precision = %d, want 1 (IEEE 32-bit)", tmpl.Precision)
	}

	values, err := UnpackIEEE(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackIEEE: %v", err)
	}

	if len(values) != 496 {
		t.Fatalf("got %d values, want 496", len(values))
	}

	// Load reference
	refValues := loadReferenceValues(t, "testdata/reference/ieee.grib2.values")
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
	t.Logf("IEEE max error vs eccodes: %.2e (%d values)", maxErr, len(values))
}

func TestPackIEEE_RoundTrip32(t *testing.T) {
	values := []float64{273.15, 300.0, 250.5, 0.0, -10.0, 1e10, 1e-10}

	packed, tmpl, err := PackIEEE(values, 1)
	if err != nil {
		t.Fatalf("PackIEEE: %v", err)
	}

	decoded, err := UnpackIEEE(packed, tmpl, uint32(len(values)))
	if err != nil {
		t.Fatalf("UnpackIEEE: %v", err)
	}

	for i, orig := range values {
		// float32 precision loss
		want := float64(float32(orig))
		if decoded[i] != want {
			t.Errorf("value[%d] = %v, want %v", i, decoded[i], want)
		}
	}
}

func TestPackIEEE_RoundTrip64(t *testing.T) {
	values := []float64{273.15, 300.0, 250.5, 0.0, -10.0, 1e10, 1e-10, math.Pi}

	packed, tmpl, err := PackIEEE(values, 2)
	if err != nil {
		t.Fatalf("PackIEEE: %v", err)
	}

	decoded, err := UnpackIEEE(packed, tmpl, uint32(len(values)))
	if err != nil {
		t.Fatalf("UnpackIEEE: %v", err)
	}

	for i, orig := range values {
		if decoded[i] != orig {
			t.Errorf("value[%d] = %v, want %v", i, decoded[i], orig)
		}
	}
}
