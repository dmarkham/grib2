package grib2

import (
	"fmt"
	"math"
)

// NearestValue returns the data value at the grid point closest to the
// given latitude and longitude (in degrees). Also returns the index,
// and the actual lat/lon of the nearest point.
//
// For regular lat/lon grids (Template30) and regular Gaussian grids
// (Template340 with Ni set), a direct index computation is used.
// For all other grid types, a brute-force search over GridCoordinates
// is performed as a fallback.
func (f *Field) NearestValue(lat, lon float64) (value float64, idx int, nearLat, nearLon float64, err error) {
	values, err := f.Values()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("grib2: NearestValue: %w", err)
	}

	switch tmpl := f.Section3.Template.(type) {
	case Template30:
		idx, nearLat, nearLon, err = nearestIndexLatLon(tmpl, lat, lon)
	case Template31:
		idx, nearLat, nearLon, err = nearestIndexRotatedLatLon(tmpl, lat, lon)
	case Template340:
		if tmpl.Ni != 0xFFFFFFFF {
			idx, nearLat, nearLon, err = nearestIndexRegularGaussian(tmpl, lat, lon)
		} else {
			idx, nearLat, nearLon, err = nearestIndexBruteForce(f, lat, lon)
		}
	default:
		idx, nearLat, nearLon, err = nearestIndexBruteForce(f, lat, lon)
	}
	if err != nil {
		return 0, 0, 0, 0, err
	}

	if idx < 0 || idx >= len(values) {
		return 0, 0, 0, 0, fmt.Errorf("grib2: NearestValue: computed index %d out of range [0, %d)", idx, len(values))
	}

	return values[idx], idx, nearLat, nearLon, nil
}

// BilinearValue returns an interpolated value at the given latitude and
// longitude using bilinear interpolation from the 4 surrounding grid points.
// Only works for regular grids (Template30, Template340 regular, Template310).
// Returns an error for irregular grids where bilinear interpolation does not
// apply cleanly.
func (f *Field) BilinearValue(lat, lon float64) (float64, error) {
	values, err := f.Values()
	if err != nil {
		return 0, fmt.Errorf("grib2: BilinearValue: %w", err)
	}

	switch tmpl := f.Section3.Template.(type) {
	case Template30:
		return bilinearLatLon(tmpl, values, lat, lon)
	case Template31:
		return bilinearRotatedLatLon(tmpl, values, lat, lon)
	case Template340:
		if tmpl.Ni == 0xFFFFFFFF {
			return 0, fmt.Errorf("grib2: BilinearValue not supported for reduced Gaussian grids")
		}
		return bilinearRegularGaussian(tmpl, values, lat, lon)
	default:
		return 0, fmt.Errorf("grib2: BilinearValue not supported for template %T", tmpl)
	}
}

// ---------------------------------------------------------------------------
// Template30 (regular lat/lon) direct index computation
// ---------------------------------------------------------------------------

// nearestIndexLatLon computes the nearest grid index for a Template30
// regular lat/lon grid using direct arithmetic (no search).
func nearestIndexLatLon(t Template30, lat, lon float64) (idx int, nearLat, nearLon float64, err error) {
	ni := int(t.Ni)
	nj := int(t.Nj)
	if ni <= 0 || nj <= 0 {
		return 0, 0, 0, fmt.Errorf("grib2: invalid grid dimensions Ni=%d, Nj=%d", ni, nj)
	}

	lat1, lon1, di, dj := latLonGridParams(t)

	// Compute fractional indices
	fi := (lon - lon1) / di
	fj := (lat - lat1) / dj

	// Handle longitude wrapping: if the query longitude is far from the
	// grid origin, try wrapping by +/- 360.
	if di > 0 {
		if fi < -0.5 {
			fi = (lon + 360 - lon1) / di
		} else if fi > float64(ni)-0.5 {
			fi = (lon - 360 - lon1) / di
		}
	}

	i := int(math.Round(fi))
	j := int(math.Round(fj))

	// Clamp to valid range
	i = clamp(i, 0, ni-1)
	j = clamp(j, 0, nj-1)

	nearLat = lat1 + float64(j)*dj
	nearLon = lon1 + float64(i)*di
	idx = j*ni + i
	return idx, nearLat, nearLon, nil
}

