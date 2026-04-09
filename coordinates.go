package grib2

import (
	"encoding/binary"
	"fmt"
	"math"
)

// GridCoordinates returns the latitude and longitude (in degrees) of every
// grid point in the field, in the same order as the data values returned
// by Field.Values(). Returns slices of length NumberOfDataPoints.
func (f *Field) GridCoordinates() (lats, lons []float64, err error) {
	s3 := f.Section3
	switch tmpl := s3.Template.(type) {
	case Template30:
		lats, lons, err = gridCoordsLatLon(tmpl, s3.NumberOfDataPoints)
	case Template31:
		lats, lons, err = gridCoordsRotatedLatLon(tmpl, s3.NumberOfDataPoints)
	case Template310:
		lats, lons, err = gridCoordsMercator(tmpl, s3.NumberOfDataPoints)
	case Template320:
		lats, lons, err = gridCoordsPolarStereographic(tmpl, s3.NumberOfDataPoints)
	case Template330:
		lats, lons, err = gridCoordsLambert(tmpl, s3.NumberOfDataPoints)
	case Template340:
		lats, lons, err = gridCoordsGaussian(tmpl, s3.NumberOfDataPoints, s3.OptionalPointNumbers, s3.OctetsForNumberOfPoints)
	case Template390:
		lats, lons, err = gridCoordsSpaceView(tmpl, s3.NumberOfDataPoints)
	default:
		return nil, nil, fmt.Errorf("grib2: GridCoordinates not implemented for template %T", tmpl)
	}
	if err != nil {
		return nil, nil, err
	}

	// Reorder coordinates to canonical order (matching Values()).
	// The grid-specific functions produce coordinates in raw scanning order;
	// Values() applies applyScanningMode to get canonical order. We must match.
	scanMode, ni, nj := gridScanParams(s3.Template)
	if scanMode != 0 && ni > 0 && nj > 0 {
		lats = applyScanningMode(lats, scanMode, ni, nj)
		lons = applyScanningMode(lons, scanMode, ni, nj)
	}

	return lats, lons, nil
}

// ---------------------------------------------------------------------------
// Scanning mode helpers
// ---------------------------------------------------------------------------

// scanFlags decodes the Flag Table 3.4 scanning mode byte.
//
//	Bit 1 (0x80): i direction: 0 = +i (west to east), 1 = -i (east to west)
//	Bit 2 (0x40): j direction: 0 = -j (north to south), 1 = +j (south to north)
//	Bit 3 (0x20): adjacency:   0 = consecutive in i, 1 = consecutive in j
func scanFlags(mode uint8) (iNeg, jPos, jConsec bool) {
	iNeg = mode&0x80 != 0
	jPos = mode&0x40 != 0
	jConsec = mode&0x20 != 0
	return
}

// ---------------------------------------------------------------------------
// Template 3.0 -- Latitude/Longitude (equidistant cylindrical)
// ---------------------------------------------------------------------------

func gridCoordsLatLon(t Template30, nDataPoints uint32) ([]float64, []float64, error) {
	ni := int(t.Ni)
	nj := int(t.Nj)

	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	lat2 := float64(t.LatitudeOfLastGridPoint) * 1e-6
	lon2 := float64(t.LongitudeOfLastGridPoint) * 1e-6

	iNeg, jPos, _ := scanFlags(t.ScanningMode)

	// Compute di from first/last longitudes (more robust than the encoded value)
	var di float64
	if ni > 1 {
		if iNeg {
			if lon1 > lon2 {
				di = (lon1 - lon2) / float64(ni-1)
			} else {
				di = (lon1 + 360.0 - lon2) / float64(ni-1)
			}
		} else {
			if lon2 > lon1 {
				di = (lon2 - lon1) / float64(ni-1)
			} else {
				di = (lon2 + 360.0 - lon1) / float64(ni-1)
			}
		}
	}
	if iNeg {
		di = -di
	}

	// Compute dj from first/last latitudes
	var dj float64
	if nj > 1 {
		if jPos {
			dj = (lat2 - lat1) / float64(nj-1)
		} else {
			dj = (lat1 - lat2) / float64(nj-1)
		}
	}
	// Ensure dj goes in the right direction: from lat1 toward lat2
	if !jPos {
		dj = -dj // north to south => decreasing latitude
	}

	// Build 1-D lat/lon arrays
	latRow := make([]float64, nj)
	lonCol := make([]float64, ni)
	for j := 0; j < nj; j++ {
		latRow[j] = lat1 + float64(j)*dj
	}
	if nj > 0 {
		latRow[nj-1] = lat2 // avoid rounding drift
	}
	for i := 0; i < ni; i++ {
		lonCol[i] = lon1 + float64(i)*di
	}
	if ni > 0 {
		lonCol[ni-1] = lon2 // avoid rounding drift
	}

	n := int(nDataPoints)
	lats := make([]float64, n)
	lons := make([]float64, n)

	for idx := 0; idx < n; idx++ {
		j := idx / ni
		i := idx % ni
		if j >= nj {
			j = nj - 1
		}
		if i >= ni {
			i = ni - 1
		}
		lats[idx] = latRow[j]
		lons[idx] = lonCol[i]
	}

	return lats, lons, nil
}

