package grib2

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dmarkham/grib2/internal/j2k"
)

// ---------------------------------------------------------------------------
// helpers (jpeg-prefixed to keep clear of other test files)
// ---------------------------------------------------------------------------

// jpegLoadValues reads a one-value-per-line reference file ("nan" for
// missing), as written by testdata/jpeg_fixtures.sh.
func jpegLoadValues(t *testing.T, path string) []float64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []float64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s == "" {
			continue
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("%s: parse %q: %v", path, s, err)
		}
		out = append(out, v)
	}
	return out
}

func jpegReadField(t *testing.T, path string) *Field {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage %s: %v", path, err)
	}
	return &msg.Fields[0]
}

// jpegCompare requires got and want to agree position by position, with
// identical NaN masks, within tol*max(1,|want|).
func jpegCompare(t *testing.T, got, want []float64, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d values, want %d", len(got), len(want))
	}
	bad := 0
	for i := range want {
		g, w := got[i], want[i]
		if math.IsNaN(w) || math.IsNaN(g) {
			if math.IsNaN(w) != math.IsNaN(g) {
				if bad < 5 {
					t.Errorf("value[%d] = %v, want %v (missing mask differs)", i, g, w)
				}
				bad++
			}
			continue
		}
		if math.Abs(g-w) > tol*math.Max(1, math.Abs(w)) {
			if bad < 5 {
				t.Errorf("value[%d] = %.17g, want %.17g", i, g, w)
			}
			bad++
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d values differ", bad, len(want))
	}
}

// jpegTemplate540 returns the Template540 of a field, also for the
// legacy template number 5.40000 when it is still parsed as raw bytes.
func jpegTemplate540(t *testing.T, f *Field) Template540 {
	t.Helper()
	switch tm := f.Section5.Template.(type) {
	case Template540:
		return tm
	case *RawDataRepTemplate:
		if tm.Number != 40000 {
			t.Fatalf("raw template %d", tm.Number)
		}
		tmpl, err := readTemplate540(tm.Data)
		if err != nil {
			t.Fatalf("readTemplate540: %v", err)
		}
		return tmpl
	}
	t.Fatalf("not a JPEG2000 template: %T", f.Section5.Template)
	return Template540{}
}

func jpegEccodesBin(name string) string {
	return filepath.Join("reference", "eccodes-install", "bin", name)
}

// jpegGribGetData runs grib_get_data on every message of a file with the
// given JPEG library and returns one value slice per message (NaN for
// missing).
func jpegGribGetData(t *testing.T, path, lib string) [][]float64 {
	t.Helper()
	cmd := exec.Command(jpegEccodesBin("grib_get_data"), "-m", "nan", "-F", "%.17g", path)
	cmd.Env = append(eccodesEnv(), "ECCODES_GRIB_JPEG="+lib)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("grib_get_data (%s) %s: %v: %s", lib, path, err, stderr.String())
	}
	var msgs [][]float64
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Latitude") {
			msgs = append(msgs, nil)
			continue
		}
		fs := strings.Fields(line)
		if len(fs) < 3 || len(msgs) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fs[2], 64)
		if err != nil {
			t.Fatalf("parse %q: %v", fs[2], err)
		}
		msgs[len(msgs)-1] = append(msgs[len(msgs)-1], v)
	}
	return msgs
}

// ---------------------------------------------------------------------------
// Existing fixtures
// ---------------------------------------------------------------------------

func TestUnpackJPEG2000_File(t *testing.T) {
	field := jpegReadField(t, "testdata/jpeg.grib2")
	tmpl, ok := field.Section5.Template.(Template540)
	if !ok {
		t.Fatalf("not Template540: %T", field.Section5.Template)
	}
	if field.Section5.TemplateNumber != 40 {
		t.Errorf("TemplateNumber = %d, want 40", field.Section5.TemplateNumber)
	}
	if tmpl.BitsPerValue != 14 {
		t.Errorf("BitsPerValue = %d, want 14", tmpl.BitsPerValue)
	}
	if tmpl.TypeOfCompression != 0 {
		t.Errorf("TypeOfCompression = %d, want 0 (lossless)", tmpl.TypeOfCompression)
	}
	values, err := UnpackJPEG2000(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackJPEG2000: %v", err)
	}
	if len(values) != 65160 {
		t.Fatalf("got %d values, want 65160", len(values))
	}
	refValues := loadReferenceValues(t, "testdata/reference/jpeg.grib2.values")
	jpegCompare(t, values, refValues, 1e-2)
}

