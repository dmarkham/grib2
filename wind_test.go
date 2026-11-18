package grib2

import (
	"math"
	"math/rand"
	"testing"
)

// hrrrWindFixture is the 500 mb u and v wind of the HRRR run of 2026-11-17
// 05Z, forecast hour 1 (messages 17 and 18 of hrrr.t05z.wrfsfcf01.grib2),
// relative to HRRR's Lambert grid. hrrrAlaskaWindFixture is the same from
// the analysis of the HRRR-Alaska run of 06Z (messages 22 and 23 of
// hrrr.t06z.wrfsfcf00.ak.grib2), relative to its polar stereographic grid
// of 1299x919 points. gfsWindFixture is the wind at the same level and
// time, 06Z, from the GFS analysis of 2026-11-17 06Z on its 0.25 degree
// lat/lon grid (messages 356 and 357 of gfs.t06z.pgrb2.0p25.f000), relative
// to east and north. All come from NOAA's public buckets.
const (
	hrrrWindFixture       = "testdata/hrrr_uv500.grib2"
	hrrrAlaskaWindFixture = "testdata/hrrr_alaska_uv500.grib2"
	gfsWindFixture        = "testdata/gfs_uv500.grib2"
)

// bearingDeg is the initial bearing of the great circle from point 1 to
// point 2, in degrees clockwise from north.
func bearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	p1, p2, dl := deg2rad(lat1), deg2rad(lat2), deg2rad(lon2-lon1)
	return math.Atan2(math.Sin(dl)*math.Cos(p2), math.Cos(p1)*math.Sin(p2)-math.Sin(p1)*math.Cos(p2)*math.Cos(dl)) * 180 / math.Pi
}

// angleDiff returns a - b in degrees, in [-180, 180).
func angleDiff(a, b float64) float64 {
	d := math.Mod(a-b+180, 360)
	if d < 0 {
		d += 360
	}
	return d - 180
}

// meanBearing averages two bearings that are close to each other.
func meanBearing(a, b float64) float64 { return a + angleDiff(b, a)/2 }

func firstField(t *testing.T, path string) *Field {
	t.Helper()
	msgs := decodeAll(t, path)
	if len(msgs) == 0 || len(msgs[0].Fields) == 0 {
		t.Fatalf("%s: no fields", path)
	}
	return &msgs[0].Fields[0]
}

// TestWindRotation_PROJ: grid north on a projected grid is where PROJ says
// it is (its meridian convergence), on every Mercator, polar stereographic
// and Lambert fixture: sphere and spheroid, north and south, tangent and
// secant.
func TestWindRotation_PROJ(t *testing.T) {
	ref := loadProjRef(t)
	widestByType := map[string]float64{}
	for p, f := range projectedFixtures(t) {
		t.Run(p, func(t *testing.T) {
			r, err := NewWindRotation(f)
			if err != nil {
				t.Fatal(err)
			}
			pts := ref.Fixtures[p].Points
			if len(pts) < 200 {
				t.Fatalf("%d reference points", len(pts))
			}
			worst, widest := 0.0, 0.0
			for _, pt := range pts {
				got := r.GridNorth(pt[0], pt[1])
				d := math.Abs(angleDiff(got, pt[4]))
				worst, widest = math.Max(worst, d), math.Max(widest, math.Abs(pt[4]))
				if d > 1e-6 {
					t.Errorf("GridNorth(%v, %v) = %v, PROJ %v", pt[0], pt[1], got, pt[4])
				}
				// Any way of writing the longitude gives the same answer.
				if g2 := r.GridNorth(pt[0], pt[1]+720); math.Abs(angleDiff(g2, got)) > 1e-9 {
					t.Errorf("GridNorth(%v, %v) = %v, but %v at longitude %v", pt[0], pt[1]+720, g2, got, pt[1])
				}
			}
			name := typeName(f.Section3.Template)
			widestByType[name] = math.Max(widestByType[name], widest)
			t.Logf("angles up to %.1f degrees, largest difference from PROJ %.2g degrees", widest, worst)
		})
	}
	// The polar stereographic and Lambert fixtures must really turn; a
	// Mercator grid's north is true north everywhere.
	if widestByType["3.20"] < 20 || widestByType["3.30"] < 20 || widestByType["3.10"] != 0 {
		t.Errorf("largest angles by grid type: %v", widestByType)
	}
}

