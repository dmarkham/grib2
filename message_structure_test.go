package grib2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// decodeAll decodes every message in a fixture file.
func decodeAll(t *testing.T, path string) []*Message {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := NewDecoder(bytes.NewReader(raw))
	var msgs []*Message
	for {
		msg, err := dec.Decode()
		if err != nil {
			if isEOF(err) {
				return msgs
			}
			t.Fatalf("%s: message %d: %v", path, len(msgs), err)
		}
		msgs = append(msgs, msg)
	}
}

func isEOF(err error) bool { return errors.Is(err, io.EOF) }

// eccodesInts runs grib_get -p key on file and returns one int per field.
func eccodesInts(t *testing.T, file, key string) []int64 {
	t.Helper()
	var out []int64
	for _, line := range runGribGet(t, file, "-p", key+":i") {
		v, err := strconv.ParseInt(strings.TrimSpace(line), 10, 64)
		if err != nil {
			t.Fatalf("grib_get %s %s: %q: %v", key, file, line, err)
		}
		out = append(out, v)
	}
	return out
}

// TestSection4_CoordinateValues pins the NV coordinate values (the pv array of
// hybrid model levels) that follow the product template (upstream eccodes
// tests/grib_update_sections_lengths.sh, codes_dump_content.sh).
func TestSection4_CoordinateValues(t *testing.T) {
	tests := []struct {
		file   string
		nv     int
		pdt    uint16
		pv0    float32 // first coordinate value (IEEE float32)
		pvLast float32
	}{
		{"testdata/upstream/test_uuid.grib2", 6, 0, 0, 0},
		{"testdata/upstream/pv_pdt8.grib2", 184, 8, 0, 1},
		{"testdata/constant_field.grib2", 2, 0, 0, 0},
		{"testdata/reduced_gaussian_model_level.grib2", 184, 0, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			msgs := decodeAll(t, tt.file)
			s4 := msgs[0].Fields[0].Section4
			if int(s4.NV) != tt.nv {
				t.Fatalf("NV = %d, want %d", s4.NV, tt.nv)
			}
			if s4.TemplateNumber != tt.pdt {
				t.Fatalf("PDT = %d, want %d", s4.TemplateNumber, tt.pdt)
			}
			if len(s4.CoordinateValues) != 4*tt.nv {
				t.Fatalf("len(CoordinateValues) = %d, want %d", len(s4.CoordinateValues), 4*tt.nv)
			}
			// The template must have parsed from the bytes before the pv array.
			if _, raw := s4.Template.(*RawProductTemplate); raw {
				t.Fatalf("PDT %d not parsed", tt.pdt)
			}
			if tt.pdt == 8 {
				tmpl := s4.Template.(Template48)
				if got, want := int(s4.Length), 9+template48MinSize+12*len(tmpl.TimeRangeSpecs)+4*tt.nv; got != want {
					t.Errorf("Section 4 length %d, want %d", got, want)
				}
			}
			pv := func(i int) float32 {
				return math.Float32frombits(binary.BigEndian.Uint32(s4.CoordinateValues[4*i:]))
			}
			if tt.file != "testdata/upstream/test_uuid.grib2" && tt.file != "testdata/constant_field.grib2" {
				// Hybrid levels: a values start at 0 and b values end at 1.
				if pv(0) != tt.pv0 || pv(tt.nv-1) != tt.pvLast {
					t.Errorf("pv[0]=%v pv[last]=%v, want %v %v", pv(0), pv(tt.nv-1), tt.pv0, tt.pvLast)
				}
			}
			if eccodesAvailable(t) {
				nv := eccodesInts(t, tt.file, "NV")
				if nv[0] != int64(tt.nv) {
					t.Errorf("eccodes NV = %d, want %d", nv[0], tt.nv)
				}
			}
		})
	}
}