// TestUnpackJPEG2000_ReducedGaussianJPEG decodes a Jasper-style stream
// whose tile-part header carries a QCC (audit M26) and compares every value
// with eccodes.
func TestUnpackJPEG2000_ReducedGaussianJPEG(t *testing.T) {
	field := jpegReadField(t, "testdata/reduced_gaussian_surface_jpeg.grib2")
	tmpl := jpegTemplate540(t, field)
	values, err := UnpackJPEG2000(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
	if err != nil {
		t.Fatalf("UnpackJPEG2000: %v", err)
	}
	want := jpegLoadValues(t, "testdata/jpeg_reduced_gaussian_surface.values")
	jpegCompare(t, values, want, 1e-12)
}

// ---------------------------------------------------------------------------
// eccodes-made fixtures (testdata/jpeg_fixtures.sh)
// ---------------------------------------------------------------------------

// TestUnpackJPEG2000_EccodesFixtures decodes streams written by eccodes
// with OpenJPEG and Jasper and compares Field.Values() with grib_get_data
// position by position, NaN mask included (audit M26, M9, L9, H3).
func TestUnpackJPEG2000_EccodesFixtures(t *testing.T) {
	for _, name := range []string{
		"jpeg_bpv1_openjpeg", "jpeg_bpv1_jasper",
		"jpeg_bpv24_openjpeg", "jpeg_bpv24_jasper",
		"jpeg_rls_openjpeg", "jpeg_rls_jasper", // Jasper: tile-part QCC, 6 resolutions on 16x31
		"jpeg_ecc802",                     // L9: 2x2, negative minimum
		"jpeg_const_d3", "jpeg_const_dm2", // H3: bpv 0 with D=3 and D=-2
		"jpeg_bitmap", "jpeg_bitmap_const", // M9: bitmap, bitmap + bpv 0
		"jpeg_bm6x2",      // M9: bitmap ending in pad bits
		"jpeg_allmissing", // M9: every point missing, empty S7
	} {
		t.Run(name, func(t *testing.T) {
			f := jpegReadField(t, filepath.Join("testdata", name+".grib2"))
			got, err := f.Values()
			if err != nil {
				t.Fatalf("Values: %v", err)
			}
			want := jpegLoadValues(t, filepath.Join("testdata", name+".values"))
			jpegCompare(t, got, want, 1e-12)
		})
	}
}

// TestUnpackJPEG2000_OpenJPEGExponentOverflow: at bitsPerValue 30 and 31
// eccodes+OpenJPEG writes QCD exponents of 32 and 33 into the 5-bit field,
// which wrap to 0 and 1. The code-blocks then claim more zero bit-planes
// than their band has. OpenJPEG silently zeroes those code-blocks
// (grib_get_data is off by up to 33 from the source field on these
// fixtures) and Jasper reports a corrupt stream; we return an error.
func TestUnpackJPEG2000_OpenJPEGExponentOverflow(t *testing.T) {
	for _, name := range []string{"jpeg_bpv30_openjpeg", "jpeg_bpv31_openjpeg"} {
		f := jpegReadField(t, filepath.Join("testdata", name+".grib2"))
		if _, err := f.Values(); err == nil || !strings.Contains(err.Error(), "zero bit-plane") {
			t.Errorf("%s: err = %v, want a zero bit-plane error", name, err)
		}
	}
}

// TestUnpackJPEG2000_ConstantIgnoresDecimalScale covers audit H3: with
// bitsPerValue 0 every value is R, not R*10^-D (eccodes
// DataJpeg2000Packing.cc, g2clib).
func TestUnpackJPEG2000_ConstantIgnoresDecimalScale(t *testing.T) {
	for _, tc := range []struct {
		r float32
		d int16
	}{{273, 3}, {273, -2}, {5.5, 2}, {42.5, 0}, {-1.25, 1}} {
		tmpl := Template540{Template50: Template50{ReferenceValue: tc.r, DecimalScaleFactor: tc.d, BinaryScaleFactor: 3}}
		got, err := UnpackJPEG2000(nil, tmpl, 7)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		for i, v := range got {
			if v != float64(tc.r) {
				t.Fatalf("R=%v D=%d: value[%d] = %v, want %v", tc.r, tc.d, i, v, tc.r)
			}
		}
	}
	// bpv > 0 with no codestream is an error, not a constant field.
	if _, err := UnpackJPEG2000(nil, Template540{Template50: Template50{BitsPerValue: 12}}, 3); err == nil {
		t.Error("bpv 12 with empty data: want error")
	}
}

// TestUnpackJPEG2000_LossyOpenJPEG covers audit H20: an OpenJPEG lossy
// (rate-truncated 5/3) stream reconstructs one sample below zero, which
// must be clamped to 0 as OpenJPEG does, so no value falls below R.
func TestUnpackJPEG2000_LossyOpenJPEG(t *testing.T) {
	f := jpegReadField(t, "testdata/jpeg_lossy_openjpeg.grib2")
	tmpl := jpegTemplate540(t, f)
	if tmpl.TypeOfCompression != 1 {
		t.Fatalf("TypeOfCompression = %d, want 1 (lossy)", tmpl.TypeOfCompression)
	}
	samples, w, h, err := j2k.Decode(f.Section7.Data)
	if err != nil {
		t.Fatalf("j2k.Decode: %v", err)
	}
	if w*h != 65160 {
		t.Fatalf("decoded %dx%d", w, h)
	}
	maxS := int32(1)<<tmpl.BitsPerValue - 1
	for i, s := range samples {
		if s < 0 || s > maxS {
			t.Fatalf("sample[%d] = %d outside [0,%d]", i, s, maxS)
		}
	}
	if samples[53548] != 0 {
		t.Errorf("sample[53548] = %d, want 0 (clamped)", samples[53548])
	}
	var buf bytes.Buffer
	for _, s := range samples {
		binary.Write(&buf, binary.LittleEndian, s) //nolint:errcheck
	}
	sum := sha256.Sum256(buf.Bytes())
	wantSum, err := os.ReadFile("testdata/jpeg_lossy_openjpeg.sha256")
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(sum[:]); got != strings.TrimSpace(string(wantSum)) {
		t.Fatalf("sample SHA-256 %s, want %s (opj_decompress output)", got, wantSum)
	}

	values, err := f.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	for i, v := range values {
		if v < float64(tmpl.ReferenceValue) {
			t.Fatalf("value[%d] = %v below the reference value %v", i, v, tmpl.ReferenceValue)
		}
	}
	if eccodesAvailable(t) {
		want := jpegGribGetData(t, "testdata/jpeg_lossy_openjpeg.grib2", "openjpeg")
		jpegCompare(t, values, want[0], 1e-12)
	}
}

// TestUnpackJPEG2000_LossyJasperRejected: Jasper's lossy mode writes an
// irreversible 9/7 stream, which the decoder rejects explicitly (H20).
func TestUnpackJPEG2000_LossyJasperRejected(t *testing.T) {
	f := jpegReadField(t, "testdata/jpeg_lossy_jasper.grib2")
	_, err := f.Values()
	if err == nil || !strings.Contains(err.Error(), "qmfbid") {
		t.Fatalf("err = %v, want an error naming qmfbid", err)
	}
	if !errors.Is(err, j2k.ErrUnsupported) {
		t.Errorf("err = %v, want it to wrap j2k.ErrUnsupported", err)
	}
}

// TestUnpackJPEG2000_LegacyTemplate40000 decodes the 26 messages of
// testdata/v.grib2, which use the pre-standard template number 5.40000
// (audit H16), and compares them with eccodes when it is available.
func TestUnpackJPEG2000_LegacyTemplate40000(t *testing.T) {
	raw, err := os.ReadFile("testdata/v.grib2")
	// v.grib2 is gitignored; testdata/upstream/multi_first3.grib2 (also
	// 5.40000) keeps this covered through TestEccodesReference_Check.
	skipIfGitignoredFixture(t, "testdata/v.grib2", err)
	if err != nil {
		t.Fatal(err)
	}
	dec := NewDecoder(bytes.NewReader(raw))
	var all [][]float64
	for {
		msg, err := dec.Decode()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("message %d: %v", len(all)+1, err)
		}
		f := &msg.Fields[0]
		if f.Section5.TemplateNumber != 40000 {
			t.Fatalf("message %d: template %d, want 40000", len(all)+1, f.Section5.TemplateNumber)
		}
		tmpl := jpegTemplate540(t, f)
		if len(all) == 1 && (tmpl.BitsPerValue != 10 || tmpl.BinaryScaleFactor != 0 || tmpl.DecimalScaleFactor != 1) {
			t.Errorf("message 2: bpv=%d E=%d D=%d, want 10 0 1", tmpl.BitsPerValue, tmpl.BinaryScaleFactor, tmpl.DecimalScaleFactor)
		}
		vals, err := UnpackJPEG2000(f.Section7.Data, tmpl, f.Section5.NumberOfValues)
		if err != nil {
			t.Fatalf("message %d: UnpackJPEG2000: %v", len(all)+1, err)
		}
		if len(vals) != 259920 {
			t.Fatalf("message %d: %d values", len(all)+1, len(vals))
		}
		all = append(all, vals)
	}
	if len(all) != 26 {
		t.Fatalf("decoded %d messages, want 26", len(all))
	}
	if !eccodesAvailable(t) || testing.Short() {
		return
	}
	want := jpegGribGetData(t, "testdata/v.grib2", "openjpeg")
	if len(want) != 26 {
		t.Fatalf("grib_get_data returned %d messages", len(want))
	}
	for i := range all {
		jpegCompare(t, all[i], want[i], 1e-9)
	}
}