// ---------------------------------------------------------------------------
// Template 3.1 -- Rotated Latitude/Longitude
// ---------------------------------------------------------------------------

func gridCoordsRotatedLatLon(t Template31, nDataPoints uint32) ([]float64, []float64, error) {
	ni := int(t.Ni)
	nj := int(t.Nj)

	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	lat2 := float64(t.LatitudeOfLastGridPoint) * 1e-6
	lon2 := float64(t.LongitudeOfLastGridPoint) * 1e-6

	iNeg, jPos, _ := scanFlags(t.ScanningMode)

	var di float64
	if ni > 1 {
		if iNeg {
			if lon1 > lon2 {
				di = (lon1 - lon2) / float64(ni-1)
			} else {
				di = (lon1 + 360.0 - lon2) / float64(ni-1)
			}
		} else {
			if lon2 > lon1 {
				di = (lon2 - lon1) / float64(ni-1)
			} else {
				di = (lon2 + 360.0 - lon1) / float64(ni-1)
			}
		}
	}
	if iNeg {
		di = -di
	}

	var dj float64
	if nj > 1 {
		if jPos {
			dj = (lat2 - lat1) / float64(nj-1)
		} else {
			dj = (lat1 - lat2) / float64(nj-1)
		}
	}
	if !jPos {
		dj = -dj
	}

	latRow := make([]float64, nj)
	lonCol := make([]float64, ni)
	for j := 0; j < nj; j++ {
		latRow[j] = lat1 + float64(j)*dj
	}
	if nj > 0 {
		latRow[nj-1] = lat2
	}
	for i := 0; i < ni; i++ {
		lonCol[i] = lon1 + float64(i)*di
	}
	if ni > 0 {
		lonCol[ni-1] = lon2
	}

	southPoleLat := float64(t.LatitudeOfSouthernPole) * 1e-6
	southPoleLon := float64(t.LongitudeOfSouthernPole) * 1e-6
	angleOfRotation := float64(t.AngleOfRotation) * 1e-6

	n := int(nDataPoints)
	lats := make([]float64, n)
	lons := make([]float64, n)

	for idx := 0; idx < n; idx++ {
		j := idx / ni
		i := idx % ni
		if j >= nj {
			j = nj - 1
		}
		if i >= ni {
			i = ni - 1
		}
		rlat, rlon := coordUnrotate(latRow[j], lonCol[i], angleOfRotation, southPoleLat, southPoleLon)
		lats[idx] = rlat
		lons[idx] = rlon
	}

	return lats, lons, nil
}

// coordRotate converts geographic (lat,lon) to rotated (lat,lon).
// This is the exact inverse of coordUnrotate.
// The unrotate applies: R_z(oAngle) * R_y(tAngle) to (x,y,z) then subtracts angleOfRot from lon.
// The rotate must: add angleOfRot to lon, then apply the TRANSPOSE: R_y(-tAngle) * R_z(-oAngle).
func coordRotate(lat, lon, angleOfRot, southPoleLat, southPoleLon float64) (float64, float64) {
	const deg2rad = math.Pi / 180.0
	const rad2deg = 180.0 / math.Pi

	// Use the same angle definitions as unrotate.
	tAngle := -(90.0 + southPoleLat)
	oAngle := -southPoleLon

	sinT := math.Sin(deg2rad * tAngle)
	cosT := math.Cos(deg2rad * tAngle)
	sinO := math.Sin(deg2rad * oAngle)
	cosO := math.Cos(deg2rad * oAngle)

	// Convert geographic to Cartesian.
	latr := lat * deg2rad
	lonr := lon * deg2rad
	xg := math.Cos(lonr) * math.Cos(latr)
	yg := math.Sin(lonr) * math.Cos(latr)
	zg := math.Sin(latr)

	// The unrotate matrix M maps rotated→geographic:
	//   xg = cosT*cosO*xr + sinO*yr + sinT*cosO*zr
	//   yg = -cosT*sinO*xr + cosO*yr - sinT*sinO*zr
	//   zg = -sinT*xr + cosT*zr
	//
	// The inverse (transpose, since M is orthogonal) maps geographic→rotated:
	//   xr = cosT*cosO*xg - cosT*sinO*yg - sinT*zg
	//   yr = sinO*xg + cosO*yg
	//   zr = sinT*cosO*xg - sinT*sinO*yg + cosT*zg

	xr := cosT*cosO*xg - cosT*sinO*yg - sinT*zg
	yr := sinO*xg + cosO*yg
	zr := sinT*cosO*xg - sinT*sinO*yg + cosT*zg

	if zr > 1.0 {
		zr = 1.0
	}
	if zr < -1.0 {
		zr = -1.0
	}

	retLat := math.Asin(zr) * rad2deg
	retLon := math.Atan2(yr, xr) * rad2deg

	retLon += angleOfRot

	return retLat, retLon
}

