package grib2

import (
	"bytes"
	"os"
	"testing"
)

func TestReadSection2_SampleFile(t *testing.T) {
	f, err := os.Open("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Skip section 0 (16 bytes) and section 1 (21 bytes)
	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}
	if _, err := ReadSection1(f); err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}

	s2, err := ReadSection2(f)
	if err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	// Cross-validated against: grib_dump -O testdata/sample.grib2
	//   section2Length = 5
	//   numberOfSection = 2
	// No local use data.
	if s2.Length != 5 {
		t.Errorf("Length = %d, want 5", s2.Length)
	}
	if len(s2.Data) != 0 {
		t.Errorf("Data length = %d, want 0", len(s2.Data))
	}
}

func TestWriteSection2_RoundTrip(t *testing.T) {
	original := Section2{
		Length: 5,
		Data:   nil,
	}

	var buf bytes.Buffer
	if err := WriteSection2(&buf, original); err != nil {
		t.Fatalf("WriteSection2: %v", err)
	}

	if buf.Len() != 5 {
		t.Fatalf("wrote %d bytes, want 5", buf.Len())
	}

	decoded, err := ReadSection2(&buf)
	if err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	if decoded.Length != original.Length {
		t.Errorf("Length mismatch: got %d, want %d", decoded.Length, original.Length)
	}
}

func TestWriteSection2_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read sample.grib2: %v", err)
	}

	s2 := Section2{Data: nil}

	var buf bytes.Buffer
	if err := WriteSection2(&buf, s2); err != nil {
		t.Fatalf("WriteSection2: %v", err)
	}

	got := buf.Bytes()
	// Section 2 starts at byte 37 (16 + 21) and is 5 bytes
	want := raw[37 : 37+5]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}