// TestUnpackJPEG2000_RDPSPositional compares the RDPS JPEG fields with
// eccodes position by position (the cross-check in eccodes_crossval_test.go
// compares sorted values, which a pixel permutation would pass; M26).
func TestUnpackJPEG2000_RDPSPositional(t *testing.T) {
	if !eccodesAvailable(t) || testing.Short() {
		t.Skip("needs eccodes; skipped in -short")
	}
	for _, name := range []string{"rdps_surface", "rdps_isobaric_850"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name+".grib2")
			f := jpegReadField(t, path)
			// Stored (scan) order, as grib_get_data prints it.
			got, err := UnpackJPEG2000(f.Section7.Data, jpegTemplate540(t, f), f.Section5.NumberOfValues)
			if err != nil {
				t.Fatalf("UnpackJPEG2000: %v", err)
			}
			want := jpegGribGetData(t, path, "openjpeg")
			jpegCompare(t, got, want[0], 1e-12)
		})
	}
}

// ---------------------------------------------------------------------------
// Encoder
// ---------------------------------------------------------------------------

func TestPackJPEG2000(t *testing.T) {
	t.Run("RoundTrip", func(t *testing.T) {
		n := 1000
		values := make([]float64, n)
		for i := range values {
			values[i] = float64(i*17+3) * 0.1
		}
		packed, tmpl, err := PackJPEG2000(values, 14)
		if err != nil {
			t.Fatalf("PackJPEG2000: %v", err)
		}
		unpacked, err := UnpackJPEG2000(packed, tmpl, uint32(n))
		if err != nil {
			t.Fatalf("UnpackJPEG2000: %v", err)
		}
		jpegCheckHalfStep(t, values, unpacked, tmpl)
	})

	t.Run("Constant", func(t *testing.T) {
		values := make([]float64, 100)
		for i := range values {
			values[i] = 42.5
		}
		packed, tmpl, err := PackJPEG2000(values, 0)
		if err != nil {
			t.Fatalf("PackJPEG2000: %v", err)
		}
		unpacked, err := UnpackJPEG2000(packed, tmpl, 100)
		if err != nil {
			t.Fatalf("UnpackJPEG2000: %v", err)
		}
		for i, v := range unpacked {
			if v != 42.5 {
				t.Fatalf("value[%d] = %v, want 42.5", i, v)
			}
		}
	})

	// Re-encode the decoded jpeg.grib2 field and require a round trip
	// within half a quantization step.
	t.Run("FileRoundTrip", func(t *testing.T) {
		field := jpegReadField(t, "testdata/jpeg.grib2")
		tmpl := jpegTemplate540(t, field)
		origValues, err := UnpackJPEG2000(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
		if err != nil {
			t.Fatalf("UnpackJPEG2000 original: %v", err)
		}
		packed, newTmpl, err := PackJPEG2000(origValues, tmpl.BitsPerValue)
		if err != nil {
			t.Fatalf("PackJPEG2000: %v", err)
		}
		unpacked, err := UnpackJPEG2000(packed, newTmpl, uint32(len(origValues)))
		if err != nil {
			t.Fatalf("UnpackJPEG2000 re-packed: %v", err)
		}
		jpegCheckHalfStep(t, origValues, unpacked, newTmpl)
	})
}

// jpegCheckHalfStep checks R <= min and |dec-orig| <= 2^E/2 (plus float64
// rounding of the reconstruction).
func jpegCheckHalfStep(t *testing.T, orig, dec []float64, tmpl Template540) {
	t.Helper()
	if len(dec) != len(orig) {
		t.Fatalf("got %d values, want %d", len(dec), len(orig))
	}
	vmin := math.Inf(1)
	for _, v := range orig {
		vmin = math.Min(vmin, v)
	}
	if float64(tmpl.ReferenceValue) > vmin {
		t.Errorf("reference value %.17g exceeds the minimum %.17g", tmpl.ReferenceValue, vmin)
	}
	half := 0.5 * math.Ldexp(1, int(tmpl.BinaryScaleFactor))
	worst, at := 0.0, 0
	for i := range orig {
		if d := math.Abs(dec[i] - orig[i]); d > worst {
			worst, at = d, i
		}
	}
	slack := 4 * math.Abs(orig[at]) * 0x1p-52
	if worst > half+slack {
		t.Errorf("bpv %d: max error %.6g at %d (orig %.17g, dec %.17g) exceeds half a step %.6g",
			tmpl.BitsPerValue, worst, at, orig[at], dec[at], half)
	}
}

// jpegRoundTrip packs and unpacks; an encode error is returned as is.
func jpegRoundTrip(t *testing.T, values []float64, bpv uint8) (Template540, []float64, error) {
	t.Helper()
	packed, tmpl, err := PackJPEG2000(values, bpv)
	if err != nil {
		return tmpl, nil, err
	}
	dec, err := UnpackJPEG2000(packed, tmpl, uint32(len(values)))
	if err != nil {
		t.Fatalf("bpv %d: UnpackJPEG2000: %v", bpv, err)
	}
	return tmpl, dec, nil
}

// isJ2KCapacityError reports the documented refusal for samples that need
// more than 30 magnitude bit-planes (possible only at bpv >= 29).
func isJ2KCapacityError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "magnitude bit-planes")
}