// TestWindRotation_GridAxes: on every fixture grid, GridNorth is the
// direction the grid's columns run on the Earth, and 90 degrees clockwise
// of it is the direction the rows run. The directions are measured from the
// coordinates of neighbouring grid points, which the eccodes reference
// checks, so this covers every grid type, shape of the Earth and scanning
// mode the fixtures have.
func TestWindRotation_GridAxes(t *testing.T) {
	byType := map[string]int{}
	for _, p := range allFixturePaths(t) {
		msgs := decodeAllLenient(p)
		if len(msgs) == 0 || len(msgs[0].Fields) == 0 {
			continue
		}
		f := &msgs[0].Fields[0]
		r, err := NewWindRotation(f)
		if err != nil {
			continue
		}
		_, nx, ny := gridScanParams(f.Section3.Template)
		lats, lons, err := f.GridCoordinates()
		if err != nil || nx < 3 || ny < 3 || nx*ny != len(lats) {
			continue // reduced, truncated or undecodable grids
		}
		oblate := false
		switch tmpl := f.Section3.Template.(type) {
		case Template330:
			oblate = tmpl.ShapeOfEarth != 0 && tmpl.ShapeOfEarth != 1 && tmpl.ShapeOfEarth != 6 && tmpl.ShapeOfEarth != 8
		case Template310:
			oblate = tmpl.ShapeOfEarth != 0 && tmpl.ShapeOfEarth != 1 && tmpl.ShapeOfEarth != 6 && tmpl.ShapeOfEarth != 8
		}
		t.Run(p, func(t *testing.T) {
			rnd := rand.New(rand.NewSource(3))
			worst, widest, n := 0.0, 0.0, 0
			for range 400 {
				row, col := 1+rnd.Intn(ny-2), 1+rnd.Intn(nx-2)
				k := row*nx + col
				if math.Abs(lats[k]) > 80 || math.Abs(lats[k-nx]) > 85 || math.Abs(lats[k+nx]) > 85 {
					continue // directions lose their meaning at the poles
				}
				// The cell's size on the Earth, in degrees of arc.
				cell := haversineDeg(lats[k], lons[k], lats[k-nx], lons[k-nx]) * 180 / math.Pi
				// Towards the northern neighbour, and away from the
				// southern one: their mean is the column's direction at
				// k up to the grid lines' curvature over one cell.
				north := meanBearing(bearingDeg(lats[k], lons[k], lats[k-nx], lons[k-nx]), bearingDeg(lats[k], lons[k], lats[k+nx], lons[k+nx])+180)
				east := meanBearing(bearingDeg(lats[k], lons[k], lats[k+1], lons[k+1]), bearingDeg(lats[k], lons[k], lats[k-1], lons[k-1])+180)
				got := r.GridNorth(lats[k], lons[k])
				// On a spheroid the latitudes are geodetic, and a bearing
				// worked out from them as if on a sphere is up to 0.2
				// degrees from the true one (the flattening is 1/298).
				// TestWindRotation_PROJ checks those grids exactly.
				tol := 1e-3 + 0.02*cell*cell
				if oblate {
					tol += 0.25
				}
				dn, de := math.Abs(angleDiff(got, north)), math.Abs(angleDiff(got+90, east))
				worst, widest, n = math.Max(worst, math.Max(dn, de)), math.Max(widest, math.Abs(got)), n+1
				if dn > tol || de > tol {
					t.Fatalf("node %d (%v, %v): GridNorth = %v; columns run towards %v, rows towards %v (tolerance %.2g)",
						k, lats[k], lons[k], got, north, east, tol)
				}
			}
			if n < 50 {
				t.Fatalf("only %d nodes checked", n)
			}
			byType[typeName(f.Section3.Template)]++
			t.Logf("%d nodes, grid north up to %.1f degrees from true north, largest difference %.2g degrees", n, widest, worst)
		})
	}
	for _, want := range []string{"3.0", "3.1", "3.10", "3.20", "3.30", "3.40"} {
		if byType[want] == 0 {
			t.Errorf("no fixture with a %s grid was checked (%v)", want, byType)
		}
	}
}

