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
	return computeSurfaceValue(tmpl.ScaleFactorOfFirstSurface, tmpl.ScaledValueOfFirstSurface, tmpl.TypeOfFirstFixedSurface), nil
}

// SurfaceLevelHPa returns the first fixed surface level in hectopascals (hPa/mbar)
// for isobaric surfaces (typeOfFirstFixedSurface == 100).
// For non-isobaric surfaces, returns the raw surface value.
func (f *Field) SurfaceLevelHPa() (float64, error) {
	tmpl := f.extractTemplate40()
	if tmpl == nil {
		return 0, errNoTemplate40("SurfaceLevelHPa")
	}
	val := computeSurfaceValue(tmpl.ScaleFactorOfFirstSurface, tmpl.ScaledValueOfFirstSurface, tmpl.TypeOfFirstFixedSurface)
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
//
// surfaceType is needed because the fallback behavior differs for isobaric
// surfaces (type 100) where the raw value from CMC/RDPS is in hPa, but the
// standard unit is Pa.
func computeSurfaceValue(scaleFactor int8, scaledValue uint32, surfaceType uint8) float64 {
	// Missing values (0xFF for unsigned, or MISSING for the template)
	if scaledValue == 0xFFFFFFFF {
		return math.NaN()
	}

	// Standard formula: value = scaledValue * 10^(-scaleFactor)
	// scaleFactor in [-10, 10] is reasonable for any real-world encoding.
	if scaleFactor >= -10 && scaleFactor <= 10 {
		return float64(scaledValue) * math.Pow(10, float64(-scaleFactor))
	}

	// Fallback: ignore the garbage scale factor.
	// CMC/RDPS uses scaleFactor values like -124/-125 with scaledValue
	// being the level in compact form (e.g., 2 for 200hPa, 85 for 850hPa).
	//
	// For isobaric surfaces (type 100), the standard unit is Pa but CMC
	// stores hPa-scale values. Detect this: if the raw value is too small
	// to be Pa (< 1100, since the lowest standard pressure level is 1000hPa
	// = 100000 Pa), convert from hPa to Pa by multiplying by 100.
	raw := float64(scaledValue)
	if surfaceType == 100 && raw < 1100 {
		return raw * 100 // treat as hPa, convert to Pa
	}
	return raw
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