// TestPackJPEG2000_ReferenceNotAboveMinimum covers audit H2: float32(min)
// rounds up for about half of all minima; the reference must be stepped
// down so that no packed integer goes negative.
func TestPackJPEG2000_ReferenceNotAboveMinimum(t *testing.T) {
	sets := map[string][]float64{}
	for _, m := range []float64{270.1, 0.1, 1.1, 7.85, -0.3, 100.3} {
		v := make([]float64, 300)
		for i := range v {
			v[i] = m + float64(i%37)*0.731 + float64(i)/1000
		}
		sets[fmt.Sprintf("min%v", m)] = v
	}
	sets["280.00002"] = []float64{280.00002, 280.0004, 280.0011, 280.00002, 280.0007}
	sets["100.3_ramp"] = func() []float64 {
		v := make([]float64, 500)
		for i := range v {
			v[i] = 100.3 + float64(i)/1000
		}
		return v
	}()
	for name, vals := range sets {
		for bpv := 1; bpv <= 30; bpv++ {
			tmpl, dec, err := jpegRoundTrip(t, vals, uint8(bpv))
			if err != nil {
				if bpv >= 29 && isJ2KCapacityError(err) {
					continue
				}
				t.Fatalf("%s bpv %d: %v", name, bpv, err)
			}
			jpegCheckHalfStep(t, vals, dec, tmpl)
		}
	}

	// A single value that float32 cannot represent: constant, so R is the
	// nearest float32 and the error is at most half a float32 ulp.
	tmpl, dec, err := jpegRoundTrip(t, []float64{7000000300}, 24)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.BitsPerValue != 0 || math.Abs(dec[0]-7000000300) > 256 {
		t.Errorf("single value: bpv %d, decoded %v", tmpl.BitsPerValue, dec[0])
	}
}

