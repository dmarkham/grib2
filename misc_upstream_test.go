package grib2

import (
	"math"
	"path/filepath"
	"testing"
)

// TestRunLength_ThroughValues: run-length packed data decodes through
// Field.Values with its missing points as NaN (eccodes
// tests/grib_run_length_packing.sh; eccodes numberOfMissing is 71493).
func TestRunLength_ThroughValues(t *testing.T) {
	f := decodeAll(t, "testdata/run_length_packing.grib2")[0].Fields[0]
	vals, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	nan := 0
	for _, v := range vals {
		if math.IsNaN(v) {
			nan++
		}
	}
	if len(vals) != 86016 || nan != 71493 {
		t.Errorf("%d values, %d NaN; want 86016 and 71493", len(vals), nan)
	}
	// A decimal scale factor scales the level values (sign-magnitude octet).
	tm := f.Section5.Template.(Template5200)
	for _, tt := range []struct {
		dsf   uint8
		scale float64
	}{{1, 0.1}, {129, 10}} {
		tm2 := tm
		tm2.DecimalScaleFactor = tt.dsf
		got, err := UnpackRunLength(f.Section7.Data, tm2, f.Section5.NumberOfValues, math.NaN())
		if err != nil {
			t.Fatal(err)
		}
		base, _ := UnpackRunLength(f.Section7.Data, tm, f.Section5.NumberOfValues, math.NaN())
		for i := range got {
			if math.IsNaN(base[i]) != math.IsNaN(got[i]) || (!math.IsNaN(got[i]) && math.Abs(got[i]-base[i]*tt.scale) > 1e-9*math.Abs(base[i])) {
				t.Fatalf("dsf %d: point %d: %v vs %v", tt.dsf, i, got[i], base[i])
			}
		}
	}
}

// TestGaussianLatitudes_LargeN pins the first Gaussian latitude of the
// operational N640/N1280/N2000 grids and their symmetry, and checks that
// every eccodes reduced Gaussian sample decodes to sum(pl) points
// (eccodes tests/grib_check_gaussian_grids.sh, grib_ecc-756.sh).
func TestGaussianLatitudes_LargeN(t *testing.T) {
	for n, want := range map[int]float64{640: 89.892396, 1280: 89.946187, 2000: 89.965557} {
		lats, err := coordGaussianLatitudes(n)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(lats[0]-want) > 1e-5 {
			t.Errorf("N=%d: first latitude %.6f, want %.6f", n, lats[0], want)
		}
		for i := 0; i < n; i++ {
			if lats[i] != -lats[2*n-1-i] || (i > 0 && lats[i] >= lats[i-1]) {
				t.Fatalf("N=%d: latitudes not symmetric and descending at %d", n, i)
			}
		}
	}
	samples, _ := filepath.Glob("reference/eccodes/samples/reduced_gg_pl_*_grib2.tmpl")
	if len(samples) == 0 {
		t.Skip("eccodes samples not available")
	}
	for _, s := range samples {
		f := decodeAll(t, s)[0].Fields[0]
		tm := f.Section3.Template.(Template340)
		pl := coordDecodePL(f.Section3.OptionalPointNumbers, f.Section3.OctetsForNumberOfPoints, int(tm.Nj))
		sum := 0
		for _, p := range pl {
			sum += p
		}
		lats, _, err := f.GridCoordinates()
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if sum != int(f.Section3.NumberOfDataPoints) || len(lats) != sum {
			t.Errorf("%s: sum(pl)=%d, N=%d, %d coordinates", s, sum, f.Section3.NumberOfDataPoints, len(lats))
		}
	}
}

// TestLookup_MissingNeighbours: NearestValue at a missing point returns NaN
// without error, and BilinearValue next to a missing point reports it
// (eccodes grib_nearest returns the missing value).
func TestLookup_MissingNeighbours(t *testing.T) {
	f := decodeAll(t, "testdata/grid_complex_spatial_differencing.grib2")[0].Fields[0]
	vals, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	lats, lons, _ := f.GridCoordinates()
	idx := -1
	for i, v := range vals {
		if math.IsNaN(v) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no missing point")
	}
	v, got, _, _, err := f.NearestValue(lats[idx], lons[idx])
	if err != nil || got != idx || !math.IsNaN(v) {
		t.Errorf("NearestValue at missing node: %v %d %v", v, got, err)
	}
	if _, err := f.BilinearValue(lats[idx], lons[idx]); err == nil {
		t.Error("BilinearValue next to a missing point: no error")
	}
}

// BenchmarkValues decodes one field per packing (eccodes tests/grib_ecc-386.cc).
func BenchmarkValues(b *testing.B) {
	for _, file := range []string{
		"testdata/gfs.complex.mvmu.grib2", "testdata/ccsds.grib2", "testdata/jpeg.grib2",
		"testdata/upstream/png_bpv16.grib2", "testdata/regular_gaussian_surface.grib2",
	} {
		msgs := decodeAllLenient(file)
		if len(msgs) == 0 {
			b.Fatalf("%s: no messages", file)
		}
		f := &msgs[0].Fields[0]
		b.Run(filepath.Base(file), func(b *testing.B) {
			for b.Loop() {
				if _, err := f.Values(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