// bilinearLatLon performs bilinear interpolation on a Template30 regular
// lat/lon grid.
func bilinearLatLon(t Template30, values []float64, lat, lon float64) (float64, error) {
	ni := int(t.Ni)
	nj := int(t.Nj)
	if ni < 2 || nj < 2 {
		return 0, fmt.Errorf("grib2: grid too small for bilinear interpolation: Ni=%d, Nj=%d", ni, nj)
	}

	lat1, lon1, di, dj := latLonGridParams(t)

	fi := (lon - lon1) / di
	fj := (lat - lat1) / dj

	// Handle longitude wrapping
	if di > 0 {
		if fi < 0 {
			fi = (lon + 360 - lon1) / di
		} else if fi >= float64(ni) {
			fi = (lon - 360 - lon1) / di
		}
	}

	// Floor indices for the surrounding cell
	i0 := int(math.Floor(fi))
	j0 := int(math.Floor(fj))

	// Fractional offsets within the cell
	xFrac := fi - float64(i0)
	yFrac := fj - float64(j0)

	// Clamp so we can always access (i0, j0), (i0+1, j0), (i0, j0+1), (i0+1, j0+1)
	if i0 < 0 {
		i0 = 0
		xFrac = 0
	}
	if i0 >= ni-1 {
		i0 = ni - 2
		xFrac = 1
	}
	if j0 < 0 {
		j0 = 0
		yFrac = 0
	}
	if j0 >= nj-1 {
		j0 = nj - 2
		yFrac = 1
	}

	i1 := i0 + 1
	j1 := j0 + 1

	// Fetch the four corner values
	v00 := gridVal(values, j0*ni+i0)
	v10 := gridVal(values, j0*ni+i1)
	v01 := gridVal(values, j1*ni+i0)
	v11 := gridVal(values, j1*ni+i1)

	// Check for NaN (missing) values in corners
	if math.IsNaN(v00) || math.IsNaN(v10) || math.IsNaN(v01) || math.IsNaN(v11) {
		return 0, fmt.Errorf("grib2: BilinearValue: one or more surrounding grid points are missing (NaN)")
	}

	// Bilinear interpolation
	result := v00*(1-xFrac)*(1-yFrac) +
		v10*xFrac*(1-yFrac) +
		v01*(1-xFrac)*yFrac +
		v11*xFrac*yFrac

	return result, nil
}

// latLonGridParams extracts the origin, step, and direction from a Template30.
// All returned values are in degrees. dj is signed (negative if lat decreases).
func latLonGridParams(t Template30) (lat1, lon1, di, dj float64) {
	lat1 = float64(t.LatitudeOfFirstGridPoint) / 1e6
	lon1 = float64(t.LongitudeOfFirstGridPoint) / 1e6
	lat2 := float64(t.LatitudeOfLastGridPoint) / 1e6

	ni := int(t.Ni)
	nj := int(t.Nj)

	if t.IDirectionIncrement != 0 && t.IDirectionIncrement != 0xFFFFFFFF {
		di = float64(t.IDirectionIncrement) / 1e6
	} else if ni > 1 {
		lon2 := float64(t.LongitudeOfLastGridPoint) / 1e6
		di = (lon2 - lon1) / float64(ni-1)
	}

	if t.JDirectionIncrement != 0 && t.JDirectionIncrement != 0xFFFFFFFF {
		dj = float64(t.JDirectionIncrement) / 1e6
	} else if nj > 1 {
		dj = math.Abs(lat2-lat1) / float64(nj-1)
	}

	// After scanning-mode reorder, the canonical order is north-to-south.
	// If lat1 > lat2, the j-step should be negative.
	if lat2 < lat1 {
		dj = -dj
	}
	return
}

// ---------------------------------------------------------------------------
// Template31 (rotated lat/lon) O(1) index computation
// ---------------------------------------------------------------------------