// TestPackJPEG2000_BitsPerValueSweep covers audit H19: every bpv from 1
// to 30 round-trips within half a step or returns an error; above 30 is an
// error (31-bit streams are mis-decoded by Jasper).
func TestPackJPEG2000_BitsPerValueSweep(t *testing.T) {
	n := 1500
	smooth := make([]float64, n)
	noisy := make([]float64, n)
	for i := range smooth {
		smooth[i] = 200 + 100*math.Sin(float64(i)/40)
		noisy[i] = 100 + 200*float64((uint64(i)*2654435761)%1000003)/1000003
	}
	for name, vals := range map[string][]float64{"smooth": smooth, "noisy": noisy} {
		for bpv := 1; bpv <= 30; bpv++ {
			tmpl, dec, err := jpegRoundTrip(t, vals, uint8(bpv))
			if err != nil {
				if bpv >= 29 && isJ2KCapacityError(err) {
					t.Logf("%s bpv %d refused: %v", name, bpv, err)
					continue
				}
				t.Fatalf("%s bpv %d: %v", name, bpv, err)
			}
			if int(tmpl.BitsPerValue) != bpv {
				t.Fatalf("%s: BitsPerValue %d, want %d", name, tmpl.BitsPerValue, bpv)
			}
			jpegCheckHalfStep(t, vals, dec, tmpl)
		}
	}
	for _, bpv := range []uint8{31, 32, 33, 64, 255} {
		if _, _, err := PackJPEG2000(smooth, bpv); err == nil {
			t.Errorf("bpv %d: want error", bpv)
		}
	}
}

