package grib2

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"testing"
)

// projReference records where PROJ puts a fixed set of points on the grid
// of every Mercator, polar stereographic and Lambert conformal fixture:
// latitude, longitude, the fractional canonical column and row, and the
// meridian convergence. scripts/gen_projection_reference.py writes it; the
// tests need neither PROJ nor eccodes.
const projReference = "testdata/reference/projected_grids_proj.json"

// hrrrCloudFixture is six cloud fields (messages 112-117: boundary layer,
// low, middle, high and total cloud cover, and the cloud ceiling, which has
// missing values) of NOAA's HRRR CONUS run of 2026-11-17 05Z, forecast hour
// 1, cut by byte range from hrrr.t05z.wrfsfcf01.grib2 of the public
// noaa-hrrr-bdp-pds bucket. The grid is 1799x1059 points, 3 km apart, Lambert
// conformal on a sphere.
const hrrrCloudFixture = "testdata/hrrr_cloud.grib2"

type projRef struct {
	ProjVersion string `json:"proj_version"`
	Fixtures    map[string]struct {
		Proj   string       `json:"proj"`
		Points [][5]float64 `json:"points"` // lat, lon, column, row, PROJ's meridian convergence in degrees
	} `json:"fixtures"`
}

func loadProjRef(t *testing.T) projRef {
	t.Helper()
	raw, err := os.ReadFile(projReference)
	if err != nil {
		t.Fatal(err)
	}
	var ref projRef
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

// projectedFixtures returns the first field of every fixture on a usable
// Mercator, polar stereographic or Lambert conformal grid. Two fixtures are
// left out because their grid definitions describe no usable grid
// (TestLookup_UnusableProjectedGrids).
func projectedFixtures(t *testing.T) map[string]*Field {
	t.Helper()
	out := map[string]*Field{}
	for _, p := range allFixturePaths(t) {
		msgs := decodeAllLenient(p)
		if len(msgs) == 0 || len(msgs[0].Fields) == 0 {
			continue
		}
		f := &msgs[0].Fields[0]
		g, ok, err := projectedGridOf(f.Section3.Template)
		if ok && err == nil && g.nx*g.ny == int(f.Section3.NumberOfDataPoints) {
			out[p] = f
		}
	}
	if len(out) < 18 {
		t.Fatalf("only %d projected fixtures", len(out))
	}
	return out
}

func mustProjectedGrid(t *testing.T, f *Field) *projectedGrid {
	t.Helper()
	g, ok, err := projectedGridOf(f.Section3.Template)
	if !ok || err != nil {
		t.Fatalf("%T: %v", f.Section3.Template, err)
	}
	return g
}

// TestProjectedWeights_PROJ checks the forward projections against PROJ:
// every reference point must land on the fractional column and row PROJ
// gives, inside and outside the grid, on Mercator, polar stereographic and
// Lambert grids, on a sphere and a spheroid, north and south of the
// equator, tangent and secant, and with -i and -j scanning. Weights must
// then pick PROJ's cell and fractions.
func TestProjectedWeights_PROJ(t *testing.T) {
	ref := loadProjRef(t)
	fixtures := projectedFixtures(t)
	for p := range ref.Fixtures {
		if fixtures[p] == nil {
			t.Errorf("%s is in %s but is not a projected fixture; regenerate", p, projReference)
		}
	}
	// PROJ and this package agree to well under a millimetre; 1e-7 of a
	// cell is 0.3 mm on HRRR's 3 km grid.
	const tol = 1e-7
	for p, f := range fixtures {
		t.Run(p, func(t *testing.T) {
			r, ok := ref.Fixtures[p]
			if !ok {
				t.Fatalf("not in %s; regenerate with scripts/gen_projection_reference.py", projReference)
			}
			g := mustProjectedGrid(t, f)
			s, err := NewSampler(f)
			if err != nil {
				t.Fatal(err)
			}
			inside, outside, worst := 0, 0, 0.0
			for _, pt := range r.Points {
				lat, lon, col, row := pt[0], pt[1], pt[2], pt[3]
				fi, fj, ok := g.frac(lat, lon)
				if !ok {
					t.Fatalf("(%v, %v): no position; PROJ has column %v, row %v", lat, lon, col, row)
				}
				// Far outside the grid the plane distances are larger, and
				// so is the rounding.
				scale := math.Max(1, math.Max(math.Abs(col), math.Abs(row))/1000)
				d := math.Max(math.Abs(fi-col), math.Abs(fj-row))
				worst = math.Max(worst, d/scale)
				if d > tol*scale {
					t.Errorf("(%v, %v): column %v, row %v; PROJ %v, %v", lat, lon, fi, fj, col, row)
					continue
				}

				w, err := s.Weights(lat, lon)
				if err != nil {
					t.Fatalf("Weights(%v, %v): %v", lat, lon, err)
				}
				// Other ways of writing the same longitude are the same place.
				for _, turn := range []float64{360, -360, 720} {
					w2, err := s.Weights(lat, lon+turn)
					if err != nil || w2.Index != w.Index || w2.Outside != w.Outside ||
						math.Abs(w2.XFrac-w.XFrac) > 1e-9 || math.Abs(w2.YFrac-w.YFrac) > 1e-9 {
						t.Errorf("Weights(%v, %v) = %+v (%v), but %+v at longitude %v", lat, lon+turn, w2, err, w, lon)
					}
				}
				// Compare with the cell PROJ's position falls in, unless the
				// point is so close to a grid line that rounding decides.
				nearLine := func(v float64) bool { return math.Abs(v-math.Round(v)) < 1e-6 }
				if nearLine(col) || nearLine(row) {
					continue
				}
				want := cornerWeights(col, row, g.nx, g.ny)
				if w.Index != want.Index || w.Outside != want.Outside ||
					math.Abs(w.XFrac-want.XFrac) > tol*scale || math.Abs(w.YFrac-want.YFrac) > tol*scale {
					t.Errorf("Weights(%v, %v) = %+v, PROJ's position gives %+v", lat, lon, w, want)
				}
				if want.Outside {
					outside++
				} else {
					inside++
				}
			}
			if inside < 150 || outside < 10 {
				t.Errorf("only %d points inside and %d outside the grid", inside, outside)
			}
			t.Logf("%d inside, %d outside, largest difference from PROJ %.2g of a cell", inside, outside, worst)
		})
	}
}

// TestProjectedWeights_EveryNode: at the coordinates GridCoordinates gives
// for a grid point, Weights must put the whole weight on that point. It
// runs over every point of every projected fixture, 1.9 million of them on
// HRRR and 1.2 million on HRRR-Alaska.
func TestProjectedWeights_EveryNode(t *testing.T) {
	for p, f := range projectedFixtures(t) {
		t.Run(p, func(t *testing.T) {
			lats, lons, err := f.GridCoordinates()
			if err != nil {
				t.Fatal(err)
			}
			g := mustProjectedGrid(t, f)
			s, err := NewSampler(f)
			if err != nil {
				t.Fatal(err)
			}
			ramp := make([]float64, len(lats)) // a field that is linear in column and row
			for k := range ramp {
				ramp[k] = 3*float64(k%g.nx) - 2*float64(k/g.nx) + 0.5
			}
			worst := 0.0
			for k := range lats {
				w, err := s.Weights(lats[k], lons[k])
				if err != nil {
					t.Fatalf("node %d (%v, %v): %v", k, lats[k], lons[k], err)
				}
				if c := w.NearestCorner(); c != k {
					t.Fatalf("node %d (%v, %v): NearestCorner = %d (%+v)", k, lats[k], lons[k], c, w)
				}
				fi, fj, _ := g.frac(lats[k], lons[k])
				d := math.Max(math.Abs(fi-float64(k%g.nx)), math.Abs(fj-float64(k/g.nx)))
				worst = math.Max(worst, d)
				if d > 1e-7 {
					t.Fatalf("node %d is at column %d, row %d; Weights places it at %v, %v", k, k%g.nx, k/g.nx, fi, fj)
				}
				v, err := w.Interpolate(ramp)
				if err != nil || math.Abs(v-ramp[k]) > 1e-6 {
					t.Fatalf("node %d: interpolated %v (%v), want %v", k, v, err, ramp[k])
				}
			}
			t.Logf("%d nodes, largest offset %.2g of a cell", len(lats), worst)
		})
	}
}

// TestProjectedNearest_MatchesFullSearch: the direct NearestValue lookup must
// return exactly what the search over every grid point returns (the same
// index and the same coordinates, bit for bit) at grid points, between
// them, at cell centres and edges where neighbours are almost equally far,
// outside the grid, at the poles and on the far side of the Earth.
func TestProjectedNearest_MatchesFullSearch(t *testing.T) {
	for p, f := range projectedFixtures(t) {
		t.Run(p, func(t *testing.T) {
			g := mustProjectedGrid(t, f)
			if !g.singleValued {
				t.Fatal("grid is not taken to be single valued, so the direct lookup is not used")
			}
			lats, lons, err := f.GridCoordinates()
			if err != nil {
				t.Fatal(err)
			}
			// The full search costs one distance per grid point per query
			// (1.9 million on HRRR), so large grids get fewer queries.
			n := min(4000, 400_000_000/len(lats))
			if testing.Short() {
				n = max(n/8, 100)
			}
			r := rand.New(rand.NewSource(330))
			queries := append([][2]float64{{90, 0}, {-90, 0}, {90, 123}, {-90, -77}, {0, 0}, {0, 180}}, nearestQueries...)
			for len(queries) < n {
				row, col := r.Intn(g.ny-1), r.Intn(g.nx-1)
				k := row*g.nx + col
				kx, ky, kd := k+1, k+g.nx, k+g.nx+1
				switch len(queries) % 8 {
				case 0: // a grid point
					queries = append(queries, [2]float64{lats[k], lons[k]})
				case 1: // the middle of a cell
					queries = append(queries, [2]float64{(lats[k] + lats[kx] + lats[ky] + lats[kd]) / 4, (lons[k] + lons[kx] + lons[ky] + lons[kd]) / 4})
				case 2: // the middle of a cell's edge
					queries = append(queries, [2]float64{(lats[k] + lats[kx]) / 2, (lons[k] + lons[kx]) / 2})
				case 3:
					queries = append(queries, [2]float64{(lats[k] + lats[ky]) / 2, (lons[k] + lons[ky]) / 2})
				case 4: // anywhere on the Earth
					queries = append(queries, [2]float64{r.Float64()*180 - 90, r.Float64()*720 - 360})
				case 5: // around the grid's edge, inside and out
					e := []int{col, (g.ny-1)*g.nx + col, row * g.nx, row*g.nx + g.nx - 1}[r.Intn(4)]
					queries = append(queries, [2]float64{lats[e] + r.NormFloat64()*(lats[k]-lats[ky]), lons[e] + r.NormFloat64()*(lons[kx]-lons[k])})
				default: // anywhere in or just beyond a cell
					a, b := r.Float64()*1.6-0.3, r.Float64()*1.6-0.3
					queries = append(queries, [2]float64{lats[k] + a*(lats[kd]-lats[k]), lons[k] + b*(lons[kd]-lons[k])})
				}
			}
			wants := make([]int, len(queries))
			var wg sync.WaitGroup
			for w := range runtime.NumCPU() {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := w; i < len(queries); i += runtime.NumCPU() {
						wants[i] = nearestAmong(lats, lons, queries[i][0], queries[i][1])
					}
				}()
			}
			wg.Wait()
			direct := 0
			for i, q := range queries {
				want := wants[i]
				idx, nLat, nLon, err := nearestIndex(f, q[0], q[1])
				if err != nil {
					t.Fatalf("nearest (%v, %v): %v", q[0], q[1], err)
				}
				if idx != want {
					t.Fatalf("nearest (%v, %v) = %d (%v, %v); the full search finds %d (%v, %v)",
						q[0], q[1], idx, nLat, nLon, want, lats[want], lons[want])
				}
				if math.Float64bits(nLat) != math.Float64bits(lats[idx]) || math.Float64bits(nLon) != math.Float64bits(lons[idx]) {
					t.Fatalf("nearest (%v, %v): point %d at (%v, %v), GridCoordinates has (%v, %v)", q[0], q[1], idx, nLat, nLon, lats[idx], lons[idx])
				}
				if fi, fj, ok := g.frac(q[0], q[1]); ok && fi >= 0 && fj >= 0 && fi <= float64(g.nx-1) && fj <= float64(g.ny-1) {
					direct++
				}
			}
			if direct < len(queries)/4 {
				t.Errorf("only %d of %d queries were inside the grid", direct, len(queries))
			}
		})
	}
}

