package grib2

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"math"
	"testing"
)

// TestConstantFields_IgnoreDecimalScale: a field with 0 bits per value is
// the reference value, whatever the decimal scale factor, for simple and PNG
// packing (eccodes DataSimplePacking.cc, DataPngPacking.cc,
// tests/grib_decimalPrecision.sh: GRIB2.tmpl with D=3 gives 273).
func TestConstantFields_IgnoreDecimalScale(t *testing.T) {
	for _, d := range []int16{3, 2, 1, 0, -2} {
		tm := Template50{ReferenceValue: 273, DecimalScaleFactor: d}
		for name, unpack := range map[string]func() ([]float64, error){
			"simple": func() ([]float64, error) { return UnpackSimple(nil, tm, 5) },
			"png":    func() ([]float64, error) { return UnpackPNG(nil, tm, 5) },
		} {
			vals, err := unpack()
			if err != nil {
				t.Fatalf("%s D=%d: %v", name, d, err)
			}
			for _, v := range vals {
				if v != 273 {
					t.Fatalf("%s D=%d: %v", name, d, vals)
				}
			}
		}
	}
	for file, want := range map[string]float64{
		"testdata/upstream/constant_d3.grib2":     273,
		"testdata/upstream/tigge_kwbc_pl_t.grib2": 1,
	} {
		vals, err := decodeAll(t, file)[0].Fields[0].Values()
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range vals {
			if v != want {
				t.Fatalf("%s: value %v, want %v", file, v, want)
			}
		}
	}
}

// TestPackComplex_FromSampleGrib2 re-packs the simple-packed sample with
// complex packing (the old TestPackComplex_SampleGrib2RoundTrip skipped).
func TestPackComplex_FromSampleGrib2(t *testing.T) {
	f := decodeAll(t, "testdata/sample.grib2")[0].Fields[0]
	vals, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	for _, order := range []uint8{0, 1, 2} {
		var got []float64
		var e int16
		if order == 0 {
			data, tm, err := PackComplex(vals, 16)
			if err != nil {
				t.Fatal(err)
			}
			got, err = UnpackComplex(data, tm, uint32(len(vals)), 0, 0)
			e = tm.BinaryScaleFactor
			if err != nil {
				t.Fatal(err)
			}
		} else {
			data, tm, err := PackComplexSpatialDiff(vals, 16, order)
			if err != nil {
				t.Fatal(err)
			}
			got, err = UnpackComplex(data, tm.Template52, uint32(len(vals)), order, tm.NumberOfOctetsExtraDescriptors)
			e = tm.BinaryScaleFactor
			if err != nil {
				t.Fatal(err)
			}
		}
		for i := range vals {
			if math.Abs(got[i]-vals[i]) > 0.5*math.Ldexp(1, int(e))+1e-9 {
				t.Fatalf("order %d point %d: %v vs %v", order, i, got[i], vals[i])
			}
		}
	}
}

// TestSpatialDifferencing_EccodesData pins eccodes-written 5.3 data:
// gfs.c255.grib2 (order 1, 3 extra octets), complex_spatial_diff.grib2
// (constant, order 1) and re-packs of regular_latlon_surface at orders 1
// and 2 (eccodes tests/grib_complex_spatial_differencing.sh).
func TestSpatialDifferencing_EccodesData(t *testing.T) {
	g := decodeAll(t, "testdata/upstream/gfs.c255.grib2")[0].Fields[0]
	t53 := g.Section5.Template.(Template53)
	if t53.OrderOfSpatialDifferencing != 1 || t53.NumberOfOctetsExtraDescriptors != 3 || t53.BitsPerValue != 20 || t53.DecimalScaleFactor != 1 {
		t.Errorf("gfs.c255 template %+v", t53)
	}
	vals, err := g.Values()
	if err != nil {
		t.Fatal(err)
	}
	nan := 0
	for _, v := range vals {
		if math.IsNaN(v) {
			nan++
		}
	}
	if nan != 7665 {
		t.Errorf("gfs.c255: %d missing, want 7665", nan)
	}

	c := decodeAll(t, "testdata/complex_spatial_diff.grib2")[0].Fields[0]
	cv, err := c.Values()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range cv {
		if math.Abs(v-273.15) > 1e-4 {
			t.Fatalf("complex_spatial_diff: %v", v)
		}
	}

	orig, err := decodeAll(t, "testdata/regular_latlon_surface.grib2")[0].Fields[0].Values()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"testdata/upstream/csd_order1.grib2", "testdata/upstream/csd_order2.grib2", "testdata/upstream/complex_rll.grib2"} {
		f := decodeAll(t, file)[0].Fields[0]
		got, err := f.Values()
		if err != nil {
			t.Fatal(err)
		}
		var e, d int16
		switch tm := f.Section5.Template.(type) {
		case Template52:
			e, d = tm.BinaryScaleFactor, tm.DecimalScaleFactor
		case Template53:
			e, d = tm.BinaryScaleFactor, tm.DecimalScaleFactor
		}
		step := math.Ldexp(1, int(e)) * math.Pow10(-int(d))
		for i := range orig {
			if math.Abs(got[i]-orig[i]) > step {
				t.Fatalf("%s point %d: %v vs original %v (step %v)", file, i, got[i], orig[i], step)
			}
		}
	}
}

