package grib2

import (
	"math"
	"testing"
)

func TestComputeSurfaceValue_Standard(t *testing.T) {
	// GFS-style: 85000 Pa with scaleFactor=0
	val := computeSurfaceValue(0, 85000)
	if val != 85000 {
		t.Errorf("got %f, want 85000", val)
	}

	// scaleFactor=2: 850 * 10^(-2) = 8.5
	val = computeSurfaceValue(2, 850)
	if math.Abs(val-8.5) > 1e-10 {
		t.Errorf("got %f, want 8.5", val)
	}

	// scaleFactor=-2: 850 * 10^(2) = 85000
	val = computeSurfaceValue(-2, 850)
	if val != 85000 {
		t.Errorf("got %f, want 85000", val)
	}
}

func TestComputeSurfaceValue_RDPS(t *testing.T) {
	// RDPS isobaric: byte 0x84 = sign-magnitude -4, NOT two's complement -124.
	// With correct sign-magnitude reading: scaleFactor=-4, scaledValue=2
	// Formula: 2 * 10^(-(-4)) = 2 * 10^4 = 20000 Pa = 200 hPa
	val := computeSurfaceValue(-4, 2)
	if val != 20000 {
		t.Errorf("RDPS 200hPa: got %f, want 20000 Pa", val)
	}

	// scaleFactor=-4, scaledValue=5 → 5 * 10^4 = 50000 Pa = 500 hPa
	val = computeSurfaceValue(-4, 5)
	if val != 50000 {
		t.Errorf("RDPS 500hPa: got %f, want 50000 Pa", val)
	}

	// scaleFactor=-3, scaledValue=85 → 85 * 10^3 = 85000 Pa = 850 hPa
	val = computeSurfaceValue(-3, 85)
	if val != 85000 {
		t.Errorf("RDPS 850hPa: got %f, want 85000 Pa", val)
	}

	// scaleFactor=-3, scaledValue=100 → 100 * 10^3 = 100000 Pa = 1000 hPa
	val = computeSurfaceValue(-3, 100)
	if val != 100000 {
		t.Errorf("RDPS 1000hPa: got %f, want 100000 Pa", val)
	}
}

func TestComputeSurfaceValue_Missing(t *testing.T) {
	val := computeSurfaceValue(0, 0xFFFFFFFF)
	if !math.IsNaN(val) {
		t.Errorf("got %f, want NaN for missing", val)
	}
}

func TestSurfaceLevelHPa_GFS(t *testing.T) {
	field := &Field{
		Section4: Section4{
			TemplateNumber: 0,
			Template: Template40{
				TypeOfFirstFixedSurface:   100,
				ScaleFactorOfFirstSurface: 0,
				ScaledValueOfFirstSurface: 85000,
			},
		},
	}
	hpa, err := field.SurfaceLevelHPa()
	if err != nil {
		t.Fatalf("SurfaceLevelHPa: %v", err)
	}
	if hpa != 850 {
		t.Errorf("got %f hPa, want 850", hpa)
	}
}

func TestSurfaceLevelHPa_RDPS(t *testing.T) {
	// These test the FULL pipeline: Template40 has int8 scale factors
	// that were read via readSignMag8. We simulate what the parser produces.
	tests := []struct {
		name        string
		scaleFactor int8   // after sign-magnitude decoding
		scaledValue uint32
		wantHPa     float64
	}{
		{"RDPS 200hPa (sf=-4, sv=2)", -4, 2, 200},
		{"RDPS 300hPa (sf=-4, sv=3)", -4, 3, 300},
		{"RDPS 500hPa (sf=-4, sv=5)", -4, 5, 500},
		{"RDPS 700hPa (sf=-4, sv=7)", -4, 7, 700},
		{"RDPS 850hPa (sf=-3, sv=85)", -3, 85, 850},
		{"RDPS 1000hPa (sf=-3, sv=100)", -3, 100, 1000},
		{"GFS 200hPa (sf=0, sv=20000)", 0, 20000, 200},
		{"GFS 850hPa (sf=0, sv=85000)", 0, 85000, 850},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := &Field{
				Section4: Section4{
					TemplateNumber: 0,
					Template: Template40{
						TypeOfFirstFixedSurface:   100,
						ScaleFactorOfFirstSurface: tt.scaleFactor,
						ScaledValueOfFirstSurface: tt.scaledValue,
					},
				},
			}
			hpa, err := field.SurfaceLevelHPa()
			if err != nil {
				t.Fatalf("SurfaceLevelHPa: %v", err)
			}
			if math.Abs(hpa-tt.wantHPa) > 0.01 {
				t.Errorf("got %f hPa, want %f", hpa, tt.wantHPa)
			}
		})
	}
}

func TestReadSignMag8(t *testing.T) {
	// Verify the sign-magnitude byte reading that fixes the root cause.
	tests := []struct {
		raw  byte
		want int8
	}{
		{0x00, 0},
		{0x04, 4},
		{0x7F, 127},
		{0x84, -4},  // RDPS: byte 0x84 = sign-mag -4, NOT two's complement -124
		{0x83, -3},  // RDPS: byte 0x83 = sign-mag -3
		{0x80, 0},   // negative zero = 0
		{0xFF, -127},
	}
	for _, tt := range tests {
		got := readSignMag8(tt.raw)
		if got != tt.want {
			t.Errorf("readSignMag8(0x%02x) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}
