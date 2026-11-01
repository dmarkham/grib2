package grib2

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestTemplate48_SeveralTimeRanges decodes statistically processed products
// with more than one time range (eccodes tests/grib_stattype.sh,
// grib_ecc-813.sh, grib_step.sh).
func TestTemplate48_SeveralTimeRanges(t *testing.T) {
	t.Run("4.8 three ranges", func(t *testing.T) {
		f := decodeAll(t, "testdata/upstream/pdt8_3ranges.grib2")[0].Fields[0]
		tmpl := f.Section4.Template.(Template48)
		if f.Section4.Length != 82 || tmpl.NumberOfTimeRangeSpecs != 3 || len(tmpl.TimeRangeSpecs) != 3 {
			t.Fatalf("length %d, %d ranges", f.Section4.Length, len(tmpl.TimeRangeSpecs))
		}
		if tmpl.NumberOfMissingInStatisticalProcess != 31 {
			t.Errorf("NumberOfMissingInStatisticalProcess = %d", tmpl.NumberOfMissingInStatisticalProcess)
		}
		for i, want := range []struct {
			stat uint8
			len  uint32
		}{{0, 720}, {6, 24}, {2, 1}} {
			tr := tmpl.TimeRangeSpecs[i]
			if tr.TypeOfStatisticalProcessing != want.stat || tr.LengthOfTimeRange != want.len || tr.IndicatorOfUnitForTimeRange != 1 {
				t.Errorf("range %d: %+v", i, tr)
			}
		}
	})
	t.Run("4.11 two ranges", func(t *testing.T) {
		f := decodeAll(t, "testdata/upstream/pdt11_2ranges.grib2")[0].Fields[0]
		tmpl := f.Section4.Template.(Template411)
		if len(tmpl.TimeRangeSpecs) != 2 || tmpl.PerturbationNumber != 7 || tmpl.NumberOfMissingInStatisticalProcess != 31 {
			t.Fatalf("%+v", tmpl)
		}
		if int(f.Section4.Length) != 9+template411MinSize+12*2 {
			t.Errorf("length %d", f.Section4.Length)
		}
		tr := tmpl.TimeRangeSpecs
		if tr[0].TimeIncrement != 24 || tr[0].TypeOfTimeIncrement != 1 || tr[1].TimeIncrement != 1 || tr[1].TypeOfTimeIncrement != 2 || tr[1].LengthOfTimeRange != 24 {
			t.Errorf("ranges %+v", tr)
		}
	})
	t.Run("4.12 two ranges", func(t *testing.T) {
		f := decodeAll(t, "testdata/upstream/pdt12_2ranges.grib2")[0].Fields[0]
		tmpl := f.Section4.Template.(Template412)
		if len(tmpl.TimeRangeSpecs) != 2 || int(f.Section4.Length) != 9+template412MinSize+24 {
			t.Fatalf("length %d, %d ranges", f.Section4.Length, len(tmpl.TimeRangeSpecs))
		}
	})
}

// TestTemplate48_TruncatedTimeRanges: a time-range count larger than the
// section is an error (eccodes tests/grib2_templates.sh, grib2_wave_spectra.sh).
func TestTemplate48_TruncatedTimeRanges(t *testing.T) {
	for _, tt := range []struct {
		file  string
		octet int // 1-based octet of the count within Section 4
	}{
		{"testdata/template48.grib2", 42},
		{"testdata/template411.grib2", 45},
		{"testdata/template412.grib2", 44},
	} {
		f := decodeAll(t, tt.file)[0].Fields[0]
		var b bytes.Buffer
		if err := WriteSection4(&b, f.Section4); err != nil {
			t.Fatal(err)
		}
		raw := b.Bytes()
		raw[tt.octet-1] += 2
		if _, err := ReadSection4(bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "too short for") {
			t.Errorf("%s: err = %v", tt.file, err)
		}
	}
	// PDT 4.0 section of 20 octets.
	hdr := []byte{0, 0, 0, 20, 4, 0, 0, 0, 0}
	if _, err := ReadSection4(bytes.NewReader(append(hdr, make([]byte, 11)...))); err == nil {
		t.Error("short PDT 4.0: no error")
	}
}