// TestBitmap_EdgeShapes: all-ones bitmaps, bitmaps ending in pad bits and
// all-missing fields with an empty Section 7 (eccodes tests/grib_bitmap.sh,
// grib_missing.sh ECC-2170, DataApplyBitmap.cc).
func TestBitmap_EdgeShapes(t *testing.T) {
	f := decodeAll(t, "testdata/sample.grib2")[0].Fields[0]
	want, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("all ones", func(t *testing.T) {
		g := f
		g.Section6 = Section6{Indicator: 0, Bitmap: bytes.Repeat([]byte{0xFF}, 62)}
		got, err := g.Values()
		if err != nil {
			t.Fatal(err)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("point %d: %v vs %v", i, got[i], want[i])
			}
		}
	})
	t.Run("pad bits", func(t *testing.T) {
		got, err := applyBitmap([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, []byte{0xFE, 0x7F}, 12)
		if err != nil {
			t.Fatal(err)
		}
		// Points 7 and 8 missing, as eccodes' 6x2 bitmap example.
		for i, v := range got {
			if (i == 7 || i == 8) != math.IsNaN(v) {
				t.Fatalf("%v", got)
			}
		}
	})
	for _, pack := range []string{"simple", "png"} {
		t.Run("all missing "+pack, func(t *testing.T) {
			g := f
			g.Section5.NumberOfValues = 0
			g.Section5.Template = Template50{ReferenceValue: 1, BitsPerValue: 8}
			if pack == "png" {
				g.Section5.TemplateNumber = 41
			}
			g.Section6 = Section6{Indicator: 0, Bitmap: make([]byte, 62)}
			g.Section7.Data = nil
			got, err := g.Values()
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range got {
				if !math.IsNaN(v) {
					t.Fatalf("value %v", v)
				}
			}
			if len(got) != 496 {
				t.Fatalf("%d values", len(got))
			}
		})
	}
}

// TestRegularGrid_ScanConsistency: -j scanning with the first latitude south
// of the last is inconsistent (eccodes tests/grib_ecc-922.sh,
// geo/iterator/Latlon.cc).
func TestRegularGrid_ScanConsistency(t *testing.T) {
	f := decodeAll(t, "testdata/sample.grib2")[0].Fields[0]
	tm := f.Section3.Template.(Template30)
	tm.LatitudeOfFirstGridPoint, tm.LatitudeOfLastGridPoint = tm.LatitudeOfLastGridPoint, tm.LatitudeOfFirstGridPoint
	f.Section3.Template = tm
	if _, _, err := f.GridCoordinates(); err == nil {
		t.Error("GridCoordinates: expected an error")
	}
	if _, _, _, _, err := f.NearestValue(30, 10); err == nil {
		t.Error("NearestValue: expected an error")
	}
}

