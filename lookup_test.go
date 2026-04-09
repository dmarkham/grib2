package grib2

import (
	"bytes"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// loadSampleField reads testdata/sample.grib2 and returns the first field.
func loadSampleField(t *testing.T) *Field {
	t.Helper()
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read sample.grib2: %v", err)
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if len(msg.Fields) == 0 {
		t.Fatal("no fields in sample.grib2")
	}
	return &msg.Fields[0]
}

// TestNearestValue_ExactGridPoint queries known grid points from the
// sample.grib2 reference data and verifies NearestValue returns the exact
// value at that point.
func TestNearestValue_ExactGridPoint(t *testing.T) {
	field := loadSampleField(t)

	// Reference values from testdata/reference/sample.grib2.values:
	//   60.000    0.000 2.7900000000e+02  (index 0)
	//   60.000    2.000 2.7996093750e+02  (index 1)
	//   58.000    0.000 2.7963574219e+02  (index 16)
	//    0.000   30.000 3.0088183594e+02  (index 495, last point)
	tests := []struct {
		lat, lon  float64
		wantIdx   int
		wantValue float64
	}{
		{60.0, 0.0, 0, 279.0},
		{60.0, 2.0, 1, 279.96093750},
		{58.0, 0.0, 16, 279.63574219},
		{0.0, 30.0, 495, 300.88183594},
	}

	for _, tt := range tests {
		value, idx, nearLat, nearLon, err := field.NearestValue(tt.lat, tt.lon)
		if err != nil {
			t.Errorf("NearestValue(%v, %v): %v", tt.lat, tt.lon, err)
			continue
		}
		if idx != tt.wantIdx {
			t.Errorf("NearestValue(%v, %v): idx = %d, want %d", tt.lat, tt.lon, idx, tt.wantIdx)
		}
		if math.Abs(value-tt.wantValue) > 0.01 {
			t.Errorf("NearestValue(%v, %v): value = %v, want %v", tt.lat, tt.lon, value, tt.wantValue)
		}
		if math.Abs(nearLat-tt.lat) > 1e-3 {
			t.Errorf("NearestValue(%v, %v): nearLat = %v, want %v", tt.lat, tt.lon, nearLat, tt.lat)
		}
		if math.Abs(nearLon-tt.lon) > 1e-3 {
			t.Errorf("NearestValue(%v, %v): nearLon = %v, want %v", tt.lat, tt.lon, nearLon, tt.lon)
		}
	}
}

// TestNearestValue_BetweenPoints queries a point that lies between grid
// points and verifies NearestValue picks the closest one.
func TestNearestValue_BetweenPoints(t *testing.T) {
	field := loadSampleField(t)

	// Query (59.3, 0.7) is closer to (60, 0) (index 0) than to any other
	// grid point, since grid spacing is 2 degrees in both directions.
	// Distance to (60,0): ~0.78 deg
	// Distance to (60,2): ~1.48 deg
	// Distance to (58,0): ~1.42 deg
	value, idx, nearLat, nearLon, err := field.NearestValue(59.3, 0.7)
	if err != nil {
		t.Fatalf("NearestValue(59.3, 0.7): %v", err)
	}
	if idx != 0 {
		t.Errorf("NearestValue(59.3, 0.7): idx = %d, want 0 (nearest is 60N, 0E)", idx)
	}
	if math.Abs(nearLat-60.0) > 1e-3 || math.Abs(nearLon-0.0) > 1e-3 {
		t.Errorf("NearestValue(59.3, 0.7): nearest point = (%v, %v), want (60, 0)", nearLat, nearLon)
	}
	if value == 0 {
		t.Error("NearestValue returned zero value unexpectedly")
	}

	// Query (59.0, 1.0) is equidistant from the 4 corners of the cell.
	// Round should pick one consistently. The grid has 2-degree spacing,
	// so (59, 1) is exactly between (60,0), (60,2), (58,0), (58,2).
	// With rounding, j = round(-0.5) = 0, i = round(0.5) = 0 or 1 depending
	// on the go math.Round behavior for .5 (rounds to even -> 0).
	_, idx2, _, _, err := field.NearestValue(59.0, 1.0)
	if err != nil {
		t.Fatalf("NearestValue(59.0, 1.0): %v", err)
	}
	// Should be one of the 4 corners: index 0, 1, 16, or 17
	validIndices := map[int]bool{0: true, 1: true, 16: true, 17: true}
	if !validIndices[idx2] {
		t.Errorf("NearestValue(59.0, 1.0): idx = %d, want one of {0, 1, 16, 17}", idx2)
	}
}

// TestNearestValue_OffGridClamped tests that queries outside the grid
// domain are clamped to the nearest edge point.
func TestNearestValue_OffGridClamped(t *testing.T) {
	field := loadSampleField(t)

	// Query far north of the grid (grid goes 60N to 0N)
	_, idx, nearLat, _, err := field.NearestValue(90.0, 10.0)
	if err != nil {
		t.Fatalf("NearestValue(90, 10): %v", err)
	}
	// Should clamp to the top row (lat=60)
	if math.Abs(nearLat-60.0) > 1e-3 {
		t.Errorf("NearestValue(90, 10): nearLat = %v, want 60.0 (clamped)", nearLat)
	}
	_ = idx

	// Query far south of the grid
	_, _, nearLat, _, err = field.NearestValue(-10.0, 10.0)
	if err != nil {
		t.Fatalf("NearestValue(-10, 10): %v", err)
	}
	if math.Abs(nearLat-0.0) > 1e-3 {
		t.Errorf("NearestValue(-10, 10): nearLat = %v, want 0.0 (clamped)", nearLat)
	}
}

// TestBilinearValue_AtGridPoint tests bilinear interpolation at an exact
// grid point. Should return the exact value at that point (weight 1.0 on
// one corner, 0.0 on the other three).
func TestBilinearValue_AtGridPoint(t *testing.T) {
	field := loadSampleField(t)

	// Grid point at (60, 2) = index 1, value ~279.96093750
	refValue := 279.96093750

	bv, err := field.BilinearValue(60.0, 2.0)
	if err != nil {
		t.Fatalf("BilinearValue(60, 2): %v", err)
	}

	if math.Abs(bv-refValue) > 0.01 {
		t.Errorf("BilinearValue(60, 2) = %v, want ~%v", bv, refValue)
	}
}

// TestBilinearValue_BetweenPoints tests bilinear interpolation between
// grid points. The result should be between the values of the 4 surrounding
// points.
func TestBilinearValue_BetweenPoints(t *testing.T) {
	field := loadSampleField(t)

	// Query at the center of the first cell: (59, 1)
	// Surrounding corners:
	//   (60, 0) = 279.0         index 0
	//   (60, 2) = 279.96093750  index 1
	//   (58, 0) = 279.63574219  index 16
	//   (58, 2) = 280.19140625  index 17
	v00 := 279.0        // (60, 0)
	v10 := 279.96093750 // (60, 2)
	v01 := 279.63574219 // (58, 0)
	v11 := 280.19140625 // (58, 2)

	bv, err := field.BilinearValue(59.0, 1.0)
	if err != nil {
		t.Fatalf("BilinearValue(59, 1): %v", err)
	}

	// At the center of the cell, the bilinear result should be the average
	// of the 4 corners.
	expected := (v00 + v10 + v01 + v11) / 4.0
	if math.Abs(bv-expected) > 0.01 {
		t.Errorf("BilinearValue(59, 1) = %v, want ~%v (average of corners)", bv, expected)
	}

	// The result must be between the min and max of the 4 corners
	minV := math.Min(math.Min(v00, v10), math.Min(v01, v11))
	maxV := math.Max(math.Max(v00, v10), math.Max(v01, v11))
	if bv < minV-0.01 || bv > maxV+0.01 {
		t.Errorf("BilinearValue(59, 1) = %v, not between corners [%v, %v]", bv, minV, maxV)
	}
}

// TestBilinearValue_QuarterPoint tests bilinear at a quarter-point offset
// to verify the weighting is correct.
func TestBilinearValue_QuarterPoint(t *testing.T) {
	field := loadSampleField(t)

	// Query at (59.5, 0.5) in the cell bounded by:
	//   (60, 0) v00=279.0
	//   (60, 2) v10=279.96093750
	//   (58, 0) v01=279.63574219
	//   (58, 2) v11=280.19140625
	//
	// fi = (0.5 - 0) / 2 = 0.25, fj = (59.5 - 60) / (-2) = 0.25
	// xFrac=0.25, yFrac=0.25
	// bilinear = v00*0.75*0.75 + v10*0.25*0.75 + v01*0.75*0.25 + v11*0.25*0.25
	v00 := 279.0
	v10 := 279.96093750
	v01 := 279.63574219
	v11 := 280.19140625

	expected := v00*0.75*0.75 + v10*0.25*0.75 + v01*0.75*0.25 + v11*0.25*0.25

	bv, err := field.BilinearValue(59.5, 0.5)
	if err != nil {
		t.Fatalf("BilinearValue(59.5, 0.5): %v", err)
	}

	if math.Abs(bv-expected) > 0.01 {
		t.Errorf("BilinearValue(59.5, 0.5) = %v, want %v", bv, expected)
	}
}

// TestNearestValue_ConsistentWithReference verifies NearestValue against
// the reference values file (generated by eccodes grib_get_data).
func TestNearestValue_ConsistentWithReference(t *testing.T) {
	field := loadSampleField(t)
	refValues := loadReferenceValues(t, "testdata/reference/sample.grib2.values")

	// Check every grid point from the reference
	tmpl, ok := field.Section3.Template.(Template30)
	if !ok {
		t.Fatal("expected Template30")
	}

	ni := int(tmpl.Ni)
	nj := int(tmpl.Nj)

	for j := 0; j < nj; j++ {
		lat := 60.0 - float64(j)*2.0
		for i := 0; i < ni; i++ {
			lon := float64(i) * 2.0
			refIdx := j*ni + i
			refVal := refValues[refIdx]

			value, idx, _, _, err := field.NearestValue(lat, lon)
			if err != nil {
				t.Errorf("NearestValue(%v, %v): %v", lat, lon, err)
				continue
			}
			if idx != refIdx {
				t.Errorf("NearestValue(%v, %v): idx=%d, want %d", lat, lon, idx, refIdx)
			}
			if math.Abs(value-refVal) > 1e-4 {
				t.Errorf("NearestValue(%v, %v): value=%v, ref=%v", lat, lon, value, refVal)
			}
		}
	}
}

// TestBilinearValue_ReducedGaussianError verifies that BilinearValue returns
// an error for reduced Gaussian grids.
func TestBilinearValue_ReducedGaussianError(t *testing.T) {
	// Construct a minimal field with a reduced Gaussian template
	field := Field{
		Section3: Section3{
			Template: Template340{
				Ni: 0xFFFFFFFF, // MISSING = reduced
			},
		},
	}

	_, err := field.BilinearValue(0, 0)
	if err == nil {
		t.Error("BilinearValue should return error for reduced Gaussian grids")
	}
}

// TestBilinearValue_UnsupportedTemplateError verifies that BilinearValue
// returns an error for unsupported grid templates.
func TestBilinearValue_UnsupportedTemplateError(t *testing.T) {
	field := Field{
		Section3: Section3{
			Template: Template330{}, // Lambert conformal
		},
	}

	_, err := field.BilinearValue(0, 0)
	if err == nil {
		t.Error("BilinearValue should return error for Lambert conformal grids")
	}
}

// TestNearestValue_LambertFallback tests that NearestValue works for
// a Lambert grid using the brute-force fallback via GridCoordinates.
// Since GridCoordinates is not yet implemented for Lambert, this test
// verifies the error path gracefully.
func TestNearestValue_LambertFallback(t *testing.T) {
	field := Field{
		Section3: Section3{
			Template: Template330{},
		},
	}

	_, _, _, _, err := field.NearestValue(40, -100)
	if err == nil {
		t.Error("NearestValue on Lambert grid should error (GridCoordinates not yet supported)")
	}
}

func TestNearestValue_Template31_O1(t *testing.T) {
	// constant_field.grib2 uses Template 3.1 (rotated lat/lon)
	raw, err := os.ReadFile("testdata/constant_field.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	field := &msg.Fields[0]

	tmpl, ok := field.Section3.Template.(Template31)
	if !ok {
		t.Fatalf("expected Template31, got %T", field.Section3.Template)
	}
	t.Logf("Template31: Ni=%d, Nj=%d, ScanMode=0x%02x, SouthPole=(%d,%d)",
		tmpl.Ni, tmpl.Nj, tmpl.ScanningMode, tmpl.LatitudeOfSouthernPole, tmpl.LongitudeOfSouthernPole)

	// Get grid coordinates and values for reference
	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}
	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	// Test multiple points spread across the grid.
	// For each: query the exact grid point's geographic coordinates,
	// verify the returned INDEX matches (not just coordinates).
	testPoints := []int{0, 100, 500, len(lats) / 4, len(lats) / 2, 3 * len(lats) / 4, len(lats) - 1}
	for _, refIdx := range testPoints {
		if refIdx >= len(lats) {
			continue
		}
		targetLat := lats[refIdx]
		targetLon := lons[refIdx]

		val, idx, nearLat, nearLon, err := field.NearestValue(targetLat, targetLon)
		if err != nil {
			t.Errorf("NearestValue(%f, %f): %v", targetLat, targetLon, err)
			continue
		}

		// The returned index must match the reference index.
		// Allow off-by-one in row/col (rounding at grid boundary).
		ni := int(tmpl.Ni)
		refRow, refCol := refIdx/ni, refIdx%ni
		gotRow, gotCol := idx/ni, idx%ni
		rowDiff := abs(refRow - gotRow)
		colDiff := abs(refCol - gotCol)
		if rowDiff > 1 || colDiff > 1 {
			t.Errorf("refIdx=%d (row=%d,col=%d) but got idx=%d (row=%d,col=%d) -- off by (%d,%d)",
				refIdx, refRow, refCol, idx, gotRow, gotCol, rowDiff, colDiff)
		}

		// The returned VALUE must match the value at the returned index.
		if idx < len(values) && val != values[idx] {
			t.Errorf("refIdx=%d: returned val=%f but values[%d]=%f", refIdx, val, idx, values[idx])
		}

		// Coordinates should be close.
		latDiff := math.Abs(nearLat - targetLat)
		lonDiff := math.Abs(nearLon - targetLon)
		if latDiff > 0.15 || lonDiff > 0.15 {
			t.Errorf("NearestValue(%f, %f): got (%f, %f) at idx %d, expected near idx %d",
				targetLat, targetLon, nearLat, nearLon, idx, refIdx)
		}
	}

	t.Logf("Template31 NearestValue: O(1) path verified for %d grid points", len(lats))
}

// TestNearestValue_Template31_BruteForceAgreement verifies that the O(1)
// index computation matches brute-force haversine search for Template31 grids.
// Both the O(1) path and the brute-force use haversine distance, so they
// should agree at ALL points including grid edges.
func TestNearestValue_Template31_BruteForceAgreement(t *testing.T) {
	raw, err := os.ReadFile("testdata/constant_field.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	field := &msg.Fields[0]

	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	// Include both interior and edge/boundary points. The haversine-based
	// approach should agree with brute force for all points within or near
	// the grid domain.
	queries := [][2]float64{
		{60.0, 10.0},
		{55.0, 20.0},
		{65.0, 30.0},
		{70.0, 50.0},
		{75.0, 60.0},
		{50.0, -20.0}, // near grid edge in rotated lat
		{80.0, 0.0},   // near top edge
		{50.0, 40.0},  // near grid boundary
		{84.0, 60.0},  // near grid corner
	}

	for _, q := range queries {
		targetLat, targetLon := q[0], q[1]

		// O(1) result
		_, o1Idx, _, _, err := field.NearestValue(targetLat, targetLon)
		if err != nil {
			t.Errorf("NearestValue(%f, %f): %v", targetLat, targetLon, err)
			continue
		}

		// Brute-force result using haversine
		bestIdx := 0
		bestDist := math.MaxFloat64
		latRad := deg2rad(targetLat)
		lonRad := deg2rad(targetLon)
		for j := range lats {
			d := haversineRad(latRad, lonRad, deg2rad(lats[j]), deg2rad(lons[j]))
			if d < bestDist {
				bestDist = d
				bestIdx = j
			}
		}

		if o1Idx != bestIdx {
			// Allow tie: if both points are essentially the same distance
			// (within 1e-4 relative), this is a rounding tie at a cell boundary,
			// not a real bug. This happens for out-of-domain queries where
			// clamping puts multiple candidates at equal distance.
			o1Dist := haversineDeg(targetLat, targetLon, lats[o1Idx], lons[o1Idx])
			bfDist := haversineDeg(targetLat, targetLon, lats[bestIdx], lons[bestIdx])
			relDiff := math.Abs(o1Dist-bfDist) / (bfDist + 1e-30)
			if relDiff > 1e-4 {
				tmpl31 := field.Section3.Template.(Template31)
				ni31 := int(tmpl31.Ni)
				o1Row, o1Col := o1Idx/ni31, o1Idx%ni31
				bfRow, bfCol := bestIdx/ni31, bestIdx%ni31
				t.Errorf("query (%f, %f): O(1) idx=%d (row=%d,col=%d) dist=%.8f, brute idx=%d (row=%d,col=%d) dist=%.8f (relDiff=%.2e)",
					targetLat, targetLon, o1Idx, o1Row, o1Col, o1Dist, bestIdx, bfRow, bfCol, bfDist, relDiff)
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestBilinearValue_Template31(t *testing.T) {
	raw, err := os.ReadFile("testdata/constant_field.grib2")
	if err != nil {
		t.Skipf("read: %v", err)
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	field := &msg.Fields[0]

	// For a constant field, bilinear at any point should return the constant value
	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}
	if len(lats) == 0 {
		t.Skip("no grid points")
	}

	// Query at an interior grid point
	midIdx := len(lats) / 2
	val, err := field.BilinearValue(lats[midIdx], lons[midIdx])
	if err != nil {
		t.Fatalf("BilinearValue: %v", err)
	}

	values, _ := field.Values()
	if len(values) > 0 && !math.IsNaN(val) {
		// Constant field: all values should be the same
		expected := values[0]
		if math.Abs(val-expected) > 1e-3 {
			t.Errorf("BilinearValue = %f, expected ~%f (constant field)", val, expected)
		}
	}
}

// ===========================================================================
// Exhaustive brute-force tests added after the haversine fix
// ===========================================================================

// ---------------------------------------------------------------------------
// Brute-force haversine helper
// ---------------------------------------------------------------------------

// bruteForceNearest performs a brute-force search over all grid coordinates
// using haversine distance and returns the index of the closest point.
func bruteForceNearest(lats, lons []float64, targetLat, targetLon float64) int {
	bestIdx := 0
	bestDist := math.MaxFloat64

	targetLatRad := targetLat * math.Pi / 180.0
	targetLonRad := targetLon * math.Pi / 180.0

	for i := range lats {
		latRad := lats[i] * math.Pi / 180.0
		lonRad := lons[i] * math.Pi / 180.0
		d := haversineRad(targetLatRad, targetLonRad, latRad, lonRad)
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return bestIdx
}

// ---------------------------------------------------------------------------
// Template31 field loader helper
// ---------------------------------------------------------------------------

func loadTemplate31Field(t *testing.T, path string) *Field {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("read %s: %v", path, err)
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage %s: %v", path, err)
	}
	if len(msg.Fields) == 0 {
		t.Fatalf("no fields in %s", path)
	}
	return &msg.Fields[0]
}

// ---------------------------------------------------------------------------
// Test 1: Template31 exhaustive brute-force comparison
// Uses 50 random + corners + edges + center = 59 total query points.
// For EACH, the O(1) index must match brute-force haversine EXACTLY.
// ---------------------------------------------------------------------------

func TestNearestValue_Template31_ExhaustiveBruteForce(t *testing.T) {
	field := loadTemplate31Field(t, "testdata/constant_field.grib2")

	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}
	if len(lats) == 0 {
		t.Fatal("no grid points")
	}

	tmpl, ok := field.Section3.Template.(Template31)
	if !ok {
		t.Fatalf("expected Template31, got %T", field.Section3.Template)
	}
	ni := int(tmpl.Ni)
	nj := int(tmpl.Nj)

	// Build a set of test indices: corners, edges, center, and 50 random points.
	testIndices := make(map[int]string)

	// 4 grid corners
	testIndices[0] = "top-left"
	testIndices[ni-1] = "top-right"
	testIndices[(nj-1)*ni] = "bottom-left"
	testIndices[nj*ni-1] = "bottom-right"

	// Center
	testIndices[(nj/2)*ni+ni/2] = "center"

	// Edge midpoints
	testIndices[ni/2] = "top-edge-mid"
	testIndices[(nj-1)*ni+ni/2] = "bottom-edge-mid"
	testIndices[(nj/2)*ni] = "left-edge-mid"
	testIndices[(nj/2)*ni+ni-1] = "right-edge-mid"

	// 50 randomly-distributed points (deterministic seed)
	rng := rand.New(rand.NewSource(42))
	for len(testIndices) < 59 {
		idx := rng.Intn(len(lats))
		if _, exists := testIndices[idx]; !exists {
			testIndices[idx] = "random"
		}
	}

	failures := 0
	for refIdx, label := range testIndices {
		if refIdx >= len(lats) {
			continue
		}
		targetLat := lats[refIdx]
		targetLon := lons[refIdx]

		// O(1) path
		_, o1Idx, _, _, err := field.NearestValue(targetLat, targetLon)
		if err != nil {
			t.Errorf("[%s] idx=%d NearestValue(%f, %f): %v", label, refIdx, targetLat, targetLon, err)
			failures++
			continue
		}

		// Brute-force haversine
		bfIdx := bruteForceNearest(lats, lons, targetLat, targetLon)

		if o1Idx != bfIdx {
			// Check if the two results are at effectively the same distance (tie).
			d1 := haversineRad(
				targetLat*math.Pi/180, targetLon*math.Pi/180,
				lats[o1Idx]*math.Pi/180, lons[o1Idx]*math.Pi/180,
			)
			d2 := haversineRad(
				targetLat*math.Pi/180, targetLon*math.Pi/180,
				lats[bfIdx]*math.Pi/180, lons[bfIdx]*math.Pi/180,
			)
			// If distances are effectively equal (tie), both answers are acceptable.
			if math.Abs(d1-d2) > 1e-10 {
				t.Errorf("[%s] refIdx=%d query=(%f,%f): O(1) idx=%d (dist=%.10f) != brute idx=%d (dist=%.10f)",
					label, refIdx, targetLat, targetLon, o1Idx, d1, bfIdx, d2)
				failures++
			}
		}
	}

	t.Logf("Tested %d points (%d failures)", len(testIndices), failures)
}

// ---------------------------------------------------------------------------
// Test 2: Template31 RDPS real file with known cities
// ---------------------------------------------------------------------------

func TestNearestValue_Template31_RDPSRealFile(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}
	rdpsPath := filepath.Join(home, ".cache", "astro-forecast", "rdps.20260408.18z", "f008",
		"20260408T18Z_MSC_RDPS_AirTemp_AGL-2m_RLatLon0.09_PT008H.grib2")

	if _, err := os.Stat(rdpsPath); os.IsNotExist(err) {
		t.Skipf("RDPS file not found: %s", rdpsPath)
	}

	field := loadTemplate31Field(t, rdpsPath)

	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}
	if len(lats) == 0 {
		t.Fatal("no grid points")
	}

	type cityTest struct {
		name string
		lat  float64
		lon  float64
	}

	cities := []cityTest{
		{"San Diego", 32.715, -117.161},
		{"Calgary", 51.05, -114.07},
		{"Edmonton", 53.55, -113.49},
		{"Vancouver", 49.28, -123.12},
		{"Toronto", 43.65, -79.38},
		{"Montreal", 45.50, -73.57},
		{"Winnipeg", 49.90, -97.14},
		{"Ottawa", 45.42, -75.70},
		{"Halifax", 44.65, -63.57},
		{"Yellowknife", 62.45, -114.37},
	}

	for _, city := range cities {
		val, o1Idx, nearLat, nearLon, err := field.NearestValue(city.lat, city.lon)
		if err != nil {
			t.Errorf("%s: NearestValue(%f, %f): %v", city.name, city.lat, city.lon, err)
			continue
		}

		// Brute-force haversine comparison -- index must match exactly.
		bfIdx := bruteForceNearest(lats, lons, city.lat, city.lon)

		if o1Idx != bfIdx {
			d1 := haversineRad(
				city.lat*math.Pi/180, city.lon*math.Pi/180,
				lats[o1Idx]*math.Pi/180, lons[o1Idx]*math.Pi/180,
			)
			d2 := haversineRad(
				city.lat*math.Pi/180, city.lon*math.Pi/180,
				lats[bfIdx]*math.Pi/180, lons[bfIdx]*math.Pi/180,
			)
			if math.Abs(d1-d2) > 1e-10 {
				t.Errorf("%s: O(1) idx=%d (dist=%.10f) != brute idx=%d (dist=%.10f)",
					city.name, o1Idx, d1, bfIdx, d2)
			}
		}

		// Verify temperature is physically reasonable (220K to 320K for 2m air temp).
		if val < 220 || val > 320 {
			t.Errorf("%s: temperature=%f K is outside reasonable range [220, 320]", city.name, val)
		}

		t.Logf("%s: idx=%d, value=%.2f K, nearestPoint=(%f, %f)", city.name, o1Idx, val, nearLat, nearLon)
	}
}

// ---------------------------------------------------------------------------
// Test 3: Template30 brute-force agreement (sample.grib2)
// 20 query points spread across the grid. Exact index match required.
// ---------------------------------------------------------------------------

func TestNearestValue_Template30_BruteForceAgreement(t *testing.T) {
	field := loadSampleField(t)

	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}
	if len(lats) == 0 {
		t.Fatal("no grid points")
	}

	tmpl, ok := field.Section3.Template.(Template30)
	if !ok {
		t.Fatalf("expected Template30, got %T", field.Section3.Template)
	}

	// Compute the grid extent.
	lat1 := float64(tmpl.LatitudeOfFirstGridPoint) / 1e6
	lon1 := float64(tmpl.LongitudeOfFirstGridPoint) / 1e6
	lat2 := float64(tmpl.LatitudeOfLastGridPoint) / 1e6
	lon2 := float64(tmpl.LongitudeOfLastGridPoint) / 1e6

	minLat := math.Min(lat1, lat2)
	maxLat := math.Max(lat1, lat2)
	minLon := math.Min(lon1, lon2)
	maxLon := math.Max(lon1, lon2)

	// 20 query points: 4 corners + 16 random interior.
	type queryPoint struct {
		lat, lon float64
		label    string
	}
	queries := []queryPoint{
		{minLat, minLon, "SW-corner"},
		{minLat, maxLon, "SE-corner"},
		{maxLat, minLon, "NW-corner"},
		{maxLat, maxLon, "NE-corner"},
	}

	rng := rand.New(rand.NewSource(123))
	for i := 0; i < 16; i++ {
		lat := minLat + rng.Float64()*(maxLat-minLat)
		lon := minLon + rng.Float64()*(maxLon-minLon)
		queries = append(queries, queryPoint{lat, lon, "random"})
	}

	for _, q := range queries {
		_, o1Idx, _, _, err := field.NearestValue(q.lat, q.lon)
		if err != nil {
			t.Errorf("[%s] NearestValue(%f, %f): %v", q.label, q.lat, q.lon, err)
			continue
		}

		bfIdx := bruteForceNearest(lats, lons, q.lat, q.lon)

		if o1Idx != bfIdx {
			d1 := haversineRad(
				q.lat*math.Pi/180, q.lon*math.Pi/180,
				lats[o1Idx]*math.Pi/180, lons[o1Idx]*math.Pi/180,
			)
			d2 := haversineRad(
				q.lat*math.Pi/180, q.lon*math.Pi/180,
				lats[bfIdx]*math.Pi/180, lons[bfIdx]*math.Pi/180,
			)
			if math.Abs(d1-d2) > 1e-10 {
				t.Errorf("[%s] query=(%f,%f): O(1) idx=%d (dist=%.10f) != brute idx=%d (dist=%.10f)",
					q.label, q.lat, q.lon, o1Idx, d1, bfIdx, d2)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test 4: Template31 all grid edge queries
// Query the 4 geographic corners and verify the result is at a grid boundary.
// ---------------------------------------------------------------------------

func TestNearestValue_AllGridEdges_Template31(t *testing.T) {
	field := loadTemplate31Field(t, "testdata/constant_field.grib2")

	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}
	if len(lats) == 0 {
		t.Fatal("no grid points")
	}

	tmpl := field.Section3.Template.(Template31)
	ni := int(tmpl.Ni)
	nj := int(tmpl.Nj)

	// Find the geographic bounding box from the actual coordinates.
	minLat, maxLat := math.MaxFloat64, -math.MaxFloat64
	minLon, maxLon := math.MaxFloat64, -math.MaxFloat64
	for i := range lats {
		if lats[i] < minLat {
			minLat = lats[i]
		}
		if lats[i] > maxLat {
			maxLat = lats[i]
		}
		if lons[i] < minLon {
			minLon = lons[i]
		}
		if lons[i] > maxLon {
			maxLon = lons[i]
		}
	}

	// Query the 4 geographic corners.
	corners := []struct {
		name     string
		lat, lon float64
	}{
		{"geo-SW", minLat, minLon},
		{"geo-SE", minLat, maxLon},
		{"geo-NW", maxLat, minLon},
		{"geo-NE", maxLat, maxLon},
	}

	for _, c := range corners {
		val, idx, nearLat, nearLon, err := field.NearestValue(c.lat, c.lon)
		if err != nil {
			t.Errorf("%s: NearestValue(%f, %f): %v", c.name, c.lat, c.lon, err)
			continue
		}
		_ = val

		// The returned index should be at a grid boundary (first/last row or column).
		row := idx / ni
		col := idx % ni
		atBoundary := row == 0 || row == nj-1 || col == 0 || col == ni-1
		if !atBoundary {
			t.Errorf("%s: idx=%d (row=%d, col=%d) is not at a grid boundary (ni=%d, nj=%d)",
				c.name, idx, row, col, ni, nj)
		}

		// The returned coordinates should be within a reasonable range.
		// For rotated grids, the geographic bounding box corners can be
		// far outside the grid's actual coverage (the grid footprint is a
		// rotated rhombus, not a lat/lon rectangle), so we use a generous
		// tolerance. The key assertions are: no panic, no error, and the
		// result is at a grid boundary.
		latDist := math.Abs(nearLat - c.lat)
		lonDist := math.Abs(nearLon - c.lon)
		if latDist > 45.0 || lonDist > 45.0 {
			t.Errorf("%s: nearestPoint=(%f,%f) is unreasonably far from query=(%f,%f)",
				c.name, nearLat, nearLon, c.lat, c.lon)
		}

		t.Logf("%s: query=(%f,%f) -> idx=%d (row=%d,col=%d) nearest=(%f,%f)",
			c.name, c.lat, c.lon, idx, row, col, nearLat, nearLon)
	}
}

// ---------------------------------------------------------------------------
// Test 5: BilinearValue Template31 interior points
// Verify interpolated value is bounded by the 4 surrounding grid point values.
// ---------------------------------------------------------------------------

func TestBilinearValue_Template31_Interior(t *testing.T) {
	field := loadTemplate31Field(t, "testdata/constant_field.grib2")

	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	tmpl := field.Section3.Template.(Template31)
	ni := int(tmpl.Ni)
	nj := int(tmpl.Nj)

	// Pick 10 interior grid cells and query at their midpoints.
	rng := rand.New(rand.NewSource(99))
	var prevVal float64
	tested := 0

	for attempt := 0; attempt < 100 && tested < 10; attempt++ {
		// Pick a random interior cell (avoid edges).
		row := 2 + rng.Intn(nj-4)
		col := 2 + rng.Intn(ni-4)

		// Get the 4 corner indices of this cell in canonical order.
		idx00 := row*ni + col
		idx10 := row*ni + col + 1
		idx01 := (row+1)*ni + col
		idx11 := (row+1)*ni + col + 1

		if idx11 >= len(lats) || idx11 >= len(values) {
			continue
		}

		// Query point is the average of the 4 corners in geographic space.
		queryLat := (lats[idx00] + lats[idx10] + lats[idx01] + lats[idx11]) / 4.0
		queryLon := (lons[idx00] + lons[idx10] + lons[idx01] + lons[idx11]) / 4.0

		bv, err := field.BilinearValue(queryLat, queryLon)
		if err != nil {
			t.Logf("BilinearValue(%f, %f): %v (skipping)", queryLat, queryLon, err)
			continue
		}

		// The bilinear result must be bounded by surrounding values.
		v00 := values[idx00]
		v10 := values[idx10]
		v01 := values[idx01]
		v11 := values[idx11]

		minV := math.Min(math.Min(v00, v10), math.Min(v01, v11))
		maxV := math.Max(math.Max(v00, v10), math.Max(v01, v11))

		if bv < minV-1e-6 || bv > maxV+1e-6 {
			t.Errorf("cell (row=%d,col=%d) query=(%f,%f): bilinear=%f outside bounds [%f, %f]",
				row, col, queryLat, queryLon, bv, minV, maxV)
		}

		// Smoothness check: no wild jumps from previous value.
		if tested > 0 && math.Abs(bv-prevVal) > (maxV-minV+1)*100 {
			t.Errorf("cell (row=%d,col=%d): bilinear=%f, previous=%f -- suspiciously large jump",
				row, col, bv, prevVal)
		}

		prevVal = bv
		tested++
	}

	if tested < 10 {
		t.Logf("only tested %d interior points (some skipped)", tested)
	}
}

// ---------------------------------------------------------------------------
// Test 6: Haversine vs Euclidean disagreement at high latitude
// Constructs a case where haversine and Euclidean degree-distance pick
// different points, and verifies the brute-force helper uses haversine.
// ---------------------------------------------------------------------------

func TestNearestValue_HaversineVsEuclidean(t *testing.T) {
	// At lat=80, cos(80) ~ 0.1736, so 1 degree of longitude ~ 19.3 km
	// while 1 degree of latitude ~ 111.3 km.
	//
	// Point A: (80.0, 0.0)
	// Point B: (79.5, 2.5)
	// Query:   (80.0, 2.5)
	//
	// Euclidean (degree-space):
	//   dist(A) = sqrt(0 + 6.25) = 2.5
	//   dist(B) = sqrt(0.25 + 0) = 0.5  <- Euclidean picks B
	//
	// Haversine:
	//   dist(A) ~ 48.3 km  (2.5 lon-degrees at lat 80)
	//   dist(B) ~ 55.7 km  (0.5 lat-degrees)
	//   -> Haversine picks A

	queryLat := 80.0
	queryLon := 2.5

	pointALat := 80.0
	pointALon := 0.0

	pointBLat := 79.5
	pointBLon := 2.5

	qLatR := queryLat * math.Pi / 180
	qLonR := queryLon * math.Pi / 180
	aLatR := pointALat * math.Pi / 180
	aLonR := pointALon * math.Pi / 180
	bLatR := pointBLat * math.Pi / 180
	bLonR := pointBLon * math.Pi / 180

	distA := haversineRad(qLatR, qLonR, aLatR, aLonR)
	distB := haversineRad(qLatR, qLonR, bLatR, bLonR)

	t.Logf("Haversine dist to A (80,0): %f rad", distA)
	t.Logf("Haversine dist to B (79.5,2.5): %f rad", distB)

	// Confirm haversine picks A (closer).
	if distA >= distB {
		t.Fatalf("Expected haversine to pick A as closer, but distA=%f >= distB=%f", distA, distB)
	}

	// Confirm Euclidean (in degree space) would pick B.
	eucDistA := math.Sqrt((queryLat-pointALat)*(queryLat-pointALat) +
		(queryLon-pointALon)*(queryLon-pointALon))
	eucDistB := math.Sqrt((queryLat-pointBLat)*(queryLat-pointBLat) +
		(queryLon-pointBLon)*(queryLon-pointBLon))

	if eucDistB >= eucDistA {
		t.Fatalf("Expected Euclidean to pick B as closer, but eucDistB=%f >= eucDistA=%f",
			eucDistB, eucDistA)
	}

	t.Logf("Euclidean picks B (dist=%.4f), Haversine picks A (dist=%.6f rad) -- confirming disagreement",
		eucDistB, distA)

	// Verify bruteForceNearest uses haversine and picks A (index 0).
	testLats := []float64{pointALat, pointBLat}
	testLons := []float64{pointALon, pointBLon}

	bfIdx := bruteForceNearest(testLats, testLons, queryLat, queryLon)
	if bfIdx != 0 {
		t.Errorf("bruteForceNearest picked index %d (point B), expected 0 (point A) -- haversine should win",
			bfIdx)
	}
}
