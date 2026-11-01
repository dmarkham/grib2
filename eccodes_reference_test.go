package grib2

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// eccodesReferenceFile holds, for every field of every fixture, what eccodes
// decodes (grib_get_data): counts, statistics and a sample of points with
// their coordinates, already put in our canonical order. It lets the value
// and coordinate checks run without eccodes installed. Regenerate with
//
//	GRIB2_WRITE_ECCODES_REFERENCE=1 go test -run TestEccodesReference_Write .
const eccodesReferenceFile = "testdata/upstream/eccodes_reference.json"

const referenceSamples = 48

type refPoint struct {
	I   int      `json:"i"`
	Lat *float64 `json:"lat,omitempty"`
	Lon *float64 `json:"lon,omitempty"`
	Val *float64 `json:"v,omitempty"` // nil when missing
}

type refField struct {
	N      int        `json:"n"`
	NaN    int        `json:"nan"`
	Min    float64    `json:"min"`
	Max    float64    `json:"max"`
	Mean   float64    `json:"mean"`
	Points []refPoint `json:"points"`
	// Meta holds eccodes' value (grib_get -p key:i) of every productKeys key.
	Meta map[string]string `json:"meta,omitempty"`
}

// canonicalEccodes returns eccodes' points for field number k (1-based,
// counting fields across the file) in our canonical order. coords is false
// when eccodes' coordinates cannot be compared with ours (projected grids
// not scanned 0x40, where eccodes treats the first point as the south-west
// corner).
func canonicalEccodes(t *testing.T, file string, k int, f *Field) (lats, lons, vals []float64, coords bool, ok bool) {
	t.Helper()
	split := filepath.Join(t.TempDir(), "split.grib2")
	cmd := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_copy"), file, split)
	cmd.Env = eccodesEnv()
	if err := cmd.Run(); err != nil {
		return nil, nil, nil, false, false
	}
	scan, ni, nj := gridScanParams(f.Section3.Template)
	coords = true
	switch f.Section3.Template.(type) {
	case Template310, Template320, Template330:
		if scan != 0x40 {
			coords = false
		}
		scan = 0x40 // eccodes lists these +i +j
	}
	if scan&0x30 == 0x30 {
		// eccodes' alternative-row swap ignores j-consecutive scanning, so it
		// is no reference here (TestApplyScanningMode_AllModes pins 0x30).
		return nil, nil, nil, false, false
	}
	if scan&0x10 != 0 {
		// eccodes' iterators ignore alternative rows; its swap is the reference.
		out := filepath.Join(t.TempDir(), "swap.grib2")
		c := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_set"),
			"-w", "count="+strconv.Itoa(k), "-s", "swapScanningAlternativeRows=1", split, out)
		c.Env = eccodesEnv()
		if c.Run() != nil {
			return nil, nil, nil, false, false
		}
		split, k = out, 1
		scan &^= 0x10
	}
	c := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_get_data"),
		"-L", "%.7f %.7f", "-F", "%.12g", "-m", "nan", "-w", "count="+strconv.Itoa(k), split)
	c.Env = eccodesEnv()
	if out, err := c.Output(); err != nil || len(out) == 0 {
		return nil, nil, nil, false, false
	}
	lats, lons, vals = eccodesPoints(t, split, k)
	if !coords {
		// Projected grid not scanned 0x40: eccodes takes the first point as
		// the south-west corner, so lay out the equivalent 0x40 grid that
		// starts at our south-west corner and use its coordinates.
		ours, ourLons, err := f.GridCoordinates()
		if err == nil && k == 1 {
			eq := equivalentScan64(t, split, ours[(nj-1)*ni], ourLons[(nj-1)*ni])
			lats, lons, _ = eccodesPoints(t, eq, 1)
			coords = true
		}
	}
	if scan != 0 && ni != missingUint32 && ni*nj == len(vals) {
		lats = applyScanningMode(lats, scan, ni, nj)
		lons = applyScanningMode(lons, scan, ni, nj)
		vals = applyScanningMode(vals, scan, ni, nj)
	}
	return lats, lons, vals, coords, len(vals) > 0
}

// eccodesProductMeta returns eccodes' productKeys values for field k (0-based).
func eccodesProductMeta(t *testing.T, file string, k int) map[string]string {
	t.Helper()
	keys := productKeys()
	var spec []string
	for _, key := range keys {
		spec = append(spec, key.name+":i")
	}
	cmd := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_get"), "-f", "-p", strings.Join(spec, ","), file)
	cmd.Env = eccodesEnv()
	out, _ := cmd.Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if k >= len(lines) {
		return nil
	}
	vals := strings.Fields(lines[k])
	if len(vals) != len(keys) {
		return nil
	}
	m := map[string]string{}
	for i, key := range keys {
		m[key.name] = vals[i]
	}
	return m
}

// checkMeta compares our productKeys values with eccodes' recorded ones.
func checkMeta(t *testing.T, m *Message, f *Field, meta map[string]string) {
	t.Helper()
	for _, key := range productKeys() {
		ev, ok := meta[key.name]
		if !ok || ev == "not_found" {
			continue
		}
		ours, have := key.get(m, f)
		if !have {
			if _, raw := f.Section4.Template.(*RawProductTemplate); raw {
				continue
			}
			t.Errorf("eccodes has %s=%s, we have no value", key.name, ev)
			continue
		}
		if ev == "MISSING" {
			if !isAllOnes(ours, key.width) {
				t.Errorf("%s = %d, eccodes MISSING", key.name, ours)
			}
			continue
		}
		e, err := strconv.ParseInt(ev, 10, 64)
		if err != nil || e != ours {
			t.Errorf("%s = %d, eccodes %s", key.name, ours, ev)
		}
	}
}

