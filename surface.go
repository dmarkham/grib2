package grib2

import (
	"fmt"
	"math"
)

// SurfaceLevel returns the value of the first fixed surface for a field,
// applying the GRIB2 scaling formula: value = scaledValue * 10^(-scaleFactor).
//
// For isobaric surfaces (typeOfFirstFixedSurface == 100), the result is in Pa.
// Use SurfaceLevelHPa for a convenience wrapper that returns hPa.
//
// Some producers (notably CMC/RDPS) use non-standard scale factors. When the
// scale factor appears to be missing or produces an unreasonable result, this
// function falls back to treating the scaled value as the raw level value.
func (f *Field) SurfaceLevel() (float64, error) {
	tmpl := f.extractTemplate40()
	if tmpl == nil {
		return 0, errNoTemplate40("SurfaceLevel")
	}
	return computeSurfaceValue(tmpl.ScaleFactorOfFirstSurface, tmpl.ScaledValueOfFirstSurface), nil
}

// SurfaceLevelHPa returns the first fixed surface level in hectopascals (hPa/mbar)
// for isobaric surfaces (typeOfFirstFixedSurface == 100).
// For non-isobaric surfaces, returns the raw surface value.
func (f *Field) SurfaceLevelHPa() (float64, error) {
	tmpl := f.extractTemplate40()
	if tmpl == nil {
		return 0, errNoTemplate40("SurfaceLevelHPa")
	}
	val := computeSurfaceValue(tmpl.ScaleFactorOfFirstSurface, tmpl.ScaledValueOfFirstSurface)
	if tmpl.TypeOfFirstFixedSurface == 100 {
		// Isobaric surface: value is in Pa, convert to hPa
		return val / 100.0, nil
	}
	return val, nil
}

// SurfaceType returns the type code of the first fixed surface (Table 4.5).
func (f *Field) SurfaceType() (uint8, error) {
	tmpl := f.extractTemplate40()
	if tmpl == nil {
		return 0, errNoTemplate40("SurfaceType")
	}
	return tmpl.TypeOfFirstFixedSurface, nil
}

// computeSurfaceValue applies the GRIB2 scaling formula with fallback for
// non-standard scale factors.
func computeSurfaceValue(scaleFactor int8, scaledValue uint32) float64 {
	// Missing values (0xFF for unsigned, or MISSING for the template)
	if scaledValue == 0xFFFFFFFF {
		return math.NaN()
	}

	// Standard formula: value = scaledValue * 10^(-scaleFactor)
	// However, some producers set garbage scale factors. Detect this:
	// - scaleFactor in [-10, 10] is reasonable
	// - Outside that range, treat the scaled value as the raw value
	if scaleFactor >= -10 && scaleFactor <= 10 {
		return float64(scaledValue) * math.Pow(10, float64(-scaleFactor))
	}

	// Fallback: ignore the scale factor, return raw scaled value.
	// This handles CMC/RDPS which uses scaleFactor=-124/-125 with
	// scaledValue being the pressure in hPa (e.g., 850 for 850hPa).
	return float64(scaledValue)
}

// extractTemplate40 gets the base Template40 from any product template.
func (f *Field) extractTemplate40() *Template40 {
	switch tmpl := f.Section4.Template.(type) {
	case Template40:
		return &tmpl
	case Template41:
		return &tmpl.Template40
	case Template42:
		return &tmpl.Template40
	case Template48:
		return &tmpl.Template40
	case Template411:
		return &tmpl.Template40
	case Template412:
		return &tmpl.Template40
	default:
		return nil
	}
}

func errNoTemplate40(funcName string) error {
	return fmt.Errorf("grib2: %s: unsupported product template type", funcName)
}
