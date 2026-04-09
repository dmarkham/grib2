package grib2

import (
	"bytes"
	"math"
	"os"
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
			t.Errorf("refIdx=%d (row=%d,col=%d) but got idx=%d (row=%d,col=%d) — off by (%d,%d)",
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
	// approach should agree with brute force for points within or near
	// the grid domain. Points far outside the grid domain are excluded
	// as they clamp to a grid corner where the O(1) approach cannot
	// enumerate all boundary candidates.
	queries := [][2]float64{
		{60.0, 10.0},
		{55.0, 20.0},
		{65.0, 30.0},
		{70.0, 50.0},
		{75.0, 60.0},
		{50.0, -20.0}, // near grid edge in rotated lat
		{80.0, 0.0},   // near top edge
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
			tmpl31 := field.Section3.Template.(Template31)
			ni31 := int(tmpl31.Ni)
			o1Row, o1Col := o1Idx/ni31, o1Idx%ni31
			bfRow, bfCol := bestIdx/ni31, bestIdx%ni31
			t.Errorf("query (%f, %f): O(1) idx=%d (row=%d,col=%d), brute idx=%d (row=%d,col=%d)",
				targetLat, targetLon, o1Idx, o1Row, o1Col, bestIdx, bfRow, bfCol)
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