// TestLambertGrid_SingleValued: the direct nearest lookup is only used on a
// grid that holds each place once. A grid with the pole inside it, or one
// that reaches round the back of the cone, falls back to the full search
// and still answers.
func TestLambertGrid_SingleValued(t *testing.T) {
	sphere := func(lat1, lon1, laD, loV float64, nx, ny, dkm uint32) Template330 {
		micro := func(d float64) int32 { return int32(math.Round(d * 1e6)) }
		return Template330{
			ShapeOfEarth: 6, Nx: nx, Ny: ny,
			LatitudeOfFirstGridPoint: micro(lat1), LongitudeOfFirstGridPoint: micro(lon1),
			LaD: micro(laD), LoV: micro(loV), Latin1: micro(laD), Latin2: micro(laD),
			Dx: dkm * 1e6, Dy: dkm * 1e6, ScanningMode: 0x40,
		}
	}
	// Where a point at an offset (km) from the pole is, on the plane of a
	// cone tangent at laD with its central meridian at 0.
	at := func(laD, xKm, yKm float64) (lat, lon float64) {
		pole := 90.0
		if laD < 0 {
			pole = -90
		}
		g, err := lambertGrid(sphere(laD, 0, laD, 0, 2, 2, 1))
		if err != nil {
			t.Fatal(err)
		}
		px, py, ok := g.forward(pole, 0)
		if !ok {
			t.Fatal("pole has no position")
		}
		lat, lon, err = g.inverse(px+xKm*1e3, py+yKm*1e3)
		if err != nil {
			t.Fatal(err)
		}
		return lat, coordNormaliseLon(lon)
	}
	corner := func(laD, xKm, yKm float64, nx, ny, dkm uint32) Template330 {
		lat, lon := at(laD, xKm, yKm)
		return sphere(lat, lon, laD, 0, nx, ny, dkm)
	}
	cases := []struct {
		name string
		tmpl Template330
		want bool
	}{
		{"HRRR", sphere(21.138123, 237.280472, 38.5, 262.5, 1799, 1059, 3), true},
		{"north, south of the pole", corner(60, -2000, -6000, 40, 40, 100), true},
		{"north, pole inside", corner(60, -2000, -2000, 40, 40, 100), false},
		{"north, astride the gap beyond the pole", corner(60, -2000, 1000, 40, 40, 100), false},
		{"north, a corner in the gap", corner(60, -2000, 1000, 19, 40, 100), false},
		{"north, beside the gap", corner(60, -4000, 500, 19, 10, 100), true},
		{"south, north of the pole", corner(-60, -2000, 2000, 40, 40, 100), true},
		{"south, pole inside", corner(-60, -2000, -2000, 40, 40, 100), false},
		{"south, astride the gap beyond the pole", corner(-60, -3000, -3000, 40, 20, 100), false},
		{"south, a corner in the gap", corner(-60, -3000, -3000, 29, 20, 100), false},
		{"south, beside the gap", corner(-60, -4000, -1400, 19, 10, 100), true},
	}
	r := rand.New(rand.NewSource(1))
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, err := lambertGrid(c.tmpl)
			if err != nil {
				t.Fatal(err)
			}
			if g.singleValued != c.want {
				t.Fatalf("singleValued = %v, want %v", g.singleValued, c.want)
			}
			if c.tmpl.Nx > 100 {
				return // HRRR has its own tests
			}
			f := &Field{Section3: Section3{NumberOfDataPoints: c.tmpl.Nx * c.tmpl.Ny, Template: c.tmpl}}
			lats, lons, err := f.GridCoordinates()
			if err != nil {
				t.Fatal(err)
			}
			for i := range 2000 {
				lat, lon := r.Float64()*180-90, r.Float64()*360
				if i%2 == 0 { // near a grid point
					k := r.Intn(len(lats))
					lat, lon = math.Max(-90, math.Min(90, lats[k]+r.NormFloat64())), lons[k]+r.NormFloat64()*3
				}
				want := nearestAmong(lats, lons, lat, lon)
				idx, _, _, err := nearestIndex(f, lat, lon)
				if err != nil {
					t.Fatal(err)
				}
				if idx != want {
					t.Fatalf("nearest (%v, %v) = %d, the full search finds %d", lat, lon, idx, want)
				}
			}
		})
	}
}