// TestSection4_TooShortForCoordinateValues: an NV that does not fit in the
// section must be an error, not a silently shortened template.
func TestSection4_TooShortForCoordinateValues(t *testing.T) {
	msgs := decodeAll(t, "testdata/sample.grib2")
	var buf bytes.Buffer
	if err := WriteSection4(&buf, msgs[0].Fields[0].Section4); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	binary.BigEndian.PutUint16(b[5:7], 100) // claims 400 bytes of pv
	if _, err := ReadSection4(bytes.NewReader(b)); err == nil {
		t.Fatal("expected error for NV larger than the section")
	}
}

// TestForecastTime_NegativeSignMagnitude: forecastTime is a sign-magnitude
// 4-octet field (eccodes tests/grib_neg_fctime.sh, grib_sub_hourly.sh).
func TestForecastTime_NegativeSignMagnitude(t *testing.T) {
	for _, tt := range []struct {
		file string
		want int32
	}{
		{"testdata/upstream/fctime_neg2.grib2", -2},
		{"testdata/upstream/fctime_neg185_pdt8.grib2", -185},
		{"testdata/upstream/fctime_neg2_pdt11.grib2", -2},
	} {
		t.Run(tt.file, func(t *testing.T) {
			f := decodeAll(t, tt.file)[0].Fields[0]
			got := f.extractTemplate40().ForecastTime
			if got != tt.want {
				t.Errorf("ForecastTime = %d, want %d", got, tt.want)
			}
			if eccodesAvailable(t) {
				if e := eccodesInts(t, tt.file, "forecastTime")[0]; e != int64(tt.want) {
					t.Errorf("eccodes forecastTime = %d", e)
				}
			}
		})
	}

	// Encoder: -2 is 0x80000002 at octets 19-22.
	var buf bytes.Buffer
	if err := WriteSection4(&buf, Section4{TemplateNumber: 0, Template: Template40{ForecastTime: -2}}); err != nil {
		t.Fatal(err)
	}
	if got := buf.Bytes()[18:22]; !bytes.Equal(got, []byte{0x80, 0, 0, 2}) {
		t.Errorf("octets 19-22 = % x, want 80 00 00 02", got)
	}
}

// TestSurfaceLevel_MissingScaleFactor: a missing (0xFF) scale factor means the
// scaled value is used unscaled (eccodes G2Level accessor, tests/grib_ecc-2013.sh).
func TestSurfaceLevel_MissingScaleFactor(t *testing.T) {
	file := "testdata/upstream/level_sf_missing.grib2"
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	f := decodeAll(t, file)[0].Fields[0]
	tmpl := f.extractTemplate40()
	if tmpl.ScaleFactorOfFirstSurface != MissingScaleFactor {
		t.Fatalf("scale factor = %d, want MissingScaleFactor", tmpl.ScaleFactorOfFirstSurface)
	}
	// Octet 24 of Section 4 must be 0xFF on the wire.
	off := int(16 + decodeAll(t, file)[0].Section1.Length)
	off += int(f.Section3.Length)
	if raw[off+23] != 0xFF {
		t.Fatalf("Section 4 octet 24 = %#x, want 0xff", raw[off+23])
	}
	lvl, err := f.SurfaceLevel()
	if err != nil {
		t.Fatal(err)
	}
	if lvl != 11 {
		t.Errorf("SurfaceLevel = %v, want 11", lvl)
	}
	if eccodesAvailable(t) {
		if e := eccodesInts(t, file, "level")[0]; e != 11 {
			t.Errorf("eccodes level = %d", e)
		}
	}
}

// TestComputeSurfaceValue_ExactDecimals: levels with a positive scale factor
// must come out as the exact decimal (eccodes tests/grib_level.sh,
// grib_ecc-1560.sh).
func TestComputeSurfaceValue_ExactDecimals(t *testing.T) {
	for _, tt := range []struct {
		sf   int8
		sv   uint32
		want float64
	}{
		{1, 24, 2.4}, {1, 3, 0.3}, {1, 15, 1.5}, {2, 7, 0.07}, {3, 3456, 3.456},
		{0, 850, 850}, {-2, 850, 85000}, {MissingScaleFactor, 2, 2},
	} {
		if got := computeSurfaceValue(tt.sf, tt.sv); got != tt.want {
			t.Errorf("computeSurfaceValue(%d, %d) = %v, want %v", tt.sf, tt.sv, got, tt.want)
		}
	}
	if got := computeSurfaceValue(MissingScaleFactor, missingUint32); !math.IsNaN(got) {
		t.Errorf("both missing = %v, want NaN", got)
	}
}