// TestLongitudes_NormalisedAndIncrementsMissing: longitudes of grids that
// cross Greenwich or go past 360 are normalised, lookups give normalised
// nearest longitudes, and missing increments do not matter (eccodes
// tests/grib_ecc-1671.sh, grib_ecc-1406.sh, grib_ecc-984.sh).
func TestLongitudes_NormalisedAndIncrementsMissing(t *testing.T) {
	g := decodeAll(t, "testdata/grid_complex_spatial_differencing.grib2")[0].Fields[0]
	for _, q := range []struct {
		lat, lon float64
		idx      int
		nearLon  float64
	}{{40, 10, 37240, 10}, {15, 35, 83440, 35}} {
		_, idx, _, nLon, err := g.NearestValue(q.lat, q.lon)
		if err != nil {
			t.Fatal(err)
		}
		if idx != q.idx || math.Abs(nLon-q.nearLon) > 1e-9 {
			t.Errorf("NearestValue(%v, %v) = idx %d lon %v, want %d and %v", q.lat, q.lon, idx, nLon, q.idx, q.nearLon)
		}
	}

	big := decodeAll(t, "testdata/upstream/ecc1671_lon540.grib2")[0].Fields[0]
	_, lons, err := big.GridCoordinates()
	if err != nil {
		t.Fatal(err)
	}
	for _, lo := range lons[:3600] {
		if lo < 0 || lo >= 360 {
			t.Fatalf("longitude %v", lo)
		}
	}
	// Canonical column 3599 is the last longitude, 539.95 = 179.95.
	if math.Abs(lons[3599]-179.95) > 1e-6 {
		t.Errorf("last longitude %v, want 179.95", lons[3599])
	}

	for file, ref := range map[string]string{
		"testdata/upstream/ecc984_noincrements.grib2":    "testdata/sample.grib2",
		"testdata/upstream/ecc984_greenwich_noinc.grib2": "testdata/grid_complex_spatial_differencing.grib2",
	} {
		f := decodeAll(t, file)[0].Fields[0]
		r := decodeAll(t, ref)[0].Fields[0]
		la, lo, err := f.GridCoordinates()
		if err != nil {
			t.Fatal(err)
		}
		rla, rlo, _ := r.GridCoordinates()
		for i := range la {
			if math.Abs(la[i]-rla[i]) > 1e-9 || math.Abs(lo[i]-rlo[i]) > 1e-9 {
				t.Fatalf("%s point %d: (%v, %v) vs (%v, %v)", file, i, la[i], lo[i], rla[i], rlo[i])
			}
		}
	}
	// eccodes tests/grib_ecc-984.sh: grib_get_data lists "2.000 20.000 302.53" (lat 2, lon 20).
	f := decodeAll(t, "testdata/upstream/ecc984_noincrements.grib2")[0].Fields[0]
	v, _, _, _, err := f.NearestValue(2, 20)
	if err != nil || math.Abs(v-302.53) > 0.005 {
		t.Errorf("value at (2, 20) = %v, %v; want 302.53", v, err)
	}
}

// TestSection1_CalendarTemplate2 and a synthetic splice: any reserved
// octets after octet 21 are kept (eccodes tests/grib_calendar.sh).
func TestSection1_CalendarTemplate2(t *testing.T) {
	s1 := decodeAll(t, "testdata/upstream/section1_calendar2.grib2")[0].Section1
	if s1.Length != 26 || len(s1.Reserved) != 5 {
		t.Fatalf("Length %d, %d reserved octets", s1.Length, len(s1.Reserved))
	}
	raw := readFixture(t, "testdata/sample.grib2")
	l1 := int(binary.BigEndian.Uint32(raw[16:20]))
	spliced := append(append(append([]byte(nil), raw[:16+l1]...), 0xAB, 0xCD), raw[16+l1:]...)
	binary.BigEndian.PutUint32(spliced[16:20], uint32(l1+2))
	binary.BigEndian.PutUint64(spliced[8:16], uint64(len(spliced)))
	msg, err := ReadMessage(bytes.NewReader(spliced))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(msg.Section1.Reserved, []byte{0xAB, 0xCD}) {
		t.Errorf("Reserved = % x", msg.Section1.Reserved)
	}
	var b bytes.Buffer
	if err := WriteMessage(&b, msg); err != nil || !bytes.Equal(b.Bytes(), spliced) {
		t.Errorf("rewrite not byte-exact: %v", err)
	}
}

// TestRawTemplates_WithCoordinateValues: an unparsed PDT followed by the pv
// array keeps both apart (eccodes tests/grib2_templates.sh), and PDT 4.99
// (wave spectra) passes through.
func TestRawTemplates_WithCoordinateValues(t *testing.T) {
	f := decodeAll(t, "testdata/upstream/raw_pdt40_nv.grib2")[0].Fields[0]
	raw, ok := f.Section4.Template.(*RawProductTemplate)
	if !ok || raw.Number != 40 || f.Section4.NV != 184 {
		t.Fatalf("%T NV=%d", f.Section4.Template, f.Section4.NV)
	}
	if len(f.Section4.CoordinateValues) != 4*184 || len(raw.Data) != int(f.Section4.Length)-9-4*184 {
		t.Errorf("%d pv octets, %d template octets, length %d", len(f.Section4.CoordinateValues), len(raw.Data), f.Section4.Length)
	}
	w := decodeAll(t, "testdata/upstream/raw_pdt99_wave.grib2")[0].Fields[0]
	if r, ok := w.Section4.Template.(*RawProductTemplate); !ok || r.Number != 99 || len(r.Data) != int(w.Section4.Length)-9 {
		t.Errorf("PDT 99: %T", w.Section4.Template)
	}
}