// coordUnrotate converts rotated (lat,lon) to geographic (lat,lon).
// Based on ecKit RotateGrid::unrotate as used in eccodes.
func coordUnrotate(inlat, inlon, angleOfRot, southPoleLat, southPoleLon float64) (float64, float64) {
	const deg2rad = math.Pi / 180.0
	const rad2deg = 180.0 / math.Pi

	latr := inlat * deg2rad
	lonr := inlon * deg2rad
	xd := math.Cos(lonr) * math.Cos(latr)
	yd := math.Sin(lonr) * math.Cos(latr)
	zd := math.Sin(latr)

	tAngle := -(90.0 + southPoleLat)
	oAngle := -southPoleLon

	sinT := math.Sin(deg2rad * tAngle)
	cosT := math.Cos(deg2rad * tAngle)
	sinO := math.Sin(deg2rad * oAngle)
	cosO := math.Cos(deg2rad * oAngle)

	x := cosT*cosO*xd + sinO*yd + sinT*cosO*zd
	y := -cosT*sinO*xd + cosO*yd - sinT*sinO*zd
	z := -sinT*xd + cosT*zd

	if z > 1.0 {
		z = 1.0
	}
	if z < -1.0 {
		z = -1.0
	}

	retLat := math.Asin(z) * rad2deg
	retLon := math.Atan2(y, x) * rad2deg

	retLat = math.Round(retLat*1e6) / 1e6
	retLon = math.Round(retLon*1e6) / 1e6

	retLon -= angleOfRot

	return retLat, retLon
}

// ---------------------------------------------------------------------------
// Template 3.10 -- Mercator
// ---------------------------------------------------------------------------

func gridCoordsMercator(t Template310, nDataPoints uint32) ([]float64, []float64, error) {
	ni := int(t.Ni)
	nj := int(t.Nj)

	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	laD := float64(t.LaD) * 1e-6

	diMetres := float64(t.Di) * 1e-3
	djMetres := float64(t.Dj) * 1e-3

	orientDeg := float64(t.OrientationOfTheGrid) * 1e-6
	orientRad := orientDeg * math.Pi / 180.0

	radius := coordEarthRadius(t.ShapeOfEarth,
		t.ScaleFactorOfRadiusOfSphericalEarth, t.ScaledValueOfRadiusOfSphericalEarth,
		t.ScaleFactorOfEarthMajorAxis, t.ScaledValueOfEarthMajorAxis,
		t.ScaleFactorOfEarthMinorAxis, t.ScaledValueOfEarthMinorAxis)
	// For a spherical Earth, major == minor == radius
	major := radius
	minor := radius

	const deg2rad = math.Pi / 180.0
	const rad2deg = 180.0 / math.Pi

	latFirstRad := lat1 * deg2rad
	lonFirstRad := lon1 * deg2rad
	laDRad := laD * deg2rad

	temp := minor / major
	es := 1.0 - temp*temp
	e := math.Sqrt(es)
	m1 := math.Cos(laDRad) / math.Sqrt(1.0-es*math.Sin(laDRad)*math.Sin(laDRad))

	// Forward: initial point -> x0, y0
	sinphi := math.Sin(latFirstRad)
	ts := coordComputeT(e, latFirstRad, sinphi)
	x0 := major * m1 * coordAdjustLonRadians(lonFirstRad-orientRad)
	y0 := -major * m1 * math.Log(ts)
	x0 = -x0
	y0 = -y0

	n := int(nDataPoints)
	lats := make([]float64, n)
	lons := make([]float64, n)

	for j := 0; j < nj; j++ {
		y := float64(j) * djMetres
		for i := 0; i < ni; i++ {
			idx := i + j*ni
			if idx >= n {
				break
			}
			x := float64(i) * diMetres
			_x := x - x0
			_y := y - y0
			ts2 := math.Exp(-_y / (major * m1))
			latRad := coordComputePhi(e, ts2)
			lonRad := coordAdjustLonRadians(orientRad + _x/(major*m1))
			lats[idx] = latRad * rad2deg
			lons[idx] = coordNormaliseLon(lonRad * rad2deg)
		}
	}

	return lats, lons, nil
}