// TestSurfaceAccessors_AllProductTemplates exercises SurfaceType/SurfaceLevel
// through every parsed product template and the unparsed-template error.
func TestSurfaceAccessors_AllProductTemplates(t *testing.T) {
	for _, file := range []string{
		"testdata/template42.grib2", "testdata/template48.grib2",
		"testdata/template411.grib2", "testdata/template412.grib2",
		"testdata/sample.grib2",
	} {
		t.Run(file, func(t *testing.T) {
			f := decodeAll(t, file)[0].Fields[0]
			typ, err := f.SurfaceType()
			if err != nil {
				t.Fatal(err)
			}
			lvl, err := f.SurfaceLevel()
			if err != nil {
				t.Fatal(err)
			}
			tmpl := f.extractTemplate40()
			if tmpl.ScaledValueOfFirstSurface == missingUint32 {
				if !math.IsNaN(lvl) {
					t.Errorf("missing scaled value: SurfaceLevel = %v, want NaN", lvl)
				}
			}
			if eccodesAvailable(t) {
				if e := eccodesInts(t, file, "typeOfFirstFixedSurface")[0]; e != int64(typ) {
					t.Errorf("SurfaceType = %d, eccodes %d", typ, e)
				}
				if !math.IsNaN(lvl) {
					if e := eccodesInts(t, file, "level")[0]; float64(e) != math.Trunc(lvl) {
						t.Errorf("SurfaceLevel = %v, eccodes level %d", lvl, e)
					}
				}
			}
		})
	}

	f := Field{Section4: Section4{TemplateNumber: 40, Template: &RawProductTemplate{Number: 40}}}
	if _, err := f.SurfaceType(); err == nil {
		t.Error("SurfaceType on raw template: expected error")
	}
	if _, err := f.SurfaceLevel(); err == nil {
		t.Error("SurfaceLevel on raw template: expected error")
	}
}

// TestSection1_ReservedOctets: octets 22-N of Section 1 (e.g. the calendar
// template set by eccodes setCalendarId, tests/grib_calendar.sh) are kept.
func TestSection1_ReservedOctets(t *testing.T) {
	file := "testdata/upstream/section1_calendar.grib2"
	s1 := decodeAll(t, file)[0].Section1
	if s1.Length != 24 || len(s1.Reserved) != 3 {
		t.Fatalf("Length=%d len(Reserved)=%d, want 24 and 3", s1.Length, len(s1.Reserved))
	}
	var buf bytes.Buffer
	if err := WriteSection1(&buf, s1); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 24 {
		t.Errorf("rewrote %d bytes, want 24", buf.Len())
	}
	if eccodesAvailable(t) {
		if e := eccodesInts(t, file, "section1Length")[0]; e != 24 {
			t.Errorf("eccodes section1Length = %d", e)
		}
	}
}

