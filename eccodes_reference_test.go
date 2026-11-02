package grib2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// eccodesReferenceFile holds, for every field of every fixture (including
// the golden encoder outputs in goldenDir), what eccodes says about it: a
// fingerprint of every decoded value, sampled points with coordinates,
// metadata keys, the pv array and the nearest grid point to a set of
// queries. Tests read it instead of running eccodes.
//
//	GRIB2_WRITE_ECCODES_REFERENCE=1 go test -run TestEccodesReference_Write .
//
// regenerates it (and the golden encoder files); with eccodes installed,
// TestEccodesReference_UpToDate checks it still matches eccodes.
const eccodesReferenceFile = "testdata/upstream/eccodes_reference.json"

const referenceSamples = 256

type refPoint struct {
	I   int      `json:"i"`
	Lat *float64 `json:"lat,omitempty"`
	Lon *float64 `json:"lon,omitempty"`
	Val *float64 `json:"v,omitempty"` // nil when missing
}

type refNearest struct {
	Lat, Lon float64 // the query
	Index    int     // eccodes' choice, as a canonical index
}

type refField struct {
	N      int               `json:"n"`
	NaN    int               `json:"nan"`
	Min    float64           `json:"min"`
	Max    float64           `json:"max"`
	Mean   float64           `json:"mean"`
	Hash   string            `json:"hash"` // valueFingerprint of all values
	Points []refPoint        `json:"points"`
	Meta   map[string]string `json:"meta,omitempty"`
	PV     []float64         `json:"pv,omitempty"`
	Near   []refNearest      `json:"near,omitempty"`
}

// nearestQueries are the points upstream's tests and our lookups probe:
// upstream reference points, poles, the 0/360 seam, outside the grid and
// several turns round in longitude.
var nearestQueries = [][2]float64{{51.5, 10}, {-40, 15}, {79, 203}, {0, 0}, {34, 241}, {89.9, 5},
	{-89.9, 355}, {10, 359.9}, {10, -5}, {45, 399.6}, {-20, 366.3}, {40, 10}, {15, 35}, {2, 20}}