// TestWindRotation_RotatedExact: on a rotated lat/lon grid, grid north is
// the direction in which the rotated latitude grows. A rotated meridian is
// a great circle, so the bearing to a point one degree further along it is
// exact. rotated_angle30 has an angle of rotation of 30 degrees, which moves
// the rotated pole.
func TestWindRotation_RotatedExact(t *testing.T) {
	for _, p := range []string{"testdata/rdps_surface.grib2", "testdata/constant_field.grib2",
		"testdata/upstream/rotated_angle30.grib2", "testdata/upstream/rotated_scan64.grib2"} {
		t.Run(p, func(t *testing.T) {
			f := firstField(t, p)
			tmpl, ok := f.Section3.Template.(Template31)
			if !ok {
				t.Fatalf("grid is %T", f.Section3.Template)
			}
			r, err := NewWindRotation(f)
			if err != nil {
				t.Fatal(err)
			}
			spLat, spLon, angle := rotation(tmpl)
			rnd := rand.New(rand.NewSource(31))
			worst, widest := 0.0, 0.0
			for range 2000 {
				rLat, rLon := rnd.Float64()*150-75, rnd.Float64()*360
				lat, lon := coordUnrotate(rLat, rLon, angle, spLat, spLon)
				lat2, lon2 := coordUnrotate(rLat+1, rLon, angle, spLat, spLon)
				if math.Abs(lat) > 80 {
					continue
				}
				want := bearingDeg(lat, lon, lat2, lon2)
				got := r.GridNorth(lat, lon)
				d := math.Abs(angleDiff(got, want))
				worst, widest = math.Max(worst, d), math.Max(widest, math.Abs(got))
				// coordUnrotate rounds to a millionth of a degree, a
				// ten-thousandth of a degree of bearing over one degree.
				if d > 2e-4 {
					t.Fatalf("(%v, %v): GridNorth = %v, the rotated meridian runs towards %v", lat, lon, got, want)
				}
			}
			t.Logf("southern pole (%v, %v), angle of rotation %v: grid north up to %.1f degrees from true north, largest difference %.2g degrees",
				spLat, spLon, angle, widest, worst)
		})
	}
}