func sampleIndices(n int) []int {
	if n <= referenceSamples {
		idx := make([]int, n)
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
	idx := make([]int, referenceSamples)
	for i := range idx {
		idx[i] = i * (n - 1) / (referenceSamples - 1)
	}
	return idx
}

func fieldStats(vals []float64) (nan int, mn, mx, mean float64) {
	mn, mx = math.Inf(1), math.Inf(-1)
	sum, cnt := 0.0, 0
	for _, v := range vals {
		if math.IsNaN(v) {
			nan++
			continue
		}
		mn, mx, sum, cnt = math.Min(mn, v), math.Max(mx, v), sum+v, cnt+1
	}
	if cnt > 0 {
		mean = sum / float64(cnt)
	} else {
		mn, mx = 0, 0 // all missing
	}
	return
}

// fixtureFieldsInOrder decodes every field of a file in order.
func fixtureFieldsInOrder(path string) []*Field {
	var out []*Field
	for _, m := range decodeAllLenient(path) {
		for i := range m.Fields {
			out = append(out, &m.Fields[i])
		}
	}
	return out
}

// TestEccodesReference_Write regenerates eccodesReferenceFile from eccodes.
func TestEccodesReference_Write(t *testing.T) {
	if os.Getenv("GRIB2_WRITE_ECCODES_REFERENCE") == "" {
		t.Skip("set GRIB2_WRITE_ECCODES_REFERENCE=1 to regenerate")
	}
	if !eccodesAvailable(t) {
		t.Fatal("eccodes not available")
	}
	ref := map[string]map[string]refField{}
	for _, p := range allFixturePaths(t) {
		for k, f := range fixtureFieldsInOrder(p) {
			if _, err := f.Values(); err != nil {
				continue
			}
			lats, lons, vals, coords, ok := canonicalEccodes(t, p, k+1, f)
			if !ok {
				continue
			}
			nan, mn, mx, mean := fieldStats(vals)
			rf := refField{N: len(vals), NaN: nan, Min: mn, Max: mx, Mean: mean}
			for _, i := range sampleIndices(len(vals)) {
				pt := refPoint{I: i}
				if !math.IsNaN(vals[i]) {
					v := vals[i]
					pt.Val = &v
				}
				if coords {
					la, lo := lats[i], lons[i]
					pt.Lat, pt.Lon = &la, &lo
				}
				rf.Points = append(rf.Points, pt)
			}
			rf.Meta = eccodesProductMeta(t, p, k)
			if ref[p] == nil {
				ref[p] = map[string]refField{}
			}
			ref[p][strconv.Itoa(k)] = rf
		}
	}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eccodesReferenceFile, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEccodesReference_Check compares every field we decode with the stored
// eccodes reference: count, missing count, statistics, and sampled values
// and coordinates. It needs no eccodes, so scanning-mode, geometry and
// decoding regressions are caught everywhere.
func TestEccodesReference_Check(t *testing.T) {
	raw, err := os.ReadFile(eccodesReferenceFile)
	if err != nil {
		t.Fatal(err)
	}
	var ref map[string]map[string]refField
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for p, fields := range ref {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("fixture %s is in the reference but missing: %v", p, err)
			continue
		}
		ff := fixtureFieldsInOrder(p)
		var fm []*Message
		for _, m := range decodeAllLenient(p) {
			for range m.Fields {
				fm = append(fm, m)
			}
		}
		for ks, rf := range fields {
			k, _ := strconv.Atoi(ks)
			t.Run(p+"#"+ks, func(t *testing.T) {
				if k >= len(ff) {
					t.Fatalf("field %d not decoded (%d fields)", k, len(ff))
				}
				f := ff[k]
				checkMeta(t, fm[k], f, rf.Meta)
				vals, err := f.Values()
				if err != nil {
					t.Fatal(err)
				}
				nan, mn, mx, mean := fieldStats(vals)
				if len(vals) != rf.N || nan != rf.NaN {
					t.Fatalf("%d values, %d missing; eccodes %d, %d", len(vals), nan, rf.N, rf.NaN)
				}
				close := func(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }
				if rf.N > rf.NaN && (!close(mn, rf.Min) || !close(mx, rf.Max) || math.Abs(mean-rf.Mean) > 1e-6*math.Max(1, math.Abs(rf.Mean))) {
					t.Errorf("min/max/mean %v/%v/%v, eccodes %v/%v/%v", mn, mx, mean, rf.Min, rf.Max, rf.Mean)
				}
				var lats, lons []float64
				needCoords := len(rf.Points) > 0 && rf.Points[0].Lat != nil
				if needCoords {
					if lats, lons, err = f.GridCoordinates(); err != nil {
						t.Fatal(err)
					}
				}
				for _, pt := range rf.Points {
					v := vals[pt.I]
					if pt.Val == nil != math.IsNaN(v) || (pt.Val != nil && !close(v, *pt.Val)) {
						t.Errorf("point %d: value %v, eccodes %v", pt.I, v, pt.Val)
					}
					if pt.Lat != nil {
						if *pt.Lat == 0 && *pt.Lon == 0 {
							continue // off the disk of a space view
						}
						if d := haversineDeg(lats[pt.I], lons[pt.I], *pt.Lat, *pt.Lon) * 180 / math.Pi; d > 2e-3 {
							t.Errorf("point %d: (%v, %v), eccodes (%v, %v)", pt.I, lats[pt.I], lons[pt.I], *pt.Lat, *pt.Lon)
						}
					}
				}
			})
			checked++
		}
	}
	if checked < 280 {
		t.Errorf("only %d reference fields", checked)
	}
}
