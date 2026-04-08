package grib2

import (
	"bytes"
	"os"
	"testing"
)

// ---- Template 3.90 (Space View Perspective) ----

func TestReadSection3_Template390(t *testing.T) {
	raw, err := os.ReadFile("testdata/space_view.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if len(msg.Fields) == 0 {
		t.Fatal("no fields in message")
	}
	s3 := msg.Fields[0].Section3

	// Cross-validated against: grib_dump -O testdata/space_view.grib2
	if s3.Length != 80 {
		t.Errorf("Length = %d, want 80", s3.Length)
	}
	if s3.Source != 0 {
		t.Errorf("Source = %d, want 0", s3.Source)
	}
	if s3.NumberOfDataPoints != 496 {
		t.Errorf("NumberOfDataPoints = %d, want 496", s3.NumberOfDataPoints)
	}
	if s3.TemplateNumber != 90 {
		t.Errorf("TemplateNumber = %d, want 90", s3.TemplateNumber)
	}

	tmpl, ok := s3.Template.(Template390)
	if !ok {
		t.Fatalf("Template type = %T, want Template390", s3.Template)
	}

	// Cross-validated against grib_dump -O:
	//   shapeOfTheEarth = 0
	//   Nx = 5424
	//   Ny = 5424
	//   latitudeOfSubSatellitePoint = 0
	//   longitudeOfSubSatellitePoint = 262150000
	//   resolutionAndComponentFlags = 48
	//   dx = 17630
	//   dy = 17630
	//   Xp = 2712000
	//   Yp = 2712000
	//   scanningMode = 0
	//   orientationOfTheGrid = 0
	//   Nr = 6610714
	//   Xo = 0
	//   Yo = 0
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ShapeOfEarth", tmpl.ShapeOfEarth, uint8(0)},
		{"ScaleFactorOfRadiusOfSphericalEarth", tmpl.ScaleFactorOfRadiusOfSphericalEarth, uint8(0xFF)},
		{"ScaledValueOfRadiusOfSphericalEarth", tmpl.ScaledValueOfRadiusOfSphericalEarth, uint32(0xFFFFFFFF)},
		{"Nx", tmpl.Nx, uint32(5424)},
		{"Ny", tmpl.Ny, uint32(5424)},
		{"LatitudeOfSubSatellitePoint", tmpl.LatitudeOfSubSatellitePoint, int32(0)},
		{"LongitudeOfSubSatellitePoint", tmpl.LongitudeOfSubSatellitePoint, int32(262150000)},
		{"ResolutionAndComponentFlags", tmpl.ResolutionAndComponentFlags, uint8(48)},
		{"Dx", tmpl.Dx, uint32(17630)},
		{"Dy", tmpl.Dy, uint32(17630)},
		{"Xp", tmpl.Xp, uint32(2712000)},
		{"Yp", tmpl.Yp, uint32(2712000)},
		{"ScanningMode", tmpl.ScanningMode, uint8(0)},
		{"OrientationOfTheGrid", tmpl.OrientationOfTheGrid, int32(0)},
		{"Nr", tmpl.Nr, uint32(6610714)},
		{"Xo", tmpl.Xo, uint32(0)},
		{"Yo", tmpl.Yo, uint32(0)},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestWriteSection3_Template390_RoundTrip(t *testing.T) {
	original := Section3{
		Source:                  0,
		NumberOfDataPoints:      29433600, // 5424*5424
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          90,
		Template: Template390{
			ShapeOfEarth:                        6, // WGS84 spheroid
			ScaleFactorOfRadiusOfSphericalEarth: 0xFF,
			ScaledValueOfRadiusOfSphericalEarth: 0xFFFFFFFF,
			ScaleFactorOfEarthMajorAxis:         0xFF,
			ScaledValueOfEarthMajorAxis:         0xFFFFFFFF,
			ScaleFactorOfEarthMinorAxis:         0xFF,
			ScaledValueOfEarthMinorAxis:         0xFFFFFFFF,
			Nx:                                  5424,
			Ny:                                  5424,
			LatitudeOfSubSatellitePoint:         0,
			LongitudeOfSubSatellitePoint:        262150000, // -97.85 degrees in GRIB convention
			ResolutionAndComponentFlags:         48,
			Dx:                                  17630,
			Dy:                                  17630,
			Xp:                                  2712000,
			Yp:                                  2712000,
			ScanningMode:                        0,
			OrientationOfTheGrid:                0,
			Nr:                                  6610714,
			Xo:                                  0,
			Yo:                                  0,
		},
	}

	var buf bytes.Buffer
	if err := WriteSection3(&buf, original); err != nil {
		t.Fatalf("WriteSection3: %v", err)
	}

	decoded, err := ReadSection3(&buf)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	// 14 (header) + 66 (template) = 80
	if decoded.Length != 80 {
		t.Errorf("Length = %d, want 80", decoded.Length)
	}
	if decoded.NumberOfDataPoints != original.NumberOfDataPoints {
		t.Errorf("NumberOfDataPoints = %d, want %d", decoded.NumberOfDataPoints, original.NumberOfDataPoints)
	}
	if decoded.TemplateNumber != 90 {
		t.Errorf("TemplateNumber = %d, want 90", decoded.TemplateNumber)
	}

	tmpl, ok := decoded.Template.(Template390)
	if !ok {
		t.Fatalf("Template type = %T, want Template390", decoded.Template)
	}
	orig := original.Template.(Template390)
	if tmpl != orig {
		t.Errorf("template mismatch:\n  got  %+v\n  want %+v", tmpl, orig)
	}
}

func TestWriteSection3_Template390_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/space_view.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	s3 := msg.Fields[0].Section3
	var buf bytes.Buffer
	if err := WriteSection3(&buf, s3); err != nil {
		t.Fatalf("WriteSection3: %v", err)
	}

	got := buf.Bytes()
	// Find section 3 in the raw file
	s3Offset := findSection3Offset(raw)
	if s3Offset < 0 {
		t.Fatal("could not find section 3 in raw data")
	}
	s3Len := int(readUint32(raw, s3Offset))
	want := raw[s3Offset : s3Offset+s3Len]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// TestWriteSection3_Template390_NegativeLat tests sign-magnitude encoding
// for negative latitude of sub-satellite point.
func TestWriteSection3_Template390_NegativeLat(t *testing.T) {
	original := Section3{
		Source:                  0,
		NumberOfDataPoints:      100,
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          90,
		Template: Template390{
			ShapeOfEarth:                        6,
			ScaleFactorOfRadiusOfSphericalEarth: 0xFF,
			ScaledValueOfRadiusOfSphericalEarth: 0xFFFFFFFF,
			ScaleFactorOfEarthMajorAxis:         0xFF,
			ScaledValueOfEarthMajorAxis:         0xFFFFFFFF,
			ScaleFactorOfEarthMinorAxis:         0xFF,
			ScaledValueOfEarthMinorAxis:         0xFFFFFFFF,
			Nx:                                  1000,
			Ny:                                  1000,
			LatitudeOfSubSatellitePoint:         -35000000, // -35 degrees
			LongitudeOfSubSatellitePoint:        140000000, // 140 degrees
			ResolutionAndComponentFlags:         48,
			Dx:                                  10000,
			Dy:                                  10000,
			Xp:                                  500000,
			Yp:                                  500000,
			ScanningMode:                        0,
			OrientationOfTheGrid:                -5000000, // -5 degrees
			Nr:                                  6610714,
			Xo:                                  0,
			Yo:                                  0,
		},
	}

	var buf bytes.Buffer
	if err := WriteSection3(&buf, original); err != nil {
		t.Fatalf("WriteSection3: %v", err)
	}

	decoded, err := ReadSection3(&buf)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	tmpl, ok := decoded.Template.(Template390)
	if !ok {
		t.Fatalf("Template type = %T, want Template390", decoded.Template)
	}
	if tmpl.LatitudeOfSubSatellitePoint != -35000000 {
		t.Errorf("LatitudeOfSubSatellitePoint = %d, want -35000000", tmpl.LatitudeOfSubSatellitePoint)
	}
	if tmpl.OrientationOfTheGrid != -5000000 {
		t.Errorf("OrientationOfTheGrid = %d, want -5000000", tmpl.OrientationOfTheGrid)
	}
}

// findSection3Offset finds the byte offset of section 3 in raw GRIB2 data.
func findSection3Offset(raw []byte) int {
	off := 16 // skip section 0 (indicator)
	for off < len(raw)-5 {
		secLen := int(readUint32(raw, off))
		secNo := int(raw[off+4])
		if secNo == 3 {
			return off
		}
		if secLen < 5 {
			break
		}
		off += secLen
	}
	return -1
}