// ---------------------------------------------------------------------------
// Template 3.20 -- Polar Stereographic
// ---------------------------------------------------------------------------

func gridCoordsPolarStereographic(t Template320, nDataPoints uint32) ([]float64, []float64, error) {
	nx := int(t.Nx)
	ny := int(t.Ny)

	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	laD := float64(t.LaD) * 1e-6
	orient := float64(t.OrientationOfTheGrid) * 1e-6

	dxMetres := float64(t.Dx) * 1e-3
	dyMetres := float64(t.Dy) * 1e-3

	radius := coordEarthRadius(t.ShapeOfEarth,
		t.ScaleFactorOfRadiusOfSphericalEarth, t.ScaledValueOfRadiusOfSphericalEarth,
		t.ScaleFactorOfEarthMajorAxis, t.ScaledValueOfEarthMajorAxis,
		t.ScaleFactorOfEarthMinorAxis, t.ScaledValueOfEarthMinorAxis)

	const deg2rad = math.Pi / 180.0
	const rad2deg = 180.0 / math.Pi
	const piOver2 = math.Pi / 2.0
	const epsilon = 1.0e-10

	centralLon := orient * deg2rad
	centralLat := laD * deg2rad
	lonFirst := lon1 * deg2rad
	latFirst := lat1 * deg2rad

	// Forward projection initialisation
	var sign float64 = 1.0
	if centralLat < 0 {
		sign = -1.0
	}
	var fwdInd int
	var fwdMcs, fwdTcs float64
	if math.Abs(math.Abs(centralLat)-piOver2) > epsilon {
		fwdInd = 1
		con1 := sign * centralLat
		fwdMcs = math.Cos(con1)
		fwdTcs = math.Tan(0.5 * (piOver2 - con1))
	}

	// Forward: first point -> x0, y0
	con1 := sign * (lonFirst - centralLon)
	ts := math.Tan(0.5 * (piOver2 - sign*latFirst))
	var height float64
	if fwdInd != 0 {
		height = radius * fwdMcs * ts / fwdTcs
	} else {
		height = 2.0 * radius * ts
	}
	x0 := sign * height * math.Sin(con1)
	y0 := -sign * height * math.Cos(con1)
	x0 = -x0
	y0 = -y0

	// Inverse projection initialisation
	var invInd int
	var invMcs, invTcs float64
	if math.Abs(math.Abs(centralLat)-piOver2) > epsilon {
		invInd = 1
		con1 = sign * centralLat
		invMcs = math.Cos(con1)
		invTcs = math.Tan(0.5 * (piOver2 - con1))
	}

	n := int(nDataPoints)
	lats := make([]float64, n)
	lons := make([]float64, n)

	y := 0.0
	for j := 0; j < ny; j++ {
		x := 0.0
		for i := 0; i < nx; i++ {
			idx := i + j*nx
			if idx >= n {
				break
			}
			_x := (x - x0) * sign
			_y := (y - y0) * sign
			rh := math.Sqrt(_x*_x + _y*_y)
			if invInd != 0 {
				ts = rh * invTcs / (radius * invMcs)
			} else {
				ts = rh / (radius * 2.0)
			}
			lat := sign * (piOver2 - 2*math.Atan(ts))
			var lon float64
			if rh == 0 {
				lon = sign * centralLon
			} else {
				lon = sign*math.Atan2(_x, -_y) + centralLon
			}
			lats[idx] = lat * rad2deg
			lonDeg := lon * rad2deg
			for lonDeg < 0 {
				lonDeg += 360
			}
			for lonDeg > 360 {
				lonDeg -= 360
			}
			lons[idx] = lonDeg

			x += dxMetres
		}
		y += dyMetres
	}

	return lats, lons, nil
}

// ---------------------------------------------------------------------------
// Template 3.30 -- Lambert Conformal Conic (spherical)
// ---------------------------------------------------------------------------

