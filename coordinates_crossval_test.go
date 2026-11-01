package grib2

import (
	"bufio"
	"bytes"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// eccodesPoints runs grib_get_data with full precision on message msgIndex
// (1-based) and returns the points in data (scanning) order. Missing values
// are NaN.
func eccodesPoints(t *testing.T, file string, msgIndex int) (lats, lons, vals []float64) {
	t.Helper()
	cmd := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_get_data"),
		"-L", "%.7f %.7f", "-F", "%.12g", "-m", "nan", "-w", "count="+strconv.Itoa(msgIndex), file)
	cmd.Env = eccodesEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("grib_get_data %s: %v", file, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 {
			continue
		}
		la, _ := strconv.ParseFloat(f[0], 64)
		lo, _ := strconv.ParseFloat(f[1], 64)
		v, err := strconv.ParseFloat(f[2], 64)
		if err != nil {
			v = math.NaN()
		}
		lats, lons, vals = append(lats, la), append(lons, lo), append(vals, v)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return
}

// lonDiff returns the absolute difference of two longitudes modulo 360.
func lonDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	return math.Min(d, 360-d)
}

// crossCheckField compares GridCoordinates and Values of field against
// eccodes grib_get_data for the same message. eccodes lists points in data
// order (or +i +j order for projected grids); ours are canonical, so the
// eccodes arrays are put through the same permutation as the data. Points
// eccodes cannot place (off the disk of a space view) are skipped. Points
// are compared by great-circle distance, in degrees.
func crossCheckField(t *testing.T, file string, msgIndex int, f *Field, coordTol float64) {
	t.Helper()
	lats, lons, err := f.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}
	vals, err := f.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	scan, ni, nj := gridScanParams(f.Section3.Template)
	checkValues := true
	switch f.Section3.Template.(type) {
	case Template310, Template320, Template330:
		// eccodes' Mercator, polar stereographic and Lambert iterators
		// always build the grid from the first point towards +x +y and
		// reorder the data to match (transform_iterator_data in
		// grib_iterator_factory.cc). That is right for scanning mode 0x40;
		// for any other mode the first point is not the south-west corner,
		// so compare against eccodes on the equivalent 0x40 grid that starts
		// at our south-west corner (coordinates only: its data are not
		// reordered).
		if scan != 0x40 {
			swLat, swLon := lats[(nj-1)*ni], lons[(nj-1)*ni]
			file = equivalentScan64(t, file, swLat, swLon)
			checkValues = false
		}
		scan = 0x40
	}
	if scan&0x10 != 0 {
		// eccodes' iterators ignore alternative row scanning; its
		// swapScanningAlternativeRows (tests/grib_change_scanning.sh) is the
		// reference for what the rows mean.
		file = eccodesSet(t, file, "swapScanningAlternativeRows=1")
		scan &^= 0x10
	}
	eLats, eLons, eVals := eccodesPoints(t, file, msgIndex)
	if len(eLats) != len(lats) || len(vals) != len(lats) {
		t.Fatalf("point counts: ours %d coords / %d values, eccodes %d", len(lats), len(vals), len(eLats))
	}
	if scan != 0 && ni != missingUint32 {
		eLats = applyScanningMode(eLats, scan, ni, nj)
		eLons = applyScanningMode(eLons, scan, ni, nj)
		eVals = applyScanningMode(eVals, scan, ni, nj)
	}
	bad := 0
	for i := range lats {
		if eLats[i] == 0 && eLons[i] == 0 && lats[i] == 0 && lons[i] == 0 {
			continue
		}
		dist := haversineDeg(lats[i], lons[i], eLats[i], eLons[i]) * 180 / math.Pi
		valOK := !checkValues || (math.IsNaN(vals[i]) && math.IsNaN(eVals[i])) ||
			math.Abs(vals[i]-eVals[i]) <= 1e-6*math.Max(1, math.Abs(eVals[i]))
		if dist > coordTol || !valOK {
			if bad < 5 {
				t.Errorf("point %d: ours (%.6f, %.6f) = %v, eccodes (%.6f, %.6f) = %v",
					i, lats[i], lons[i], vals[i], eLats[i], eLons[i], eVals[i])
			}
			bad++
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d points differ", bad, len(lats))
	}
	for i, lo := range lons {
		if lo < 0 || lo >= 360 || math.IsNaN(lo) {
			t.Fatalf("lons[%d] = %v, want [0, 360)", i, lo)
		}
	}
}

// eccodesSet writes a copy of file with grib_set -s settings applied.
func eccodesSet(t *testing.T, file, settings string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "set.grib2")
	cmd := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_set"), "-s", settings, file, out)
	cmd.Env = eccodesEnv()
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("grib_set %s: %v: %s", settings, err, b)
	}
	return out
}

