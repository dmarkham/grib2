package grib2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
)

// TestApplyScanningMode_AllModes pins the data order of every scanning mode
// on a 3x2 grid whose canonical (north row first, west to east) values are
// 0..5:
//
//	0 1 2   <- northern row
//	3 4 5
//
// The 0x10 case is upstream's (tests/grib_change_scanning.sh: values 12..17
// with alternativeRowScanning give 12 13 14 / 17 16 15).
func TestApplyScanningMode_AllModes(t *testing.T) {
	canonical := []float64{0, 1, 2, 3, 4, 5}
	for _, tt := range []struct {
		mode uint8
		data []float64
	}{
		{0x00, []float64{0, 1, 2, 3, 4, 5}},
		{0x10, []float64{0, 1, 2, 5, 4, 3}}, // rows alternate
		{0x20, []float64{0, 3, 1, 4, 2, 5}}, // j consecutive
		{0x30, []float64{0, 3, 4, 1, 2, 5}}, // j consecutive, columns alternate
		{0x40, []float64{3, 4, 5, 0, 1, 2}}, // +j
		{0x50, []float64{3, 4, 5, 2, 1, 0}},
		{0x60, []float64{3, 0, 4, 1, 5, 2}},
		{0x80, []float64{2, 1, 0, 5, 4, 3}}, // -i
		{0x90, []float64{2, 1, 0, 3, 4, 5}},
		{0xA0, []float64{2, 5, 1, 4, 0, 3}},
		{0xC0, []float64{5, 4, 3, 2, 1, 0}},
		{0xE0, []float64{5, 2, 4, 1, 3, 0}},
	} {
		got := applyScanningMode(tt.data, tt.mode, 3, 2)
		for i := range canonical {
			if got[i] != canonical[i] {
				t.Errorf("mode 0x%02x: canonical = %v, want %v", tt.mode, got, canonical)
				break
			}
		}
	}

	// Upstream alternativeRowScanning example, end to end through Values().
	f := decodeAll(t, "testdata/sample.grib2")[0].Fields[0]
	tmpl := f.Section3.Template.(Template30)
	tmpl.Ni, tmpl.Nj, tmpl.ScanningMode = 3, 2, 0x10
	f.Section3.Template = tmpl
	f.Section3.NumberOfDataPoints = 6
	f.Section5.NumberOfValues = 6
	f.Section5.Template = Template50{ReferenceValue: 12, BitsPerValue: 8}
	f.Section7.Data = []byte{0, 1, 2, 3, 4, 5}
	vals, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	if want := []float64{12, 13, 14, 17, 16, 15}; fmt.Sprint(vals) != fmt.Sprint(want) {
		t.Errorf("alternative rows: Values = %v, want %v", vals, want)
	}
}

// regularFixtureFields returns the first field of every fixture on a grid
// the direct lookups support (3.0, 3.1 and regular 3.40).
func regularFixtureFields(t *testing.T) map[string]*Field {
	t.Helper()
	out := map[string]*Field{}
	for _, p := range allFixturePaths(t) {
		for m, msg := range decodeAllLenient(p) {
			for i := range msg.Fields {
				f := &msg.Fields[i]
				if _, err := NewSampler(f); err != nil {
					continue
				}
				if _, err := f.Values(); err != nil {
					continue
				}
				out[fmt.Sprintf("%s#%d.%d", p, m, i)] = f
				break
			}
			if m >= 2 {
				break
			}
		}
	}
	return out
}

// decodeAllLenient decodes the messages of a file up to the first error.
func decodeAllLenient(path string) []*Message {
	var msgs []*Message
	raw, err := readFileBytes(path)
	if err != nil {
		return nil
	}
	dec := NewDecoder(bytesReader(raw))
	for {
		msg, err := dec.Decode()
		if err != nil {
			return msgs
		}
		msgs = append(msgs, msg)
	}
}

