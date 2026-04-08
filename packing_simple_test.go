package grib2

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// loadReferenceValues loads the eccodes reference values from
// testdata/reference/sample.grib2.values (grib_get_data output format).
func loadReferenceValues(t *testing.T, path string) []float64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open reference values: %v", err)
	}
	defer f.Close()

	var values []float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Latitude") {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		v, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			t.Fatalf("parse float %q: %v", fields[2], err)
		}
		values = append(values, v)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}
	return values
}

func TestUnpackSimple_SampleFile(t *testing.T) {
	// Read the message
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

	// Unpack
	values, err := UnpackSimple(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackSimple: %v", err)
	}

	if len(values) != 496 {
		t.Fatalf("got %d values, want 496", len(values))
	}

	// Load eccodes reference values
	refValues := loadReferenceValues(t, "testdata/reference/sample.grib2.values")
	if len(refValues) != len(values) {
		t.Fatalf("reference has %d values, got %d", len(refValues), len(values))
	}

	// Compare — they should match exactly (both are computing the same formula)
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

	// Spot check first few values from grib_dump output
	expected := []float64{
		2.7900000000e+02, 2.7996093750e+02, 2.7853125000e+02,
		2.7516503906e+02, 2.7046679688e+02, 2.7419140625e+02,
	}
	for i, want := range expected {
		if math.Abs(values[i]-want) > 1e-6 {
			t.Errorf("spot check[%d] = %.10e, want %.10e", i, values[i], want)
		}
	}
}

func TestPackSimple_RoundTrip(t *testing.T) {
	// Read and unpack sample.grib2
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

	// Re-pack with same bits per value
	packed, newTmpl, err := PackSimple(origValues, origTmpl.BitsPerValue)
	if err != nil {
		t.Fatalf("PackSimple: %v", err)
	}

	// Unpack the re-packed data
	reValues, err := UnpackSimple(packed, newTmpl, uint32(len(origValues)))
	if err != nil {
		t.Fatalf("UnpackSimple (re-packed): %v", err)
	}

	// Values should be very close (within the quantization error of the packing)
	maxErr := 0.0
	for i, orig := range origValues {
		diff := math.Abs(orig - reValues[i])
		if diff > maxErr {
			maxErr = diff
		}
	}

	// The quantization error bound is: 2^E (where E is binary scale factor)
	quantErr := math.Pow(2, float64(newTmpl.BinaryScaleFactor))
	t.Logf("round-trip max error: %.6e, quantization bound: %.6e", maxErr, quantErr)

	if maxErr > quantErr*1.01 { // allow tiny float rounding
		t.Errorf("round-trip error %.6e exceeds quantization bound %.6e", maxErr, quantErr)
	}
}

func TestExtractBits(t *testing.T) {
	// Test bit extraction with known values
	data := []byte{0xAB, 0xCD} // 1010_1011 1100_1101

	tests := []struct {
		offset uint64
		width  int
		want   uint64
	}{
		{0, 8, 0xAB},
		{8, 8, 0xCD},
		{0, 4, 0x0A}, // 1010
		{4, 4, 0x0B}, // 1011
		{0, 16, 0xABCD},
		{4, 8, 0xBC}, // 1011_1100
	}

	for _, tt := range tests {
		got := extractBits(data, tt.offset, tt.width)
		if got != tt.want {
			t.Errorf("extractBits(offset=%d, width=%d) = 0x%X, want 0x%X", tt.offset, tt.width, got, tt.want)
		}
	}
}

func TestStoreBits(t *testing.T) {
	data := make([]byte, 2)
	storeBits(data, 0, 16, 0xABCD)
	if data[0] != 0xAB || data[1] != 0xCD {
		t.Errorf("storeBits = %X, want ABCD", data)
	}

	data = make([]byte, 2)
	storeBits(data, 4, 8, 0xBC)
	want := []byte{0x0B, 0xC0}
	if !bytes.Equal(data, want) {
		t.Errorf("storeBits(offset=4) = %X, want %X", data, want)
	}
}

func TestUnpackSimple_ConstantField(t *testing.T) {
	tmpl := Template50{
		ReferenceValue:    300.0,
		BinaryScaleFactor: 0,
		DecimalScaleFactor: 0,
		BitsPerValue:      0, // constant field
	}

	values, err := UnpackSimple(nil, tmpl, 100)
	if err != nil {
		t.Fatalf("UnpackSimple: %v", err)
	}

	for i, v := range values {
		if v != 300.0 {
			t.Errorf("value[%d] = %f, want 300.0", i, v)
		}
	}
}

func TestUnpackSimple_RegularLatLonFile(t *testing.T) {
	// Also validate against regular_latlon_surface.grib2
	raw, err := os.ReadFile("testdata/regular_latlon_surface.grib2")
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
		t.Skipf("not Template50: %T", field.Section5.Template)
	}

	values, err := UnpackSimple(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackSimple: %v", err)
	}

	// Cross-validate with eccodes
	_ = saveAndCompareWithEccodes(t, "regular_latlon_surface.grib2", values)
}

// saveAndCompareWithEccodes generates eccodes reference data and compares.
// Returns max absolute error.
func saveAndCompareWithEccodes(t *testing.T, filename string, values []float64) float64 {
	t.Helper()

	// Check if we have a reference file
	refPath := fmt.Sprintf("testdata/reference/%s.values", filename)
	if _, err := os.Stat(refPath); os.IsNotExist(err) {
		t.Skipf("no reference file at %s (generate with grib_get_data)", refPath)
	}

	refValues := loadReferenceValues(t, refPath)
	if len(refValues) != len(values) {
		t.Fatalf("reference has %d values, got %d", len(refValues), len(values))
	}

	maxErr := 0.0
	for i, got := range values {
		diff := math.Abs(got - refValues[i])
		if diff > maxErr {
			maxErr = diff
		}
		if diff > 1e-4 {
			t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, refValues[i], diff)
			if i > 10 {
				t.Fatal("too many errors")
			}
		}
	}
	return maxErr
}