// TestRawTemplates_Passthrough: templates the library does not parse are
// kept raw and rewritten byte for byte (eccodes tests/grib2_templates.sh,
// grib2_probability.sh, data/grid_ieee.grib with PDT 4.40).
func TestRawTemplates_Passthrough(t *testing.T) {
	for _, tt := range []struct {
		file string
		pdt  uint16
	}{
		{"testdata/upstream/raw_pdt5.grib2", 5},
		{"testdata/upstream/raw_pdt40.grib2", 40},
		{"testdata/upstream/raw_pdt113.grib2", 113},
		{"testdata/upstream/grid_ieee.grib2", 40},
	} {
		f := decodeAll(t, tt.file)[0].Fields[0]
		raw, ok := f.Section4.Template.(*RawProductTemplate)
		if !ok || raw.Number != tt.pdt || raw.TemplateNumber() != tt.pdt {
			t.Fatalf("%s: template %T %+v", tt.file, f.Section4.Template, f.Section4.Template)
		}
		if len(raw.Data) != int(f.Section4.Length)-9-4*int(f.Section4.NV) {
			t.Errorf("%s: %d raw octets, section length %d", tt.file, len(raw.Data), f.Section4.Length)
		}
		if _, err := f.SurfaceLevel(); err == nil {
			t.Errorf("%s: SurfaceLevel on a raw template: no error", tt.file)
		}
		if _, err := f.Values(); err != nil {
			t.Errorf("%s: Values: %v", tt.file, err)
		}
	}
	// Raw grid and data representation templates.
	sp := decodeAll(t, "testdata/spectral_simple.grib2")[0].Fields[0]
	if g, ok := sp.Section3.Template.(*RawGridTemplate); !ok || g.Number != 50 {
		t.Errorf("spectral grid template: %T", sp.Section3.Template)
	}
	if d, ok := sp.Section5.Template.(*RawDataRepTemplate); !ok || d.Number != 50 {
		t.Errorf("spectral data template: %T", sp.Section5.Template)
	}
}

// TestSecondFixedSurface: the second surface of a layer, and the missing
// (0xFF) scale factor, survive decode and encode (eccodes tests/grib_level.sh,
// grib_typeOfLevel.sh, grib_missing.sh).
func TestSecondFixedSurface(t *testing.T) {
	f := decodeAll(t, "testdata/upstream/layer_106.grib2")[0].Fields[0]
	tmpl := f.extractTemplate40()
	if tmpl.TypeOfFirstFixedSurface != 106 || tmpl.ScaleFactorOfFirstSurface != 2 || tmpl.ScaledValueOfFirstSurface != 13 ||
		tmpl.TypeOfSecondFixedSurface != 106 || tmpl.ScaleFactorOfSecondSurface != 2 || tmpl.ScaledValueOfSecondSurface != 54 {
		t.Fatalf("%+v", tmpl)
	}
	if lvl, _ := f.SurfaceLevel(); lvl != 0.13 {
		t.Errorf("SurfaceLevel = %v, want 0.13", lvl)
	}
	// MissingScaleFactor is written as 0xFF.
	var b bytes.Buffer
	t40 := Template40{ScaleFactorOfFirstSurface: MissingScaleFactor, ScaledValueOfFirstSurface: missingUint32,
		ScaleFactorOfSecondSurface: MissingScaleFactor, ScaledValueOfSecondSurface: missingUint32}
	if err := WriteSection4(&b, Section4{Template: t40}); err != nil {
		t.Fatal(err)
	}
	if o := b.Bytes(); o[23] != 0xFF || o[29] != 0xFF || binary.BigEndian.Uint32(o[30:34]) != missingUint32 {
		t.Errorf("octets 24/30 = %#x %#x", o[23], o[29])
	}
	// Negative zero (0x80) reads as 0 and is written back as 0x00; document that.
	if readSignMag8(0x80) != 0 || writeSignMag8(readSignMag8(0x80)) != 0 {
		t.Error("negative zero handling changed")
	}
}

// TestSignMagnitudeHelpers pins the 16- and 32-bit sign-magnitude readers
// and writers (scale factors, latitudes; eccodes tests/grib_decimalPrecision.sh).
func TestSignMagnitudeHelpers(t *testing.T) {
	for _, v := range []int16{0, 1, -1, 32767, -32767} {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], writeSignMag16(v))
		if got := readSignMag16(b[:], 0); got != v {
			t.Errorf("int16 %d round-trips to %d", v, got)
		}
	}
	for _, v := range []int32{0, 1, -1, 2147483647, -2147483647, -90000000} {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], writeSignMag32(v))
		if got := readSignMag32(b[:], 0); got != v {
			t.Errorf("int32 %d round-trips to %d", v, got)
		}
	}
	if got := readSignMag16([]byte{0x80, 0x01}, 0); got != -1 {
		t.Errorf("0x8001 = %d", got)
	}
	// Negative decimal scale factor through a whole message.
	f := decodeAll(t, "testdata/upstream/simple_dneg.grib2")[0].Fields[0]
	if tm := f.Section5.Template.(Template50); tm.DecimalScaleFactor != -1 {
		t.Errorf("D = %d, want -1", tm.DecimalScaleFactor)
	}
	// grib_set only changed D, so every value is ten times the original
	// (279 becomes 2790), as eccodes also decodes it.
	if v, err := f.Values(); err != nil || v[0] != 2790 {
		t.Errorf("values[0] = %v, %v", v[0], err)
	}
}

