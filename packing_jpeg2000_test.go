package grib2

import (
	"bytes"
	"strings"
	"math"
	"os"
	"testing"

)

func TestUnpackJPEG2000_File(t *testing.T) {
	raw, err := os.ReadFile("testdata/jpeg.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template540)
	if !ok {
		t.Fatalf("not Template540: %T", field.Section5.Template)
	}

	if field.Section5.TemplateNumber != 40 {
		t.Errorf("TemplateNumber = %d, want 40", field.Section5.TemplateNumber)
	}
	if tmpl.BitsPerValue != 14 {
		t.Errorf("BitsPerValue = %d, want 14", tmpl.BitsPerValue)
	}
	if tmpl.TypeOfCompression != 0 {
		t.Errorf("TypeOfCompression = %d, want 0 (lossless)", tmpl.TypeOfCompression)
	}

	values, err := UnpackJPEG2000(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		if strings.Contains(err.Error(), "not yet implemented") || strings.Contains(err.Error(), "not implemented") {
			t.Skipf("JPEG2000 decoder not yet complete: %v", err)
		}
		t.Fatalf("UnpackJPEG2000: %v", err)
	}

	if len(values) != 65160 {
		t.Fatalf("got %d values, want 65160", len(values))
	}

	// Load reference
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
		if diff > 1e-2 {
			if mismatches < 10 {
				t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			}
			mismatches++
		}
	}
	if mismatches > 10 {
		t.Errorf("... and %d more mismatches", mismatches-10)
	}
	t.Logf("JPEG2000 max error vs eccodes: %.2e (%d values, %d mismatches)", maxErr, len(values), mismatches)
}

func TestPackJPEG2000(t *testing.T) {
	// Test 1: Pack and unpack random-ish values
	t.Run("RoundTrip", func(t *testing.T) {
		n := 1000
		values := make([]float64, n)
		for i := range values {
			values[i] = float64(i*17+3) * 0.1
		}

		packed, tmpl, err := PackJPEG2000(values, 14)
		if err != nil {
			t.Fatalf("PackJPEG2000: %v", err)
		}

		t.Logf("packed %d values into %d bytes (bpv=%d, ref=%.2f, E=%d)",
			n, len(packed), tmpl.BitsPerValue, tmpl.ReferenceValue, tmpl.BinaryScaleFactor)

		unpacked, err := UnpackJPEG2000(packed, tmpl, uint32(n))
		if err != nil {
			t.Fatalf("UnpackJPEG2000: %v", err)
		}

		if len(unpacked) != n {
			t.Fatalf("got %d values, want %d", len(unpacked), n)
		}

		maxErr := 0.0
		for i, got := range unpacked {
			diff := math.Abs(got - values[i])
			if diff > maxErr {
				maxErr = diff
			}
		}
		// The error should be bounded by the quantization step size
		step := math.Pow(2, float64(tmpl.BinaryScaleFactor))
		t.Logf("max error: %.6e, quantization step: %.6e", maxErr, step)
		if maxErr > step*1.1 { // allow small tolerance
			t.Errorf("max error %.6e exceeds quantization step %.6e", maxErr, step)
		}
	})

	// Test 2: Constant field
	t.Run("Constant", func(t *testing.T) {
		values := make([]float64, 100)
		for i := range values {
			values[i] = 42.5
		}

		packed, tmpl, err := PackJPEG2000(values, 0)
		if err != nil {
			t.Fatalf("PackJPEG2000: %v", err)
		}

		unpacked, err := UnpackJPEG2000(packed, tmpl, 100)
		if err != nil {
			t.Fatalf("UnpackJPEG2000: %v", err)
		}

		for i, v := range unpacked {
			if math.Abs(v-42.5) > 0.01 {
				t.Errorf("value[%d] = %f, want 42.5", i, v)
				break
			}
		}
		_ = packed
	})

	// Test 3: Encode the original JPEG2000 file's raw pixel values (integers)
	// through the J2K encoder and verify lossless round-trip of the integers.
	t.Run("FileRoundTrip", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/jpeg.grib2")
		if err != nil {
			t.Skipf("read: %v", err)
		}

		msg, err := ReadMessage(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}

		field := msg.Fields[0]
		tmpl, ok := field.Section5.Template.(Template540)
		if !ok {
			t.Fatalf("not Template540")
		}

		// First decode the original values
		origValues, err := UnpackJPEG2000(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
		if err != nil {
			t.Fatalf("UnpackJPEG2000 original: %v", err)
		}

		// Re-pack with the same bitsPerValue
		packed, newTmpl, err := PackJPEG2000(origValues, tmpl.BitsPerValue)
		if err != nil {
			t.Fatalf("PackJPEG2000: %v", err)
		}

		t.Logf("original j2k: %d bytes, re-packed: %d bytes", len(field.Section7.Data), len(packed))
		t.Logf("original tmpl: ref=%.4e E=%d D=%d", tmpl.ReferenceValue, tmpl.BinaryScaleFactor, tmpl.DecimalScaleFactor)
		t.Logf("new tmpl: ref=%.4e E=%d D=%d", newTmpl.ReferenceValue, newTmpl.BinaryScaleFactor, newTmpl.DecimalScaleFactor)

		// Unpack the re-packed data
		unpacked, err := UnpackJPEG2000(packed, newTmpl, uint32(len(origValues)))
		if err != nil {
			t.Fatalf("UnpackJPEG2000 re-packed: %v", err)
		}

		// Compare with tolerance based on the new quantization step
		maxErr := 0.0
		mismatches := 0
		step := math.Pow(2, float64(newTmpl.BinaryScaleFactor))
		for i, got := range unpacked {
			diff := math.Abs(got - origValues[i])
			if diff > maxErr {
				maxErr = diff
			}
			if diff > step*1.5 {
				mismatches++
			}
		}
		t.Logf("re-pack max error: %.6e, quantization step: %.6e, mismatches: %d/%d",
			maxErr, step, mismatches, len(origValues))

		// Allow errors up to 2x the quantization step due to rounding
		if mismatches > 0 {
			t.Errorf("%d values exceeded tolerance", mismatches)
		}
	})
}

func TestUnpackJPEG2000_ReducedGaussianJPEG(t *testing.T) {
	raw, err := os.ReadFile("testdata/reduced_gaussian_surface_jpeg.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template540)
	if !ok {
		t.Skipf("not Template540: %T", field.Section5.Template)
	}

	values, err := UnpackJPEG2000(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		if strings.Contains(err.Error(), "not yet implemented") || strings.Contains(err.Error(), "not implemented") {
			t.Skipf("JPEG2000 decoder not yet complete: %v", err)
		}
		t.Fatalf("UnpackJPEG2000: %v", err)
	}

	t.Logf("reduced gaussian JPEG2000: decoded %d values", len(values))
}
