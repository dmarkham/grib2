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
		"-L", "%.7f %.7f", "-F", "%.17g", "-m", "nan", "-w", "count="+strconv.Itoa(msgIndex), file)
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