func gridCoordsLambert(t Template330, nDataPoints uint32) ([]float64, []float64, error) {
	nx := int(t.Nx)
	ny := int(t.Ny)

	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	loV := float64(t.LoV) * 1e-6
	laD := float64(t.LaD) * 1e-6
	latin1 := float64(t.Latin1) * 1e-6
	latin2 := float64(t.Latin2) * 1e-6

	dxMetres := float64(t.Dx) * 1e-3
	dyMetres := float64(t.Dy) * 1e-3

	radius := coordEarthRadius(t.ShapeOfEarth,
		t.ScaleFactorOfRadiusOfSphericalEarth, t.ScaledValueOfRadiusOfSphericalEarth,
		t.ScaleFactorOfEarthMajorAxis, t.ScaledValueOfEarthMajorAxis,
		t.ScaleFactorOfEarthMinorAxis, t.ScaledValueOfEarthMinorAxis)

	const deg2rad = math.Pi / 180.0

	latFirstRad := lat1 * deg2rad
	lonFirstRad := lon1 * deg2rad
	loVRad := loV * deg2rad
	latin1Rad := latin1 * deg2rad
	latin2Rad := latin2 * deg2rad
	laDRad := laD * deg2rad

	// Compute cone constant n
	var n float64
	if math.Abs(latin1Rad-latin2Rad) < 1e-9 {
		n = math.Sin(latin1Rad)
	} else {
		n = math.Log(math.Cos(latin1Rad)/math.Cos(latin2Rad)) /
			math.Log(math.Tan(math.Pi/4+latin2Rad/2)/math.Tan(math.Pi/4+latin1Rad/2))
	}

	f := (math.Cos(latin1Rad) * math.Pow(math.Tan(math.Pi/4+latin1Rad/2), n)) / n
	rho0Bare := f * math.Pow(math.Tan(math.Pi/4+laDRad/2), -n)

	rho := radius * f * math.Pow(math.Tan(math.Pi/4+latFirstRad/2), -n)
	rho0 := radius * rho0Bare

	lonDiff := lonFirstRad - loVRad
	if lonDiff > math.Pi {
		lonDiff -= 2 * math.Pi
	}
	if lonDiff < -math.Pi {
		lonDiff += 2 * math.Pi
	}
	angle := n * lonDiff
	x0 := rho * math.Sin(angle)
	y0 := rho0 - rho*math.Cos(angle)

	nn := int(nDataPoints)
	lats := make([]float64, nn)
	lons := make([]float64, nn)

	for j := 0; j < ny; j++ {
		yy := y0 + float64(j)*dyMetres
		for i := 0; i < nx; i++ {
			idx := i + j*nx
			if idx >= nn {
				break
			}
			xx := x0 + float64(i)*dxMetres
			latDeg, lonDeg := coordLambertXY2LatLon(radius, n, f, rho0Bare, loVRad, xx, yy)
			lons[idx] = coordNormaliseLon(lonDeg)
			lats[idx] = latDeg
		}
	}

	return lats, lons, nil
}

// coordLambertXY2LatLon converts projection (x,y) to (lat,lon) degrees
// for the spherical Lambert conformal conic projection.
// Based on eccodes xy2lonlat.
func coordLambertXY2LatLon(radius, n, f, rho0Bare, loVRad, x, y float64) (latDeg, lonDeg float64) {
	const rad2deg = 180.0 / math.Pi
	x /= radius
	y /= radius
	y = rho0Bare - y
	rho := math.Hypot(x, y)
	if rho != 0 {
		if n < 0 {
			rho = -rho
			x = -x
			y = -y
		}
		latRad := 2*math.Atan(math.Pow(f/rho, 1.0/n)) - math.Pi/2
		lonRad := math.Atan2(x, y) / n
		lonDeg = (lonRad + loVRad) * rad2deg
		latDeg = latRad * rad2deg
	} else {
		lonDeg = 0.0
		if n > 0 {
			latDeg = 90.0
		} else {
			latDeg = -90.0
		}
	}
	return
}

// ---------------------------------------------------------------------------
// Template 3.40 -- Gaussian Latitude/Longitude
// ---------------------------------------------------------------------------