// TestIEEE_TinyValuesAndBitmap: tiny values round-trip exactly and a
// bitmapped IEEE field packs only the present values (eccodes
// tests/grib_ieee.sh, grib_ccsds.sh).
func TestIEEE_TinyValuesAndBitmap(t *testing.T) {
	vals := []float64{1.234567e-36, -1.234567e-36, 0, 3.4e38}
	data, tm, err := PackIEEE(vals, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnpackIEEE(data, tm, uint32(len(vals)))
	if err != nil {
		t.Fatal(err)
	}
	for i := range vals {
		if got[i] != vals[i] {
			t.Errorf("64-bit: %v -> %v", vals[i], got[i])
		}
	}
	f := decodeAll(t, "testdata/upstream/ieee_bitmap.grib2")[0].Fields[0]
	width := 4
	if f.Section5.Template.(Template54).Precision == 2 {
		width = 8
	}
	if len(f.Section7.Data) != width*int(f.Section5.NumberOfValues) || f.Section5.NumberOfValues >= f.Section3.NumberOfDataPoints {
		t.Errorf("Section 7 %d octets for %d values of %d points", len(f.Section7.Data), f.Section5.NumberOfValues, f.Section3.NumberOfDataPoints)
	}
}

// TestUnpackPNG_DepthMismatch: a PNG whose sample depth does not match the
// bits per value is an error (eccodes DataPngPacking.cc).
func TestUnpackPNG_DepthMismatch(t *testing.T) {
	enc := func(img image.Image) []byte {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	r := image.Rect(0, 0, 4, 1)
	for name, c := range map[string]struct {
		data []byte
		bpv  uint8
	}{
		"gray8 at bpv 16": {enc(image.NewGray(r)), 16},
		"gray16 at bpv 8": {enc(image.NewGray16(r)), 8},
		"rgba at bpv 12":  {enc(image.NewNRGBA(r)), 12},
		"gray8 at bpv 24": {enc(image.NewGray(r)), 24},
	} {
		if _, err := UnpackPNG(c.data, Template50{BitsPerValue: c.bpv}, 4); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestRunLength_EccodesEncoded decodes the field eccodes'
// tests/grib_run_length_packing.cc writes: {missing, 10.1, 10.2, 10.3}
// with +j scanning.
func TestRunLength_EccodesEncoded(t *testing.T) {
	f := decodeAll(t, "testdata/upstream/rl_enc.grib2")[0].Fields[0]
	vals, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	// Data order is S->N; canonical puts the northern row first.
	want := []float64{10.2, 10.3, math.NaN(), 10.1}
	for i := range want {
		if math.IsNaN(want[i]) != math.IsNaN(vals[i]) || (!math.IsNaN(want[i]) && math.Abs(vals[i]-want[i]) > 1e-9) {
			t.Fatalf("values %v, want %v", vals, want)
		}
	}
}

// TestNearestValue_UpstreamAnswers pins eccodes' answers from
// tests/grib_nearest_test.sh and grib_ls -l so they hold without eccodes.
func TestNearestValue_UpstreamAnswers(t *testing.T) {
	for _, q := range []struct {
		file     string
		lat, lon float64
		idx      int // eccodes grib_ls -l index; these grids scan in mode 0
		val      float64
	}{
		{"testdata/reduced_gaussian_pressure_level.grib2", -40, 15, 4838, 284.07421875},
		{"testdata/ccsds.grib2", 79, 203, 4163, 103484},
		{"testdata/jpeg.grib2", 0, 0, 32400, 101309},
	} {
		f := decodeAll(t, q.file)[0].Fields[0]
		v, idx, _, _, err := f.NearestValue(q.lat, q.lon)
		if err != nil {
			t.Fatal(err)
		}
		if idx != q.idx || v != q.val {
			t.Errorf("%s (%v, %v): idx %d value %v, eccodes idx %d value %v", q.file, q.lat, q.lon, idx, v, q.idx, q.val)
		}
	}
	// tigge_egrr: basic angle 360/864; eccodes picks (51.67, 10.0).
	eg := decodeAll(t, "testdata/upstream/tigge_egrr_sfc_2t.grib2")[0].Fields[0]
	if _, _, nLat, nLon, err := eg.NearestValue(51.5, 10); err != nil || math.Abs(nLat-51.666667) > 1e-3 || math.Abs(nLon-10) > 1e-6 {
		t.Errorf("tigge_egrr: nearest (%v, %v), %v; want (51.67, 10)", nLat, nLon, err)
	}
	// Node identity on reduced grids (brute-force path).
	for _, file := range []string{"testdata/reduced_gaussian_surface.grib2", "testdata/upstream/reduced_gaussian_sub_area.grib2",
		"testdata/upstream/reduced_latlon_surface_constant.grib2"} {
		f := decodeAll(t, file)[0].Fields[0]
		lats, lons, err := f.GridCoordinates()
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(lats); i += len(lats) / 25 {
			if math.Abs(lats[i]) > 89.99 {
				continue
			}
			idx, _, _, err := nearestIndex(&f, lats[i], lons[i])
			if err != nil || idx != i {
				t.Fatalf("%s node %d: nearest %d, %v", file, i, idx, err)
			}
		}
	}
}

// TestLookups_LongitudeTurns: BilinearValue and Sampler give the same
// answer for a longitude any number of turns away.
func TestLookups_LongitudeTurns(t *testing.T) {
	for _, file := range []string{"testdata/sample.grib2", "testdata/rdps_surface.grib2"} {
		f := decodeAll(t, file)[0].Fields[0]
		s, err := NewSampler(&f)
		if err != nil {
			t.Fatal(err)
		}
		want, err := s.Weights(30.3, 10.7)
		if err != nil {
			t.Fatal(err)
		}
		for k := -3; k <= 3; k++ {
			w, err := s.Weights(30.3, 10.7+360*float64(k))
			if err != nil {
				t.Fatal(err)
			}
			if w.Index != want.Index || math.Abs(w.XFrac-want.XFrac) > 1e-9 || math.Abs(w.YFrac-want.YFrac) > 1e-9 {
				t.Errorf("%s k=%d: %+v vs %+v", file, k, w, want)
			}
		}
	}
	f := decodeAll(t, "testdata/sample.grib2")[0].Fields[0]
	b0, _ := f.BilinearValue(30.3, 10.7)
	for _, k := range []float64{-3, 2} {
		if b, _ := f.BilinearValue(30.3, 10.7+360*k); math.Abs(b-b0) > 1e-9 {
			t.Errorf("BilinearValue k=%v: %v vs %v", k, b, b0)
		}
	}
}

// TestSectionLength_Overflow: writers refuse sections too long for the
// 4-octet length (eccodes tests/grib_set_large_message_fail.cc).
func TestSectionLength_Overflow(t *testing.T) {
	if _, err := sectionLength(7, math.MaxUint32+1); err == nil {
		t.Error("2^32 octets accepted")
	}
	if n, err := sectionLength(7, 5); err != nil || n != 5 {
		t.Errorf("5 octets: %d, %v", n, err)
	}
}

// TestConstantSamples: eccodes' CCSDS and JPEG samples and tigge
// soil moisture (bpv 0 with a bitmap) decode to the reference value.
func TestConstantSamples(t *testing.T) {
	for _, file := range []string{"testdata/upstream/ccsds_sample.grib2", "testdata/upstream/jpeg_sample_reduced_gg.grib2",
		"testdata/upstream/tigge_kwbc_sfc_sm.grib2"} {
		f := decodeAll(t, file)[0].Fields[0]
		vals, err := f.Values()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var ref float64
		switch tm := f.Section5.Template.(type) {
		case Template50:
			ref = float64(tm.ReferenceValue)
		case Template540:
			ref = float64(tm.ReferenceValue)
		case Template542:
			ref = float64(tm.ReferenceValue)
		}
		nan := 0
		for _, v := range vals {
			if math.IsNaN(v) {
				nan++
			} else if v != ref {
				t.Fatalf("%s: value %v, reference %v", file, v, ref)
			}
		}
		if len(vals) != int(f.Section3.NumberOfDataPoints) || nan != int(f.Section3.NumberOfDataPoints-f.Section5.NumberOfValues) {
			t.Errorf("%s: %d values, %d missing", file, len(vals), nan)
		}
	}
}
