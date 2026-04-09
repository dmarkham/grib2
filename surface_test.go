package grib2

import (
	"math"
	"testing"
)

func TestComputeSurfaceValue_Standard(t *testing.T) {
	// GFS-style: 85000 Pa with scaleFactor=0, isobaric surface
	val := computeSurfaceValue(0, 85000, 100)
	if val != 85000 {
		t.Errorf("got %f, want 85000", val)
	}

	// With scaleFactor=2: 850 * 10^(-2) = 8.5 (non-isobaric)
	val = computeSurfaceValue(2, 850, 103)
	if math.Abs(val-8.5) > 1e-10 {
		t.Errorf("got %f, want 8.5", val)
	}

	// With scaleFactor=-2: 850 * 10^(2) = 85000, isobaric
	val = computeSurfaceValue(-2, 850, 100)
	if val != 85000 {
		t.Errorf("got %f, want 85000", val)
	}
}

func TestComputeSurfaceValue_CMCFallback_Isobaric(t *testing.T) {
	// RDPS-style isobaric: scaleFactor=-124, scaledValue=85, surfType=100
	// Fallback detects raw value 85 < 1100, converts to Pa: 85 * 100 = 8500
	val := computeSurfaceValue(-124, 85, 100)
	if val != 8500 {
		t.Errorf("got %f, want 8500 (85 hPa → 8500 Pa)", val)
	}

	// RDPS 200hPa: scaleFactor=-124, scaledValue=2, surfType=100
	// Fallback: 2 < 1100, so 2 * 100 = 200 Pa (which is 2 hPa)
	val = computeSurfaceValue(-124, 2, 100)
	if val != 200 {
		t.Errorf("got %f, want 200 (2 hPa → 200 Pa)", val)
	}

	// RDPS 850hPa: scaleFactor=-125, scaledValue=850, surfType=100
	val = computeSurfaceValue(-125, 850, 100)
	if val != 85000 {
		t.Errorf("got %f, want 85000 (850 hPa → 85000 Pa)", val)
	}
}

func TestComputeSurfaceValue_CMCFallback_NonIsobaric(t *testing.T) {
	// Non-isobaric: garbage scale factor, return raw value
	val := computeSurfaceValue(-124, 2, 103) // height above ground
	if val != 2 {
		t.Errorf("got %f, want 2 (raw fallback for non-isobaric)", val)
	}
}

func TestComputeSurfaceValue_Missing(t *testing.T) {
	val := computeSurfaceValue(0, 0xFFFFFFFF, 100)
	if !math.IsNaN(val) {
		t.Errorf("got %f, want NaN for missing", val)
	}
}

func TestSurfaceLevelHPa_GFS(t *testing.T) {
	// GFS: isobaric at 85000 Pa → 850 hPa
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
	tests := []struct {
		name        string
		scaleFactor int8
		scaledValue uint32
		wantHPa     float64
	}{
		{"RDPS 200hPa", -124, 2, 2},
		{"RDPS 850hPa", -125, 85, 85},
		{"RDPS 500hPa", -124, 500, 500},
		{"GFS 200hPa", 0, 20000, 200},
		{"GFS 850hPa", 0, 85000, 850},
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