func gridCoordsGaussian(t Template340, nDataPoints uint32, optPL []byte, octetsPerPL uint8) ([]float64, []float64, error) {
	nj := int(t.Nj)
	N := int(t.N)

	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	lon2 := float64(t.LongitudeOfLastGridPoint) * 1e-6

	iNeg, jPos, _ := scanFlags(t.ScanningMode)

	// Compute full Gaussian latitudes (2*N values, descending from north to south)
	gaussLats, err := coordGaussianLatitudes(N)
	if err != nil {
		return nil, nil, err
	}

	// Find the starting index matching lat1
	istart := coordGaussianSearch(gaussLats, lat1)
	if istart < 0 {
		return nil, nil, fmt.Errorf("grib2: gaussian latitude %.6f not found in N=%d grid", lat1, N)
	}

	// Extract nj latitudes starting from istart
	size := len(gaussLats)
	latRow := make([]float64, nj)
	if jPos {
		for j := 0; j < nj; j++ {
			idx := istart - j
			if idx < 0 {
				idx += size
			}
			latRow[j] = gaussLats[idx]
		}
	} else {
		for j := 0; j < nj; j++ {
			idx := istart + j
			if idx >= size {
				idx -= size
			}
			latRow[j] = gaussLats[idx]
		}
	}

	// Check if this is a reduced Gaussian grid
	isReduced := t.Ni == 0xFFFFFFFF && len(optPL) > 0

	if isReduced {
		return gridCoordsReducedGaussian(latRow, nj, nDataPoints, optPL, octetsPerPL)
	}

	// Regular Gaussian grid
	ni := int(t.Ni)

	var di float64
	if ni > 1 {
		if iNeg {
			if lon1 > lon2 {
				di = (lon1 - lon2) / float64(ni-1)
			} else {
				di = (lon1 + 360.0 - lon2) / float64(ni-1)
			}
		} else {
			if lon2 > lon1 {
				di = (lon2 - lon1) / float64(ni-1)
			} else {
				di = (lon2 + 360.0 - lon1) / float64(ni-1)
			}
		}
	}
	if iNeg {
		di = -di
	}

	lonCol := make([]float64, ni)
	for i := 0; i < ni; i++ {
		lonCol[i] = lon1 + float64(i)*di
	}
	if ni > 0 {
		lonCol[ni-1] = lon2
	}

	n := int(nDataPoints)
	lats := make([]float64, n)
	lons := make([]float64, n)

	for idx := 0; idx < n; idx++ {
		j := idx / ni
		i := idx % ni
		if j >= nj {
			j = nj - 1
		}
		if i >= ni {
			i = ni - 1
		}
		lats[idx] = latRow[j]
		lons[idx] = lonCol[i]
	}

	return lats, lons, nil
}

func gridCoordsReducedGaussian(latRow []float64, nj int, nDataPoints uint32, optPL []byte, octetsPerPL uint8) ([]float64, []float64, error) {
	pl := coordDecodePL(optPL, octetsPerPL, nj)

	n := int(nDataPoints)
	lats := make([]float64, n)
	lons := make([]float64, n)

	idx := 0
	for j := 0; j < nj && j < len(pl); j++ {
		rowCount := pl[j]
		for i := 0; i < rowCount; i++ {
			if idx >= n {
				break
			}
			lons[idx] = float64(i) * 360.0 / float64(rowCount)
			lats[idx] = latRow[j]
			idx++
		}
	}

	return lats, lons, nil
}

// ---------------------------------------------------------------------------
// Template 3.90 -- Space View Perspective (Geostationary Satellite)
// ---------------------------------------------------------------------------

