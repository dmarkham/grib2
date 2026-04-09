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
// regular lat/lon grid. It finds the 4 surrounding grid points and picks
// the one with the smallest haversine (great-circle) distance.
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

	// Handle longitude wrapping: try +360 and -360 and pick whichever
	// puts fi closer to the valid range [0, ni-1].
	if di > 0 {
		fiPlus := (lon + 360 - lon1) / di
		fiMinus := (lon - 360 - lon1) / di
		mid := float64(ni-1) / 2
		if math.Abs(fiPlus-mid) < math.Abs(fi-mid) {
			fi = fiPlus
		}
		if math.Abs(fiMinus-mid) < math.Abs(fi-mid) {
			fi = fiMinus
		}
	}

	// Find the 4 surrounding grid points
	i0 := int(math.Floor(fi))
	j0 := int(math.Floor(fj))
	i1 := i0 + 1
	j1 := j0 + 1

	// Clamp to valid range
	i0 = clamp(i0, 0, ni-1)
	i1 = clamp(i1, 0, ni-1)
	j0 = clamp(j0, 0, nj-1)
	j1 = clamp(j1, 0, nj-1)

	// Evaluate haversine distance to all 4 candidates, pick the closest
	candidates := [4][2]int{{j0, i0}, {j0, i1}, {j1, i0}, {j1, i1}}
	bestDist := math.MaxFloat64
	bestJ, bestI := j0, i0
	for _, c := range candidates {
		cj, ci := c[0], c[1]
		cLat := lat1 + float64(cj)*dj
		cLon := lon1 + float64(ci)*di
		d := haversineDeg(lat, lon, cLat, cLon)
		if d < bestDist {
			bestDist = d
			bestJ = cj
			bestI = ci
		}
	}

	nearLat = lat1 + float64(bestJ)*dj
	nearLon = lon1 + float64(bestI)*di
	idx = bestJ*ni + bestI
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

	// Handle longitude wrapping: pick the shift that puts fi closest
	// to the valid range [0, ni-1].
	if di > 0 {
		fiPlus := (lon + 360 - lon1) / di
		fiMinus := (lon - 360 - lon1) / di
		mid := float64(ni-1) / 2
		if math.Abs(fiPlus-mid) < math.Abs(fi-mid) {
			fi = fiPlus
		}
		if math.Abs(fiMinus-mid) < math.Abs(fi-mid) {
			fi = fiMinus
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

// rotatedCanonicalParams computes the canonical grid origin and step sizes
// for a Template31 rotated lat/lon grid. The canonical order is
// north-to-south, west-to-east (after applyScanningMode).
func rotatedCanonicalParams(t Template31) (canonLat0, canonLon0, canonDi, canonDj float64) {
	ni := int(t.Ni)
	nj := int(t.Nj)

	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	lat2 := float64(t.LatitudeOfLastGridPoint) * 1e-6
	lon2 := float64(t.LongitudeOfLastGridPoint) * 1e-6

	iNeg, jPos, _ := scanFlags(t.ScanningMode)

	// Compute di from the grid endpoints (same logic as gridCoordsRotatedLatLon).
	var di float64
	if ni > 1 {
		if iNeg {
			if lon1 > lon2 {
				di = (lon1 - lon2) / float64(ni-1)
			} else {
				di = (lon1 + 360 - lon2) / float64(ni-1)
			}
		} else {
			if lon2 > lon1 {
				di = (lon2 - lon1) / float64(ni-1)
			} else {
				di = (lon2 + 360 - lon1) / float64(ni-1)
			}
		}
	}

	var dj float64
	if nj > 1 {
		if jPos {
			dj = (lat2 - lat1) / float64(nj-1)
		} else {
			dj = (lat1 - lat2) / float64(nj-1)
		}
	}

	// Canonical origin after applyScanningMode:
	// - iNeg is flipped: col 0 = westernmost
	// - jPos is flipped: row 0 = northernmost
	if jPos {
		canonLat0 = lat2
		canonDj = -dj
	} else {
		canonLat0 = lat1
		canonDj = -dj
	}
	if iNeg {
		canonLon0 = lon2
	} else {
		canonLon0 = lon1
	}
	canonDi = di
	return
}

// nearestIndexRotatedLatLon finds the nearest grid point for a Template31
// rotated lat/lon grid. It converts the target to rotated coordinates,
// finds the 4 surrounding grid points, unrotates each back to geographic,
// and picks the one with the smallest haversine distance. This matches
// the eccodes approach and is correct at grid edges where simple rounding
// in rotated space picks the wrong point.
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

	canonLat0, canonLon0, canonDi, canonDj := rotatedCanonicalParams(t)

	fi := (rLon - canonLon0) / canonDi
	fj := (rLat - canonLat0) / canonDj

	// Handle longitude wrapping: try +360 and -360 and pick whichever
	// puts fi closer to the valid range [0, ni-1].
	if canonDi > 0 {
		fiPlus := (rLon + 360 - canonLon0) / canonDi
		fiMinus := (rLon - 360 - canonLon0) / canonDi
		mid := float64(ni-1) / 2
		if math.Abs(fiPlus-mid) < math.Abs(fi-mid) {
			fi = fiPlus
		}
		if math.Abs(fiMinus-mid) < math.Abs(fi-mid) {
			fi = fiMinus
		}
	}

	// Find the 4 surrounding grid points
	i0 := int(math.Floor(fi))
	j0 := int(math.Floor(fj))
	i1 := i0 + 1
	j1 := j0 + 1

	i0 = clamp(i0, 0, ni-1)
	i1 = clamp(i1, 0, ni-1)
	j0 = clamp(j0, 0, nj-1)
	j1 = clamp(j1, 0, nj-1)

	// For each of the 4 candidates, unrotate to geographic and compute
	// haversine distance from the target geographic point.
	candidates := [4][2]int{{j0, i0}, {j0, i1}, {j1, i0}, {j1, i1}}
	bestDist := math.MaxFloat64
	bestJ, bestI := j0, i0
	var bestLat, bestLon float64

	for _, c := range candidates {
		cj, ci := c[0], c[1]
		cRotLat := canonLat0 + float64(cj)*canonDj
		cRotLon := canonLon0 + float64(ci)*canonDi
		cLat, cLon := coordUnrotate(cRotLat, cRotLon, angleOfRot, southPoleLat, southPoleLon)
		d := haversineDeg(lat, lon, cLat, cLon)
		if d < bestDist {
			bestDist = d
			bestJ = cj
			bestI = ci
			bestLat = cLat
			bestLon = cLon
		}
	}

	idx = bestJ*ni + bestI
	return idx, bestLat, bestLon, nil
}

// bilinearRotatedLatLon performs bilinear interpolation on a Template31
// rotated lat/lon grid. It uses the same 4-point finding logic as
// nearestIndexRotatedLatLon, with bilinear weights computed in rotated
// space (which is correct since the grid IS regular in rotated space).
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

	cLat0, cLon0, cDi, cDj := rotatedCanonicalParams(t)

	fi := (rLon - cLon0) / cDi
	fj := (rLat - cLat0) / cDj

	// Handle longitude wrapping: pick the shift that puts fi closest
	// to the valid range [0, ni-1].
	if cDi > 0 {
		fiPlus := (rLon + 360 - cLon0) / cDi
		fiMinus := (rLon - 360 - cLon0) / cDi
		mid := float64(ni-1) / 2
		if math.Abs(fiPlus-mid) < math.Abs(fi-mid) {
			fi = fiPlus
		}
		if math.Abs(fiMinus-mid) < math.Abs(fi-mid) {
			fi = fiMinus
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

// haversineDeg computes the great-circle angular distance between two
// points given in degrees. The returned value is in radians and is only
// used for relative comparison (smaller = closer).
func haversineDeg(lat1, lon1, lat2, lon2 float64) float64 {
	return haversineRad(deg2rad(lat1), deg2rad(lon1), deg2rad(lat2), deg2rad(lon2))
}
