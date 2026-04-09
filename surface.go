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

// computeSurfaceValue applies the GRIB2 scaling formula:
//
//	value = scaledValue * 10^(-scaleFactor)
//
// The scale factor is read as sign-magnitude (per the WMO spec), so
// values like byte 0x84 are correctly interpreted as -4, not -124.
// This means the standard formula works for all known producers
// (GFS, ECMWF, CMC/RDPS, etc.) without any special-case hacks.
func computeSurfaceValue(scaleFactor int8, scaledValue uint32) float64 {
	if scaledValue == 0xFFFFFFFF {
		return math.NaN()
	}
	return float64(scaledValue) * math.Pow(10, float64(-scaleFactor))
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
