package grib2

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestReadSection1_SampleFile(t *testing.T) {
	f, err := os.Open("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Skip section 0 (16 bytes)
	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}

	s1, err := ReadSection1(f)
	if err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}

	// Cross-validated against: grib_dump -O testdata/sample.grib2
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Length", s1.Length, uint32(21)},
		{"Centre", s1.Centre, uint16(98)},       // ECMWF
		{"SubCentre", s1.SubCentre, uint16(0)},
		{"MasterTablesVersion", s1.MasterTablesVersion, uint8(4)},
		{"LocalTablesVersion", s1.LocalTablesVersion, uint8(0)},
		{"SignificanceOfReferenceTime", s1.SignificanceOfReferenceTime, uint8(1)}, // Start of forecast
		{"Year", s1.Year, uint16(2008)},
		{"Month", s1.Month, uint8(2)},
		{"Day", s1.Day, uint8(6)},
		{"Hour", s1.Hour, uint8(12)},
		{"Minute", s1.Minute, uint8(0)},
		{"Second", s1.Second, uint8(0)},
		{"ProductionStatus", s1.ProductionStatus, uint8(0)}, // Operational
		{"TypeOfData", s1.TypeOfData, uint8(2)},             // Analysis and forecast
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}

	// Verify reference time
	refTime := s1.ReferenceTime()
	wantTime := time.Date(2008, 2, 6, 12, 0, 0, 0, time.UTC)
	if !refTime.Equal(wantTime) {
		t.Errorf("ReferenceTime = %v, want %v", refTime, wantTime)
	}
}

func TestWriteSection1_RoundTrip(t *testing.T) {
	original := Section1{
		Length:                      21,
		Centre:                     98,
		SubCentre:                  0,
		MasterTablesVersion:        4,
		LocalTablesVersion:         0,
		SignificanceOfReferenceTime: 1,
		Year:                       2008,
		Month:                      2,
		Day:                        6,
		Hour:                       12,
		Minute:                     0,
		Second:                     0,
		ProductionStatus:           0,
		TypeOfData:                 2,
	}

	var buf bytes.Buffer
	if err := WriteSection1(&buf, original); err != nil {
		t.Fatalf("WriteSection1: %v", err)
	}

	if buf.Len() != section1MinLength {
		t.Fatalf("wrote %d bytes, want %d", buf.Len(), section1MinLength)
	}

	decoded, err := ReadSection1(&buf)
	if err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", decoded, original)
	}
}

func TestWriteSection1_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read sample.grib2: %v", err)
	}

	s1 := Section1{
		Centre:                     98,
		SubCentre:                  0,
		MasterTablesVersion:        4,
		LocalTablesVersion:         0,
		SignificanceOfReferenceTime: 1,
		Year:                       2008,
		Month:                      2,
		Day:                        6,
		Hour:                       12,
		Minute:                     0,
		Second:                     0,
		ProductionStatus:           0,
		TypeOfData:                 2,
	}

	var buf bytes.Buffer
	if err := WriteSection1(&buf, s1); err != nil {
		t.Fatalf("WriteSection1: %v", err)
	}

	got := buf.Bytes()
	// Section 1 starts at byte 16 (after section 0) and is 21 bytes
	want := raw[16 : 16+21]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}