// TestReferenceTime_Components: the reference time keeps its seconds and
// rejects impossible dates instead of normalising them (eccodes
// tests/grib_check_message_validity.sh).
func TestReferenceTime_Components(t *testing.T) {
	s := Section1{Year: 2024, Month: 3, Day: 1, Hour: 2, Minute: 4, Second: 9}
	if got := s.ReferenceTime(); !got.Equal(time.Date(2024, 3, 1, 2, 4, 9, 0, time.UTC)) {
		t.Errorf("ReferenceTime = %v", got)
	}
	for _, bad := range []Section1{
		{Year: 2024, Month: 13, Day: 1},
		{Year: 2025, Month: 2, Day: 29},
		{Year: 2024, Month: 1, Day: 1, Hour: 24},
		{Year: 2024, Month: 1, Day: 1, Minute: 255},
	} {
		if err := bad.ValidateReferenceTime(); err == nil {
			t.Errorf("%+v: no error", bad)
		}
	}
	if err := s.ValidateReferenceTime(); err != nil {
		t.Error(err)
	}
}

// TestCodeTables_Complete: every code in the WMO tables eccodes ships has a
// name, and names have no stray whitespace (eccodes tests/grib2_templates.sh,
// grib_levtype.sh, codes_codetable.cc).
func TestCodeTables_Complete(t *testing.T) {
	// Copied from eccodes definitions/grib2/tables/35 (scripts/gen_codetables.py
	// reads the same tables).
	dir := "testdata/wmo_tables/35"
	codes := func(file string) []int {
		f, err := os.Open(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		var out []int
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fl := strings.Fields(sc.Text())
			if len(fl) < 3 || strings.HasPrefix(fl[0], "#") || strings.HasPrefix(strings.ToLower(fl[2]), "reserved") || fl[2] == "Missing" {
				continue
			}
			if c, err := strconv.Atoi(fl[0]); err == nil && c < 65535 {
				out = append(out, c)
			}
		}
		return out
	}
	check := func(table string, name func(c int) string) {
		for _, c := range codes(table) {
			n := name(c)
			if strings.HasPrefix(n, "Unknown") || strings.TrimSpace(n) != n || n == "" {
				t.Errorf("table %s code %d: %q", table, c, n)
			}
		}
	}
	check("0.0.table", func(c int) string { return DisciplineName(uint8(c)) })
	check("1.2.table", func(c int) string { return ReferenceTimeName(uint8(c)) })
	check("1.3.table", func(c int) string { return ProductionStatusName(uint8(c)) })
	check("1.4.table", func(c int) string { return DataTypeName(uint8(c)) })
	check("3.1.table", func(c int) string { return GridDefinitionTemplateName(uint16(c)) })
	check("4.0.table", func(c int) string { return ProductDefinitionTemplateName(uint16(c)) })
	check("4.5.table", func(c int) string { return SurfaceTypeName(uint8(c)) })
	check("5.0.table", func(c int) string { return DataRepresentationTemplateName(uint16(c)) })
	cats, _ := filepath.Glob(filepath.Join(dir, "4.1.*.table"))
	for _, path := range cats {
		parts := strings.Split(filepath.Base(path), ".")
		d, err := strconv.Atoi(parts[2])
		if err != nil || d > 255 {
			continue
		}
		check(filepath.Base(path), func(c int) string { return ParameterCategoryName(uint8(d), uint8(c)) })
	}
	params, _ := filepath.Glob(filepath.Join(dir, "4.2.*.*.table"))
	if len(params) < 50 {
		t.Fatalf("only %d parameter tables", len(params))
	}
	for _, path := range params {
		parts := strings.Split(filepath.Base(path), ".")
		d, err1 := strconv.Atoi(parts[2])
		c, err2 := strconv.Atoi(parts[3])
		if err1 != nil || err2 != nil || d > 255 || c > 255 {
			continue
		}
		check(filepath.Base(path), func(n int) string {
			if n > 255 {
				return "ok"
			}
			return ParameterName(uint8(d), uint8(c), uint8(n))
		})
	}
	if n := SurfaceTypeName(50); !strings.HasPrefix(n, "Unknown") {
		t.Errorf("SurfaceTypeName(50) = %q, a reserved code", n)
	}
}

// TestSectionStrings: String/Describe name the parameter in the right
// discipline for every parsed product template.
func TestSectionStrings(t *testing.T) {
	for _, file := range []string{"testdata/template48.grib2", "testdata/template411.grib2", "testdata/template412.grib2"} {
		m := decodeAll(t, file)[0]
		s := m.Fields[0].Section4.Describe(m.Section0.Discipline)
		if !strings.Contains(s, "Parameter:") || !strings.Contains(s, "Surface:") || strings.Contains(s, "Unknown parameter") {
			t.Errorf("%s: %s", file, s)
		}
	}
	m := decodeAll(t, "testdata/grid_complex_spatial_differencing.grib2")[0]
	if m.Section0.Discipline != 2 {
		t.Fatalf("discipline %d", m.Section0.Discipline)
	}
	if s := m.Fields[0].Section4.Describe(2); !strings.Contains(s, "oil") {
		t.Errorf("land-surface product described as %s", s)
	}
	if s := m.Fields[0].Section3.String(); !strings.Contains(s, "83441") {
		t.Errorf("Section3.String = %s", s)
	}
}