// TestLookup_NodeIdentity: querying exactly at a grid node must return that
// node, from NearestValue, BilinearValue and Sampler alike, on every regular
// grid fixture and every scanning mode (eccodes tests/grib_ecc-1258.sh:
// exact grid point gives distance 0; grib_change_scanning.sh).
func TestLookup_NodeIdentity(t *testing.T) {
	fields := regularFixtureFields(t)
	if len(fields) < 15 {
		t.Fatalf("only %d regular fixtures", len(fields))
	}
	for name, f := range fields {
		t.Run(name, func(t *testing.T) {
			lats, lons, err := f.GridCoordinates()
			if err != nil {
				t.Fatal(err)
			}
			vals, err := f.Values()
			if err != nil {
				t.Fatal(err)
			}
			s, err := NewSampler(f)
			if err != nil {
				t.Fatal(err)
			}
			step := max(1, len(lats)/400)
			for i := 0; i < len(lats); i += step {
				if math.Abs(lats[i]) > 90-1e-9 {
					continue // every point of a pole row is the same place
				}
				// The public methods unpack the field on every call, so use
				// them for a few nodes and the shared lookups for the rest.
				public := i%(50*step) == 0
				var idx int
				var nLat, nLon float64
				if public {
					v, pidx, la, lo, err := f.NearestValue(lats[i], lons[i])
					if err != nil {
						t.Fatalf("NearestValue(%v, %v): %v", lats[i], lons[i], err)
					}
					if !(v == vals[i] || math.IsNaN(v) && math.IsNaN(vals[i])) {
						t.Fatalf("node %d: value %v, want %v", i, v, vals[i])
					}
					idx, nLat, nLon = pidx, la, lo
				} else {
					idx, nLat, nLon, err = nearestIndex(f, lats[i], lons[i])
					if err != nil {
						t.Fatal(err)
					}
				}
				if idx != i {
					t.Fatalf("nearest at node %d (%v, %v) returned %d (%v, %v)", i, lats[i], lons[i], idx, nLat, nLon)
				}
				w, err := s.Weights(lats[i], lons[i])
				if err != nil {
					t.Fatal(err)
				}
				if c := w.NearestCorner(); c != i {
					t.Fatalf("node %d: NearestCorner = %d", i, c)
				}
				bv, err := w.Interpolate(vals)
				if public {
					bv, err = f.BilinearValue(lats[i], lons[i])
				}
				// Rotated grids go through a rotation and back, so allow rounding.
				if err == nil && math.Abs(bv-vals[i]) > 1e-6*math.Max(1, math.Abs(vals[i])) {
					t.Fatalf("node %d: bilinear = %v, want %v", i, bv, vals[i])
				}
				if err != nil && !math.IsNaN(vals[i]) && !anyCornerNaN(w, vals) {
					t.Fatalf("node %d: bilinear: %v", i, err)
				}
			}
		})
	}
}

func anyCornerNaN(w BilinearWeights, vals []float64) bool {
	for _, k := range w.Index {
		if math.IsNaN(gridVal(vals, k)) {
			return true
		}
	}
	return false
}

// TestBilinear_SeamWrap: on a grid that goes all the way round, points
// between the last column and 360 interpolate across the seam (eccodes
// geo/nearest/Regular.cc).
func TestBilinear_SeamWrap(t *testing.T) {
	for _, file := range []string{"testdata/ccsds.grib2", "testdata/regular_gaussian_surface.grib2"} {
		t.Run(file, func(t *testing.T) {
			f := decodeAll(t, file)[0].Fields[0]
			g, err := regularGridOf(f.Section3.Template)
			if err != nil {
				t.Fatal(err)
			}
			if !g.globalLon {
				t.Fatal("fixture is not longitude-global")
			}
			vals, err := f.Values()
			if err != nil {
				t.Fatal(err)
			}
			r := g.nj / 3
			lat := g.rowLats[r]
			lon := g.lon(g.ni-1) + g.dlon/2
			want := 0.5 * (vals[r*g.ni+g.ni-1] + vals[r*g.ni])
			got, err := f.BilinearValue(lat, lon)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-want) > 1e-9*math.Abs(want) {
				t.Errorf("BilinearValue(%v, %v) = %v, want %v", lat, lon, got, want)
			}
			s, _ := NewSampler(&f)
			w, _ := s.Weights(lat, lon)
			if w.Outside {
				t.Error("seam point reported Outside")
			}
			// Same point expressed as a negative longitude.
			if got2, _ := f.BilinearValue(lat, lon-360); got2 != got {
				t.Errorf("lon-360 gives %v, want %v", got2, got)
			}
		})
	}
}