// TestMercator_LongitudeAndPoles: a Mercator map repeats every turn of
// longitude, so a point off one side of the grid must be found just off
// that side, not a whole turn away off the other; a grid that goes all the
// way round interpolates across its seam; and the poles, which the map
// cannot show, are an error.
func TestMercator_LongitudeAndPoles(t *testing.T) {
	f := firstField(t, "testdata/mercator.grib2") // 56x44 points, 262.04 to 284.98 E
	s, err := NewSampler(f)
	if err != nil {
		t.Fatal(err)
	}
	const nx = 56
	for _, c := range []struct {
		lon      float64
		wantCol0 int // column of the western corners of the clamped cell
	}{
		{262.036 - 3, 0}, {262.036 - 3 - 360, 0}, {262.036 - 30, 0}, {-97.964 - 30, 0},
		{284.975 + 3, nx - 2}, {284.975 + 3 + 720, nx - 2}, {284.975 + 30, nx - 2},
	} {
		w, err := s.Weights(20, c.lon)
		if err != nil {
			t.Fatal(err)
		}
		if !w.Outside || w.Index[0]%nx != c.wantCol0 {
			t.Errorf("longitude %v: Weights = %+v, want outside, clamped to column %d", c.lon, w, c.wantCol0)
		}
	}
	for _, lat := range []float64{90, -90} {
		if w, err := s.Weights(lat, 270); err == nil {
			t.Errorf("latitude %v: Weights = %+v, want an error", lat, w)
		}
		if _, nLat, _, err := nearestIndex(f, lat, 270); err != nil || (lat > 0) != (nLat > 25) {
			t.Errorf("latitude %v: nearest grid point at latitude %v (%v)", lat, nLat, err)
		}
	}

	// A grid of 360 columns one degree apart, all the way round.
	const radius, cols, rows = 6371229.0, 360, 121
	cell := 2 * math.Pi * radius / cols
	world := Template310{
		ShapeOfEarth: 6, Ni: cols, Nj: rows,
		LatitudeOfFirstGridPoint: -45_000_000, LongitudeOfFirstGridPoint: 0, LaD: 0,
		ScanningMode: 0x40, Di: uint32(math.Round(cell * 1e3)), Dj: uint32(math.Round(cell * 1e3)),
	}
	wf := &Field{Section3: Section3{NumberOfDataPoints: cols * rows, Template: world}}
	g, err := mercatorGrid(world)
	if err != nil {
		t.Fatal(err)
	}
	if !g.aroundTheWorld() || !g.singleValued {
		t.Fatalf("aroundTheWorld %v, singleValued %v, %v columns to a turn", g.aroundTheWorld(), g.singleValued, g.period)
	}
	ws, err := NewSampler(wf)
	if err != nil {
		t.Fatal(err)
	}
	lats, lons, err := wf.GridCoordinates()
	if err != nil {
		t.Fatal(err)
	}
	lonField := make([]float64, len(lats)) // a field that is linear across the seam
	for k := range lonField {
		lonField[k] = math.Cos(deg2rad(lons[k]))
	}
	// Between the last column (359 E) and the first (0 E). The grid length
	// is stored in whole millimetres, so 360 of them miss a full turn by
	// 0.15 m, a millionth of a cell.
	w, err := ws.Weights(10, 359.25)
	if err != nil {
		t.Fatal(err)
	}
	if w.Outside || w.Index[0]%cols != cols-1 || w.Index[1]%cols != 0 || math.Abs(w.XFrac-0.25) > 1e-5 {
		t.Errorf("in the seam: Weights = %+v, want columns %d and 0, a quarter of the way", w, cols-1)
	}
	v, err := w.Interpolate(lonField)
	if want := 0.75*math.Cos(deg2rad(359)) + 0.25; err != nil || math.Abs(v-want) > 1e-8 {
		t.Errorf("in the seam: interpolated %v (%v), want %v", v, err, want)
	}
	if w2, _ := ws.Weights(10, -0.75); w2.Index != w.Index || math.Abs(w2.XFrac-w.XFrac) > 1e-5 {
		t.Errorf("longitude -0.75: Weights = %+v, at 359.25 %+v", w2, w)
	}
	// Nearest, across the seam and everywhere else, is the full search's.
	rnd := rand.New(rand.NewSource(10))
	for i := range 3000 {
		lat, lon := rnd.Float64()*120-60, rnd.Float64()*360
		if i%3 == 0 {
			lon = 359 + rnd.Float64()*2 // around the seam
		}
		want := nearestAmong(lats, lons, lat, lon)
		if idx, _, _, err := nearestIndex(wf, lat, lon); err != nil || idx != want {
			t.Fatalf("nearest (%v, %v) = %d (%v), the full search finds %d", lat, lon, idx, err, want)
		}
	}

	// Twice round holds every place twice, so nearest uses the full search.
	twice := world
	twice.Ni = 2 * cols
	g2, err := mercatorGrid(twice)
	if err != nil {
		t.Fatal(err)
	}
	if g2.singleValued || g2.aroundTheWorld() {
		t.Errorf("720 columns: singleValued %v, aroundTheWorld %v", g2.singleValued, g2.aroundTheWorld())
	}
}