// nearestIndexRotatedLatLon converts the target geographic (lat,lon) into
// rotated coordinates, then applies the same direct index arithmetic as
// Template30. This is O(1) instead of O(n) brute-force search.
func nearestIndexRotatedLatLon(t Template31, lat, lon float64) (idx int, nearLat, nearLon float64, err error) {
	ni := int(t.Ni)
	nj := int(t.Nj)
	if ni <= 0 || nj <= 0 {
		return 0, 0, 0, fmt.Errorf("grib2: invalid grid dimensions Ni=%d, Nj=%d", ni, nj)
	}

	southPoleLat := float64(t.LatitudeOfSouthernPole) * 1e-6
	southPoleLon := float64(t.LongitudeOfSouthernPole) * 1e-6
	angleOfRot := float64(t.AngleOfRotation) * 1e-6

	// Convert target geographic coordinates to rotated coordinates.
	rLat, rLon := coordRotate(lat, lon, angleOfRot, southPoleLat, southPoleLon)

	// Both GridCoordinates() and Values() now return data in canonical order
	// (north-to-south, west-to-east). Compute index in canonical order.
	//
	// Canonical layout: row 0 = northernmost rotated lat, row nj-1 = southernmost.
	// col 0 = westernmost rotated lon, col ni-1 = easternmost.
	rawLat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	rawLon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	rawLat2 := float64(t.LatitudeOfLastGridPoint) * 1e-6
	rawLon2 := float64(t.LongitudeOfLastGridPoint) * 1e-6

	var di, dj float64
	if t.IDirectionIncrement != 0 && t.IDirectionIncrement != 0xFFFFFFFF {
		di = float64(t.IDirectionIncrement) * 1e-6
	} else if ni > 1 {
		di = math.Abs(rawLon2-rawLon1) / float64(ni-1)
	}
	if t.JDirectionIncrement != 0 && t.JDirectionIncrement != 0xFFFFFFFF {
		dj = float64(t.JDirectionIncrement) * 1e-6
	} else if nj > 1 {
		dj = math.Abs(rawLat2-rawLat1) / float64(nj-1)
	}

	// Canonical origin: NW corner of rotated grid.
	canonLat0 := math.Max(rawLat1, rawLat2) // northernmost
	canonLon0 := math.Min(rawLon1, rawLon2) // westernmost
	canonDj := -dj                           // south = increasing j

	fi := (rLon - canonLon0) / di
	fj := (rLat - canonLat0) / canonDj

	// Handle longitude wrapping.
	if di > 0 {
		if fi < -0.5 {
			fi = (rLon + 360 - canonLon0) / di
		} else if fi > float64(ni)-0.5 {
			fi = (rLon - 360 - canonLon0) / di
		}
	}

	i := int(math.Round(fi))
	j := int(math.Round(fj))
	i = clamp(i, 0, ni-1)
	j = clamp(j, 0, nj-1)

	// The nearest point in canonical rotated space — unrotate to geographic.
	nearRotLat := canonLat0 + float64(j)*canonDj
	nearRotLon := canonLon0 + float64(i)*di
	nearLat, nearLon = coordUnrotate(nearRotLat, nearRotLon, angleOfRot, southPoleLat, southPoleLon)

	idx = j*ni + i
	return idx, nearLat, nearLon, nil
}

// bilinearRotatedLatLon performs bilinear interpolation on a Template31
// rotated lat/lon grid by working in rotated coordinates.
func bilinearRotatedLatLon(t Template31, values []float64, lat, lon float64) (float64, error) {
	ni := int(t.Ni)
	nj := int(t.Nj)
	if ni < 2 || nj < 2 {
		return 0, fmt.Errorf("grib2: grid too small for bilinear interpolation: Ni=%d, Nj=%d", ni, nj)
	}

	southPoleLat := float64(t.LatitudeOfSouthernPole) * 1e-6
	southPoleLon := float64(t.LongitudeOfSouthernPole) * 1e-6
	angleOfRot := float64(t.AngleOfRotation) * 1e-6

	rLat, rLon := coordRotate(lat, lon, angleOfRot, southPoleLat, southPoleLon)

	bRawLat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	bRawLon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	bRawLat2 := float64(t.LatitudeOfLastGridPoint) * 1e-6
	bRawLon2 := float64(t.LongitudeOfLastGridPoint) * 1e-6

	var di, dj float64
	if t.IDirectionIncrement != 0 && t.IDirectionIncrement != 0xFFFFFFFF {
		di = float64(t.IDirectionIncrement) * 1e-6
	} else if ni > 1 {
		di = math.Abs(bRawLon2-bRawLon1) / float64(ni-1)
	}
	if t.JDirectionIncrement != 0 && t.JDirectionIncrement != 0xFFFFFFFF {
		dj = float64(t.JDirectionIncrement) * 1e-6
	} else if nj > 1 {
		dj = math.Abs(bRawLat2-bRawLat1) / float64(nj-1)
	}

	canonLat0 := math.Max(bRawLat1, bRawLat2)
	canonLon0 := math.Min(bRawLon1, bRawLon2)
	canonDj := -dj

	fi := (rLon - canonLon0) / di
	fj := (rLat - canonLat0) / canonDj

	if di > 0 {
		if fi < 0 {
			fi = (rLon + 360 - canonLon0) / di
		} else if fi >= float64(ni) {
			fi = (rLon - 360 - canonLon0) / di
		}
	}

	i0 := int(math.Floor(fi))
	j0 := int(math.Floor(fj))
	xFrac := fi - float64(i0)
	yFrac := fj - float64(j0)

	if i0 < 0 {
		i0 = 0
		xFrac = 0
	}
	if j0 < 0 {
		j0 = 0
		yFrac = 0
	}
	if i0 >= ni-1 {
		i0 = ni - 2
		xFrac = 1
	}
	if j0 >= nj-1 {
		j0 = nj - 2
		yFrac = 1
	}

	v00 := gridVal(values, j0*ni+i0)
	v10 := gridVal(values, j0*ni+i0+1)
	v01 := gridVal(values, (j0+1)*ni+i0)
	v11 := gridVal(values, (j0+1)*ni+i0+1)

	if math.IsNaN(v00) || math.IsNaN(v10) || math.IsNaN(v01) || math.IsNaN(v11) {
		return 0, fmt.Errorf("grib2: BilinearValue: one or more surrounding grid points are missing (NaN)")
	}

	return v00*(1-xFrac)*(1-yFrac) + v10*xFrac*(1-yFrac) + v01*(1-xFrac)*yFrac + v11*xFrac*yFrac, nil
}