// TestMultiField_SharedSection3 decodes eccodes data/multi_created.grib2: one
// message with 21 fields where every field after the first repeats from
// Section 4 (tests/grib_multi.sh, grib_multi_from_message.cc).
func TestMultiField_SharedSection3(t *testing.T) {
	file := "testdata/upstream/multi_created.grib2"
	msgs := decodeAll(t, file)
	if len(msgs) != 1 || len(msgs[0].Fields) != 21 {
		t.Fatalf("got %d messages / %d fields, want 1 / 21", len(msgs), len(msgs[0].Fields))
	}
	fields := msgs[0].Fields
	if fields[0].SharesSection3 {
		t.Error("field 0 must own its Section 3")
	}
	var refMin, refMax []float64
	if eccodesAvailable(t) {
		for _, line := range runGribGet(t, file, "-F", "%.10g", "-p", "min,max") {
			p := strings.Fields(line)
			mn, _ := strconv.ParseFloat(p[0], 64)
			mx, _ := strconv.ParseFloat(p[1], 64)
			refMin, refMax = append(refMin, mn), append(refMax, mx)
		}
		if len(refMin) != 21 {
			t.Fatalf("eccodes sees %d fields", len(refMin))
		}
	}
	for i, f := range fields {
		if i > 0 && !f.SharesSection3 {
			t.Errorf("field %d: SharesSection3 = false", i)
		}
		if !reflect.DeepEqual(f.Section3, fields[0].Section3) {
			t.Errorf("field %d: Section3 differs from field 0", i)
		}
		vals, err := f.Values()
		if err != nil {
			t.Fatalf("field %d: %v", i, err)
		}
		if refMin != nil {
			mn, mx, _ := stats(vals)
			if math.Abs(mn-refMin[i]) > 1e-6*math.Max(1, math.Abs(refMin[i])) ||
				math.Abs(mx-refMax[i]) > 1e-6*math.Max(1, math.Abs(refMax[i])) {
				t.Errorf("field %d: min/max %v/%v, eccodes %v/%v", i, mn, mx, refMin[i], refMax[i])
			}
		}
	}
}

// buildMessage assembles a message from sections; parts are raw section bytes.
func buildMessage(t *testing.T, s0 Section0, parts ...[]byte) []byte {
	t.Helper()
	var body bytes.Buffer
	for _, p := range parts {
		body.Write(p)
	}
	body.WriteString("7777")
	s0.TotalLength = uint64(16 + body.Len())
	var buf bytes.Buffer
	if err := WriteSection0(&buf, s0); err != nil {
		t.Fatal(err)
	}
	buf.Write(body.Bytes())
	return buf.Bytes()
}

