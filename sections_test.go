package grib2

import (
	"bytes"
	"os"
	"testing"
)

// TestReadAllSections_SampleFile parses every section of sample.grib2
// and cross-validates against grib_dump -O output.
func TestReadAllSections_SampleFile(t *testing.T) {
	f, err := os.Open("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Section 0
	s0, err := ReadSection0(f)
	if err != nil {
		t.Fatalf("Section0: %v", err)
	}
	if s0.TotalLength != 1179 {
		t.Errorf("S0.TotalLength = %d, want 1179", s0.TotalLength)
	}

	// Section 1
	s1, err := ReadSection1(f)
	if err != nil {
		t.Fatalf("Section1: %v", err)
	}
	if s1.Centre != 98 {
		t.Errorf("S1.Centre = %d, want 98", s1.Centre)
	}

	// Section 2
	s2, err := ReadSection2(f)
	if err != nil {
		t.Fatalf("Section2: %v", err)
	}
	if s2.Length != 5 {
		t.Errorf("S2.Length = %d, want 5", s2.Length)
	}

	// Section 3
	s3, err := ReadSection3(f)
	if err != nil {
		t.Fatalf("Section3: %v", err)
	}
	if s3.NumberOfDataPoints != 496 {
		t.Errorf("S3.NumberOfDataPoints = %d, want 496", s3.NumberOfDataPoints)
	}

	// Section 4
	s4, err := ReadSection4(f)
	if err != nil {
		t.Fatalf("Section4: %v", err)
	}
	// sample.grib2 uses template 4.1 (ensemble)
	if s4.TemplateNumber != 1 {
		t.Errorf("S4.TemplateNumber = %d, want 1", s4.TemplateNumber)
	}
	if s4.Length != 37 {
		t.Errorf("S4.Length = %d, want 37", s4.Length)
	}
	tmpl41, ok := s4.Template.(Template41)
	if !ok {
		t.Fatalf("S4.Template type = %T, want Template41", s4.Template)
	}
	// grib_dump: parameterCategory = 0, parameterNumber = 0 (Temperature)
	if tmpl41.ParameterCategory != 0 {
		t.Errorf("ParameterCategory = %d, want 0", tmpl41.ParameterCategory)
	}
	if tmpl41.ParameterNumber != 0 {
		t.Errorf("ParameterNumber = %d, want 0", tmpl41.ParameterNumber)
	}
	if tmpl41.TypeOfGeneratingProcess != 0 {
		t.Errorf("TypeOfGeneratingProcess = %d, want 0", tmpl41.TypeOfGeneratingProcess)
	}
	if tmpl41.GeneratingProcessID != 130 {
		t.Errorf("GeneratingProcessID = %d, want 130", tmpl41.GeneratingProcessID)
	}
	if tmpl41.ForecastTime != 0 {
		t.Errorf("ForecastTime = %d, want 0", tmpl41.ForecastTime)
	}

	// Section 5
	s5, err := ReadSection5(f)
	if err != nil {
		t.Fatalf("Section5: %v", err)
	}
	if s5.Length != 21 {
		t.Errorf("S5.Length = %d, want 21", s5.Length)
	}
	if s5.NumberOfValues != 496 {
		t.Errorf("S5.NumberOfValues = %d, want 496", s5.NumberOfValues)
	}
	if s5.TemplateNumber != 0 {
		t.Errorf("S5.TemplateNumber = %d, want 0 (simple packing)", s5.TemplateNumber)
	}
	tmpl50, ok := s5.Template.(Template50)
	if !ok {
		t.Fatalf("S5.Template type = %T, want Template50", s5.Template)
	}
	// grib_dump: referenceValue = 270.467, binaryScaleFactor = -10,
	//            decimalScaleFactor = 0, bitsPerValue = 16
	if tmpl50.BinaryScaleFactor != -10 {
		t.Errorf("BinaryScaleFactor = %d, want -10", tmpl50.BinaryScaleFactor)
	}
	if tmpl50.DecimalScaleFactor != 0 {
		t.Errorf("DecimalScaleFactor = %d, want 0", tmpl50.DecimalScaleFactor)
	}
	if tmpl50.BitsPerValue != 16 {
		t.Errorf("BitsPerValue = %d, want 16", tmpl50.BitsPerValue)
	}

	// Section 6
	s6, err := ReadSection6(f)
	if err != nil {
		t.Fatalf("Section6: %v", err)
	}
	if s6.Length != 6 {
		t.Errorf("S6.Length = %d, want 6", s6.Length)
	}
	if s6.Indicator != 255 {
		t.Errorf("S6.Indicator = %d, want 255 (no bitmap)", s6.Indicator)
	}

	// Section 7
	s7, err := ReadSection7(f)
	if err != nil {
		t.Fatalf("Section7: %v", err)
	}
	if s7.Length != 997 {
		t.Errorf("S7.Length = %d, want 997", s7.Length)
	}
	// 997 - 5 (header) = 992 bytes of packed data
	if len(s7.Data) != 992 {
		t.Errorf("S7.Data length = %d, want 992", len(s7.Data))
	}

	// Section 8
	if err := ReadSection8(f); err != nil {
		t.Fatalf("Section8: %v", err)
	}

	// Verify total: 16 + 21 + 5 + 72 + 37 + 21 + 6 + 997 + 4 = 1179
	total := uint64(16 + s1.Length + s2.Length + s3.Length + s4.Length + s5.Length + s6.Length + s7.Length + 4)
	if total != s0.TotalLength {
		t.Errorf("sum of sections = %d, want %d (from S0.TotalLength)", total, s0.TotalLength)
	}
}

// TestWriteAllSections_ByteExact writes all sections back and compares
// against the original file byte-for-byte.
func TestWriteAllSections_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// First read everything
	r := bytes.NewReader(raw)
	s0, _ := ReadSection0(r)
	s1, _ := ReadSection1(r)
	s2, _ := ReadSection2(r)
	s3, _ := ReadSection3(r)
	s4, _ := ReadSection4(r)
	s5, _ := ReadSection5(r)
	s6, _ := ReadSection6(r)
	s7, _ := ReadSection7(r)

	// Write everything back
	var buf bytes.Buffer
	if err := WriteSection0(&buf, s0); err != nil {
		t.Fatalf("WriteS0: %v", err)
	}
	if err := WriteSection1(&buf, s1); err != nil {
		t.Fatalf("WriteS1: %v", err)
	}
	if err := WriteSection2(&buf, s2); err != nil {
		t.Fatalf("WriteS2: %v", err)
	}
	if err := WriteSection3(&buf, s3); err != nil {
		t.Fatalf("WriteS3: %v", err)
	}
	if err := WriteSection4(&buf, s4); err != nil {
		t.Fatalf("WriteS4: %v", err)
	}
	if err := WriteSection5(&buf, s5); err != nil {
		t.Fatalf("WriteS5: %v", err)
	}
	if err := WriteSection6(&buf, s6); err != nil {
		t.Fatalf("WriteS6: %v", err)
	}
	if err := WriteSection7(&buf, s7); err != nil {
		t.Fatalf("WriteS7: %v", err)
	}
	if err := WriteSection8(&buf); err != nil {
		t.Fatalf("WriteS8: %v", err)
	}

	got := buf.Bytes()
	if !bytes.Equal(got, raw) {
		// Find first differing byte
		for i := range got {
			if i >= len(raw) || got[i] != raw[i] {
				t.Fatalf("first diff at byte %d: got 0x%02x, want 0x%02x", i, got[i], raw[i])
			}
		}
		if len(got) != len(raw) {
			t.Fatalf("length mismatch: got %d, want %d", len(got), len(raw))
		}
	}
}