// ---------------------------------------------------------------------------
// Template340 regular Gaussian direct index computation
// ---------------------------------------------------------------------------

func nearestIndexRegularGaussian(t Template340, lat, lon float64) (int, float64, float64, error) {
	ni := int(t.Ni)
	nj := int(t.Nj)
	if ni <= 0 || nj <= 0 {
		return 0, 0, 0, fmt.Errorf("grib2: invalid grid dimensions Ni=%d, Nj=%d", ni, nj)
	}

	lat1 := float64(t.LatitudeOfFirstGridPoint) / 1e6
	lon1 := float64(t.LongitudeOfFirstGridPoint) / 1e6
	lat2 := float64(t.LatitudeOfLastGridPoint) / 1e6

	var di float64
	if t.IDirectionIncrement != 0 && t.IDirectionIncrement != 0xFFFFFFFF {
		di = float64(t.IDirectionIncrement) / 1e6
	} else if ni > 1 {
		lon2 := float64(t.LongitudeOfLastGridPoint) / 1e6
		di = (lon2 - lon1) / float64(ni-1)
	}

	// Approximate dj for Gaussian (evenly-spaced approximation)
	dj := (lat2 - lat1) / float64(nj-1)

	fi := (lon - lon1) / di
	fj := (lat - lat1) / dj

	i := int(math.Round(fi))
	j := int(math.Round(fj))

	i = clamp(i, 0, ni-1)
	j = clamp(j, 0, nj-1)

	nearLat := lat1 + float64(j)*dj
	nearLon := lon1 + float64(i)*di
	idx := j*ni + i
	return idx, nearLat, nearLon, nil
}

func bilinearRegularGaussian(t Template340, values []float64, lat, lon float64) (float64, error) {
	ni := int(t.Ni)
	nj := int(t.Nj)
	if ni < 2 || nj < 2 {
		return 0, fmt.Errorf("grib2: grid too small for bilinear interpolation: Ni=%d, Nj=%d", ni, nj)
	}

	lat1 := float64(t.LatitudeOfFirstGridPoint) / 1e6
	lon1 := float64(t.LongitudeOfFirstGridPoint) / 1e6
	lat2 := float64(t.LatitudeOfLastGridPoint) / 1e6

	var di float64
	if t.IDirectionIncrement != 0 && t.IDirectionIncrement != 0xFFFFFFFF {
		di = float64(t.IDirectionIncrement) / 1e6
	} else if ni > 1 {
		lon2 := float64(t.LongitudeOfLastGridPoint) / 1e6
		di = (lon2 - lon1) / float64(ni-1)
	}

	dj := (lat2 - lat1) / float64(nj-1)

	fi := (lon - lon1) / di
	fj := (lat - lat1) / dj

	i0 := int(math.Floor(fi))
	j0 := int(math.Floor(fj))

	xFrac := fi - float64(i0)
	yFrac := fj - float64(j0)

	if i0 < 0 {
		i0 = 0
		xFrac = 0
	}
	if i0 >= ni-1 {
		i0 = ni - 2
		xFrac = 1
	}
	if j0 < 0 {
		j0 = 0
		yFrac = 0
	}
	if j0 >= nj-1 {
		j0 = nj - 2
		yFrac = 1
	}

	i1 := i0 + 1
	j1 := j0 + 1

	v00 := gridVal(values, j0*ni+i0)
	v10 := gridVal(values, j0*ni+i1)
	v01 := gridVal(values, j1*ni+i0)
	v11 := gridVal(values, j1*ni+i1)

	if math.IsNaN(v00) || math.IsNaN(v10) || math.IsNaN(v01) || math.IsNaN(v11) {
		return 0, fmt.Errorf("grib2: BilinearValue: one or more surrounding grid points are missing (NaN)")
	}

	result := v00*(1-xFrac)*(1-yFrac) +
		v10*xFrac*(1-yFrac) +
		v01*(1-xFrac)*yFrac +
		v11*xFrac*yFrac

	return result, nil
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