// TestPackJPEG2000_RejectsInvalidInput covers audit M4.
func TestPackJPEG2000_RejectsInvalidInput(t *testing.T) {
	for name, vals := range map[string][]float64{
		"+Inf":        {1, 2, math.Inf(1), 4},
		"-Inf":        {1, math.Inf(-1), 3},
		"leading NaN": {math.NaN(), 1, 2},
		"middle NaN":  {1, math.NaN(), 2},
		"1e39":        {1, 1e39},
		"-1e39":       {-1e39, 1},
		"all NaN":     {math.NaN(), math.NaN()},
	} {
		if _, _, err := PackJPEG2000(vals, 12); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, _, err := PackJPEG2000([]float64{1, 2, 3}, 0); err == nil {
		t.Error("bpv 0 on varying data: want error")
	}
	// The float32 extremes themselves are valid.
	vals := []float64{-math.MaxFloat32, 0, math.MaxFloat32}
	tmpl, dec, err := jpegRoundTrip(t, vals, 24)
	if err != nil {
		t.Fatalf("float32 extremes: %v", err)
	}
	jpegCheckHalfStep(t, vals, dec, tmpl)
}

// TestPackJPEG2000_ConstantCollapses covers audit L8: constant input is
// written with bpv 0 and no data whatever bpv was asked for (as eccodes).
func TestPackJPEG2000_ConstantCollapses(t *testing.T) {
	for _, c := range []float64{42.5, 0.1, -273.15, 0} {
		vals := []float64{c, c, c, c, c, c}
		packed, tmpl, err := PackJPEG2000(vals, 14)
		if err != nil {
			t.Fatalf("%v: %v", c, err)
		}
		if tmpl.BitsPerValue != 0 || len(packed) != 0 {
			t.Fatalf("%v: bpv %d, %d bytes; want bpv 0 and no data", c, tmpl.BitsPerValue, len(packed))
		}
		if tmpl.ReferenceValue != float32(c) {
			t.Errorf("%v: R = %v, want float32(c)", c, tmpl.ReferenceValue)
		}
		dec, err := UnpackJPEG2000(packed, tmpl, uint32(len(vals)))
		if err != nil {
			t.Fatal(err)
		}
		for i := range dec {
			if dec[i] != float64(float32(c)) {
				t.Fatalf("%v: value[%d] = %v", c, i, dec[i])
			}
		}
	}
}

// TestPackJPEG2000_TinyGrids covers audit L9 (ECC-802): tiny fields with a
// negative minimum, at several bit depths.
func TestPackJPEG2000_TinyGrids(t *testing.T) {
	cases := [][]float64{
		{-0.01, 11.99, 56.11, 98.99},
		{-5.5},
		{-0.01, 98.99},
		{-3.3, 0.2, 7.7},
		{-1, -0.5, 0, 0.5, 1, 1.5, -2.25},
	}
	for _, vals := range cases {
		for _, bpv := range []uint8{1, 8, 12, 16, 24} {
			tmpl, dec, err := jpegRoundTrip(t, vals, bpv)
			if err != nil {
				t.Fatalf("%v bpv %d: %v", vals, bpv, err)
			}
			if len(vals) == 1 {
				if dec[0] != float64(float32(vals[0])) {
					t.Errorf("%v: decoded %v", vals, dec[0])
				}
				continue
			}
			jpegCheckHalfStep(t, vals, dec, tmpl)
		}
	}
}

// jpegBuildMessage returns regular_latlon_surface.grib2 with its data
// replaced by a Template 5.40 field.
func jpegBuildMessage(t *testing.T, values []float64, bpv uint8) ([]byte, Template540, []byte) {
	t.Helper()
	raw, err := os.ReadFile("testdata/regular_latlon_surface.grib2")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	packed, tmpl, err := PackJPEG2000(values, bpv)
	if err != nil {
		t.Fatalf("PackJPEG2000 bpv %d: %v", bpv, err)
	}
	f := &msg.Fields[0]
	f.Section5 = Section5{NumberOfValues: uint32(len(values)), TemplateNumber: 40, Template: tmpl}
	f.Section7 = Section7{Data: packed}
	var buf bytes.Buffer
	if err := WriteMessage(&buf, msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	return buf.Bytes(), tmpl, packed
}

// TestPackJPEG2000_EccodesDecodes covers audit H18: PackJPEG2000 output is
// decoded by eccodes with both Jasper (the decoder of wgrib2, g2clib and
// the default eccodes build, which rejected Psot=0) and OpenJPEG.
func TestPackJPEG2000_EccodesDecodes(t *testing.T) {
	if !eccodesAvailable(t) {
		t.Skip("eccodes not available")
	}
	base := jpegReadField(t, "testdata/regular_latlon_surface.grib2")
	orig, err := base.Values()
	if err != nil {
		t.Fatal(err)
	}
	shifted := make([]float64, len(orig)) // minimum not representable in float32
	for i, v := range orig {
		shifted[i] = v + 0.1
	}
	constant := make([]float64, len(orig))
	for i := range constant {
		constant[i] = 273.15
	}
	type tc struct {
		name string
		vals []float64
		bpv  uint8
		libs []string
	}
	both := []string{"jasper", "openjpeg"}
	cases := []tc{
		{"orig_b8", orig, 8, both},
		{"orig_b14", orig, 14, both},
		{"shifted_b16", shifted, 16, both},
		{"shifted_b24", shifted, 24, both},
		{"shifted_b28", shifted, 28, both},
		{"shifted_b29", shifted, 29, both},
		{"shifted_b30", shifted, 30, both},
		{"constant", constant, 12, both},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			data, tmpl, packed := jpegBuildMessage(t, c.vals, c.bpv)
			path := filepath.Join(t.TempDir(), c.name+".grib2")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			ours, err := UnpackJPEG2000(packed, tmpl, uint32(len(c.vals)))
			if err != nil {
				t.Fatal(err)
			}
			for _, lib := range c.libs {
				got := jpegGribGetData(t, path, lib)
				if len(got) != 1 || len(got[0]) != len(c.vals) {
					t.Fatalf("%s: grib_get_data returned %d messages", lib, len(got))
				}
				jpegCompare(t, got[0], ours, 1e-12)
				jpegCheckHalfStep(t, c.vals, got[0], tmpl)
			}
			cmd := exec.Command(jpegEccodesBin("grib_get"), "-p", "numberOfValues,section7Length,bitsPerValue", path)
			cmd.Env = eccodesEnv()
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("grib_get: %v", err)
			}
			want := fmt.Sprintf("%d %d %d", len(c.vals), 5+len(packed), tmpl.BitsPerValue)
			if got := strings.TrimSpace(string(out)); got != want {
				t.Errorf("grib_get = %q, want %q", got, want)
			}
		})
	}
}