// sectionBytes serialises one section with the matching Write function.
func sectionBytes(t *testing.T, write func(*bytes.Buffer) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestMultiField_RepeatStructures builds messages that repeat from Section 2,
// 3 and 4 and checks they decode and rewrite byte-exact, and that malformed
// structures are rejected.
func TestMultiField_RepeatStructures(t *testing.T) {
	m := decodeAll(t, "testdata/sample.grib2")[0]
	f := m.Fields[0]
	s1 := sectionBytes(t, func(b *bytes.Buffer) error { return WriteSection1(b, m.Section1) })
	s2 := sectionBytes(t, func(b *bytes.Buffer) error { return WriteSection2(b, Section2{Data: []byte{1, 2, 3}}) })
	s3 := sectionBytes(t, func(b *bytes.Buffer) error { return WriteSection3(b, f.Section3) })
	s4 := sectionBytes(t, func(b *bytes.Buffer) error { return WriteSection4(b, f.Section4) })
	s5 := sectionBytes(t, func(b *bytes.Buffer) error { return WriteSection5(b, f.Section5) })
	s6 := sectionBytes(t, func(b *bytes.Buffer) error { return WriteSection6(b, f.Section6) })
	s7 := sectionBytes(t, func(b *bytes.Buffer) error { return WriteSection7(b, f.Section7) })
	want, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}

	good := []struct {
		name          string
		parts         [][]byte
		fields        int
		sharesSecond  bool
		section2InFld bool
	}{
		{"repeat from 2", [][]byte{s1, s2, s3, s4, s5, s6, s7, s2, s3, s4, s5, s6, s7}, 2, false, true},
		{"repeat from 3", [][]byte{s1, s3, s4, s5, s6, s7, s3, s4, s5, s6, s7}, 2, false, false},
		{"repeat from 4", [][]byte{s1, s3, s4, s5, s6, s7, s4, s5, s6, s7}, 2, true, false},
	}
	for _, tt := range good {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildMessage(t, m.Section0, tt.parts...)
			msg, err := ReadMessage(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(msg.Fields) != tt.fields {
				t.Fatalf("%d fields", len(msg.Fields))
			}
			second := msg.Fields[1]
			if second.SharesSection3 != tt.sharesSecond || (second.Section2 != nil) != tt.section2InFld {
				t.Errorf("field 1: SharesSection3=%v Section2=%v", second.SharesSection3, second.Section2)
			}
			for i, fld := range msg.Fields {
				got, err := fld.Values()
				if err != nil {
					t.Fatalf("field %d: %v", i, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("field %d values differ", i)
				}
			}
			var out bytes.Buffer
			if err := WriteMessage(&out, msg); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out.Bytes(), raw) {
				t.Errorf("rewrite differs: %s", describeDiff(raw, out.Bytes()))
			}
		})
	}

	bad := []struct {
		name  string
		parts [][]byte
	}{
		{"section 4 with no section 3", [][]byte{s1, s4, s5, s6, s7}},
		{"no fields", [][]byte{s1}},
		{"repeated section 2 then section 4", [][]byte{s1, s3, s4, s5, s6, s7, s2, s4, s5, s6, s7}},
		{"section 5 where 3 expected", [][]byte{s1, s5, s6, s7}},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildMessage(t, m.Section0, tt.parts...)
			if _, err := ReadMessage(bytes.NewReader(raw)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestBitmapIndicator254 covers "bitmap defined previously in this message"
// (eccodes tests/grib_bitmap.sh: a lone 254 fails with "missing bitmap").
func TestBitmapIndicator254(t *testing.T) {
	t.Run("no previous bitmap", func(t *testing.T) {
		f := decodeAll(t, "testdata/upstream/bitmap_indicator_254.grib2")[0].Fields[0]
		if _, err := f.Values(); err == nil {
			t.Fatal("expected error for indicator 254 with no earlier bitmap")
		}
	})

	t.Run("reuses previous bitmap", func(t *testing.T) {
		m := decodeAll(t, "testdata/grid_complex_spatial_differencing.grib2")[0]
		first := m.Fields[0]
		if !first.Section6.HasBitmap() {
			t.Fatal("fixture has no bitmap")
		}
		second := first
		second.SharesSection3 = true
		second.Section6 = Section6{Indicator: 254}
		m.Fields = []Field{first, second}
		var buf bytes.Buffer
		if err := WriteMessage(&buf, m); err != nil {
			t.Fatal(err)
		}
		msg, err := ReadMessage(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		v0, err := msg.Fields[0].Values()
		if err != nil {
			t.Fatal(err)
		}
		v1, err := msg.Fields[1].Values()
		if err != nil {
			t.Fatal(err)
		}
		if len(v0) != len(v1) {
			t.Fatalf("lengths %d vs %d", len(v0), len(v1))
		}
		nan := 0
		for i := range v0 {
			if math.IsNaN(v0[i]) != math.IsNaN(v1[i]) {
				t.Fatalf("NaN mask differs at %d", i)
			}
			if math.IsNaN(v0[i]) {
				nan++
			}
		}
		if nan == 0 {
			t.Fatal("expected missing points")
		}
		var out bytes.Buffer
		if err := WriteMessage(&out, msg); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out.Bytes(), buf.Bytes()) {
			t.Error("rewrite of indicator-254 message not byte-exact")
		}
	})

	t.Run("predefined bitmap unsupported", func(t *testing.T) {
		f := decodeAll(t, "testdata/sample.grib2")[0].Fields[0]
		f.Section6 = Section6{Indicator: 7}
		if _, err := f.Values(); err == nil || !strings.Contains(err.Error(), "predefined") {
			t.Fatalf("err = %v, want predefined-bitmap error", err)
		}
	})
}

// TestValues_CountMismatches: inconsistent counts between Sections 3, 5, 6
// and 7 must be errors (eccodes tests/grib_ecc-1792.sh,
// grib_check_message_validity.sh).
func TestValues_CountMismatches(t *testing.T) {
	t.Run("numberOfValues=2 on 496-point grid", func(t *testing.T) {
		f := decodeAll(t, "testdata/upstream/numvalues_mismatch.grib2")[0].Fields[0]
		if _, err := f.Values(); err == nil {
			t.Error("Values: expected error")
		}
		if _, _, _, _, err := f.NearestValue(50, 0); err == nil {
			t.Error("NearestValue: expected error")
		}
		if _, err := f.BilinearValue(50, 0); err == nil {
			t.Error("BilinearValue: expected error")
		}
	})

	base := decodeAll(t, "testdata/grid_complex_spatial_differencing.grib2")[0].Fields[0]
	if _, err := base.Values(); err != nil {
		t.Fatal(err)
	}

	t.Run("bitmap popcount differs from packed count", func(t *testing.T) {
		f := base
		f.Section6.Bitmap = append([]byte(nil), base.Section6.Bitmap...)
		// Clear four set bits.
		cleared := 0
		for i := range f.Section6.Bitmap {
			for b := 0; b < 8 && cleared < 4; b++ {
				if f.Section6.Bitmap[i]&(1<<b) != 0 {
					f.Section6.Bitmap[i] &^= 1 << b
					cleared++
				}
			}
		}
		if _, err := f.Values(); err == nil {
			t.Error("expected error")
		}
	})

	t.Run("short bitmap", func(t *testing.T) {
		f := base
		f.Section6.Bitmap = base.Section6.Bitmap[:len(base.Section6.Bitmap)/2]
		if _, err := f.Values(); err == nil {
			t.Error("expected error")
		}
	})

	t.Run("section 5 count differs from section 3 without bitmap", func(t *testing.T) {
		f := decodeAll(t, "testdata/sample.grib2")[0].Fields[0]
		f.Section3.NumberOfDataPoints++
		if _, err := f.Values(); err == nil {
			t.Error("expected error")
		}
	})

	t.Run("scanned grid dimensions differ from count", func(t *testing.T) {
		f := decodeAll(t, "testdata/sample.grib2")[0].Fields[0]
		tmpl := f.Section3.Template.(Template30)
		tmpl.ScanningMode = 0x40
		tmpl.Ni++
		f.Section3.Template = tmpl
		if _, err := f.Values(); err == nil {
			t.Error("expected error")
		}
	})
}

// TestEccodes_CoordinateValuesMatch compares every pv value with eccodes'
// pv array (grib_filter print [pv]).
func TestEccodes_CoordinateValuesMatch(t *testing.T) {
	if !eccodesAvailable(t) {
		t.Skip("eccodes not available")
	}
	filt := filepath.Join(t.TempDir(), "pv.filt")
	if err := os.WriteFile(filt, []byte(`print "[pv%.9g!1]";`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"testdata/upstream/pv_pdt8.grib2", "testdata/upstream/raw_pdt40_nv.grib2",
		// test_uuid.grib2's 6 coordinate values describe a general vertical
		// coordinate, which eccodes does not expose as pv.
		"testdata/reduced_gaussian_model_level.grib2"} {
		cmd := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_filter"), filt, file)
		cmd.Env = eccodesEnv()
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var want []float64
		for _, f := range strings.Fields(string(out)) {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil {
				t.Fatalf("%s: %q", file, f)
			}
			want = append(want, v)
		}
		s4 := decodeAll(t, file)[0].Fields[0].Section4
		if len(want) > int(s4.NV) && len(want)%int(s4.NV) == 0 {
			want = want[:s4.NV] // one array per message; compare the first
		}
		if len(want) != int(s4.NV) {
			t.Fatalf("%s: eccodes has %d pv values, NV is %d", file, len(want), s4.NV)
		}
		for i, w := range want {
			got := float64(math.Float32frombits(binary.BigEndian.Uint32(s4.CoordinateValues[4*i:])))
			if math.Abs(got-w) > 1e-6*math.Max(1, math.Abs(w)) {
				t.Errorf("%s: pv[%d] = %v, eccodes %v", file, i, got, w)
			}
		}
	}
}
