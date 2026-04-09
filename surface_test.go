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

	// With scaleFactor=2: 850 * 10^(-2) = 8.5
	val = computeSurfaceValue(2, 850)
	if math.Abs(val-8.5) > 1e-10 {
		t.Errorf("got %f, want 8.5", val)
	}

	// With scaleFactor=-2: 850 * 10^(2) = 85000
	val = computeSurfaceValue(-2, 850)
	if val != 85000 {
		t.Errorf("got %f, want 85000", val)
	}
}

func TestComputeSurfaceValue_CMCFallback(t *testing.T) {
	// RDPS-style: scaleFactor=-124, scaledValue=850
	// -124 is outside [-10,10] range, so fallback returns raw value
	val := computeSurfaceValue(-124, 850)
	if val != 850 {
		t.Errorf("got %f, want 850 (CMC fallback)", val)
	}

	val = computeSurfaceValue(-125, 200)
	if val != 200 {
		t.Errorf("got %f, want 200 (CMC fallback)", val)
	}
}

func TestComputeSurfaceValue_Missing(t *testing.T) {
	val := computeSurfaceValue(0, 0xFFFFFFFF)
	if !math.IsNaN(val) {
		t.Errorf("got %f, want NaN for missing", val)
	}
}

func TestSurfaceLevelHPa(t *testing.T) {
	// Create a field with isobaric surface at 85000 Pa
	field := &Field{
		Section4: Section4{
			TemplateNumber: 0,
			Template: Template40{
				TypeOfFirstFixedSurface:    100, // isobaric
				ScaleFactorOfFirstSurface:  0,
				ScaledValueOfFirstSurface:  85000,
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