// TestPolarStereographic_Poles: the pole a polar stereographic grid is
// centred on is a point of the map; the opposite one is not.
func TestPolarStereographic_Poles(t *testing.T) {
	for _, c := range []struct {
		fixture       string
		pole, farPole float64
	}{{hrrrAlaskaWindFixture, 90, -90}, {"testdata/upstream/ps_south.grib2", -90, 90}} {
		f := firstField(t, c.fixture)
		s, err := NewSampler(f)
		if err != nil {
			t.Fatal(err)
		}
		lats, lons, err := f.GridCoordinates()
		if err != nil {
			t.Fatal(err)
		}
		if w, err := s.Weights(c.pole, 0); err != nil || !w.Outside {
			t.Errorf("%s: Weights at its pole = %+v (%v), want outside the grid", c.fixture, w, err)
		}
		if w, err := s.Weights(c.farPole, 0); err == nil {
			t.Errorf("%s: Weights at the far pole = %+v, want an error", c.fixture, w)
		}
		for _, lat := range []float64{c.pole, c.farPole} {
			want := nearestAmong(lats, lons, lat, 0)
			if idx, _, _, err := nearestIndex(f, lat, 0); err != nil || idx != want {
				t.Errorf("%s: nearest to latitude %v = %d (%v), the full search finds %d", c.fixture, lat, idx, err, want)
			}
		}
	}
}

