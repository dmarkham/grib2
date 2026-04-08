package grib2

import (
	"bufio"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Template 3.0 -- Regular Lat/Lon
// ---------------------------------------------------------------------------

func TestGridCoordLatLon(t *testing.T) {
	f := openTestField(t, "testdata/regular_latlon_surface.grib2")

	lats, lons, err := f.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	n := int(f.Section3.NumberOfDataPoints)
	if len(lats) != n || len(lons) != n {
		t.Fatalf("len(lats)=%d, len(lons)=%d, want %d", len(lats), len(lons), n)
	}

	// First point should match the template values
	tmpl := f.Section3.Template.(Template30)
	wantLat1 := float64(tmpl.LatitudeOfFirstGridPoint) * 1e-6
	wantLon1 := float64(tmpl.LongitudeOfFirstGridPoint) * 1e-6

	if math.Abs(lats[0]-wantLat1) > 0.001 {
		t.Errorf("lats[0] = %f, want %f", lats[0], wantLat1)
	}
	if math.Abs(lons[0]-wantLon1) > 0.001 {
		t.Errorf("lons[0] = %f, want %f", lons[0], wantLon1)
	}

	// Last point should match last grid point
	wantLat2 := float64(tmpl.LatitudeOfLastGridPoint) * 1e-6
	wantLon2 := float64(tmpl.LongitudeOfLastGridPoint) * 1e-6

	ni := int(tmpl.Ni)
	nj := int(tmpl.Nj)
	lastIdx := nj*ni - 1
	if lastIdx >= n {
		lastIdx = n - 1
	}

	if math.Abs(lats[lastIdx]-wantLat2) > 0.001 {
		t.Errorf("lats[last] = %f, want %f", lats[lastIdx], wantLat2)
	}
	if math.Abs(lons[lastIdx]-wantLon2) > 0.001 {
		t.Errorf("lons[last] = %f, want %f", lons[lastIdx], wantLon2)
	}

	// Validate against eccodes grib_get_data
	t.Run("eccodes_cross_validate", func(t *testing.T) {
		eccLats, eccLons := eccodesGetData(t, "testdata/regular_latlon_surface.grib2")
		if eccLats == nil {
			return // skip if eccodes not available
		}
		crossValidate(t, lats, lons, eccLats, eccLons, 0.01)
	})
}

// ---------------------------------------------------------------------------
// Template 3.30 -- Lambert Conformal
// ---------------------------------------------------------------------------

func TestGridCoordLambert(t *testing.T) {
	f := openTestField(t, "testdata/lambert.grib2")

	lats, lons, err := f.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	n := int(f.Section3.NumberOfDataPoints)
	if len(lats) != n || len(lons) != n {
		t.Fatalf("len(lats)=%d, len(lons)=%d, want %d", len(lats), len(lons), n)
	}

	// The first point should match latitudeOfFirstGridPoint / longitudeOfFirstGridPoint
	tmpl := f.Section3.Template.(Template330)
	wantLat1 := float64(tmpl.LatitudeOfFirstGridPoint) * 1e-6
	wantLon1 := float64(tmpl.LongitudeOfFirstGridPoint) * 1e-6

	// Lambert first point should reconstruct to the same lat/lon
	if math.Abs(lats[0]-wantLat1) > 0.01 {
		t.Errorf("lats[0] = %f, want ~%f", lats[0], wantLat1)
	}

	// Normalise expected longitude to [0,360)
	for wantLon1 < 0 {
		wantLon1 += 360
	}
	lon0 := lons[0]
	for lon0 < 0 {
		lon0 += 360
	}
	if math.Abs(lon0-wantLon1) > 0.01 {
		t.Errorf("lons[0] = %f, want ~%f", lons[0], wantLon1)
	}

	t.Logf("Lambert first point: lat=%f lon=%f (expected lat=%f lon=%f)", lats[0], lons[0], wantLat1, wantLon1)
	if n > 1 {
		t.Logf("Lambert second point: lat=%f lon=%f", lats[1], lons[1])
	}
}

// ---------------------------------------------------------------------------
// Template 3.10 -- Mercator
// ---------------------------------------------------------------------------

func TestGridCoordMercator(t *testing.T) {
	f := openTestField(t, "testdata/mercator.grib2")

	lats, lons, err := f.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	n := int(f.Section3.NumberOfDataPoints)
	if len(lats) != n || len(lons) != n {
		t.Fatalf("len(lats)=%d, len(lons)=%d, want %d", len(lats), len(lons), n)
	}

	// First point validation against eccodes output
	tmpl := f.Section3.Template.(Template310)
	wantLat1 := float64(tmpl.LatitudeOfFirstGridPoint) * 1e-6
	wantLon1 := float64(tmpl.LongitudeOfFirstGridPoint) * 1e-6

	if math.Abs(lats[0]-wantLat1) > 0.01 {
		t.Errorf("lats[0] = %f, want ~%f", lats[0], wantLat1)
	}
	if math.Abs(lons[0]-wantLon1) > 0.01 {
		t.Errorf("lons[0] = %f, want ~%f", lons[0], wantLon1)
	}

	t.Logf("Mercator first: lat=%f lon=%f", lats[0], lons[0])
	t.Logf("Mercator last:  lat=%f lon=%f", lats[n-1], lons[n-1])

	// Cross-validate with eccodes
	t.Run("eccodes_cross_validate", func(t *testing.T) {
		eccLats, eccLons := eccodesGetData(t, "testdata/mercator.grib2")
		if eccLats == nil {
			return
		}
		crossValidate(t, lats, lons, eccLats, eccLons, 0.01)
	})
}

// ---------------------------------------------------------------------------
// Template 3.20 -- Polar Stereographic
// ---------------------------------------------------------------------------

func TestGridCoordPolarStereographic(t *testing.T) {
	f := openTestField(t, "testdata/polar_stereographic.grib2")

	lats, lons, err := f.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	n := int(f.Section3.NumberOfDataPoints)
	if len(lats) != n || len(lons) != n {
		t.Fatalf("len(lats)=%d, len(lons)=%d, want %d", len(lats), len(lons), n)
	}

	// First point: lat=60, lon=0 (from the template)
	tmpl := f.Section3.Template.(Template320)
	wantLat1 := float64(tmpl.LatitudeOfFirstGridPoint) * 1e-6
	wantLon1 := float64(tmpl.LongitudeOfFirstGridPoint) * 1e-6

	if math.Abs(lats[0]-wantLat1) > 0.01 {
		t.Errorf("lats[0] = %f, want ~%f", lats[0], wantLat1)
	}
	if math.Abs(lons[0]-wantLon1) > 0.01 && math.Abs(lons[0]-360-wantLon1) > 0.01 {
		t.Errorf("lons[0] = %f, want ~%f", lons[0], wantLon1)
	}

	t.Logf("Polar Stereo first: lat=%f lon=%f", lats[0], lons[0])
}

// ---------------------------------------------------------------------------
// Template 3.40 -- Gaussian (regular)
// ---------------------------------------------------------------------------

func TestGridCoordGaussianRegular(t *testing.T) {
	f := openTestField(t, "testdata/regular_gaussian_surface.grib2")

	lats, lons, err := f.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	n := int(f.Section3.NumberOfDataPoints)
	if len(lats) != n || len(lons) != n {
		t.Fatalf("len(lats)=%d, len(lons)=%d, want %d", len(lats), len(lons), n)
	}

	// First Gaussian latitude should be ~87.864 for N=32
	// (eccodes gives 87.864 for the first data point)
	if math.Abs(lats[0]-87.864) > 0.01 {
		t.Errorf("lats[0] = %f, want ~87.864", lats[0])
	}
	if math.Abs(lons[0]-0.0) > 0.01 {
		t.Errorf("lons[0] = %f, want ~0.0", lons[0])
	}

	t.Logf("Gaussian regular first: lat=%f lon=%f", lats[0], lons[0])
	t.Logf("Gaussian regular last:  lat=%f lon=%f", lats[n-1], lons[n-1])

	// Cross-validate with eccodes
	t.Run("eccodes_cross_validate", func(t *testing.T) {
		eccLats, eccLons := eccodesGetData(t, "testdata/regular_gaussian_surface.grib2")
		if eccLats == nil {
			return
		}
		crossValidate(t, lats, lons, eccLats, eccLons, 0.01)
	})
}

// ---------------------------------------------------------------------------
// Template 3.40 -- Gaussian (reduced)
// ---------------------------------------------------------------------------

func TestGridCoordGaussianReduced(t *testing.T) {
	f := openTestField(t, "testdata/reduced_gaussian_surface.grib2")

	lats, lons, err := f.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	n := int(f.Section3.NumberOfDataPoints)
	if len(lats) != n || len(lons) != n {
		t.Fatalf("len(lats)=%d, len(lons)=%d, want %d", len(lats), len(lons), n)
	}

	// First point should have lat ~87.864 and lon ~0
	if math.Abs(lats[0]-87.864) > 0.01 {
		t.Errorf("lats[0] = %f, want ~87.864", lats[0])
	}
	if math.Abs(lons[0]-0.0) > 0.01 {
		t.Errorf("lons[0] = %f, want ~0.0", lons[0])
	}

	t.Logf("Gaussian reduced first: lat=%f lon=%f", lats[0], lons[0])
	t.Logf("Gaussian reduced last:  lat=%f lon=%f", lats[n-1], lons[n-1])

	// Cross-validate with eccodes
	t.Run("eccodes_cross_validate", func(t *testing.T) {
		eccLats, eccLons := eccodesGetData(t, "testdata/reduced_gaussian_surface.grib2")
		if eccLats == nil {
			return
		}
		crossValidate(t, lats, lons, eccLats, eccLons, 0.01)
	})
}

// ---------------------------------------------------------------------------
// Template 3.90 -- Space View
// ---------------------------------------------------------------------------

func TestGridCoordSpaceView(t *testing.T) {
	f := openTestField(t, "testdata/space_view.grib2")

	// This test file has NumberOfDataPoints=496 but Nx*Ny=5424*5424,
	// meaning it is a truncated test file. We should still get coordinates
	// for the declared number of data points without error.
	lats, lons, err := f.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	n := int(f.Section3.NumberOfDataPoints)
	if len(lats) != n || len(lons) != n {
		t.Fatalf("len(lats)=%d, len(lons)=%d, want %d", len(lats), len(lons), n)
	}

	t.Logf("Space View first: lat=%f lon=%f", lats[0], lons[0])
	t.Logf("Space View middle: lat=%f lon=%f", lats[n/2], lons[n/2])
}

// ---------------------------------------------------------------------------
// Gaussian latitude computation validation
// ---------------------------------------------------------------------------

func TestGridCoordGaussianLatitudes(t *testing.T) {
	// Well-known Gaussian latitudes for small N values
	// N=32: first lat should be ~87.8638 degrees
	lats, err := coordGaussianLatitudes(32)
	if err != nil {
		t.Fatalf("gaussianLatitudes(32): %v", err)
	}
	if len(lats) != 64 {
		t.Fatalf("len(lats)=%d, want 64", len(lats))
	}
	// The first latitude (closest to north pole) for N=32
	if math.Abs(lats[0]-87.8638) > 0.001 {
		t.Errorf("gaussianLatitudes(32)[0] = %f, want ~87.8638", lats[0])
	}
	// Symmetry: last should be -first
	if math.Abs(lats[63]+lats[0]) > 1e-10 {
		t.Errorf("gaussianLatitudes(32): not symmetric: first=%f, last=%f", lats[0], lats[63])
	}
	// Should be descending
	for i := 1; i < len(lats); i++ {
		if lats[i] > lats[i-1] {
			t.Errorf("gaussianLatitudes(32) not descending at index %d: %f > %f", i, lats[i], lats[i-1])
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func openTestField(t *testing.T, path string) Field {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	msg, err := ReadMessage(file)
	if err != nil {
		t.Fatalf("ReadMessage %s: %v", path, err)
	}
	if len(msg.Fields) == 0 {
		t.Fatalf("no fields in %s", path)
	}
	return msg.Fields[0]
}

// eccodesGetData runs grib_get_data and parses the lat/lon columns.
// Returns nil slices if eccodes is not available.
func eccodesGetData(t *testing.T, path string) ([]float64, []float64) {
	t.Helper()
	gribGetData := "/home/dmarkham/git/dmarkham/grib2/reference/eccodes-install/bin/grib_get_data"
	if _, err := os.Stat(gribGetData); os.IsNotExist(err) {
		t.Skip("eccodes not available")
		return nil, nil
	}

	cmd := exec.Command(gribGetData, path)
	cmd.Env = append(os.Environ(),
		"ECCODES_DEFINITION_PATH=/home/dmarkham/git/dmarkham/grib2/reference/eccodes-install/share/eccodes/definitions",
		"LD_LIBRARY_PATH=/home/dmarkham/git/dmarkham/grib2/reference/eccodes-install/lib",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Logf("grib_get_data failed for %s (may be truncated test file): %v", path, err)
		return nil, nil
	}

	var lats, lons []float64
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if first {
			first = false
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		lat, err1 := strconv.ParseFloat(fields[0], 64)
		lon, err2 := strconv.ParseFloat(fields[1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		lats = append(lats, lat)
		lons = append(lons, lon)
	}
	return lats, lons
}

// crossValidate compares our coordinates against eccodes coordinates.
func crossValidate(t *testing.T, lats, lons, eccLats, eccLons []float64, tolerance float64) {
	t.Helper()
	n := len(lats)
	ne := len(eccLats)
	if n != ne {
		t.Errorf("coordinate count mismatch: got %d, eccodes has %d", n, ne)
		if ne < n {
			n = ne
		}
	}

	maxLatErr := 0.0
	maxLonErr := 0.0
	var badCount int
	for i := 0; i < n; i++ {
		latErr := math.Abs(lats[i] - eccLats[i])
		lonErr := math.Abs(lons[i] - eccLons[i])
		// Handle lon wrapping (e.g. 359.99 vs 0.01)
		if lonErr > 180 {
			lonErr = 360 - lonErr
		}
		if latErr > maxLatErr {
			maxLatErr = latErr
		}
		if lonErr > maxLonErr {
			maxLonErr = lonErr
		}
		if latErr > tolerance || lonErr > tolerance {
			if badCount < 5 {
				t.Errorf("point %d: lat=%f lon=%f, eccodes lat=%f lon=%f (latErr=%f lonErr=%f)",
					i, lats[i], lons[i], eccLats[i], eccLons[i], latErr, lonErr)
			}
			badCount++
		}
	}
	if badCount > 0 {
		t.Errorf("total mismatched points: %d/%d (maxLatErr=%f maxLonErr=%f)", badCount, n, maxLatErr, maxLonErr)
	} else {
		t.Logf("all %d points match within tolerance %f (maxLatErr=%f maxLonErr=%f)", n, tolerance, maxLatErr, maxLonErr)
	}
}

func TestGridCoordGaussianLatitudesSmall(t *testing.T) {
	// N=4 Gaussian latitudes computed from Legendre polynomial roots.
	// Reference values from eccodes compute_gaussian_latitudes(4).
	lats, err := coordGaussianLatitudes(4)
	if err != nil {
		t.Fatalf("gaussianLatitudes(4): %v", err)
	}
	if len(lats) != 8 {
		t.Fatalf("len=%d, want 8", len(lats))
	}
	// Expected N=4 Gaussian latitudes (8 values, descending)
	knownApprox := []float64{
		73.7992, 52.8129, 31.7041, 10.5699,
		-10.5699, -31.7041, -52.8129, -73.7992,
	}
	for i, want := range knownApprox {
		got := lats[i]
		if math.Abs(got-want) > 0.01 {
			t.Errorf("lats[%d] = %f, want ~%f", i, got, want)
		}
	}
	t.Logf("N=4 Gaussian lats: %v", lats)
}
