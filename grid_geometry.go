package grib2

import (
	"fmt"
	"math"
	"math/big"
	"sort"
)

// This file holds the geometry shared by GridCoordinates, NearestValue,
// BilinearValue and Sampler, so all of them agree on where every canonical
// grid point is. Canonical order is the order Field.Values returns: rows from
// north to south, columns from west to east, row-major.

// angleUnit returns the size in degrees of one unit of the angles in a
// Template 3.0/3.1/3.40 grid: 10^-6 degree unless the basic angle of the
// initial production domain and its subdivisions are set, in which case a
// unit is basicAngle/subdivisions degrees (regulation 92.1.6; UK Met Office
// TIGGE data uses this).
func angleUnit(basicAngle, subdivisions uint32) float64 {
	if basicAngle == 0 || basicAngle == missingUint32 || subdivisions == 0 || subdivisions == missingUint32 {
		return 1e-6
	}
	return float64(basicAngle) / float64(subdivisions)
}

// earthAxes returns the Earth's semi-major and semi-minor axes in metres for
// a shape of the Earth code (Code Table 3.2), with the same values eccodes
// uses (definitions/grib2/templates/template.3.shape_of_the_earth.def).
// major == minor for a spherical Earth.
func earthAxes(shape uint8, sfRadius uint8, svRadius uint32,
	sfMajor uint8, svMajor uint32, sfMinor uint8, svMinor uint32) (major, minor float64, err error) {
	scaled := func(what string, sf uint8, sv uint32) (float64, error) {
		if sf == 0xFF || sv == missingUint32 {
			return 0, fmt.Errorf("grib2: shape of the Earth %d: %s is missing", shape, what)
		}
		v := float64(sv) / math.Pow10(int(sf))
		if v <= 0 {
			return 0, fmt.Errorf("grib2: shape of the Earth %d: %s is %v", shape, what, v)
		}
		return v, nil
	}
	switch shape {
	case 0:
		return 6367470, 6367470, nil
	case 1:
		r, err := scaled("radius", sfRadius, svRadius)
		return r, r, err
	case 2: // IAU 1965
		return 6378160.0, 6356775.0, nil
	case 3, 7: // axes given by the producer, in km (3) or m (7)
		a, err := scaled("major axis", sfMajor, svMajor)
		if err != nil {
			return 0, 0, err
		}
		b, err := scaled("minor axis", sfMinor, svMinor)
		if err != nil {
			return 0, 0, err
		}
		if shape == 3 {
			a, b = a*1000, b*1000
		}
		return a, b, nil
	case 4, 5: // IAG-GRS80, WGS84
		return 6378137.0, 6356752.314, nil
	case 6:
		return 6371229, 6371229, nil
	case 8:
		return 6371200, 6371200, nil
	case 9: // Airy 1830
		return 6377563.396, 6356256.909, nil
	default:
		return 0, 0, fmt.Errorf("grib2: unsupported shape of the Earth %d", shape)
	}
}

// regularGrid describes a grid whose canonical rows are latitudes and whose
// columns are equally spaced longitudes: regular lat/lon, regular Gaussian,
// and (in rotated coordinates) rotated lat/lon.
type regularGrid struct {
	ni, nj    int
	rowLats   []float64 // latitude of each canonical row, north to south
	west      float64   // longitude of canonical column 0, in [0, 360)
	dlon      float64   // column spacing in degrees, > 0 (0 if ni == 1)
	globalLon bool      // columns go all the way round, so column ni-1 neighbours column 0
}

func (g *regularGrid) lon(col int) float64 { return coordNormaliseLon(g.west + float64(col)*g.dlon) }