// TestWindRotation_ToEarth: the arithmetic. A wind along grid north blows
// towards GridNorth; speed is kept; and components that are already east
// and north come back unchanged.
func TestWindRotation_ToEarth(t *testing.T) {
	f := firstField(t, hrrrWindFixture)
	r, err := NewWindRotation(f)
	if err != nil {
		t.Fatal(err)
	}
	if !r.GridRelative() {
		t.Fatal("HRRR winds are flagged relative to the grid")
	}
	// HRRR's cone touches the Earth at 38.5 N, so meridians converge by
	// sin(38.5 deg) per degree of longitude from 97.5 W.
	for _, c := range [][3]float64{{35, -97.5, 0}, {40, -122.5, -25}, {30, -72.5, 25}, {45, 262.5 + 10, 10}} {
		want := math.Sin(deg2rad(38.5)) * c[2]
		if got := r.GridNorth(c[0], c[1]); math.Abs(got-want) > 1e-9 {
			t.Errorf("GridNorth(%v, %v) = %v, want %v", c[0], c[1], got, want)
		}
	}
	rnd := rand.New(rand.NewSource(8))
	for range 1000 {
		lat, lon := 25+rnd.Float64()*25, -125+rnd.Float64()*60
		u, v := rnd.NormFloat64()*20, rnd.NormFloat64()*20
		e, n := r.ToEarth(u, v, lat, lon)
		if math.Abs(math.Hypot(e, n)-math.Hypot(u, v)) > 1e-9 {
			t.Fatalf("speed changed: (%v, %v) -> (%v, %v)", u, v, e, n)
		}
		// The direction the wind blows towards, relative to the grid and
		// then to true north.
		onGrid := math.Atan2(u, v) * 180 / math.Pi
		onEarth := math.Atan2(e, n) * 180 / math.Pi
		if d := angleDiff(onEarth, onGrid+r.GridNorth(lat, lon)); math.Abs(d) > 1e-9 {
			t.Fatalf("(%v, %v): direction %v on the grid became %v, grid north is at %v", lat, lon, onGrid, onEarth, r.GridNorth(lat, lon))
		}
	}
	// Straight along the axes at a place where grid north is 15.56 degrees
	// west of true north.
	e, n := r.ToEarth(0, 10, 40, -122.5)
	if math.Abs(math.Atan2(e, n)*180/math.Pi-r.GridNorth(40, -122.5)) > 1e-9 || e >= 0 {
		t.Errorf("a wind along grid north became (%v, %v)", e, n)
	}
	e, n = r.ToEarth(10, 0, 40, -122.5)
	if e <= 0 || n <= 0 {
		t.Errorf("a wind along grid east became (%v, %v): it should blow a little north of east", e, n)
	}

	// The same grid with the flag cleared: the components are already east
	// and north, whatever the grid's shape.
	tmpl := f.Section3.Template.(Template330)
	tmpl.ResolutionAndComponentFlags &^= 0x08
	earth, err := NewWindRotation(&Field{Section3: Section3{Template: tmpl}})
	if err != nil {
		t.Fatal(err)
	}
	if earth.GridRelative() {
		t.Fatal("flag cleared but GridRelative is true")
	}
	if e, n := earth.ToEarth(3, 4, 40, -122.5); e != 3 || n != 4 {
		t.Errorf("earth-relative components changed to (%v, %v)", e, n)
	}
	if g := earth.GridNorth(40, -122.5); math.Abs(g-r.GridNorth(40, -122.5)) > 1e-12 {
		t.Errorf("GridNorth = %v with the flag cleared", g)
	}

	gfs, err := NewWindRotation(firstField(t, gfsWindFixture))
	if err != nil {
		t.Fatal(err)
	}
	if gfs.GridRelative() || gfs.GridNorth(40, -100) != 0 {
		t.Errorf("GFS lat/lon grid: GridRelative %v, GridNorth %v", gfs.GridRelative(), gfs.GridNorth(40, -100))
	}
	rdps, err := NewWindRotation(firstField(t, "testdata/rdps_surface.grib2"))
	if err != nil {
		t.Fatal(err)
	}
	if !rdps.GridRelative() {
		t.Error("RDPS winds are flagged relative to the grid")
	}
	if _, err := NewWindRotation(&Field{Section3: Section3{Template: Template390{}}}); err == nil {
		t.Error("space view grid accepted")
	}
	if _, err := NewWindRotation(&Field{Section3: Section3{Template: Template330{}}}); err == nil {
		t.Error("empty Lambert grid definition accepted")
	}
}

