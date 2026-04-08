package grib2

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestReadSection3_SampleFile(t *testing.T) {
	f, err := os.Open("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Skip sections 0, 1, 2
	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}
	if _, err := ReadSection1(f); err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}
	if _, err := ReadSection2(f); err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	s3, err := ReadSection3(f)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	// Cross-validated against: grib_dump -O testdata/sample.grib2
	if s3.Length != 72 {
		t.Errorf("Length = %d, want 72", s3.Length)
	}
	if s3.Source != 0 {
		t.Errorf("Source = %d, want 0", s3.Source)
	}
	if s3.NumberOfDataPoints != 496 {
		t.Errorf("NumberOfDataPoints = %d, want 496", s3.NumberOfDataPoints)
	}
	if s3.OctetsForNumberOfPoints != 0 {
		t.Errorf("OctetsForNumberOfPoints = %d, want 0", s3.OctetsForNumberOfPoints)
	}
	if s3.InterpretationOfPoints != 0 {
		t.Errorf("InterpretationOfPoints = %d, want 0", s3.InterpretationOfPoints)
	}
	if s3.TemplateNumber != 0 {
		t.Errorf("TemplateNumber = %d, want 0", s3.TemplateNumber)
	}

	tmpl, ok := s3.Template.(Template30)
	if !ok {
		t.Fatalf("Template type = %T, want Template30", s3.Template)
	}

	// Cross-validated against grib_dump -O:
	//   15        shapeOfTheEarth = 0
	//   31-34     Ni = 16
	//   35-38     Nj = 31
	//   39-42     basicAngleOfTheInitialProductionDomain = 0
	//   43-46     subdivisionsOfBasicAngle = MISSING (0xFFFFFFFF)
	//   47-50     latitudeOfFirstGridPoint = 60000000
	//   51-54     longitudeOfFirstGridPoint = 0
	//   55        resolutionAndComponentFlags = 48
	//   56-59     latitudeOfLastGridPoint = 0
	//   60-63     longitudeOfLastGridPoint = 30000000
	//   64-67     iDirectionIncrement = 2000000
	//   68-71     jDirectionIncrement = 2000000
	//   72        scanningMode = 0
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ShapeOfEarth", tmpl.ShapeOfEarth, uint8(0)},
		{"Ni", tmpl.Ni, uint32(16)},
		{"Nj", tmpl.Nj, uint32(31)},
		{"BasicAngle", tmpl.BasicAngle, uint32(0)},
		{"SubdivisionsOfBasicAngle", tmpl.SubdivisionsOfBasicAngle, uint32(0xFFFFFFFF)}, // MISSING
		{"LatitudeOfFirstGridPoint", tmpl.LatitudeOfFirstGridPoint, int32(60000000)},
		{"LongitudeOfFirstGridPoint", tmpl.LongitudeOfFirstGridPoint, int32(0)},
		{"ResolutionAndComponentFlags", tmpl.ResolutionAndComponentFlags, uint8(48)},
		{"LatitudeOfLastGridPoint", tmpl.LatitudeOfLastGridPoint, int32(0)},
		{"LongitudeOfLastGridPoint", tmpl.LongitudeOfLastGridPoint, int32(30000000)},
		{"IDirectionIncrement", tmpl.IDirectionIncrement, uint32(2000000)},
		{"JDirectionIncrement", tmpl.JDirectionIncrement, uint32(2000000)},
		{"ScanningMode", tmpl.ScanningMode, uint8(0)},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestWriteSection3_RoundTrip(t *testing.T) {
	original := Section3{
		Source:                  0,
		NumberOfDataPoints:      496,
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          0,
		Template: Template30{
			ShapeOfEarth:                       0,
			ScaleFactorOfRadiusOfSphericalEarth: 0xFF,
			ScaledValueOfRadiusOfSphericalEarth: 0xFFFFFFFF,
			ScaleFactorOfEarthMajorAxis:        0xFF,
			ScaledValueOfEarthMajorAxis:        0xFFFFFFFF,
			ScaleFactorOfEarthMinorAxis:        0xFF,
			ScaledValueOfEarthMinorAxis:        0xFFFFFFFF,
			Ni:                                 16,
			Nj:                                 31,
			BasicAngle:                         0,
			SubdivisionsOfBasicAngle:           0xFFFFFFFF,
			LatitudeOfFirstGridPoint:           60000000,
			LongitudeOfFirstGridPoint:          0,
			ResolutionAndComponentFlags:        48,
			LatitudeOfLastGridPoint:            0,
			LongitudeOfLastGridPoint:           30000000,
			IDirectionIncrement:                2000000,
			JDirectionIncrement:                2000000,
			ScanningMode:                       0,
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

	if decoded.Length != 72 {
		t.Errorf("Length = %d, want 72", decoded.Length)
	}
	if decoded.NumberOfDataPoints != original.NumberOfDataPoints {
		t.Errorf("NumberOfDataPoints = %d, want %d", decoded.NumberOfDataPoints, original.NumberOfDataPoints)
	}

	tmpl, ok := decoded.Template.(Template30)
	if !ok {
		t.Fatalf("Template type = %T, want Template30", decoded.Template)
	}
	orig := original.Template.(Template30)
	if tmpl != orig {
		t.Errorf("template mismatch:\n  got  %+v\n  want %+v", tmpl, orig)
	}
}

func TestWriteSection3_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	s3 := Section3{
		Source:                  0,
		NumberOfDataPoints:      496,
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          0,
		Template: Template30{
			ShapeOfEarth:                       0,
			ScaleFactorOfRadiusOfSphericalEarth: 0xFF,
			ScaledValueOfRadiusOfSphericalEarth: 0xFFFFFFFF,
			ScaleFactorOfEarthMajorAxis:        0xFF,
			ScaledValueOfEarthMajorAxis:        0xFFFFFFFF,
			ScaleFactorOfEarthMinorAxis:        0xFF,
			ScaledValueOfEarthMinorAxis:        0xFFFFFFFF,
			Ni:                                 16,
			Nj:                                 31,
			BasicAngle:                         0,
			SubdivisionsOfBasicAngle:           0xFFFFFFFF,
			LatitudeOfFirstGridPoint:           60000000,
			LongitudeOfFirstGridPoint:          0,
			ResolutionAndComponentFlags:        48,
			LatitudeOfLastGridPoint:            0,
			LongitudeOfLastGridPoint:           30000000,
			IDirectionIncrement:                2000000,
			JDirectionIncrement:                2000000,
			ScanningMode:                       0,
		},
	}

	var buf bytes.Buffer
	if err := WriteSection3(&buf, s3); err != nil {
		t.Fatalf("WriteSection3: %v", err)
	}

	got := buf.Bytes()
	// Section 3 starts at offset 42 (16 + 21 + 5) and is 72 bytes
	want := raw[42 : 42+72]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// ---- Template 3.1 (Rotated Lat/Lon) ----

func TestReadSection3_Template31_ConstantField(t *testing.T) {
	// constant_field.grib2 uses template 3.1 (rotated lat/lon) with no section 2.
	raw, err := os.ReadFile("testdata/constant_field.grib2")
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

	// Cross-validated against: grib_dump -O testdata/constant_field.grib2
	if s3.Length != 84 {
		t.Errorf("Length = %d, want 84", s3.Length)
	}
	if s3.Source != 0 {
		t.Errorf("Source = %d, want 0", s3.Source)
	}
	if s3.NumberOfDataPoints != 99200 {
		t.Errorf("NumberOfDataPoints = %d, want 99200", s3.NumberOfDataPoints)
	}
	if s3.TemplateNumber != 1 {
		t.Errorf("TemplateNumber = %d, want 1", s3.TemplateNumber)
	}

	tmpl, ok := s3.Template.(Template31)
	if !ok {
		t.Fatalf("Template type = %T, want Template31", s3.Template)
	}

	// Cross-validated against grib_dump -O:
	//   shapeOfTheEarth = 6
	//   Ni = 248, Nj = 400
	//   basicAngle = 0, subdivisionsOfBasicAngle = MISSING
	//   latitudeOfFirstGridPoint = -13250000
	//   longitudeOfFirstGridPoint = 5750000
	//   resolutionAndComponentFlags = 56
	//   latitudeOfLastGridPoint = 26650000
	//   longitudeOfLastGridPoint = 30450000
	//   iDirectionIncrement = 100000
	//   jDirectionIncrement = 100000
	//   scanningMode = 64
	//   latitudeOfSouthernPole = -22000000
	//   longitudeOfSouthernPole = 320000000
	//   angleOfRotation = 0
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ShapeOfEarth", tmpl.ShapeOfEarth, uint8(6)},
		{"Ni", tmpl.Ni, uint32(248)},
		{"Nj", tmpl.Nj, uint32(400)},
		{"BasicAngle", tmpl.BasicAngle, uint32(0)},
		{"SubdivisionsOfBasicAngle", tmpl.SubdivisionsOfBasicAngle, uint32(0xFFFFFFFF)},
		{"LatitudeOfFirstGridPoint", tmpl.LatitudeOfFirstGridPoint, int32(-13250000)},
		{"LongitudeOfFirstGridPoint", tmpl.LongitudeOfFirstGridPoint, int32(5750000)},
		{"ResolutionAndComponentFlags", tmpl.ResolutionAndComponentFlags, uint8(56)},
		{"LatitudeOfLastGridPoint", tmpl.LatitudeOfLastGridPoint, int32(26650000)},
		{"LongitudeOfLastGridPoint", tmpl.LongitudeOfLastGridPoint, int32(30450000)},
		{"IDirectionIncrement", tmpl.IDirectionIncrement, uint32(100000)},
		{"JDirectionIncrement", tmpl.JDirectionIncrement, uint32(100000)},
		{"ScanningMode", tmpl.ScanningMode, uint8(64)},
		{"LatitudeOfSouthernPole", tmpl.LatitudeOfSouthernPole, int32(-22000000)},
		{"LongitudeOfSouthernPole", tmpl.LongitudeOfSouthernPole, int32(320000000)},
		{"AngleOfRotation", tmpl.AngleOfRotation, uint32(0)},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestWriteSection3_Template31_RoundTrip(t *testing.T) {
	original := Section3{
		Source:                  0,
		NumberOfDataPoints:      99200,
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          1,
		Template: Template31{
			ShapeOfEarth:                        6,
			ScaleFactorOfRadiusOfSphericalEarth: 0xFF,
			ScaledValueOfRadiusOfSphericalEarth: 0xFFFFFFFF,
			ScaleFactorOfEarthMajorAxis:         0xFF,
			ScaledValueOfEarthMajorAxis:         0xFFFFFFFF,
			ScaleFactorOfEarthMinorAxis:         0xFF,
			ScaledValueOfEarthMinorAxis:         0xFFFFFFFF,
			Ni:                                  248,
			Nj:                                  400,
			BasicAngle:                          0,
			SubdivisionsOfBasicAngle:            0xFFFFFFFF,
			LatitudeOfFirstGridPoint:            -13250000,
			LongitudeOfFirstGridPoint:           5750000,
			ResolutionAndComponentFlags:         56,
			LatitudeOfLastGridPoint:             26650000,
			LongitudeOfLastGridPoint:            30450000,
			IDirectionIncrement:                 100000,
			JDirectionIncrement:                 100000,
			ScanningMode:                        64,
			LatitudeOfSouthernPole:              -22000000,
			LongitudeOfSouthernPole:             320000000,
			AngleOfRotation:                     0,
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

	if decoded.Length != 84 {
		t.Errorf("Length = %d, want 84", decoded.Length)
	}
	if decoded.NumberOfDataPoints != original.NumberOfDataPoints {
		t.Errorf("NumberOfDataPoints = %d, want %d", decoded.NumberOfDataPoints, original.NumberOfDataPoints)
	}

	tmpl, ok := decoded.Template.(Template31)
	if !ok {
		t.Fatalf("Template type = %T, want Template31", decoded.Template)
	}
	orig := original.Template.(Template31)
	if tmpl != orig {
		t.Errorf("template mismatch:\n  got  %+v\n  want %+v", tmpl, orig)
	}
}

func TestWriteSection3_Template31_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/constant_field.grib2")
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
	// Section 3 starts at offset 37 (16 + 21, no section 2) and is 84 bytes
	want := raw[37 : 37+84]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// ---- Template 3.10 (Mercator) ----

func TestReadSection3_Template310_Mercator(t *testing.T) {
	raw, err := os.ReadFile("testdata/mercator.grib2")
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

	// Cross-validated against: grib_dump -O testdata/mercator.grib2
	if s3.Length != 72 {
		t.Errorf("Length = %d, want 72", s3.Length)
	}
	if s3.Source != 0 {
		t.Errorf("Source = %d, want 0", s3.Source)
	}
	if s3.NumberOfDataPoints != 2464 {
		t.Errorf("NumberOfDataPoints = %d, want 2464", s3.NumberOfDataPoints)
	}
	if s3.TemplateNumber != 10 {
		t.Errorf("TemplateNumber = %d, want 10", s3.TemplateNumber)
	}

	tmpl, ok := s3.Template.(Template310)
	if !ok {
		t.Fatalf("Template type = %T, want Template310", s3.Template)
	}

	// Cross-validated against grib_dump -O:
	//   shapeOfTheEarth = 6
	//   Ni = 56, Nj = 44
	//   latitudeOfFirstGridPoint = 14736453
	//   longitudeOfFirstGridPoint = 262036499
	//   resolutionAndComponentFlags = 48
	//   LaD = 14000000
	//   latitudeOfLastGridPoint = 31173058
	//   longitudeOfLastGridPoint = 284975281
	//   scanningMode = 64
	//   orientationOfTheGrid = 0
	//   Di = 45000000
	//   Dj = 45000000
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ShapeOfEarth", tmpl.ShapeOfEarth, uint8(6)},
		{"ScaleFactorOfRadiusOfSphericalEarth", tmpl.ScaleFactorOfRadiusOfSphericalEarth, uint8(0)},
		{"ScaledValueOfRadiusOfSphericalEarth", tmpl.ScaledValueOfRadiusOfSphericalEarth, uint32(0)},
		{"Ni", tmpl.Ni, uint32(56)},
		{"Nj", tmpl.Nj, uint32(44)},
		{"LatitudeOfFirstGridPoint", tmpl.LatitudeOfFirstGridPoint, int32(14736453)},
		{"LongitudeOfFirstGridPoint", tmpl.LongitudeOfFirstGridPoint, int32(262036499)},
		{"ResolutionAndComponentFlags", tmpl.ResolutionAndComponentFlags, uint8(48)},
		{"LaD", tmpl.LaD, int32(14000000)},
		{"LatitudeOfLastGridPoint", tmpl.LatitudeOfLastGridPoint, int32(31173058)},
		{"LongitudeOfLastGridPoint", tmpl.LongitudeOfLastGridPoint, int32(284975281)},
		{"ScanningMode", tmpl.ScanningMode, uint8(64)},
		{"OrientationOfTheGrid", tmpl.OrientationOfTheGrid, uint32(0)},
		{"Di", tmpl.Di, uint32(45000000)},
		{"Dj", tmpl.Dj, uint32(45000000)},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestWriteSection3_Template310_RoundTrip(t *testing.T) {
	original := Section3{
		Source:                  0,
		NumberOfDataPoints:      2464,
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          10,
		Template: Template310{
			ShapeOfEarth:                        6,
			ScaleFactorOfRadiusOfSphericalEarth: 0,
			ScaledValueOfRadiusOfSphericalEarth: 0,
			ScaleFactorOfEarthMajorAxis:         0,
			ScaledValueOfEarthMajorAxis:         0,
			ScaleFactorOfEarthMinorAxis:         0,
			ScaledValueOfEarthMinorAxis:         0,
			Ni:                                  56,
			Nj:                                  44,
			LatitudeOfFirstGridPoint:            14736453,
			LongitudeOfFirstGridPoint:           262036499,
			ResolutionAndComponentFlags:         48,
			LaD:                                 14000000,
			LatitudeOfLastGridPoint:             31173058,
			LongitudeOfLastGridPoint:            284975281,
			ScanningMode:                        64,
			OrientationOfTheGrid:                0,
			Di:                                  45000000,
			Dj:                                  45000000,
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

	if decoded.Length != 72 {
		t.Errorf("Length = %d, want 72", decoded.Length)
	}
	if decoded.NumberOfDataPoints != original.NumberOfDataPoints {
		t.Errorf("NumberOfDataPoints = %d, want %d", decoded.NumberOfDataPoints, original.NumberOfDataPoints)
	}

	tmpl, ok := decoded.Template.(Template310)
	if !ok {
		t.Fatalf("Template type = %T, want Template310", decoded.Template)
	}
	orig := original.Template.(Template310)
	if tmpl != orig {
		t.Errorf("template mismatch:\n  got  %+v\n  want %+v", tmpl, orig)
	}
}

func TestWriteSection3_Template310_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/mercator.grib2")
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
	// Section 3 starts at offset 37 (16 + 21, no section 2) and is 72 bytes
	want := raw[37 : 37+72]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// ---- Template 3.20 (Polar Stereographic) ----

func TestReadSection3_Template320_PolarStereographic(t *testing.T) {
	raw, err := os.ReadFile("testdata/polar_stereographic.grib2")
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

	// Cross-validated against: grib_dump -O polar_stereographic_sfc_grib2.tmpl
	if s3.Length != 65 {
		t.Errorf("Length = %d, want 65", s3.Length)
	}
	if s3.Source != 0 {
		t.Errorf("Source = %d, want 0", s3.Source)
	}
	if s3.NumberOfDataPoints != 496 {
		t.Errorf("NumberOfDataPoints = %d, want 496", s3.NumberOfDataPoints)
	}
	if s3.TemplateNumber != 20 {
		t.Errorf("TemplateNumber = %d, want 20", s3.TemplateNumber)
	}

	tmpl, ok := s3.Template.(Template320)
	if !ok {
		t.Fatalf("Template type = %T, want Template320", s3.Template)
	}

	// Cross-validated against grib_dump -O:
	//   shapeOfTheEarth = 6
	//   Nx = 16, Ny = 31
	//   latitudeOfFirstGridPoint = 60000000
	//   longitudeOfFirstGridPoint = 0
	//   resolutionAndComponentFlags = 0
	//   LaD = 60000000
	//   orientationOfTheGrid = 0
	//   Dx = 0, Dy = 0
	//   projectionCentreFlag = 0
	//   scanningMode = 0
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ShapeOfEarth", tmpl.ShapeOfEarth, uint8(6)},
		{"ScaleFactorOfRadiusOfSphericalEarth", tmpl.ScaleFactorOfRadiusOfSphericalEarth, uint8(0xFF)},
		{"ScaledValueOfRadiusOfSphericalEarth", tmpl.ScaledValueOfRadiusOfSphericalEarth, uint32(0xFFFFFFFF)},
		{"Nx", tmpl.Nx, uint32(16)},
		{"Ny", tmpl.Ny, uint32(31)},
		{"LatitudeOfFirstGridPoint", tmpl.LatitudeOfFirstGridPoint, int32(60000000)},
		{"LongitudeOfFirstGridPoint", tmpl.LongitudeOfFirstGridPoint, int32(0)},
		{"ResolutionAndComponentFlags", tmpl.ResolutionAndComponentFlags, uint8(0)},
		{"LaD", tmpl.LaD, int32(60000000)},
		{"OrientationOfTheGrid", tmpl.OrientationOfTheGrid, int32(0)},
		{"Dx", tmpl.Dx, uint32(0)},
		{"Dy", tmpl.Dy, uint32(0)},
		{"ProjectionCentreFlag", tmpl.ProjectionCentreFlag, uint8(0)},
		{"ScanningMode", tmpl.ScanningMode, uint8(0)},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestWriteSection3_Template320_RoundTrip(t *testing.T) {
	original := Section3{
		Source:                  0,
		NumberOfDataPoints:      496,
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          20,
		Template: Template320{
			ShapeOfEarth:                        6,
			ScaleFactorOfRadiusOfSphericalEarth: 0xFF,
			ScaledValueOfRadiusOfSphericalEarth: 0xFFFFFFFF,
			ScaleFactorOfEarthMajorAxis:         0xFF,
			ScaledValueOfEarthMajorAxis:         0xFFFFFFFF,
			ScaleFactorOfEarthMinorAxis:         0xFF,
			ScaledValueOfEarthMinorAxis:         0xFFFFFFFF,
			Nx:                                  16,
			Ny:                                  31,
			LatitudeOfFirstGridPoint:            60000000,
			LongitudeOfFirstGridPoint:           0,
			ResolutionAndComponentFlags:         0,
			LaD:                                 60000000,
			OrientationOfTheGrid:                0,
			Dx:                                  0,
			Dy:                                  0,
			ProjectionCentreFlag:                0,
			ScanningMode:                        0,
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

	if decoded.Length != 65 {
		t.Errorf("Length = %d, want 65", decoded.Length)
	}
	if decoded.NumberOfDataPoints != original.NumberOfDataPoints {
		t.Errorf("NumberOfDataPoints = %d, want %d", decoded.NumberOfDataPoints, original.NumberOfDataPoints)
	}

	tmpl, ok := decoded.Template.(Template320)
	if !ok {
		t.Fatalf("Template type = %T, want Template320", decoded.Template)
	}
	orig := original.Template.(Template320)
	if tmpl != orig {
		t.Errorf("template mismatch:\n  got  %+v\n  want %+v", tmpl, orig)
	}
}

func TestWriteSection3_Template320_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/polar_stereographic.grib2")
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
	// Section 3 starts at offset 54 (16 + 21 + 17 for section 2) and is 65 bytes
	want := raw[54 : 54+65]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// --- Template 3.30 (Lambert Conformal Conic) tests ---

func TestReadSection3_Template330_Lambert(t *testing.T) {
	f, err := os.Open("testdata/lambert.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Skip sections 0, 1, 2
	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}
	if _, err := ReadSection1(f); err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}
	if _, err := ReadSection2(f); err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	s3, err := ReadSection3(f)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	// Cross-validated against: grib_dump -O testdata/lambert.grib2
	if s3.Length != 81 {
		t.Errorf("Length = %d, want 81", s3.Length)
	}
	if s3.Source != 0 {
		t.Errorf("Source = %d, want 0", s3.Source)
	}
	if s3.NumberOfDataPoints != 496 {
		t.Errorf("NumberOfDataPoints = %d, want 496", s3.NumberOfDataPoints)
	}
	if s3.TemplateNumber != 30 {
		t.Errorf("TemplateNumber = %d, want 30", s3.TemplateNumber)
	}

	tmpl, ok := s3.Template.(Template330)
	if !ok {
		t.Fatalf("Template type = %T, want Template330", s3.Template)
	}

	// Cross-validated against grib_dump -O:
	//   15        shapeOfTheEarth = 6
	//   31-34     Nx = 349
	//   35-38     Ny = 277
	//   39-42     latitudeOfFirstGridPoint = 21138000
	//   43-46     longitudeOfFirstGridPoint = 239372000
	//   47        resolutionAndComponentFlags = 48
	//   48-51     LaD = 25000000
	//   52-55     LoV = 265000000
	//   56-59     Dx = 13545000
	//   60-63     Dy = 13545000
	//   64        projectionCentreFlag = 0
	//   65        scanningMode = 64
	//   66-69     Latin1 = 25000000
	//   70-73     Latin2 = 25000000
	//   74-77     latitudeOfSouthernPole = 0
	//   78-81     longitudeOfSouthernPole = 0
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ShapeOfEarth", tmpl.ShapeOfEarth, uint8(6)},
		{"Nx", tmpl.Nx, uint32(349)},
		{"Ny", tmpl.Ny, uint32(277)},
		{"LatitudeOfFirstGridPoint", tmpl.LatitudeOfFirstGridPoint, int32(21138000)},
		{"LongitudeOfFirstGridPoint", tmpl.LongitudeOfFirstGridPoint, int32(239372000)},
		{"ResolutionAndComponentFlags", tmpl.ResolutionAndComponentFlags, uint8(48)},
		{"LaD", tmpl.LaD, int32(25000000)},
		{"LoV", tmpl.LoV, int32(265000000)},
		{"Dx", tmpl.Dx, uint32(13545000)},
		{"Dy", tmpl.Dy, uint32(13545000)},
		{"ProjectionCentreFlag", tmpl.ProjectionCentreFlag, uint8(0)},
		{"ScanningMode", tmpl.ScanningMode, uint8(64)},
		{"Latin1", tmpl.Latin1, int32(25000000)},
		{"Latin2", tmpl.Latin2, int32(25000000)},
		{"LatitudeOfSouthernPole", tmpl.LatitudeOfSouthernPole, int32(0)},
		{"LongitudeOfSouthernPole", tmpl.LongitudeOfSouthernPole, int32(0)},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestWriteSection3_Template330_RoundTrip(t *testing.T) {
	original := Section3{
		Source:                  0,
		NumberOfDataPoints:      496,
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          30,
		Template: Template330{
			ShapeOfEarth:                        6,
			ScaleFactorOfRadiusOfSphericalEarth: 0xFF,
			ScaledValueOfRadiusOfSphericalEarth: 0xFFFFFFFF,
			ScaleFactorOfEarthMajorAxis:         0xFF,
			ScaledValueOfEarthMajorAxis:         0xFFFFFFFF,
			ScaleFactorOfEarthMinorAxis:         0xFF,
			ScaledValueOfEarthMinorAxis:         0xFFFFFFFF,
			Nx:                                  349,
			Ny:                                  277,
			LatitudeOfFirstGridPoint:            21138000,
			LongitudeOfFirstGridPoint:           239372000,
			ResolutionAndComponentFlags:         48,
			LaD:                                 25000000,
			LoV:                                 265000000,
			Dx:                                  13545000,
			Dy:                                  13545000,
			ProjectionCentreFlag:                0,
			ScanningMode:                        64,
			Latin1:                              25000000,
			Latin2:                              25000000,
			LatitudeOfSouthernPole:              0,
			LongitudeOfSouthernPole:             0,
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

	if decoded.Length != 81 {
		t.Errorf("Length = %d, want 81", decoded.Length)
	}
	if decoded.NumberOfDataPoints != original.NumberOfDataPoints {
		t.Errorf("NumberOfDataPoints = %d, want %d", decoded.NumberOfDataPoints, original.NumberOfDataPoints)
	}
	if decoded.TemplateNumber != 30 {
		t.Errorf("TemplateNumber = %d, want 30", decoded.TemplateNumber)
	}

	tmpl, ok := decoded.Template.(Template330)
	if !ok {
		t.Fatalf("Template type = %T, want Template330", decoded.Template)
	}
	orig := original.Template.(Template330)
	if tmpl != orig {
		t.Errorf("template mismatch:\n  got  %+v\n  want %+v", tmpl, orig)
	}
}

func TestWriteSection3_Template330_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/lambert.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Read the file to parse section 3
	f, err := os.Open("testdata/lambert.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}
	if _, err := ReadSection1(f); err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}
	if _, err := ReadSection2(f); err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	s3, err := ReadSection3(f)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteSection3(&buf, s3); err != nil {
		t.Fatalf("WriteSection3: %v", err)
	}

	got := buf.Bytes()
	// Section 3 starts at offset 16 (s0) + 21 (s1) + 5 (s2) = 42, length is 81 bytes
	want := raw[42 : 42+81]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// --- Template 3.40 (Gaussian Grid) tests ---

func TestReadSection3_Template340_RegularGaussian(t *testing.T) {
	f, err := os.Open("testdata/regular_gaussian_surface.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Skip sections 0, 1, 2
	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}
	if _, err := ReadSection1(f); err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}
	if _, err := ReadSection2(f); err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	s3, err := ReadSection3(f)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	// Cross-validated against: grib_dump -O testdata/regular_gaussian_surface.grib2
	if s3.Length != 72 {
		t.Errorf("Length = %d, want 72", s3.Length)
	}
	if s3.Source != 0 {
		t.Errorf("Source = %d, want 0", s3.Source)
	}
	if s3.NumberOfDataPoints != 8192 {
		t.Errorf("NumberOfDataPoints = %d, want 8192", s3.NumberOfDataPoints)
	}
	if s3.OctetsForNumberOfPoints != 0 {
		t.Errorf("OctetsForNumberOfPoints = %d, want 0", s3.OctetsForNumberOfPoints)
	}
	if s3.InterpretationOfPoints != 0 {
		t.Errorf("InterpretationOfPoints = %d, want 0", s3.InterpretationOfPoints)
	}
	if s3.TemplateNumber != 40 {
		t.Errorf("TemplateNumber = %d, want 40", s3.TemplateNumber)
	}

	tmpl, ok := s3.Template.(Template340)
	if !ok {
		t.Fatalf("Template type = %T, want Template340", s3.Template)
	}

	// Cross-validated against grib_dump -O:
	//   15        shapeOfTheEarth = 6
	//   31-34     Ni = 128
	//   35-38     Nj = 64
	//   39-42     basicAngleOfTheInitialProductionDomain = 0
	//   43-46     subdivisionsOfBasicAngle = MISSING (0xFFFFFFFF)
	//   47-50     latitudeOfFirstGridPoint = 87863799
	//   51-54     longitudeOfFirstGridPoint = 0
	//   55        resolutionAndComponentFlags = 48
	//   56-59     latitudeOfLastGridPoint = -87863799
	//   60-63     longitudeOfLastGridPoint = 357187500
	//   64-67     iDirectionIncrement = 2812500
	//   68-71     N = 32
	//   72        scanningMode = 0
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ShapeOfEarth", tmpl.ShapeOfEarth, uint8(6)},
		{"Ni", tmpl.Ni, uint32(128)},
		{"Nj", tmpl.Nj, uint32(64)},
		{"BasicAngle", tmpl.BasicAngle, uint32(0)},
		{"SubdivisionsOfBasicAngle", tmpl.SubdivisionsOfBasicAngle, uint32(0xFFFFFFFF)},
		{"LatitudeOfFirstGridPoint", tmpl.LatitudeOfFirstGridPoint, int32(87863799)},
		{"LongitudeOfFirstGridPoint", tmpl.LongitudeOfFirstGridPoint, int32(0)},
		{"ResolutionAndComponentFlags", tmpl.ResolutionAndComponentFlags, uint8(48)},
		{"LatitudeOfLastGridPoint", tmpl.LatitudeOfLastGridPoint, int32(-87863799)},
		{"LongitudeOfLastGridPoint", tmpl.LongitudeOfLastGridPoint, int32(357187500)},
		{"IDirectionIncrement", tmpl.IDirectionIncrement, uint32(2812500)},
		{"N", tmpl.N, uint32(32)},
		{"ScanningMode", tmpl.ScanningMode, uint8(0)},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}

	// Regular Gaussian should have no optional PL array
	if len(s3.OptionalPointNumbers) != 0 {
		t.Errorf("OptionalPointNumbers length = %d, want 0", len(s3.OptionalPointNumbers))
	}
}

func TestReadSection3_Template340_ReducedGaussian(t *testing.T) {
	f, err := os.Open("testdata/reduced_gaussian_surface.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Skip sections 0, 1, 2
	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}
	if _, err := ReadSection1(f); err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}
	if _, err := ReadSection2(f); err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	s3, err := ReadSection3(f)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	// Cross-validated against: grib_dump -O testdata/reduced_gaussian_surface.grib2
	if s3.Length != 200 {
		t.Errorf("Length = %d, want 200", s3.Length)
	}
	if s3.NumberOfDataPoints != 6114 {
		t.Errorf("NumberOfDataPoints = %d, want 6114", s3.NumberOfDataPoints)
	}
	if s3.OctetsForNumberOfPoints != 2 {
		t.Errorf("OctetsForNumberOfPoints = %d, want 2", s3.OctetsForNumberOfPoints)
	}
	if s3.InterpretationOfPoints != 1 {
		t.Errorf("InterpretationOfPoints = %d, want 1", s3.InterpretationOfPoints)
	}
	if s3.TemplateNumber != 40 {
		t.Errorf("TemplateNumber = %d, want 40", s3.TemplateNumber)
	}

	tmpl, ok := s3.Template.(Template340)
	if !ok {
		t.Fatalf("Template type = %T, want Template340", s3.Template)
	}

	// Cross-validated against grib_dump -O:
	//   31-34     Ni = MISSING (0xFFFFFFFF)
	//   35-38     Nj = 64
	//   47-50     latitudeOfFirstGridPoint = 87863799
	//   64-67     iDirectionIncrement = MISSING (0xFFFFFFFF)
	//   68-71     N = 32
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ShapeOfEarth", tmpl.ShapeOfEarth, uint8(6)},
		{"Ni", tmpl.Ni, uint32(0xFFFFFFFF)},
		{"Nj", tmpl.Nj, uint32(64)},
		{"LatitudeOfFirstGridPoint", tmpl.LatitudeOfFirstGridPoint, int32(87863799)},
		{"LongitudeOfFirstGridPoint", tmpl.LongitudeOfFirstGridPoint, int32(0)},
		{"ResolutionAndComponentFlags", tmpl.ResolutionAndComponentFlags, uint8(0)},
		{"LatitudeOfLastGridPoint", tmpl.LatitudeOfLastGridPoint, int32(-87863799)},
		{"LongitudeOfLastGridPoint", tmpl.LongitudeOfLastGridPoint, int32(357187500)},
		{"IDirectionIncrement", tmpl.IDirectionIncrement, uint32(0xFFFFFFFF)},
		{"N", tmpl.N, uint32(32)},
		{"ScanningMode", tmpl.ScanningMode, uint8(0)},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}

	// Reduced Gaussian should have PL array: 64 entries * 2 bytes = 128 bytes
	if len(s3.OptionalPointNumbers) != 128 {
		t.Fatalf("OptionalPointNumbers length = %d, want 128", len(s3.OptionalPointNumbers))
	}

	// Verify first and last PL values (2-byte big-endian unsigned integers)
	// pl[0] = 20, pl[63] = 20
	firstPL := binary.BigEndian.Uint16(s3.OptionalPointNumbers[0:2])
	if firstPL != 20 {
		t.Errorf("PL[0] = %d, want 20", firstPL)
	}
	lastPL := binary.BigEndian.Uint16(s3.OptionalPointNumbers[126:128])
	if lastPL != 20 {
		t.Errorf("PL[63] = %d, want 20", lastPL)
	}
	// pl[20] = 128 (the equatorial band value)
	midPL := binary.BigEndian.Uint16(s3.OptionalPointNumbers[40:42])
	if midPL != 128 {
		t.Errorf("PL[20] = %d, want 128", midPL)
	}
}

func TestWriteSection3_Template340_RoundTrip(t *testing.T) {
	original := Section3{
		Source:                  0,
		NumberOfDataPoints:      8192,
		OctetsForNumberOfPoints: 0,
		InterpretationOfPoints:  0,
		TemplateNumber:          40,
		Template: Template340{
			ShapeOfEarth:                        6,
			ScaleFactorOfRadiusOfSphericalEarth: 0xFF,
			ScaledValueOfRadiusOfSphericalEarth: 0xFFFFFFFF,
			ScaleFactorOfEarthMajorAxis:         0xFF,
			ScaledValueOfEarthMajorAxis:         0xFFFFFFFF,
			ScaleFactorOfEarthMinorAxis:         0xFF,
			ScaledValueOfEarthMinorAxis:         0xFFFFFFFF,
			Ni:                                  128,
			Nj:                                  64,
			BasicAngle:                          0,
			SubdivisionsOfBasicAngle:            0xFFFFFFFF,
			LatitudeOfFirstGridPoint:            87863799,
			LongitudeOfFirstGridPoint:           0,
			ResolutionAndComponentFlags:         48,
			LatitudeOfLastGridPoint:             -87863799,
			LongitudeOfLastGridPoint:            357187500,
			IDirectionIncrement:                 2812500,
			N:                                   32,
			ScanningMode:                        0,
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

	if decoded.Length != 72 {
		t.Errorf("Length = %d, want 72", decoded.Length)
	}
	if decoded.NumberOfDataPoints != original.NumberOfDataPoints {
		t.Errorf("NumberOfDataPoints = %d, want %d", decoded.NumberOfDataPoints, original.NumberOfDataPoints)
	}
	if decoded.TemplateNumber != 40 {
		t.Errorf("TemplateNumber = %d, want 40", decoded.TemplateNumber)
	}

	tmpl, ok := decoded.Template.(Template340)
	if !ok {
		t.Fatalf("Template type = %T, want Template340", decoded.Template)
	}
	orig := original.Template.(Template340)
	if tmpl != orig {
		t.Errorf("template mismatch:\n  got  %+v\n  want %+v", tmpl, orig)
	}
}

func TestWriteSection3_Template340_RegularGaussian_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/regular_gaussian_surface.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	f, err := os.Open("testdata/regular_gaussian_surface.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}
	if _, err := ReadSection1(f); err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}
	if _, err := ReadSection2(f); err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	s3, err := ReadSection3(f)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteSection3(&buf, s3); err != nil {
		t.Fatalf("WriteSection3: %v", err)
	}

	got := buf.Bytes()
	// Section 3 starts at offset 16 (s0) + 21 (s1) + 17 (s2) = 54, length is 72 bytes
	want := raw[54 : 54+72]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

func TestWriteSection3_Template340_ReducedGaussian_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/reduced_gaussian_surface.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	f, err := os.Open("testdata/reduced_gaussian_surface.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	if _, err := ReadSection0(f); err != nil {
		t.Fatalf("ReadSection0: %v", err)
	}
	if _, err := ReadSection1(f); err != nil {
		t.Fatalf("ReadSection1: %v", err)
	}
	if _, err := ReadSection2(f); err != nil {
		t.Fatalf("ReadSection2: %v", err)
	}

	s3, err := ReadSection3(f)
	if err != nil {
		t.Fatalf("ReadSection3: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteSection3(&buf, s3); err != nil {
		t.Fatalf("WriteSection3: %v", err)
	}

	got := buf.Bytes()
	// Section 3 starts at offset 16 (s0) + 21 (s1) + 17 (s2) = 54, length is 200 bytes
	want := raw[54 : 54+200]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch (len got=%d, want=%d):\n  got  %x\n  want %x", len(got), len(want), got, want)
	}
}