// newRegularGrid builds the canonical geometry from the first and last grid
// points (in degrees) and the scanning mode. The first point is the first
// point in data order, so with +j scanning it is the southern end.
func newRegularGrid(ni, nj int, lat1, lon1, lat2, lon2, di float64, scanMode uint8, rowLats func(north, south float64) ([]float64, error)) (*regularGrid, error) {
	if ni <= 0 || nj <= 0 || ni == missingUint32 || nj == missingUint32 {
		return nil, fmt.Errorf("grib2: invalid grid dimensions Ni=%d, Nj=%d", ni, nj)
	}
	iNeg, jPos, _ := scanFlags(scanMode)
	if nj > 1 && ((jPos && lat1 > lat2) || (!jPos && lat1 < lat2)) {
		return nil, fmt.Errorf("grib2: first and last latitudes (%v, %v) are inconsistent with the scanning mode 0x%02x", lat1, lat2, scanMode)
	}
	north, south := lat1, lat2
	if jPos {
		north, south = lat2, lat1
	}
	if north > 90+1e-6 || south < -90-1e-6 {
		return nil, fmt.Errorf("grib2: latitudes (%v, %v) out of range", lat1, lat2)
	}
	west, east := lon1, lon2
	if iNeg {
		west, east = lon2, lon1
	}
	g := &regularGrid{ni: ni, nj: nj, west: coordNormaliseLon(west)}
	if ni > 1 {
		span := math.Mod(east-west, 360)
		if span < 0 {
			span += 360
		}
		if span < 1e-9 {
			// First and last longitudes coincide: the row covers 360 degrees
			// and its last point repeats the first, or only the increment
			// says how wide it is.
			if di > 0 {
				span = di * float64(ni-1)
			} else {
				span = 360
			}
		}
		g.dlon = span / float64(ni-1)
		g.globalLon = math.Abs(float64(ni)*g.dlon-360) < g.dlon*1e-3
	}
	lats, err := rowLats(north, south)
	if err != nil {
		return nil, err
	}
	if len(lats) != nj {
		return nil, fmt.Errorf("grib2: grid has %d latitudes between %v and %v, Nj is %d", len(lats), north, south, nj)
	}
	g.rowLats = lats
	return g, nil
}

// evenRows returns nj equally spaced latitudes from north to south.
func evenRows(nj int) func(north, south float64) ([]float64, error) {
	return func(north, south float64) ([]float64, error) {
		lats := make([]float64, nj)
		for j := range lats {
			if nj == 1 {
				lats[j] = north
				continue
			}
			lats[j] = north - float64(j)*(north-south)/float64(nj-1)
		}
		lats[nj-1] = south // avoid rounding drift
		return lats, nil
	}
}

// gaussianRows returns the Gaussian latitudes of an N grid from north to
// south inclusive.
func gaussianRows(n int) func(north, south float64) ([]float64, error) {
	return func(north, south float64) ([]float64, error) {
		all, err := coordGaussianLatitudes(n)
		if err != nil {
			return nil, err
		}
		in := coordGaussianSearch(all, north)
		is := coordGaussianSearch(all, south)
		if in < 0 || is < 0 {
			return nil, fmt.Errorf("grib2: latitudes %v and %v are not Gaussian latitudes of N=%d", north, south, n)
		}
		return all[in : is+1], nil
	}
}

// regularGridOf returns the canonical geometry of a regular lat/lon, rotated
// lat/lon (in rotated coordinates) or regular Gaussian grid.
func regularGridOf(tmpl GridDefinitionTemplate) (*regularGrid, error) {
	switch t := tmpl.(type) {
	case Template30:
		u := angleUnit(t.BasicAngle, t.SubdivisionsOfBasicAngle)
		return newRegularGrid(int(t.Ni), int(t.Nj),
			float64(t.LatitudeOfFirstGridPoint)*u, float64(t.LongitudeOfFirstGridPoint)*u,
			float64(t.LatitudeOfLastGridPoint)*u, float64(t.LongitudeOfLastGridPoint)*u,
			increment(t.IDirectionIncrement, u), t.ScanningMode, evenRows(int(t.Nj)))
	case Template31:
		u := angleUnit(t.BasicAngle, t.SubdivisionsOfBasicAngle)
		return newRegularGrid(int(t.Ni), int(t.Nj),
			float64(t.LatitudeOfFirstGridPoint)*u, float64(t.LongitudeOfFirstGridPoint)*u,
			float64(t.LatitudeOfLastGridPoint)*u, float64(t.LongitudeOfLastGridPoint)*u,
			increment(t.IDirectionIncrement, u), t.ScanningMode, evenRows(int(t.Nj)))
	case Template340:
		if t.Ni == missingUint32 {
			return nil, fmt.Errorf("grib2: reduced Gaussian grid is not regular")
		}
		u := angleUnit(t.BasicAngle, t.SubdivisionsOfBasicAngle)
		return newRegularGrid(int(t.Ni), int(t.Nj),
			float64(t.LatitudeOfFirstGridPoint)*u, float64(t.LongitudeOfFirstGridPoint)*u,
			float64(t.LatitudeOfLastGridPoint)*u, float64(t.LongitudeOfLastGridPoint)*u,
			increment(t.IDirectionIncrement, u), t.ScanningMode, gaussianRows(int(t.N)))
	default:
		return nil, fmt.Errorf("grib2: template %T is not a regular grid", tmpl)
	}
}

