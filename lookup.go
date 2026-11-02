package grib2

import (
	"fmt"
	"math"
)

// NearestValue returns the data value at the grid point closest to the
// given latitude and longitude (in degrees). Also returns the index,
// and the actual lat/lon of the nearest point.
//
// For regular lat/lon (Template30), rotated lat/lon (Template31) and regular
// Gaussian grids (Template340 with Ni set) the four points around the target
// are found directly and the closest by great-circle distance is returned.
// For all other grid types, a brute-force search over GridCoordinates
// is performed.
func (f *Field) NearestValue(lat, lon float64) (value float64, idx int, nearLat, nearLon float64, err error) {
	values, err := f.Values()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("grib2: NearestValue: %w", err)
	}

	idx, nearLat, nearLon, err = nearestIndex(f, lat, lon)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if idx < 0 || idx >= len(values) {
		return 0, 0, 0, 0, fmt.Errorf("grib2: NearestValue: computed index %d out of range [0, %d)", idx, len(values))
	}
	return values[idx], idx, nearLat, nearLon, nil
}

func nearestIndex(f *Field, lat, lon float64) (idx int, nearLat, nearLon float64, err error) {
	if ni, nj, ok := gridDims(f.Section3.Template); ok && !isReduced(f.Section3.Template) && ni*nj != int(f.Section3.NumberOfDataPoints) {
		return 0, 0, 0, fmt.Errorf("grib2: grid is %dx%d but section 3 declares %d points", ni, nj, f.Section3.NumberOfDataPoints)
	}
	switch tmpl := f.Section3.Template.(type) {
	case Template30, Template340:
		if g, err := regularGridOf(tmpl); err == nil {
			idx, nearLat, nearLon = g.nearest(lat, lon, lat, lon, identityGeo)
			return idx, nearLat, nearLon, nil
		} else if !isReduced(tmpl) {
			return 0, 0, 0, err
		}
	case Template31:
		g, err := regularGridOf(tmpl)
		if err != nil {
			return 0, 0, 0, err
		}
		spLat, spLon, angle := rotation(tmpl)
		rLat, rLon := coordRotate(lat, lon, angle, spLat, spLon)
		idx, nearLat, nearLon = g.nearest(rLat, rLon, lat, lon, func(la, lo float64) (float64, float64) {
			la, lo = coordUnrotate(la, lo, angle, spLat, spLon)
			return la, coordNormaliseLon(lo)
		})
		return idx, nearLat, nearLon, nil
	}
	return nearestIndexBruteForce(f, lat, lon)
}

func identityGeo(la, lo float64) (float64, float64) { return la, lo }

// isReduced reports whether a 3.0 or 3.40 grid has Ni missing.
func isReduced(tmpl GridDefinitionTemplate) bool {
	switch t := tmpl.(type) {
	case Template30:
		return t.Ni == missingUint32
	case Template340:
		return t.Ni == missingUint32
	}
	return false
}

// BilinearValue returns an interpolated value at the given latitude and
// longitude using bilinear interpolation from the 4 surrounding grid points.
// Only works for regular grids (Template30, Template31, Template340 regular).
// On grids that go all the way round in longitude the cell between the last
// and the first column is interpolated across the 0/360 seam.
// Returns an error for irregular grids where bilinear interpolation does not
// apply cleanly.
func (f *Field) BilinearValue(lat, lon float64) (float64, error) {
	values, err := f.Values()
	if err != nil {
		return 0, fmt.Errorf("grib2: BilinearValue: %w", err)
	}
	s, err := NewSampler(f)
	if err != nil {
		return 0, fmt.Errorf("grib2: BilinearValue: %w", err)
	}
	w, err := s.Weights(lat, lon)
	if err != nil {
		return 0, err
	}
	return w.Interpolate(values)
}

// bilinearWeights locates (lat, lon) on a regular, rotated or regular
// Gaussian grid.
func bilinearWeights(tmpl GridDefinitionTemplate) (func(lat, lon float64) (BilinearWeights, error), error) {
	switch t := tmpl.(type) {
	case Template30, Template340:
		if isReduced(t) {
			return nil, fmt.Errorf("grib2: bilinear interpolation is not supported for reduced grids")
		}
		g, err := regularGridOf(t)
		if err != nil {
			return nil, err
		}
		return g.weights, nil
	case Template31:
		g, err := regularGridOf(t)
		if err != nil {
			return nil, err
		}
		spLat, spLon, angle := rotation(t)
		return func(lat, lon float64) (BilinearWeights, error) {
			rLat, rLon := coordRotate(lat, lon, angle, spLat, spLon)
			return g.weights(rLat, rLon)
		}, nil
	default:
		return nil, fmt.Errorf("grib2: bilinear interpolation not supported for template %T", tmpl)
	}
}

// ---------------------------------------------------------------------------
// Brute-force fallback using GridCoordinates
// ---------------------------------------------------------------------------

// nearestIndexBruteForce uses GridCoordinates to enumerate all grid points
// and finds the one closest to (lat, lon) using great-circle distance.
func nearestIndexBruteForce(f *Field, lat, lon float64) (int, float64, float64, error) {
	lats, lons, err := f.GridCoordinates()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("grib2: NearestValue fallback: %w", err)
	}

	if len(lats) == 0 {
		return 0, 0, 0, fmt.Errorf("grib2: NearestValue: grid has no points")
	}

	bestIdx := 0
	bestDist := math.MaxFloat64

	latRad := deg2rad(lat)
	lonRad := deg2rad(lon)

	for i := range lats {
		d := haversineRad(latRad, lonRad, deg2rad(lats[i]), deg2rad(lons[i]))
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}

	return bestIdx, lats[bestIdx], lons[bestIdx], nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// gridVal safely retrieves a value from the grid, returning NaN if out of range.
func gridVal(values []float64, idx int) float64 {
	if idx < 0 || idx >= len(values) {
		return math.NaN()
	}
	return values[idx]
}

// haversineRad computes the great-circle distance between two points given
// in radians. Returns the angular distance (multiply by Earth radius for metres).
func haversineRad(lat1, lon1, lat2, lon2 float64) float64 {
	dlat := lat2 - lat1
	dlon := lon2 - lon1
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * math.Asin(math.Sqrt(a))
}

func deg2rad(d float64) float64 {
	return d * math.Pi / 180
}

// haversineDeg computes the great-circle angular distance between two
// points given in degrees. The returned value is in radians and is only
// used for relative comparison (smaller = closer).
func haversineDeg(lat1, lon1, lat2, lon2 float64) float64 {
	return haversineRad(deg2rad(lat1), deg2rad(lon1), deg2rad(lat2), deg2rad(lon2))
}