// bilinearAt interpolates by hand at a fractional column and row.
func bilinearAt(vals []float64, nx int, col, row float64) float64 {
	i, j := int(math.Floor(col)), int(math.Floor(row))
	x, y := col-float64(i), row-float64(j)
	return vals[j*nx+i]*(1-x)*(1-y) + vals[j*nx+i+1]*x*(1-y) + vals[(j+1)*nx+i]*(1-x)*y + vals[(j+1)*nx+i+1]*x*y
}

// TestHRRR_Sampler interpolates the real HRRR cloud fields. The expected
// values are worked out by hand from the decoded values (which
// TestEccodesReference_Check compares with eccodes) and the positions PROJ
// gives, so nothing in them comes from the code under test.
func TestHRRR_Sampler(t *testing.T) {
	msgs := decodeAll(t, hrrrCloudFixture)
	var fields []*Field
	for _, m := range msgs {
		for i := range m.Fields {
			fields = append(fields, &m.Fields[i])
		}
	}
	if len(fields) != 6 {
		t.Fatalf("%d fields, want 6", len(fields))
	}
	const nx, ny = 1799, 1059
	s, err := NewSampler(fields[0])
	if err != nil {
		t.Fatal(err)
	}
	ref := loadProjRef(t).Fixtures[hrrrCloudFixture]
	if len(ref.Points) == 0 {
		t.Fatalf("%s has no points for %s", projReference, hrrrCloudFixture)
	}

	for k, f := range fields {
		tmpl, ok := f.Section3.Template.(Template330)
		if !ok || tmpl.Nx != nx || tmpl.Ny != ny {
			t.Fatalf("field %d: grid %T, want %dx%d Lambert", k, f.Section3.Template, nx, ny)
		}
		if !s.Matches(f) {
			t.Fatalf("field %d is not on the first field's grid", k)
		}
		vals, err := f.Values()
		if err != nil {
			t.Fatalf("field %d: %v", k, err)
		}
		nan := 0
		for _, v := range vals {
			if math.IsNaN(v) {
				nan++
			}
		}
		// Only the cloud ceiling has missing values: 1,530,248 of them
		// (eccodes' numberOfMissing), where there is no ceiling.
		if want := map[int]int{5: 1530248}[k]; nan != want {
			t.Fatalf("field %d: %d missing values, want %d", k, nan, want)
		}

		interpolated, refused := 0, 0
		for i, pt := range ref.Points {
			lat, lon, col, row := pt[0], pt[1], pt[2], pt[3]
			w, err := s.Weights(lat, lon)
			if err != nil {
				t.Fatal(err)
			}
			if w.Outside != (col < 0 || row < 0 || col > nx-1 || row > ny-1) {
				t.Fatalf("(%v, %v) at column %v, row %v: Outside = %v", lat, lon, col, row, w.Outside)
			}
			if w.Outside {
				continue
			}
			got, err := w.Interpolate(vals)
			if anyCornerNaN(w, vals) {
				if err == nil {
					t.Fatalf("field %d (%v, %v): interpolated %v across a missing value", k, lat, lon, got)
				}
				// A categorical lookup still answers.
				if c := w.NearestCorner(); c != int(math.Round(row))*nx+int(math.Round(col)) {
					t.Fatalf("field %d (%v, %v): NearestCorner = %d, column %v, row %v", k, lat, lon, c, col, row)
				}
				refused++
				continue
			}
			if err != nil {
				t.Fatalf("field %d (%v, %v): %v", k, lat, lon, err)
			}
			want := bilinearAt(vals, nx, col, row)
			if math.Abs(got-want) > 1e-6*math.Max(1, math.Abs(want)) {
				t.Fatalf("field %d (%v, %v): interpolated %v, by hand %v", k, lat, lon, got, want)
			}
			interpolated++
			if i%40 == 0 { // the method that unpacks the field itself
				bv, err := f.BilinearValue(lat, lon)
				if err != nil || math.Float64bits(bv) != math.Float64bits(got) {
					t.Fatalf("field %d (%v, %v): BilinearValue = %v (%v), Sampler %v", k, lat, lon, bv, err, got)
				}
			}
		}
		if interpolated < 20 || (k == 5 && refused < 20) || (k != 5 && refused != 0) {
			t.Errorf("field %d: %d points interpolated, %d refused for a missing corner", k, interpolated, refused)
		}
		t.Logf("field %d: %d points interpolated, %d refused for a missing corner", k, interpolated, refused)
	}

	// Places: Denver is on the grid, and its nearest grid point is within
	// one 3 km cell's half diagonal of it.
	total := fields[4]
	v, idx, nLat, nLon, err := total.NearestValue(39.7392, -104.9903)
	if err != nil {
		t.Fatal(err)
	}
	if km := haversineDeg(39.7392, -104.9903, nLat, nLon) * 6371.229; km > 3*math.Sqrt2/2 {
		t.Errorf("Denver: nearest grid point %d (%v, %v) is %.2f km away", idx, nLat, nLon, km)
	}
	if v < 0 || v > 100 {
		t.Errorf("Denver: total cloud cover %v%%", v)
	}
	if w, err := s.Weights(39.7392, -104.9903); err != nil || w.Outside || w.NearestCorner() != idx {
		t.Errorf("Denver: Weights = %+v (%v), nearest grid point %d", w, err, idx)
	}
	// Honolulu is west of the grid: Weights clamps it to the western column.
	if w, err := s.Weights(21.3069, -157.8583); err != nil || !w.Outside || w.Index[0]%nx != 0 || w.XFrac != 0 {
		t.Errorf("Honolulu: Weights = %+v (%v), want outside, on the western column", w, err)
	}
	// The north pole is where the cone's meridians meet, north of the grid.
	if w, err := s.Weights(90, 0); err != nil || !w.Outside || w.Index[0]/nx != 0 || w.YFrac != 0 {
		t.Errorf("north pole: Weights = %+v (%v), want outside, on the northern row", w, err)
	}
	// The south pole is the one place a northern cone cannot show.
	if w, err := s.Weights(-90, 0); err == nil {
		t.Errorf("south pole: Weights = %+v, want an error", w)
	}
	if _, err := total.BilinearValue(-90, 0); err == nil {
		t.Error("south pole: BilinearValue did not return an error")
	}
	if _, err := s.Weights(math.NaN(), 0); err == nil {
		t.Error("Weights accepted a NaN latitude")
	}
	if _, err := s.Weights(40, math.Inf(1)); err == nil {
		t.Error("Weights accepted an infinite longitude")
	}
	// NearestValue still answers there: some grid point is the closest.
	if _, _, nLat, _, err := total.NearestValue(-90, 0); err != nil || nLat > 25 {
		t.Errorf("south pole: NearestValue found latitude %v (%v), want a point of the southern row", nLat, err)
	}
}