// increment converts a direction increment to degrees, or 0 if missing.
func increment(v uint32, unit float64) float64 {
	if v == 0 || v == missingUint32 {
		return 0
	}
	return float64(v) * unit
}

// coords returns the canonical coordinates of every point.
func (g *regularGrid) coords() (lats, lons []float64) {
	n := g.ni * g.nj
	lats = make([]float64, n)
	lons = make([]float64, n)
	colLons := make([]float64, g.ni)
	for i := range colLons {
		colLons[i] = g.lon(i)
	}
	for j := 0; j < g.nj; j++ {
		for i := 0; i < g.ni; i++ {
			lats[j*g.ni+i] = g.rowLats[j]
			lons[j*g.ni+i] = colLons[i]
		}
	}
	return lats, lons
}

// fracRow returns the fractional row index of lat (0 at the northern row,
// increasing southwards). Outside the grid it extrapolates linearly from the
// edge row spacing.
func (g *regularGrid) fracRow(lat float64) float64 {
	r := g.rowLats
	if g.nj == 1 {
		return 0
	}
	// rowLats is descending; find the first row at or south of lat.
	k := sort.Search(g.nj, func(j int) bool { return r[j] <= lat })
	switch {
	case k == 0:
		return (r[0] - lat) / (r[0] - r[1])
	case k == g.nj:
		return float64(g.nj-1) + (r[g.nj-1]-lat)/(r[g.nj-2]-r[g.nj-1])
	default:
		return float64(k-1) + (r[k-1]-lat)/(r[k-1]-r[k])
	}
}

// fracCol returns the fractional column index of lon. For a grid that does
// not go round the globe, a point outside it is placed on whichever side is
// nearer in longitude, so the result may be negative or exceed ni-1.
func (g *regularGrid) fracCol(lon float64) float64 {
	if g.ni == 1 || g.dlon == 0 {
		return 0
	}
	d := math.Mod(lon-g.west, 360)
	if d < 0 {
		d += 360
	}
	fi := d / g.dlon
	if g.globalLon {
		return fi
	}
	last := float64(g.ni - 1)
	if fi > last {
		east := fi - last
		west := (360 - d) / g.dlon
		if west < east {
			return -west
		}
	}
	return fi
}

// colIndex wraps or clamps a column index.
func (g *regularGrid) colIndex(i int) int {
	if g.globalLon {
		i %= g.ni
		if i < 0 {
			i += g.ni
		}
		return i
	}
	return clamp(i, 0, g.ni-1)
}

// nearest returns the canonical index of the grid point closest (great
// circle) to (lat, lon), choosing among the four points around it.
// toGeo maps grid coordinates to geographic ones (identity unless rotated).
func (g *regularGrid) nearest(gLat, gLon, lat, lon float64, toGeo func(la, lo float64) (float64, float64)) (idx int, nearLat, nearLon float64) {
	fi, fj := g.fracCol(gLon), g.fracRow(gLat)
	outside := fj < 0 || fj > float64(g.nj-1) || (!g.globalLon && (fi < 0 || fi > float64(g.ni-1)))
	if outside {
		// The nearest point to a target outside the grid lies on the grid's
		// boundary, but not necessarily next to the clamped cell (great
		// circles bend), so search the whole boundary.
		return g.nearestOnBoundary(lat, lon, toGeo)
	}
	i0, j0 := int(math.Floor(fi)), int(math.Floor(fj))
	best := math.MaxFloat64
	for _, c := range [4][2]int{{j0, i0}, {j0, i0 + 1}, {j0 + 1, i0}, {j0 + 1, i0 + 1}} {
		j := clamp(c[0], 0, g.nj-1)
		i := g.colIndex(c[1])
		cLat, cLon := toGeo(g.rowLats[j], g.lon(i))
		if d := haversineDeg(lat, lon, cLat, cLon); d < best {
			best, idx, nearLat, nearLon = d, j*g.ni+i, cLat, cLon
		}
	}
	return idx, nearLat, nearLon
}

