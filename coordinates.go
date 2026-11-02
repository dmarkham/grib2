package grib2

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
)

// GridCoordinates returns the latitude and longitude (in degrees) of every
// grid point in the field, in the same order as the data values returned
// by Field.Values(). Returns slices of length NumberOfDataPoints.
// Longitudes are in [0, 360).
func (f *Field) GridCoordinates() (lats, lons []float64, err error) {
	s3 := f.Section3
	n := int(s3.NumberOfDataPoints)
	switch tmpl := s3.Template.(type) {
	case Template30:
		if tmpl.Ni == missingUint32 {
			lats, lons, err = gridCoordsReducedLatLon(tmpl, s3)
		} else {
			lats, lons, err = gridCoordsRegular(tmpl)
		}
	case Template31:
		lats, lons, err = gridCoordsRotatedLatLon(tmpl)
	case Template310:
		lats, lons, err = gridCoordsMercator(tmpl)
	case Template320:
		lats, lons, err = gridCoordsPolarStereographic(tmpl)
	case Template330:
		lats, lons, err = gridCoordsLambert(tmpl)
	case Template340:
		if tmpl.Ni == missingUint32 {
			lats, lons, err = gridCoordsReducedGaussian(tmpl, s3)
		} else {
			lats, lons, err = gridCoordsRegular(tmpl)
		}
	case Template390:
		// The space view generator lists points in data order.
		lats, lons, err = gridCoordsSpaceView(tmpl, s3.NumberOfDataPoints)
		if err == nil && tmpl.ScanningMode != 0 {
			lats = applyScanningMode(lats, tmpl.ScanningMode, int(tmpl.Nx), int(tmpl.Ny))
			lons = applyScanningMode(lons, tmpl.ScanningMode, int(tmpl.Nx), int(tmpl.Ny))
		}
	default:
		return nil, nil, fmt.Errorf("grib2: GridCoordinates not implemented for template %T", tmpl)
	}
	if err != nil {
		return nil, nil, err
	}
	if len(lats) != n {
		return nil, nil, fmt.Errorf("grib2: grid has %d points, section 3 declares %d", len(lats), n)
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

// projectedCanonical lays out a projected grid in canonical order. point
// maps an offset (x, y) in metres from the first grid point to geographic
// coordinates. The first grid point is the first point in data order, so
// the i axis runs west (-x) when iScansNegatively and the j axis runs north
// (+y) when jScansPositively. The order of consecutive points (0x20) and
// alternating rows (0x10) only affect the data order, not where the points
// are.
func projectedCanonical(nx, ny int, dx, dy float64, scanMode uint8, point func(x, y float64) (lat, lon float64, err error)) ([]float64, []float64, error) {
	if nx <= 0 || ny <= 0 || nx == missingUint32 || ny == missingUint32 {
		return nil, nil, fmt.Errorf("grib2: invalid grid dimensions Nx=%d, Ny=%d", nx, ny)
	}
	if dx <= 0 || dy <= 0 {
		return nil, nil, fmt.Errorf("grib2: invalid grid lengths Dx=%v, Dy=%v", dx, dy)
	}
	iNeg, jPos, _ := scanFlags(scanMode)
	sx, sy := 1.0, -1.0
	if iNeg {
		sx = -1
	}
	if jPos {
		sy = 1
	}
	lats := make([]float64, nx*ny)
	lons := make([]float64, nx*ny)
	for j := 0; j < ny; j++ { // j and i count in scanning order
		row := j
		if jPos {
			row = ny - 1 - j
		}
		for i := 0; i < nx; i++ {
			col := i
			if iNeg {
				col = nx - 1 - i
			}
			lat, lon, err := point(sx*float64(i)*dx, sy*float64(j)*dy)
			if err != nil {
				return nil, nil, err
			}
			if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
				return nil, nil, fmt.Errorf("grib2: grid point (%d, %d) cannot be projected", i, j)
			}
			lats[row*nx+col] = lat
			lons[row*nx+col] = coordNormaliseLon(lon)
		}
	}
	return lats, lons, nil
}

// ---------------------------------------------------------------------------
// Template 3.0 / 3.40 -- regular Latitude/Longitude and regular Gaussian
// ---------------------------------------------------------------------------

func gridCoordsRegular(tmpl GridDefinitionTemplate) ([]float64, []float64, error) {
	g, err := regularGridOf(tmpl)
	if err != nil {
		return nil, nil, err
	}
	lats, lons := g.coords()
	return lats, lons, nil
}

// ---------------------------------------------------------------------------
// Template 3.1 -- Rotated Latitude/Longitude
// ---------------------------------------------------------------------------

// rotation returns the southern pole and angle of rotation of a rotated
// lat/lon grid in degrees. The angle of rotation is an IEEE float (octets
// 81-84), not a scaled integer.
func rotation(t Template31) (southPoleLat, southPoleLon, angle float64) {
	u := angleUnit(t.BasicAngle, t.SubdivisionsOfBasicAngle)
	return float64(t.LatitudeOfSouthernPole) * u, float64(t.LongitudeOfSouthernPole) * u,
		float64(math.Float32frombits(t.AngleOfRotation))
}

func gridCoordsRotatedLatLon(t Template31) ([]float64, []float64, error) {
	g, err := regularGridOf(t)
	if err != nil {
		return nil, nil, err
	}
	rLats, rLons := g.coords()
	spLat, spLon, angle := rotation(t)
	for i := range rLats {
		la, lo := coordUnrotate(rLats[i], rLons[i], angle, spLat, spLon)
		rLats[i], rLons[i] = la, coordNormaliseLon(lo)
	}
	return rLats, rLons, nil
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

	// Undo the final rotation by angleOfRot first: coordUnrotate subtracts
	// it after rotating, so the inverse adds it before.
	lon += angleOfRot

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

func gridCoordsMercator(t Template310) ([]float64, []float64, error) {
	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	laD := float64(t.LaD) * 1e-6
	orientRad := float64(t.OrientationOfTheGrid) * 1e-6 * math.Pi / 180.0

	major, minor, err := earthAxes(t.ShapeOfEarth,
		t.ScaleFactorOfRadiusOfSphericalEarth, t.ScaledValueOfRadiusOfSphericalEarth,
		t.ScaleFactorOfEarthMajorAxis, t.ScaledValueOfEarthMajorAxis,
		t.ScaleFactorOfEarthMinorAxis, t.ScaledValueOfEarthMinorAxis)
	if err != nil {
		return nil, nil, err
	}
	if math.Abs(lat1) >= 90 || math.Abs(laD) >= 90 {
		return nil, nil, fmt.Errorf("grib2: Mercator: latitudes must be within (-90, 90): lat1=%v LaD=%v", lat1, laD)
	}

	const deg2rad = math.Pi / 180.0
	const rad2deg = 180.0 / math.Pi

	latFirstRad := lat1 * deg2rad
	lonFirstRad := lon1 * deg2rad
	laDRad := laD * deg2rad

	temp := minor / major
	es := 1.0 - temp*temp
	e := math.Sqrt(es)
	m1 := math.Cos(laDRad) / math.Sqrt(1.0-es*math.Sin(laDRad)*math.Sin(laDRad))

	// Forward: first point -> x0, y0
	ts := coordComputeT(e, latFirstRad, math.Sin(latFirstRad))
	x0 := major * m1 * coordAdjustLonRadians(lonFirstRad-orientRad)
	y0 := -major * m1 * math.Log(ts)

	return projectedCanonical(int(t.Ni), int(t.Nj), float64(t.Di)*1e-3, float64(t.Dj)*1e-3, t.ScanningMode,
		func(x, y float64) (float64, float64, error) {
			ts2 := math.Exp(-(y + y0) / (major * m1))
			latRad := coordComputePhi(e, ts2)
			lonRad := coordAdjustLonRadians(orientRad + (x+x0)/(major*m1))
			return latRad * rad2deg, lonRad * rad2deg, nil
		})
}

// ---------------------------------------------------------------------------
// Template 3.20 -- Polar Stereographic
// ---------------------------------------------------------------------------

func gridCoordsPolarStereographic(t Template320) ([]float64, []float64, error) {
	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6
	laD := float64(t.LaD) * 1e-6
	orient := float64(t.OrientationOfTheGrid) * 1e-6

	major, minor, err := earthAxes(t.ShapeOfEarth,
		t.ScaleFactorOfRadiusOfSphericalEarth, t.ScaledValueOfRadiusOfSphericalEarth,
		t.ScaleFactorOfEarthMajorAxis, t.ScaledValueOfEarthMajorAxis,
		t.ScaleFactorOfEarthMinorAxis, t.ScaledValueOfEarthMinorAxis)
	if err != nil {
		return nil, nil, err
	}
	if major != minor {
		// eccodes refuses this too (tests/grib_grid_polar_stereographic.sh).
		return nil, nil, fmt.Errorf("grib2: polar stereographic on an oblate Earth (shape %d) is not supported", t.ShapeOfEarth)
	}
	radius := major

	const deg2rad = math.Pi / 180.0
	const rad2deg = 180.0 / math.Pi
	const piOver2 = math.Pi / 2.0
	const epsilon = 1.0e-10

	centralLon := orient * deg2rad
	// The projection centre flag (bit 1) says which pole is in the plane.
	southPole := t.ProjectionCentreFlag&0x80 != 0
	centralLat := laD * deg2rad
	sign := 1.0
	if southPole {
		sign = -1.0
		if centralLat > 0 {
			centralLat = -centralLat
		}
	}

	var mcs, tcs float64
	trueScale := math.Abs(math.Abs(centralLat)-piOver2) > epsilon
	if trueScale {
		con := sign * centralLat
		mcs = math.Cos(con)
		tcs = math.Tan(0.5 * (piOver2 - con))
	}
	height := func(ts float64) float64 {
		if trueScale {
			return radius * mcs * ts / tcs
		}
		return 2.0 * radius * ts
	}

	// Forward: first point -> x0, y0
	con1 := sign * (lon1*deg2rad - centralLon)
	h := height(math.Tan(0.5 * (piOver2 - sign*lat1*deg2rad)))
	x0 := sign * h * math.Sin(con1)
	y0 := -sign * h * math.Cos(con1)

	return projectedCanonical(int(t.Nx), int(t.Ny), float64(t.Dx)*1e-3, float64(t.Dy)*1e-3, t.ScanningMode,
		func(x, y float64) (float64, float64, error) {
			px := (x + x0) * sign
			py := (y + y0) * sign
			rh := math.Hypot(px, py)
			var ts float64
			if trueScale {
				ts = rh * tcs / (radius * mcs)
			} else {
				ts = rh / (radius * 2.0)
			}
			lat := sign * (piOver2 - 2*math.Atan(ts))
			lon := sign * centralLon
			if rh != 0 {
				lon = sign*math.Atan2(px, -py) + centralLon
			}
			return lat * rad2deg, lon * rad2deg, nil
		})
}

// ---------------------------------------------------------------------------
// Template 3.30 -- Lambert Conformal Conic (sphere or oblate spheroid)
// ---------------------------------------------------------------------------

func gridCoordsLambert(t Template330) ([]float64, []float64, error) {
	const deg2rad = math.Pi / 180.0
	const rad2deg = 180.0 / math.Pi
	const epsilon = 1e-10

	lat1 := float64(t.LatitudeOfFirstGridPoint) * 1e-6 * deg2rad
	lon1 := float64(t.LongitudeOfFirstGridPoint) * 1e-6 * deg2rad
	loV := float64(t.LoV) * 1e-6 * deg2rad
	laD := float64(t.LaD) * 1e-6 * deg2rad
	latin1 := float64(t.Latin1) * 1e-6 * deg2rad
	latin2 := float64(t.Latin2) * 1e-6 * deg2rad

	if math.Abs(latin1+latin2) < epsilon {
		// eccodes: standard parallels cannot be equal and on opposite sides of the equator.
		return nil, nil, fmt.Errorf("grib2: Lambert: standard parallels %v and %v are on opposite sides of the equator",
			latin1*rad2deg, latin2*rad2deg)
	}

	major, minor, err := earthAxes(t.ShapeOfEarth,
		t.ScaleFactorOfRadiusOfSphericalEarth, t.ScaledValueOfRadiusOfSphericalEarth,
		t.ScaleFactorOfEarthMajorAxis, t.ScaledValueOfEarthMajorAxis,
		t.ScaleFactorOfEarthMinorAxis, t.ScaledValueOfEarthMinorAxis)
	if err != nil {
		return nil, nil, err
	}

	// Snyder's ellipsoidal formulas (eccodes LambertConformal::init_oblate);
	// with e = 0 they reduce exactly to the spherical projection.
	e := 0.0
	if major != minor {
		e = math.Sqrt(1 - (minor/major)*(minor/major))
	}
	mOf := func(phi float64) float64 {
		s := math.Sin(phi)
		return math.Cos(phi) / math.Sqrt(1-e*e*s*s)
	}
	tOf := func(phi float64) float64 { return coordComputeT(e, phi, math.Sin(phi)) }

	var ns float64
	if math.Abs(latin1-latin2) > epsilon {
		ns = math.Log(mOf(latin1)/mOf(latin2)) / math.Log(tOf(latin1)/tOf(latin2))
	} else {
		ns = math.Sin(latin1)
	}
	F := mOf(latin1) / (ns * math.Pow(tOf(latin1), ns))
	rh := major * F * math.Pow(tOf(laD), ns)

	var rh1 float64
	if math.Abs(math.Abs(lat1)-math.Pi/2) > epsilon {
		rh1 = major * F * math.Pow(tOf(lat1), ns)
	} else if lat1*ns <= 0 {
		return nil, nil, fmt.Errorf("grib2: Lambert: first grid point cannot be projected")
	}
	theta := ns * coordAdjustLonRadians(lon1-loV)
	x0 := rh1 * math.Sin(theta)
	y0 := rh - rh1*math.Cos(theta)

	return projectedCanonical(int(t.Nx), int(t.Ny), float64(t.Dx)*1e-3, float64(t.Dy)*1e-3, t.ScanningMode,
		func(x, y float64) (float64, float64, error) {
			px := x + x0
			py := rh - (y + y0)
			r := math.Hypot(px, py)
			con := 1.0
			if ns <= 0 {
				r, con = -r, -1
			}
			th := 0.0
			if r != 0 {
				th = math.Atan2(con*px, con*py)
			}
			lat := -math.Pi / 2
			if r != 0 || ns > 0 {
				lat = coordComputePhi(e, math.Pow(r/(major*F), 1/ns))
			}
			return lat * rad2deg, coordAdjustLonRadians(th/ns+loV) * rad2deg, nil
		})
}

// ---------------------------------------------------------------------------
// Reduced grids: Template 3.0 with Ni missing, Template 3.40 with a PL list
// ---------------------------------------------------------------------------

// reducedPL decodes and checks the PL list of a reduced grid.
func reducedPL(s3 Section3, nj int) ([]int, error) {
	if len(s3.OptionalPointNumbers) == 0 || s3.OctetsForNumberOfPoints == 0 {
		return nil, fmt.Errorf("grib2: reduced grid (Ni missing) has no list of points per row")
	}
	pl := coordDecodePL(s3.OptionalPointNumbers, s3.OctetsForNumberOfPoints, nj)
	if len(pl) != nj {
		return nil, fmt.Errorf("grib2: reduced grid has %d PL entries, Nj is %d", len(pl), nj)
	}
	return pl, nil
}

func gridCoordsReducedGaussian(t Template340, s3 Section3) ([]float64, []float64, error) {
	nj := int(t.Nj)
	if nj <= 0 || t.Nj == missingUint32 {
		return nil, nil, fmt.Errorf("grib2: invalid reduced Gaussian Nj=%d", t.Nj)
	}
	if _, jPos, _ := scanFlags(t.ScanningMode); jPos || t.ScanningMode&0x80 != 0 {
		return nil, nil, fmt.Errorf("grib2: scanning mode 0x%02x on a reduced Gaussian grid is not supported", t.ScanningMode)
	}
	pl, err := reducedPL(s3, nj)
	if err != nil {
		return nil, nil, err
	}
	u := angleUnit(t.BasicAngle, t.SubdivisionsOfBasicAngle)
	lat1 := float64(t.LatitudeOfFirstGridPoint) * u
	lat2 := float64(t.LatitudeOfLastGridPoint) * u
	all, err := coordGaussianLatitudes(int(t.N))
	if err != nil {
		return nil, nil, err
	}
	start := coordGaussianSearch(all, lat1)
	if start < 0 || start+nj > len(all) {
		return nil, nil, fmt.Errorf("grib2: latitude %v is not a Gaussian latitude of N=%d", lat1, t.N)
	}
	rowLats := all[start : start+nj]

	lonFirst := ratAngle(t.LongitudeOfFirstGridPoint, t.BasicAngle, t.SubdivisionsOfBasicAngle)
	lonLast := ratAngle(t.LongitudeOfLastGridPoint, t.BasicAngle, t.SubdivisionsOfBasicAngle)
	for lonFirst.Sign() < 0 {
		lonFirst.Add(lonFirst, big.NewRat(360, 1))
	}
	for lonLast.Sign() < 0 {
		lonLast.Add(lonLast, big.NewRat(360, 1))
	}

	// eccodes is_gaussian_global (geo/grib_geography.cc).
	maxPL := 0
	for _, p := range pl {
		maxPL = max(maxPL, p)
	}
	d := math.Abs(all[0] - all[1])
	delta := 360.0 / float64(maxPL)
	ll, _ := lonLast.Float64()
	global := math.Abs(lat1-all[0]) < d && math.Abs(lat2+all[0]) < d &&
		lonFirst.Sign() == 0 && math.Abs(ll-(360-delta))-delta <= u

	lats, lons, err := reducedCoords(rowLats, pl, global, lonFirst, lonLast, int(s3.NumberOfDataPoints))
	if err != nil && global {
		// eccodes retries a "global" grid as a sub-area when the counts disagree.
		lats, lons, err = reducedCoords(rowLats, pl, false, lonFirst, lonLast, int(s3.NumberOfDataPoints))
	}
	return lats, lons, err
}

// gridCoordsReducedLatLon lays out a reduced lat/lon grid (Template 3.0 with
// Ni missing): rows equally spaced in latitude, each with pl[j] points
// equally spaced between the first and last longitude (eccodes
// geo/iterator/LatlonReduced.cc).
func gridCoordsReducedLatLon(t Template30, s3 Section3) ([]float64, []float64, error) {
	nj := int(t.Nj)
	if nj <= 0 || t.Nj == missingUint32 {
		return nil, nil, fmt.Errorf("grib2: invalid reduced lat/lon Nj=%d", t.Nj)
	}
	if t.ScanningMode != 0 {
		return nil, nil, fmt.Errorf("grib2: scanning mode 0x%02x on a reduced lat/lon grid is not supported", t.ScanningMode)
	}
	pl, err := reducedPL(s3, nj)
	if err != nil {
		return nil, nil, err
	}
	u := angleUnit(t.BasicAngle, t.SubdivisionsOfBasicAngle)
	lat1 := float64(t.LatitudeOfFirstGridPoint) * u
	lat2 := float64(t.LatitudeOfLastGridPoint) * u
	lon1 := float64(t.LongitudeOfFirstGridPoint) * u
	lon2 := float64(t.LongitudeOfLastGridPoint) * u
	if lat1 < lat2 {
		return nil, nil, fmt.Errorf("grib2: reduced lat/lon: first latitude %v south of last %v with scanning mode 0", lat1, lat2)
	}
	rows, _ := evenRows(nj)(lat1, lat2)

	maxPL := 0
	for _, p := range pl {
		maxPL = max(maxPL, p)
	}
	if maxPL == 0 {
		return nil, nil, fmt.Errorf("grib2: reduced lat/lon grid has no points")
	}
	// eccodes geo/iterator/LatlonReduced.cc: a row within two steps of 360
	// degrees is global; otherwise its pl[j] points run from the first to
	// the last longitude inclusive.
	dlon, local := 0.0, 1
	switch {
	case 360-math.Abs(lon2-lon1) < 2*360/float64(maxPL):
		dlon, local = 360, 0
	case lon2 < lon1:
		dlon = lon2 + 360 - lon1
	default:
		dlon = lon2 - lon1
	}

	n := int(s3.NumberOfDataPoints)
	lats := make([]float64, 0, n)
	lons := make([]float64, 0, n)
	for j, p := range pl {
		step := dlon / float64(max(p-local, 1)) // rows may have no points
		for i := 0; i < p; i++ {
			lats = append(lats, rows[j])
			lons = append(lons, coordNormaliseLon(lon1+float64(i)*step))
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

	major, minor, err := earthAxes(t.ShapeOfEarth,
		t.ScaleFactorOfRadiusOfSphericalEarth, t.ScaledValueOfRadiusOfSphericalEarth,
		t.ScaleFactorOfEarthMajorAxis, t.ScaledValueOfEarthMajorAxis,
		t.ScaleFactorOfEarthMinorAxis, t.ScaledValueOfEarthMinorAxis)
	if err != nil {
		return nil, nil, err
	}
	rEq := major * 0.001  // metres to km
	rPol := minor * 0.001 // equals rEq on a sphere

	if t.Nr == missingUint32 {
		return nil, nil, fmt.Errorf("grib2: space view with Nr missing (orthographic view) is not supported")
	}
	if nrInRadiusOfEarth <= 1 {
		return nil, nil, fmt.Errorf("grib2: space view Nr must be greater than one Earth radius")
	}
	if t.LatitudeOfSubSatellitePoint != 0 {
		return nil, nil, fmt.Errorf("grib2: space view with sub-satellite latitude %v is not supported", float64(t.LatitudeOfSubSatellitePoint)*1e-6)
	}
	if nx <= 0 || ny <= 0 || int(nDataPoints) != nx*ny {
		return nil, nil, fmt.Errorf("grib2: space view grid is %dx%d but section 3 declares %d points", nx, ny, nDataPoints)
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