// TestNearestValue_LongitudeWraps: a query longitude any number of turns
// away selects the same point (eccodes geo/nearest/Regular.cc,
// tests/grib_nearest_test.sh queries at lon 399.6 and 366.3).
func TestNearestValue_LongitudeWraps(t *testing.T) {
	for _, file := range []string{"testdata/sample.grib2", "testdata/regular_gaussian_surface.grib2", "testdata/rdps_surface.grib2"} {
		f := decodeAll(t, file)[0].Fields[0]
		_, want, _, _, err := f.NearestValue(30, 10)
		if err != nil {
			t.Fatal(err)
		}
		for k := -3; k <= 3; k++ {
			_, idx, _, _, err := f.NearestValue(30, 10+360*float64(k))
			if err != nil {
				t.Fatal(err)
			}
			if idx != want {
				t.Errorf("%s: NearestValue(30, %v) = %d, want %d", file, 10+360*float64(k), idx, want)
			}
		}
	}
}

// TestEarthAxes pins Code Table 3.2 against the eccodes definitions
// (tests/grib_ecc-979.sh, grib_nearest_test.sh, grib_grid_lambert_conformal.sh).
func TestEarthAxes(t *testing.T) {
	for _, tt := range []struct {
		shape        uint8
		sfR          uint8
		svR          uint32
		sfA, sfB     uint8
		svA, svB     uint32
		major, minor float64
		wantErr      bool
	}{
		{shape: 0, major: 6367470, minor: 6367470},
		{shape: 1, svR: 6371229, major: 6371229, minor: 6371229},
		{shape: 1, sfR: 4, svR: 6781234, major: 678.1234, minor: 678.1234},
		{shape: 1, sfR: 0xFF, svR: missingUint32, wantErr: true},
		{shape: 2, major: 6378160, minor: 6356775},
		{shape: 3, sfA: 0, svA: 6378, sfB: 1, svB: 63568, major: 6378000, minor: 6356800},
		{shape: 4, major: 6378137, minor: 6356752.314},
		{shape: 5, major: 6378137, minor: 6356752.314},
		{shape: 6, major: 6371229, minor: 6371229},
		{shape: 7, sfA: 0, svA: 6378137, sfB: 2, svB: 635675231, major: 6378137, minor: 6356752.31},
		{shape: 7, sfA: 0xFF, svA: missingUint32, wantErr: true},
		{shape: 8, major: 6371200, minor: 6371200},
		{shape: 9, major: 6377563.396, minor: 6356256.909},
		{shape: 10, wantErr: true},
	} {
		a, b, err := earthAxes(tt.shape, tt.sfR, tt.svR, tt.sfA, tt.svA, tt.sfB, tt.svB)
		if tt.wantErr {
			if err == nil {
				t.Errorf("shape %d: expected error", tt.shape)
			}
			continue
		}
		if err != nil || math.Abs(a-tt.major) > 1e-6 || math.Abs(b-tt.minor) > 1e-6 {
			t.Errorf("shape %d: got %v, %v, %v; want %v, %v", tt.shape, a, b, err, tt.major, tt.minor)
		}
	}
}