func gridCoordsSpaceView(t Template390, nDataPoints uint32) ([]float64, []float64, error) {
	nx := int(t.Nx)
	ny := int(t.Ny)

	lop := float64(t.LongitudeOfSubSatellitePoint) * 1e-6

	dx := float64(t.Dx)
	dy := float64(t.Dy)
	xp := float64(t.Xp) * 1e-3
	yp := float64(t.Yp) * 1e-3
	nrInRadiusOfEarth := float64(t.Nr) * 1e-6

	x0 := int(t.Xo)
	y0 := int(t.Yo)

	iNeg, jPos, _ := scanFlags(t.ScanningMode)

	radius := coordEarthRadius(t.ShapeOfEarth,
		t.ScaleFactorOfRadiusOfSphericalEarth, t.ScaledValueOfRadiusOfSphericalEarth,
		t.ScaleFactorOfEarthMajorAxis, t.ScaledValueOfEarthMajorAxis,
		t.ScaleFactorOfEarthMinorAxis, t.ScaledValueOfEarthMinorAxis)
	rEq := radius * 0.001  // metres to km
	rPol := radius * 0.001 // spherical: same as rEq

	if nrInRadiusOfEarth == 0 {
		return nil, nil, fmt.Errorf("grib2: space view Nr must be > 0")
	}

	angularSize := 2.0 * math.Asin(1.0/nrInRadiusOfEarth)
	height := nrInRadiusOfEarth * rEq

	if dx == 0 || dy == 0 {
		return nil, nil, fmt.Errorf("grib2: space view Dx and Dy must be > 0")
	}
	rx := angularSize / dx
	ry := (rPol / rEq) * angularSize / dy

	// Adjust xp, yp for scanning mode and origin offset
	if !iNeg {
		xp = xp - float64(x0)
	} else {
		xp = float64(nx-1) - (xp - float64(x0))
	}
	if jPos {
		yp = yp - float64(y0)
	} else {
		yp = float64(ny-1) - (yp - float64(y0))
	}

	factor2 := (rEq / rPol) * (rEq / rPol)
	factor1 := height*height - rEq*rEq

	const rad2deg = 180.0 / math.Pi

	// Pre-compute sin/cos for x
	sxArr := make([]float64, nx)
	cxArr := make([]float64, nx)
	for ix := 0; ix < nx; ix++ {
		xAngle := (float64(ix) - xp) * rx
		sxArr[ix] = math.Sin(xAngle)
		cxArr[ix] = math.Sqrt(1.0 - sxArr[ix]*sxArr[ix])
	}

	n := int(nDataPoints)
	lats := make([]float64, n)
	lons := make([]float64, n)

	idx := 0
	for iy := ny - 1; iy >= 0; iy-- {
		yAngle := (float64(iy) - yp) * ry
		sinY := math.Sin(yAngle)
		cosY := math.Sqrt(1.0 - sinY*sinY)

		tmp1 := 1 + (factor2-1.0)*sinY*sinY

		for ix := 0; ix < nx; ix++ {
			if idx >= n {
				break
			}
			sinX := sxArr[ix]
			cosX := cxArr[ix]

			sd := height * cosX * cosY
			sd = sd*sd - tmp1*factor1
			if sd <= 0.0 {
				lats[idx] = 0
				lons[idx] = 0
			} else {
				sd = math.Sqrt(sd)
				sn := (height*cosX*cosY - sd) / tmp1
				s1 := height - sn*cosX*cosY
				s2 := sn * sinX * cosY
				s3 := sn * sinY
				sxy := math.Sqrt(s1*s1 + s2*s2)
				lons[idx] = math.Atan(s2/s1)*rad2deg + lop
				lats[idx] = math.Atan(factor2*s3/sxy) * rad2deg
			}
			lon := lons[idx]
			for lon < 0 {
				lon += 360
			}
			for lon > 360 {
				lon -= 360
			}
			lons[idx] = lon
			idx++
		}
	}

	return lats, lons, nil
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// coordEarthRadius returns the Earth radius in metres for the given
// ShapeOfEarth code (GRIB2 Table 3.2).
func coordEarthRadius(shape uint8, scaleFacRadius uint8, scaledRadius uint32,
	scaleFacMajor uint8, scaledMajor uint32,
	scaleFacMinor uint8, scaledMinor uint32) float64 {
	switch shape {
	case 0:
		return 6367470.0
	case 1:
		r := float64(scaledRadius)
		if scaleFacRadius > 0 {
			r /= math.Pow(10, float64(scaleFacRadius))
		}
		return r
	case 6:
		return 6371229.0
	case 8:
		return 6371200.0
	default:
		if scaledMajor > 0 {
			r := float64(scaledMajor)
			if scaleFacMajor > 0 {
				r /= math.Pow(10, float64(scaleFacMajor))
			}
			return r
		}
		return 6371229.0
	}
}

// coordComputeT computes the Snyder small t parameter.
func coordComputeT(eccent, phi, sinphi float64) float64 {
	con := eccent * sinphi
	com := 0.5 * eccent
	con = math.Pow((1.0-con)/(1.0+con), com)
	return math.Tan(0.5*(math.Pi/2-phi)) / con
}

// coordComputePhi computes the latitude from Snyder small t (inverse Mercator).
func coordComputePhi(eccent, ts float64) float64 {
	eccnth := 0.5 * eccent
	phi := math.Pi/2 - 2*math.Atan(ts)
	for i := 0; i <= 15; i++ {
		sinpi := math.Sin(phi)
		con := eccent * sinpi
		dphi := math.Pi/2 - 2*math.Atan(ts*math.Pow((1.0-con)/(1.0+con), eccnth)) - phi
		phi += dphi
		if math.Abs(dphi) <= 1e-10 {
			return phi
		}
	}
	return phi
}

// coordAdjustLonRadians adjusts longitude (radians) to [-pi, pi].
func coordAdjustLonRadians(lon float64) float64 {
	if lon > math.Pi {
		lon -= 2 * math.Pi
	}
	if lon < -math.Pi {
		lon += 2 * math.Pi
	}
	return lon
}

// coordNormaliseLon normalises longitude (degrees) to [0, 360).
func coordNormaliseLon(lon float64) float64 {
	for lon < 0 {
		lon += 360
	}
	for lon >= 360 {
		lon -= 360
	}
	return lon
}

// ---------------------------------------------------------------------------
// Gaussian latitude computation
// ---------------------------------------------------------------------------

// coordGaussianLatitudes computes 2*N Gaussian latitude values in degrees,
// ordered from north to south (descending).
// Based on the eccodes compute_gaussian_latitudes function.
func coordGaussianLatitudes(N int) ([]float64, error) {
	if N <= 0 {
		return nil, fmt.Errorf("grib2: invalid Gaussian N=%d", N)
	}
	nlat := N * 2
	lats := make([]float64, nlat)

	firstGuess := coordGaussFirstGuess(N)

	convval := 1.0 - (2.0/math.Pi)*(2.0/math.Pi)*0.25
	denom := math.Sqrt((float64(nlat)+0.5)*(float64(nlat)+0.5) + convval)

	const precision = 1.0e-14
	const maxIter = 10

	for jlat := 0; jlat < N; jlat++ {
		root := math.Cos(firstGuess[jlat] / denom)
		conv := 1.0
		iter := 0
		var legfonc float64
		for math.Abs(conv) >= precision {
			mem2 := 1.0
			mem1 := root
			for legi := 0; legi < nlat; legi++ {
				legfonc = ((2.0*float64(legi+1) - 1.0) * root * mem1 - float64(legi)*mem2) / float64(legi+1)
				mem2 = mem1
				mem1 = legfonc
			}
			conv = legfonc / (float64(nlat) * (mem2 - root*legfonc) / (1.0 - root*root))
			root -= conv
			iter++
			if iter > maxIter {
				return nil, fmt.Errorf("grib2: gaussian latitude computation did not converge for N=%d, jlat=%d", N, jlat)
			}
		}
		lats[jlat] = math.Asin(root) * 180.0 / math.Pi
		lats[nlat-1-jlat] = -lats[jlat]
	}

	return lats, nil
}

// coordGaussFirstGuess returns initial guesses for Gaussian latitude roots
// based on Bessel function zeros.
func coordGaussFirstGuess(N int) []float64 {
	gvals := []float64{
		2.4048255577e0, 5.5200781103e0, 8.6537279129e0,
		11.7915344391e0, 14.9309177086e0, 18.0710639679e0,
		21.2116366299e0, 24.3524715308e0, 27.4934791320e0,
		30.6346064684e0, 33.7758202136e0, 36.9170983537e0,
		40.0584257646e0, 43.1997917132e0, 46.3411883717e0,
		49.4826098974e0, 52.6240518411e0, 55.7655107550e0,
		58.9069839261e0, 62.0484691902e0, 65.1899648002e0,
		68.3314693299e0, 71.4729816036e0, 74.6145006437e0,
		77.7560256304e0, 80.8975558711e0, 84.0390907769e0,
		87.1806298436e0, 90.3221726372e0, 93.4637187819e0,
		96.6052679510e0, 99.7468198587e0, 102.8883742542e0,
		106.0299309165e0, 109.1714896498e0, 112.3130502805e0,
		115.4546126537e0, 118.5961766309e0, 121.7377420880e0,
		124.8793089132e0, 128.0208770059e0, 131.1624462752e0,
		134.3040166383e0, 137.4455880203e0, 140.5871603528e0,
		143.7287335737e0, 146.8703076258e0, 150.0118824570e0,
		153.1534580192e0, 156.2950342685e0,
	}
	vals := make([]float64, N)
	for i := 0; i < N; i++ {
		if i < len(gvals) {
			vals[i] = gvals[i]
		} else {
			vals[i] = vals[i-1] + math.Pi
		}
	}
	return vals
}

// coordGaussianSearch finds the index of 'target' in the descending Gaussian
// latitudes array. Returns -1 if not found.
func coordGaussianSearch(lats []float64, target float64) int {
	const epsilon = 1e-3
	low, high := 0, len(lats)-1
	for low <= high {
		mid := (low + high) / 2
		if math.Abs(target-lats[mid]) < epsilon {
			return mid
		}
		// lats is descending
		if target < lats[mid] {
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return -1
}

// coordDecodePL decodes the optional PL (points per latitude) array from raw bytes.
func coordDecodePL(data []byte, octetsPerEntry uint8, nj int) []int {
	if octetsPerEntry == 0 {
		octetsPerEntry = 2
	}
	step := int(octetsPerEntry)
	pl := make([]int, 0, nj)
	for off := 0; off+step <= len(data) && len(pl) < nj; off += step {
		switch step {
		case 1:
			pl = append(pl, int(data[off]))
		case 2:
			pl = append(pl, int(binary.BigEndian.Uint16(data[off:off+2])))
		case 4:
			pl = append(pl, int(binary.BigEndian.Uint32(data[off:off+4])))
		default:
			var val uint32
			for b := 0; b < step && off+b < len(data); b++ {
				val = val<<8 | uint32(data[off+b])
			}
			pl = append(pl, int(val))
		}
	}
	return pl
}