func BenchmarkSamplerWeights_Lambert(b *testing.B) {
	msgs := decodeAllLenient(hrrrCloudFixture)
	if len(msgs) == 0 {
		b.Skip("no HRRR fixture")
	}
	s, err := NewSampler(&msgs[0].Fields[0])
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		s.Weights(39.7392, -104.9903)
	}
}

func BenchmarkNearestIndex_Lambert(b *testing.B) {
	msgs := decodeAllLenient(hrrrCloudFixture)
	if len(msgs) == 0 {
		b.Skip("no HRRR fixture")
	}
	f := &msgs[0].Fields[0]
	for b.Loop() {
		nearestIndex(f, 39.7392, -104.9903)
	}
}

// TestLookup_UnusableProjectedGrids: testdata/lambert.grib2 declares 496
// data points on a 349x277 grid, and testdata/polar_stereographic.grib2 has
// grid lengths of 0. Every lookup must refuse them instead of indexing
// values that are not there.
func TestLookup_UnusableProjectedGrids(t *testing.T) {
	for _, p := range []string{"testdata/lambert.grib2", "testdata/polar_stereographic.grib2"} {
		f := &decodeAll(t, p)[0].Fields[0]
		if _, err := NewSampler(f); err == nil {
			t.Errorf("%s: NewSampler accepted a grid definition that describes no usable grid", p)
		}
		if _, err := f.BilinearValue(40, -100); err == nil {
			t.Errorf("%s: BilinearValue accepted a grid definition that describes no usable grid", p)
		}
		if _, _, _, _, err := f.NearestValue(40, -100); err == nil {
			t.Errorf("%s: NearestValue accepted a grid definition that describes no usable grid", p)
		}
	}
}