// TestGridGeometry_Malformed: malformed grid descriptions give errors from
// GridCoordinates and the lookups, never garbage or a panic (eccodes
// tests/grib_iterator_fail.sh, grib_grid_mercator.sh, grib_grid_lambert_conformal.sh,
// grib_check_gaussian_grids.sh).
func TestGridGeometry_Malformed(t *testing.T) {
	base := func(file string) Field { return decodeAll(t, file)[0].Fields[0] }
	cases := map[string]func() Field{
		"3.0 Ni*Nj != N": func() Field {
			f := base("testdata/sample.grib2")
			tmpl := f.Section3.Template.(Template30)
			tmpl.Ni = 33
			f.Section3.Template = tmpl
			return f
		},
		"3.0 Ni missing without PL": func() Field {
			f := base("testdata/sample.grib2")
			tmpl := f.Section3.Template.(Template30)
			tmpl.Ni = missingUint32
			f.Section3.Template = tmpl
			return f
		},
		"3.10 first latitude 90": func() Field {
			f := base("testdata/mercator.grib2")
			tmpl := f.Section3.Template.(Template310)
			tmpl.LatitudeOfFirstGridPoint = 90000000
			f.Section3.Template = tmpl
			return f
		},
		"3.30 Latin1 = -Latin2": func() Field {
			f := base("testdata/upstream/lambert_small.grib2")
			tmpl := f.Section3.Template.(Template330)
			tmpl.Latin2 = -tmpl.Latin1
			f.Section3.Template = tmpl
			return f
		},
		"3.30 shape 1 radius missing": func() Field {
			f := base("testdata/upstream/lambert_small.grib2")
			tmpl := f.Section3.Template.(Template330)
			tmpl.ShapeOfEarth, tmpl.ScaleFactorOfRadiusOfSphericalEarth, tmpl.ScaledValueOfRadiusOfSphericalEarth = 1, 0xFF, missingUint32
			f.Section3.Template = tmpl
			return f
		},
		"3.20 oblate Earth": func() Field {
			f := base("testdata/upstream/ps_sample.grib2")
			tmpl := f.Section3.Template.(Template320)
			tmpl.ShapeOfEarth = 5
			f.Section3.Template = tmpl
			return f
		},
		"3.40 PL entry 0": func() Field {
			f := base("testdata/reduced_gaussian_surface.grib2")
			f.Section3.OptionalPointNumbers = append([]byte(nil), f.Section3.OptionalPointNumbers...)
			// Move row 0's points to row 1 so sum(pl) still equals N.
			pl := f.Section3.OptionalPointNumbers
			p0 := binary.BigEndian.Uint16(pl[0:2])
			binary.BigEndian.PutUint16(pl[2:4], binary.BigEndian.Uint16(pl[2:4])+p0)
			pl[0], pl[1] = 0, 0
			return f
		},
		"3.40 sum(PL) != N": func() Field {
			f := base("testdata/reduced_gaussian_surface.grib2")
			f.Section3.NumberOfDataPoints += 4
			return f
		},
		"3.40 latitude not Gaussian": func() Field {
			f := base("testdata/regular_gaussian_surface.grib2")
			tmpl := f.Section3.Template.(Template340)
			tmpl.LatitudeOfFirstGridPoint += 500000
			f.Section3.Template = tmpl
			return f
		},
		"3.90 Nr missing": func() Field {
			f := base("testdata/upstream/space_view_small.grib2")
			tmpl := f.Section3.Template.(Template390)
			tmpl.Nr = missingUint32
			f.Section3.Template = tmpl
			return f
		},
		"3.90 Dx 0": func() Field {
			f := base("testdata/upstream/space_view_small.grib2")
			tmpl := f.Section3.Template.(Template390)
			tmpl.Dx = 0
			f.Section3.Template = tmpl
			return f
		},
		"3.90 sub-satellite latitude": func() Field {
			f := base("testdata/upstream/space_view_small.grib2")
			tmpl := f.Section3.Template.(Template390)
			tmpl.LatitudeOfSubSatellitePoint = 1000000
			f.Section3.Template = tmpl
			return f
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			f := mk()
			if _, _, err := f.GridCoordinates(); err == nil {
				t.Error("GridCoordinates: expected an error")
			}
			if _, _, _, _, err := f.NearestValue(40, 10); err == nil {
				t.Error("NearestValue: expected an error")
			}
		})
	}
}

// TestReducedGrids_NoBilinear: reduced grids are refused by the bilinear
// paths but answer NearestValue.
func TestReducedGrids_NoBilinear(t *testing.T) {
	for _, file := range []string{
		"testdata/upstream/reduced_latlon_surface_constant.grib2",
		"testdata/reduced_gaussian_surface.grib2",
	} {
		f := decodeAll(t, file)[0].Fields[0]
		if _, err := NewSampler(&f); err == nil {
			t.Errorf("%s: NewSampler: expected error", file)
		}
		if _, err := f.BilinearValue(10, 10); err == nil {
			t.Errorf("%s: BilinearValue: expected error", file)
		}
		if _, _, _, _, err := f.NearestValue(10, 10); err != nil {
			t.Errorf("%s: NearestValue: %v", file, err)
		}
		if s3 := f.Section3; strings.Contains(file, "latlon") && len(s3.OptionalPointNumbers) != 2*int(s3.Template.(Template30).Nj) {
			t.Errorf("%s: %d PL octets, want 2*Nj", file, len(s3.OptionalPointNumbers))
		}
	}
}

func readFileBytes(path string) ([]byte, error) { return os.ReadFile(path) }

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