// nearestOnBoundary searches the first and last rows and columns.
func (g *regularGrid) nearestOnBoundary(lat, lon float64, toGeo func(la, lo float64) (float64, float64)) (idx int, nearLat, nearLon float64) {
	best := math.MaxFloat64
	try := func(j, i int) {
		cLat, cLon := toGeo(g.rowLats[j], g.lon(i))
		if d := haversineDeg(lat, lon, cLat, cLon); d < best {
			best, idx, nearLat, nearLon = d, j*g.ni+i, cLat, cLon
		}
	}
	for i := 0; i < g.ni; i++ {
		try(0, i)
		try(g.nj-1, i)
	}
	for j := 0; j < g.nj; j++ {
		try(j, 0)
		try(j, g.ni-1)
	}
	return idx, nearLat, nearLon
}

// weights returns the bilinear weights of the cell holding (gLat, gLon).
func (g *regularGrid) weights(gLat, gLon float64) (BilinearWeights, error) {
	if g.ni < 2 || g.nj < 2 {
		return BilinearWeights{}, fmt.Errorf("grib2: grid too small for bilinear interpolation: Ni=%d, Nj=%d", g.ni, g.nj)
	}
	fi, fj := g.fracCol(gLon), g.fracRow(gLat)
	if !g.globalLon {
		return cornerWeights(fi, fj, g.ni, g.nj), nil
	}
	// Longitude-global grid: the cell between the last and first columns
	// is a real cell, so wrap instead of clamping.
	w := cornerWeights(0, fj, 2, g.nj) // rows only
	i0 := int(math.Floor(fi))
	xFrac := fi - float64(i0)
	i0 = g.colIndex(i0)
	i1 := g.colIndex(i0 + 1)
	j0, j1 := w.Index[0]/2, w.Index[2]/2
	return BilinearWeights{
		Index:   [4]int{j0*g.ni + i0, j0*g.ni + i1, j1*g.ni + i0, j1*g.ni + i1},
		XFrac:   xFrac,
		YFrac:   w.YFrac,
		Outside: w.Outside,
	}, nil
}

// ---------------------------------------------------------------------------
// Reduced grids (Template 3.0 with Ni missing, Template 3.40 with a PL list)
// ---------------------------------------------------------------------------

// reducedRowPoints returns the number of points of a reduced row with pl
// points round the globe that lie in [lonFirst, lonLast], and the longitude
// of the first one. It ports eccodes grib_get_reduced_row_p, using exact
// rational arithmetic on the microdegree (or basic-angle) inputs.
func reducedRowPoints(pl int64, lonFirst, lonLast *big.Rat) (count int64, first float64) {
	for lonLast.Cmp(lonFirst) < 0 {
		lonLast = new(big.Rat).Add(lonLast, big.NewRat(360, 1))
	}
	inc := big.NewRat(360, pl)
	nw := ratFloor(new(big.Rat).Quo(lonFirst, inc))
	if new(big.Rat).Mul(new(big.Rat).SetInt64(nw), inc).Cmp(lonFirst) < 0 {
		nw++
	}
	ne := ratFloor(new(big.Rat).Quo(lonLast, inc))
	if new(big.Rat).Mul(new(big.Rat).SetInt64(ne), inc).Cmp(lonLast) > 0 {
		ne--
	}
	if nw > ne {
		return 0, 0
	}
	count = ne - nw + 1
	if count > pl {
		count = pl
	}
	f, _ := new(big.Rat).Mul(new(big.Rat).SetInt64(nw), inc).Float64()
	return count, f
}

