package grib2

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestReadSection0_SampleFile(t *testing.T) {
	f, err := os.Open("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("open sample.grib2: %v", err)
	}
	defer f.Close()

	s0, err := ReadSection0(f)
	if err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}

	// Cross-validated against: grib_dump -O testdata/sample.grib2
	//   1-4       identifier = GRIB
	//   5-6       reserved = MISSING (0xFF 0xFF)
	//   7         discipline = 0 [Meteorological products]
	//   8         editionNumber = 2
	//   9-16      totalLength = 1179
	if s0.Reserved != [2]byte{0xFF, 0xFF} {
		t.Errorf("Reserved = %x, want [ff ff]", s0.Reserved)
	}
	if s0.Discipline != 0 {
		t.Errorf("Discipline = %d, want 0 (Meteorological)", s0.Discipline)
	}
	if s0.Edition != 2 {
		t.Errorf("Edition = %d, want 2", s0.Edition)
	}
	if s0.TotalLength != 1179 {
		t.Errorf("TotalLength = %d, want 1179", s0.TotalLength)
	}
}

// TestReadSection0_MercatorReservedZero verifies that a file with reserved=0x00,0x00
// (as opposed to the common 0xFF,0xFF) round-trips correctly through Section0.
func TestReadSection0_MercatorReservedZero(t *testing.T) {
	raw, err := os.ReadFile("testdata/mercator.grib2")
	if err != nil {
		t.Fatalf("read mercator.grib2: %v", err)
	}

	s0, err := ReadSection0(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}

	// Cross-validated against: grib_dump -O testdata/mercator.grib2
	//   5-6       reserved = 0
	if s0.Reserved != [2]byte{0x00, 0x00} {
		t.Errorf("Reserved = %x, want [00 00]", s0.Reserved)
	}
	if s0.Discipline != 0 {
		t.Errorf("Discipline = %d, want 0", s0.Discipline)
	}
	if s0.TotalLength != 6339 {
		t.Errorf("TotalLength = %d, want 6339", s0.TotalLength)
	}

	// Round-trip: write back and compare first 16 bytes
	var buf bytes.Buffer
	if err := WriteSection0(&buf, s0); err != nil {
		t.Fatalf("WriteSection0: %v", err)
	}
	got := buf.Bytes()
	want := raw[:16]
	if !bytes.Equal(got, want) {
		t.Errorf("round-trip byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

func TestReadSection0_JPEG(t *testing.T) {
	f, err := os.Open("testdata/jpeg.grib2")
	if err != nil {
		t.Fatalf("open jpeg.grib2: %v", err)
	}
	defer f.Close()

	s0, err := ReadSection0(f)
	if err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}

	// jpeg.grib2 is 40526 bytes (per download), discipline=0, edition=2
	if s0.Discipline != 0 {
		t.Errorf("Discipline = %d, want 0", s0.Discipline)
	}
	if s0.Edition != 2 {
		t.Errorf("Edition = %d, want 2", s0.Edition)
	}
	if s0.TotalLength != 40526 {
		t.Errorf("TotalLength = %d, want 40526", s0.TotalLength)
	}
}

func TestReadSection0_NotGRIB(t *testing.T) {
	r := bytes.NewReader([]byte("NOT A GRIB FILE AT ALL"))
	_, err := ReadSection0(r)
	if err == nil {
		t.Fatal("expected error for non-GRIB data")
	}
	if err != ErrNotGRIB {
		t.Errorf("error = %v, want ErrNotGRIB", err)
	}
}

func TestReadSection0_Edition1(t *testing.T) {
	var buf [16]byte
	copy(buf[0:4], "GRIB")
	buf[6] = 0
	buf[7] = 1 // edition 1
	binary.BigEndian.PutUint64(buf[8:16], 100)

	_, err := ReadSection0(bytes.NewReader(buf[:]))
	if err == nil {
		t.Fatal("expected error for edition 1")
	}
}

func TestReadSection0_Truncated(t *testing.T) {
	_, err := ReadSection0(bytes.NewReader([]byte("GRIB")))
	if err == nil {
		t.Fatal("expected error for truncated input")
	}
}

func TestWriteSection0_RoundTrip(t *testing.T) {
	original := Section0{
		Discipline:  0,
		Edition:     2,
		TotalLength: 1179,
	}

	var buf bytes.Buffer
	if err := WriteSection0(&buf, original); err != nil {
		t.Fatalf("WriteSection0: %v", err)
	}

	if buf.Len() != section0Length {
		t.Fatalf("wrote %d bytes, want %d", buf.Len(), section0Length)
	}

	decoded, err := ReadSection0(&buf)
	if err != nil {
		t.Fatalf("ReadSection0 on round-trip: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", decoded, original)
	}
}

func TestWriteSection0_ByteExact(t *testing.T) {
	// Write and compare against the actual first 16 bytes of sample.grib2
	f, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read sample.grib2: %v", err)
	}

	s0 := Section0{
		Reserved:    [2]byte{0xFF, 0xFF},
		Discipline:  0,
		Edition:     2,
		TotalLength: 1179,
	}

	var buf bytes.Buffer
	if err := WriteSection0(&buf, s0); err != nil {
		t.Fatalf("WriteSection0: %v", err)
	}

	got := buf.Bytes()
	want := f[:16]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}