// valueFingerprint hashes a field's values so two decoders can be compared
// exactly without storing every value. Values of a scaled packing are
// reduced to their packed integer X (Y = (R + X*2^E) * 10^-D), which both
// sides recover exactly (above 40 bits only the top 40 are kept: a double
// cannot hold more); IEEE values are hashed exactly; anything else to 9
// significant digits.
func valueFingerprint(f *Field, vals []float64) string {
	var r, scale, dec float64
	ints, exact := false, false
	t50 := func(t Template50) {
		if t.BitsPerValue > 0 {
			r, scale, dec, ints = float64(t.ReferenceValue), math.Ldexp(1, int(t.BinaryScaleFactor)), math.Pow10(int(t.DecimalScaleFactor)), true
			if t.BitsPerValue > 40 {
				scale = math.Ldexp(scale, int(t.BitsPerValue)-40)
			}
		}
	}
	if _, ok := f.Section5.Template.(Template54); ok {
		exact = true
	}
	switch t := f.Section5.Template.(type) {
	case Template50:
		t50(t)
	case Template52:
		t50(t.Template50)
	case Template53:
		t50(t.Template50)
	case Template540:
		t50(Template50{ReferenceValue: t.ReferenceValue, BinaryScaleFactor: t.BinaryScaleFactor,
			DecimalScaleFactor: t.DecimalScaleFactor, BitsPerValue: t.BitsPerValue})
	case Template542:
		t50(Template50{ReferenceValue: t.ReferenceValue, BinaryScaleFactor: t.BinaryScaleFactor,
			DecimalScaleFactor: t.DecimalScaleFactor, BitsPerValue: t.BitsPerValue})
	}
	h := sha256.New()
	var b []byte
	for _, v := range vals {
		b = b[:0]
		switch {
		case math.IsNaN(v):
			b = append(b, 'n')
		case ints:
			b = strconv.AppendInt(b, int64(math.Round((v*dec-r)/scale)), 10)
		case exact:
			b = strconv.AppendFloat(b, v, 'g', -1, 64)
		default:
			b = strconv.AppendFloat(b, v, 'g', 9, 64)
		}
		b = append(b, ',')
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// referencePaths are the files the reference covers.
func referencePaths(t *testing.T) []string {
	g, _ := filepath.Glob(filepath.Join(goldenDir, "*.grib2"))
	return append(allFixturePaths(t), g...)
}

func runEccodes(t *testing.T, env []string, name string, args ...string) ([]byte, bool) {
	t.Helper()
	cmd := execEccodes(name, args...)
	cmd.Env = append(cmd.Env, env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return out, err == nil && !strings.Contains(stderr.String(), "ECCODES ERROR")
}

// canonicalIndexMap returns, for each data-order index, its canonical index.
func canonicalIndexMap(f *Field, n int) []int {
	scan, ni, nj := gridScanParams(f.Section3.Template)
	idx := make([]float64, n)
	for i := range idx {
		idx[i] = float64(i)
	}
	if scan != 0 && ni != missingUint32 && ni*nj == n {
		idx = applyScanningMode(idx, scan, ni, nj)
	}
	m := make([]int, n)
	for c, d := range idx {
		m[int(d)] = c
	}
	return m
}

// canonicalEccodes returns eccodes' points for message k (1-based) of a
// file with one field per message, in our canonical order. coords is false
// when eccodes cannot place the points.
func canonicalEccodes(t *testing.T, split string, k int, f *Field) (lats, lons, vals []float64, coords, ok bool) {
	t.Helper()
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
		if _, ok := runEccodes(t, nil, "grib_set", "-w", "count="+strconv.Itoa(k), "-s", "swapScanningAlternativeRows=1", split, out); !ok {
			return nil, nil, nil, false, false
		}
		split, k = out, 1
		scan &^= 0x10
	}
	if _, ok := runEccodes(t, nil, "grib_get_data", "-w", "count="+strconv.Itoa(k), split); !ok {
		return nil, nil, nil, false, false
	}
	lats, lons, vals = eccodesPoints(t, split, k)
	if !coords {
		// Projected grid not scanned 0x40: eccodes takes the first point as
		// the south-west corner, so lay out the equivalent 0x40 grid that
		// starts at our south-west corner and use its coordinates.
		if ours, ourLons, err := f.GridCoordinates(); err == nil && k == 1 {
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

// fixtureFieldsInOrder decodes every field of a file in order, with the
// message each belongs to.
func fixtureFieldsInOrder(path string) ([]*Field, []*Message) {
	var fs []*Field
	var ms []*Message
	for _, m := range decodeAllLenient(path) {
		for i := range m.Fields {
			fs = append(fs, &m.Fields[i])
			ms = append(ms, m)
		}
	}
	return fs, ms
}

// eccodesProductMeta returns eccodes' productKeys values for every field.
func eccodesProductMeta(t *testing.T, file string) [][]string {
	t.Helper()
	var spec []string
	for _, key := range productKeys() {
		spec = append(spec, key.name+":i")
	}
	out, _ := runEccodes(t, nil, "grib_get", "-f", "-p", strings.Join(spec, ","), file)
	var rows [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if f := strings.Fields(line); len(f) == len(spec) {
			rows = append(rows, f)
		}
	}
	return rows
}

// eccodesPV returns the pv array of the first message, or nil.
func eccodesPV(t *testing.T, file string) []float64 {
	t.Helper()
	filt := filepath.Join(t.TempDir(), "pv.filt")
	if err := os.WriteFile(filt, []byte(`print "[pv%.9g!1]";`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, ok := runEccodes(t, nil, "grib_filter", filt, file)
	if !ok {
		return nil
	}
	var pv []float64
	for _, s := range strings.Fields(string(out)) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		pv = append(pv, v)
	}
	return pv
}

// eccodesNearestIndex returns eccodes' nearest data-order index (grib_ls -l).
func eccodesNearestIndex(t *testing.T, file string, lat, lon float64) (int, bool) {
	t.Helper()
	out, ok := runEccodes(t, nil, "grib_ls", "-j", "-w", "count=1", "-l", fmt.Sprintf("%v,%v,1", lat, lon), "-p", "count", file)
	if !ok {
		return 0, false
	}
	var res []struct {
		Neighbours []struct {
			Index int `json:"index"`
		} `json:"neighbours"`
	}
	if json.Unmarshal(out, &res) != nil || len(res) == 0 || len(res[0].Neighbours) == 0 {
		return 0, false
	}
	return res[0].Neighbours[0].Index, true
}

// buildEccodesReference asks eccodes about every field of every reference file.
func buildEccodesReference(t *testing.T) map[string]map[string]refField {
	t.Helper()
	ref := map[string]map[string]refField{}
	for _, p := range referencePaths(t) {
		split := filepath.Join(t.TempDir(), "split.grib2")
		if _, ok := runEccodes(t, nil, "grib_copy", p, split); !ok {
			continue
		}
		fields, _ := fixtureFieldsInOrder(p)
		meta := eccodesProductMeta(t, p)
		for k, f := range fields {
			if _, err := f.Values(); err != nil {
				continue
			}
			lats, lons, vals, coords, ok := canonicalEccodes(t, split, k+1, f)
			if !ok {
				continue
			}
			if strings.HasPrefix(p, goldenDir) && f.Section5.TemplateNumber == 40 {
				// Our JPEG2000 output must decode identically with OpenJPEG
				// and with Jasper (the decoder of wgrib2 and g2clib).
				oj, ok1 := runEccodes(t, []string{"ECCODES_GRIB_JPEG=openjpeg"}, "grib_get_data", "-F", "%.17g", "-m", "nan", split)
				ja, ok2 := runEccodes(t, []string{"ECCODES_GRIB_JPEG=jasper"}, "grib_get_data", "-F", "%.17g", "-m", "nan", split)
				if !ok1 || !ok2 || !bytes.Equal(oj, ja) {
					t.Fatalf("%s: Jasper and OpenJPEG do not decode our JPEG2000 alike", p)
				}
			}
			nan, mn, mx, mean := fieldStats(vals)
			rf := refField{N: len(vals), NaN: nan, Min: mn, Max: mx, Mean: mean, Hash: valueFingerprint(f, vals)}
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
			if k < len(meta) {
				rf.Meta = map[string]string{}
				for i, key := range productKeys() {
					rf.Meta[key.name] = meta[k][i]
				}
			}
			if k == 0 && f.Section4.NV > 0 {
				rf.PV = eccodesPV(t, p)
				if len(rf.PV) > int(f.Section4.NV) {
					rf.PV = rf.PV[:f.Section4.NV] // one array per message
				}
			}
			if k == 0 && len(fields) == 1 && coords {
				cm := canonicalIndexMap(f, len(vals))
				for _, q := range nearestQueries {
					if d, ok := eccodesNearestIndex(t, p, q[0], q[1]); ok && d < len(cm) {
						rf.Near = append(rf.Near, refNearest{q[0], q[1], cm[d]})
					}
				}
			}
			if ref[p] == nil {
				ref[p] = map[string]refField{}
			}
			ref[p][strconv.Itoa(k)] = rf
		}
	}
	return ref
}

func loadEccodesReference(t *testing.T) map[string]map[string]refField {
	t.Helper()
	raw, err := os.ReadFile(eccodesReferenceFile)
	if err != nil {
		t.Fatal(err)
	}
	var ref map[string]map[string]refField
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

// TestEccodesReference_Write regenerates the golden encoder files and
// eccodesReferenceFile from eccodes.
func TestEccodesReference_Write(t *testing.T) {
	if os.Getenv("GRIB2_WRITE_ECCODES_REFERENCE") == "" {
		t.Skip("set GRIB2_WRITE_ECCODES_REFERENCE=1 to regenerate (needs eccodes)")
	}
	if !eccodesAvailable(t) {
		t.Fatal("eccodes not available")
	}
	writeGoldenEncoded(t)
	b, err := json.Marshal(buildEccodesReference(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eccodesReferenceFile, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEccodesReference_UpToDate re-derives the reference from eccodes and
// checks the committed one still matches it. It is the only test that runs
// eccodes; everything else reads the committed reference.
func TestEccodesReference_UpToDate(t *testing.T) {
	if os.Getenv("GRIB2_WRITE_ECCODES_REFERENCE") != "" {
		t.Skip("regenerating")
	}
	if !eccodesAvailable(t) || testing.Short() {
		t.Skip("needs eccodes in reference/eccodes-install; TestEccodesReference_Check checks the committed reference")
	}
	got, err := json.Marshal(buildEccodesReference(t))
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]map[string]refField
	if err := json.Unmarshal(got, &round); err != nil {
		t.Fatal(err)
	}
	want := loadEccodesReference(t)
	for p := range want {
		if !reflect.DeepEqual(round[p], want[p]) {
			t.Errorf("%s: committed reference differs from eccodes; regenerate with GRIB2_WRITE_ECCODES_REFERENCE=1", p)
		}
	}
	for p := range round {
		if _, ok := want[p]; !ok {
			t.Errorf("%s: not in the committed reference; regenerate", p)
		}
	}
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
			if _, raw := f.Section4.Template.(*RawProductTemplate); raw || optionalMetaKeys[key.name] {
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

// TestEccodesReference_Check compares every field we decode with what
// eccodes decoded: every value (fingerprint), missing count, statistics,
// sampled values and coordinates, metadata, pv and nearest points. It needs
// no eccodes.
func TestEccodesReference_Check(t *testing.T) {
	ref := loadEccodesReference(t)
	checked := 0
	for p, fields := range ref {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("fixture %s is in the reference but missing: %v", p, err)
			continue
		}
		ff, fm := fixtureFieldsInOrder(p)
		for ks, rf := range fields {
			k, _ := strconv.Atoi(ks)
			checked++
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
				if h := valueFingerprint(f, vals); h != rf.Hash {
					t.Errorf("values differ from eccodes' (fingerprint %.12s, eccodes %.12s)", h, rf.Hash)
				}
				close := func(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }
				if rf.N > rf.NaN && (!close(mn, rf.Min) || !close(mx, rf.Max) || math.Abs(mean-rf.Mean) > 1e-6*math.Max(1, math.Abs(rf.Mean))) {
					t.Errorf("min/max/mean %v/%v/%v, eccodes %v/%v/%v", mn, mx, mean, rf.Min, rf.Max, rf.Mean)
				}
				var lats, lons []float64
				if (len(rf.Points) > 0 && rf.Points[0].Lat != nil) || len(rf.Near) > 0 {
					if lats, lons, err = f.GridCoordinates(); err != nil {
						t.Fatal(err)
					}
				}
				for _, pt := range rf.Points {
					v := vals[pt.I]
					if pt.Val == nil != math.IsNaN(v) || (pt.Val != nil && !close(v, *pt.Val)) {
						t.Errorf("point %d: value %v, eccodes %v", pt.I, v, pt.Val)
					}
					if pt.Lat != nil && !(*pt.Lat == 0 && *pt.Lon == 0) { // (0, 0): off a space view's disk
						if d := haversineDeg(lats[pt.I], lons[pt.I], *pt.Lat, *pt.Lon) * 180 / math.Pi; d > 2e-3 {
							t.Errorf("point %d: (%v, %v), eccodes (%v, %v)", pt.I, lats[pt.I], lons[pt.I], *pt.Lat, *pt.Lon)
						}
					}
				}
				if rf.PV != nil {
					cv := f.Section4.CoordinateValues
					if len(cv) != 4*len(rf.PV) {
						t.Fatalf("%d pv octets, eccodes has %d values", len(cv), len(rf.PV))
					}
					for i, w := range rf.PV {
						got := float64(math.Float32frombits(uint32(cv[4*i])<<24 | uint32(cv[4*i+1])<<16 | uint32(cv[4*i+2])<<8 | uint32(cv[4*i+3])))
						if math.Abs(got-w) > 1e-6*math.Max(1, math.Abs(w)) {
							t.Errorf("pv[%d] = %v, eccodes %v", i, got, w)
						}
					}
				}
				for _, q := range rf.Near {
					idx, _, _, err := nearestIndex(f, q.Lat, q.Lon)
					if err != nil {
						t.Fatalf("nearest (%v, %v): %v", q.Lat, q.Lon, err)
					}
					if idx == q.Index {
						continue
					}
					// Ours must be at least as close as eccodes' choice
					// (eccodes' search is approximate outside projected grids).
					dq := haversineDeg(q.Lat, q.Lon, lats[idx], lons[idx])
					de := haversineDeg(q.Lat, q.Lon, lats[q.Index], lons[q.Index])
					if dq > de+1e-9 {
						t.Errorf("nearest (%v, %v): ours %d is farther than eccodes' %d", q.Lat, q.Lon, idx, q.Index)
					}
				}
			})
		}
	}
	if checked < 300 {
		t.Errorf("only %d reference fields", checked)
	}
}