// TestWindRotation_HRRRvsGFS checks the rotation, and above all its sign,
// on real winds: the 500 mb wind of HRRR (relative to its Lambert grid) and
// of HRRR-Alaska (relative to its polar stereographic grid) against the GFS
// analysis for the same time (relative to east and north). Rotated to
// earth, their wind directions agree with GFS's; left alone they are off by
// the angle of grid north, in the direction GridNorth gives; turned the
// wrong way they are off by twice that.
func TestWindRotation_HRRRvsGFS(t *testing.T) {
	gm := decodeAll(t, gfsWindFixture)
	if len(gm) != 2 {
		t.Fatalf("%d GFS messages, want 2", len(gm))
	}
	values := func(m *Message) []float64 {
		v, err := m.Fields[0].Values()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	gu, gv := values(gm[0]), values(gm[1])
	gs, err := NewSampler(&gm[0].Fields[0])
	if err != nil {
		t.Fatal(err)
	}
	if !gs.Matches(&gm[1].Fields[0]) {
		t.Fatal("GFS u and v are not on the same grid")
	}
	for _, c := range []struct {
		name, fixture                  string
		lat0, lat1, lon0, lon1         float64
		maxMean, maxRMS, minRaw, minWr float64 // bounds, with what was measured below
	}{
		// Measured: rotated mean +0.01 rms 5.99, not rotated rms 13.13,
		// rotated the wrong way rms 23.98. What is left after rotating is
		// the two models' own disagreement.
		{"HRRR, Lambert", hrrrWindFixture, 22, 52, -134, -61, 1, 8, 1.5, 3},
		// Measured: rotated mean +0.94 rms 8.95, not rotated mean +26.44
		// rms 33.43, rotated the wrong way rms 63.79. Grid north is up to
		// 55 degrees from true north on this grid.
		{"HRRR-Alaska, polar stereographic", hrrrAlaskaWindFixture, 40, 77, -190, -105, 2, 12, 1.5, 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			hm := decodeAll(t, c.fixture)
			if len(hm) != 2 {
				t.Fatalf("%d messages, want 2", len(hm))
			}
			hu, hv := values(hm[0]), values(hm[1])
			hs, err := NewSampler(&hm[0].Fields[0])
			if err != nil {
				t.Fatal(err)
			}
			if !hs.Matches(&hm[1].Fields[0]) {
				t.Fatal("u and v are not on the same grid")
			}
			rot, err := NewWindRotation(&hm[0].Fields[0])
			if err != nil {
				t.Fatal(err)
			}
			if !rot.GridRelative() {
				t.Fatal("winds are flagged relative to east and north")
			}

			var n int
			var sumRot, sumRaw, sqRot, sqRaw, sqWrong, widest float64
			for lat := c.lat0; lat <= c.lat1; lat += 0.5 {
				for lon := c.lon0; lon <= c.lon1; lon += 0.5 {
					hw, err := hs.Weights(lat, lon)
					if err != nil {
						t.Fatal(err)
					}
					if hw.Outside {
						continue
					}
					gw, _ := gs.Weights(lat, lon)
					u, _ := hw.Interpolate(hu)
					v, _ := hw.Interpolate(hv)
					ge, _ := gw.Interpolate(gu)
					gn, _ := gw.Interpolate(gv)
					if math.Hypot(u, v) < 10 || math.Hypot(ge, gn) < 10 {
						continue // direction means little in light wind
					}
					e, nn := rot.ToEarth(u, v, lat, lon)
					north := rot.GridNorth(lat, lon)
					gfsDir := math.Atan2(ge, gn) * 180 / math.Pi
					raw := angleDiff(math.Atan2(u, v)*180/math.Pi, gfsDir)
					rotated := angleDiff(math.Atan2(e, nn)*180/math.Pi, gfsDir)
					wrong := angleDiff(raw-north, 0) // turned by the same angle the other way
					n++
					sumRot, sumRaw = sumRot+rotated, sumRaw+raw
					sqRot, sqRaw, sqWrong = sqRot+rotated*rotated, sqRaw+raw*raw, sqWrong+wrong*wrong
					widest = math.Max(widest, math.Abs(north))
				}
			}
			if n < 2000 {
				t.Fatalf("only %d points with wind over 10 m/s in both models", n)
			}
			rms := func(sq float64) float64 { return math.Sqrt(sq / float64(n)) }
			t.Logf("%d points, grid north up to %.1f degrees from true north", n, widest)
			t.Logf("direction minus GFS direction: rotated mean %+.2f rms %.2f; not rotated mean %+.2f rms %.2f; rotated the wrong way rms %.2f",
				sumRot/float64(n), rms(sqRot), sumRaw/float64(n), rms(sqRaw), rms(sqWrong))
			if math.Abs(sumRot/float64(n)) > c.maxMean || rms(sqRot) > c.maxRMS {
				t.Errorf("rotated winds differ from GFS by %+.2f degrees on average, rms %.2f", sumRot/float64(n), rms(sqRot))
			}
			if rms(sqRaw) < c.minRaw*rms(sqRot) || rms(sqWrong) < c.minWr*rms(sqRot) {
				t.Errorf("rotation does not explain the difference: rms %.2f rotated, %.2f not rotated, %.2f rotated the wrong way",
					rms(sqRot), rms(sqRaw), rms(sqWrong))
			}
		})
	}
}