// reducedRowPointsLegacy ports eccodes grib_get_reduced_row_legacy, the
// point-count rule of older (PRODGEN/libemos) sub-area products. It returns
// the count and the index of the first point round the row.
func reducedRowPointsLegacy(pl int64, lonFirst, lonLast float64) (count, iFirst int64) {
	rng := lonLast - lonFirst
	if rng < 0 {
		rng += 360
		lonFirst -= 360
	}
	p := float64(pl)
	count = int64(rng*p/360.0 + 1)
	iFirst = int64(lonFirst * p / 360.0)
	iLast := int64(lonLast * p / 360.0)
	irange := iLast - iFirst + 1
	if irange != count {
		if irange > count {
			if float64(iFirst)*360.0/p < lonFirst {
				iFirst++
				irange--
			}
			if float64(iLast)*360.0/p > lonLast {
				iLast--
				irange--
			}
		} else {
			ok := false
			if float64(iFirst-1)*360.0/p > lonFirst {
				iFirst--
				irange++
				ok = true
			}
			if float64(iLast+1)*360.0/p < lonLast {
				iLast++
				irange++
				ok = true
			}
			if !ok {
				count--
			}
		}
	} else if float64(iFirst)*360.0/p < lonFirst {
		iFirst++
	}
	if iFirst < 0 {
		iFirst += pl
	}
	return count, iFirst
}

func ratFloor(r *big.Rat) int64 {
	q := new(big.Int).Div(r.Num(), r.Denom()) // Euclidean: floors for positive denominators
	return q.Int64()
}

// ratAngle returns v*unit degrees as an exact rational.
func ratAngle(v int32, basicAngle, subdivisions uint32) *big.Rat {
	if basicAngle == 0 || basicAngle == missingUint32 || subdivisions == 0 || subdivisions == missingUint32 {
		return big.NewRat(int64(v), 1000000)
	}
	return new(big.Rat).Mul(big.NewRat(int64(v), 1), big.NewRat(int64(basicAngle), int64(subdivisions)))
}

// reducedCoords lays out a reduced grid row by row. rowLats are the
// latitudes of the rows (north to south) and pl the number of points of each
// row round the globe. For a global grid every row starts at longitude 0;
// for a sub-area only the points within [lonFirst, lonLast] are kept,
// following eccodes (geo/iterator/GaussianReduced.cc).
func reducedCoords(rowLats []float64, pl []int, global bool, lonFirst, lonLast *big.Rat, nPoints int) (lats, lons []float64, err error) {
	if len(pl) != len(rowLats) {
		return nil, nil, fmt.Errorf("grib2: reduced grid has %d rows but %d PL entries", len(rowLats), len(pl))
	}
	for j, p := range pl {
		if p <= 0 {
			return nil, nil, fmt.Errorf("grib2: invalid PL entry %d at row %d", p, j)
		}
	}
	lats = make([]float64, 0, nPoints)
	lons = make([]float64, 0, nPoints)
	if global {
		for j, p := range pl {
			for i := 0; i < p; i++ {
				lats = append(lats, rowLats[j])
				lons = append(lons, float64(i)*360.0/float64(p))
			}
		}
	} else {
		for j, p := range pl {
			count, first := reducedRowPoints(int64(p), lonFirst, lonLast)
			for i := int64(0); i < count; i++ {
				lats = append(lats, rowLats[j])
				lons = append(lons, coordNormaliseLon(first+float64(i)*360.0/float64(p)))
			}
		}
		if len(lats) != nPoints {
			// Older products counted the points of each row differently.
			lf, _ := lonFirst.Float64()
			ll, _ := lonLast.Float64()
			lats, lons = lats[:0], lons[:0]
			for j, p := range pl {
				count, iFirst := reducedRowPointsLegacy(int64(p), lf, ll)
				for i := iFirst; i < iFirst+count; i++ {
					lats = append(lats, rowLats[j])
					lons = append(lons, coordNormaliseLon(float64(i)*360.0/float64(p)))
				}
			}
		}
	}
	if len(lats) != nPoints {
		return nil, nil, fmt.Errorf("grib2: reduced grid has %d points, section 3 declares %d", len(lats), nPoints)
	}
	return lats, lons, nil
}