// equivalentScan64 writes a copy of file whose scanning mode is 0x40 and
// whose first grid point is (lat, lon).
func equivalentScan64(t *testing.T, file string, lat, lon float64) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "scan64.grib2")
	cmd := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_set"), "-s",
		"scanningMode=64,latitudeOfFirstGridPoint="+strconv.Itoa(int(math.Round(lat*1e6)))+
			",longitudeOfFirstGridPoint="+strconv.Itoa(int(math.Round(lon*1e6))), file, out)
	cmd.Env = eccodesEnv()
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("grib_set: %v: %s", err, b)
	}
	return out
}

// TestEccodes_GridCoordinatesAllGrids cross-checks coordinates and their
// pairing with values for every supported grid type, every scanning mode
// eccodes can produce, and the upstream grids with unusual geometry.
// Upstream: tests/grib_change_scanning.sh, grib_iterator.sh,
// grib_grid_{lambert_conformal,polar_stereographic,mercator,space_view}.sh,
// grib_ecc-1364.sh (southern Lambert), grib_nearest_test.sh.
func TestEccodes_GridCoordinatesAllGrids(t *testing.T) {
	if !eccodesAvailable(t) {
		t.Skip("eccodes not available")
	}
	files := []string{
		"testdata/sample.grib2",
		"testdata/regular_latlon_surface.grib2",
		"testdata/grid_complex_spatial_differencing.grib2", // crosses Greenwich, bitmap
		"testdata/gfs_multifield.grib2",                    // scan 64
		"testdata/rdps_surface.grib2",                      // rotated
		"testdata/regular_gaussian_surface.grib2",
		"testdata/regular_gaussian_pressure_level.grib2",
		"testdata/reduced_gaussian_surface.grib2",
		"testdata/reduced_gaussian_pressure_level.grib2",
		"testdata/mercator.grib2",
		"testdata/ccsds.grib2",
		"testdata/upstream/ll_scan16.grib2",
		"testdata/upstream/ll_scan32.grib2",
		"testdata/upstream/ll_scan128.grib2",
		"testdata/upstream/ll_swapx.grib2",
		"testdata/upstream/ll_swapy.grib2",
		"testdata/upstream/gg_swapx.grib2",
		"testdata/upstream/gg_swapy.grib2",
		"testdata/upstream/rotated_angle30.grib2",
		"testdata/upstream/mercator_ineg.grib2",
		"testdata/upstream/mercator_jpos.grib2",
		"testdata/upstream/mercator_shape5.grib2",
		"testdata/upstream/ps_sample.grib2",
		"testdata/upstream/ps_ineg.grib2",
		"testdata/upstream/ps_jpos.grib2",
		"testdata/upstream/ps_south.grib2",
		"testdata/upstream/lambert_small.grib2",
		"testdata/upstream/lambert_small_ineg.grib2",
		"testdata/upstream/lambert_small_jneg.grib2",
		"testdata/upstream/lambert_small_secant.grib2",
		"testdata/upstream/lambert_south.grib2",
		"testdata/upstream/lambert_shape2.grib2",
		"testdata/upstream/space_view_small.grib2",
		"testdata/upstream/space_view_small_ineg.grib2",
		"testdata/upstream/space_view_small_jpos.grib2",
		"testdata/upstream/tigge_egrr_sfc_2t.grib2",
		"testdata/upstream/reduced_gaussian_sub_area.grib2",
		"testdata/upstream/reduced_latlon_surface_constant.grib2",
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			msg := decodeAll(t, file)[0]
			crossCheckField(t, file, 1, &msg.Fields[0], 2e-3)
		})
	}
}

// TestGridCoordinates_InconsistentGrids: grids whose description does not
// hold together must give an error, not coordinates (eccodes
// tests/grib_iterator_fail.sh, grib_global.sh, grib_grid_lambert_conformal.sh
// "Wrong number of points", grib_change_scanning.sh).
func TestGridCoordinates_InconsistentGrids(t *testing.T) {
	for _, file := range []string{
		"testdata/upstream/ll_scan64.grib2",  // +j scanning but lat1 > lat2
		"testdata/upstream/ll_scan96.grib2",  // same, j consecutive
		"testdata/upstream/ll_scan192.grib2", // same, -i
		"testdata/upstream/ll_scan224.grib2",
		"testdata/upstream/rotated_scan64.grib2",
		"testdata/lambert.grib2",             // 349x277 grid, 496 points
		"testdata/space_view.grib2",          // 5424x5424 grid, 496 points
		"testdata/polar_stereographic.grib2", // Dx = Dy = 0
	} {
		t.Run(file, func(t *testing.T) {
			f := decodeAll(t, file)[0].Fields[0]
			if _, _, err := f.GridCoordinates(); err == nil {
				t.Error("GridCoordinates: expected an error")
			}
			if _, _, _, _, err := f.NearestValue(50, 10); err == nil {
				t.Error("NearestValue: expected an error")
			}
		})
	}
}
