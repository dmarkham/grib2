package j2k

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fullRangeSamples returns pseudo-random samples covering [0, 2^prec-1].
func fullRangeSamples(n, prec int) []int32 {
	out := make([]int32, n)
	mask := uint64(1)<<uint(prec) - 1
	for i := range out {
		out[i] = int32((uint64(i) * 2654435761) & mask)
	}
	return out
}

// smoothSamples returns a smooth field spanning most of [0, 2^prec-1].
func smoothSamples(w, h, prec int) []int32 {
	out := make([]int32, w*h)
	top := float64(uint64(1)<<uint(prec) - 1)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			f := 0.5 + 0.45*float64((x*7+y*3)%97)/97.0 - 0.2*float64(y)/float64(h)
			out[y*w+x] = int32(f * top)
		}
	}
	return out
}

// sotPsot returns the SOT position and its Psot value.
func sotPsot(t *testing.T, cs []byte) (int, int) {
	t.Helper()
	i := bytes.Index(cs, []byte{0xFF, 0x90, 0x00, 0x0A})
	if i < 0 {
		t.Fatal("no SOT marker")
	}
	return i, int(binary.BigEndian.Uint32(cs[i+6:]))
}

// TestEncodePsot checks that the tile-part length is written (audit H18:
// Psot=0 is rejected by Jasper, the decoder of wgrib2, g2clib and the
// default eccodes build).
func TestEncodePsot(t *testing.T) {
	for _, tc := range []struct{ w, h, prec int }{{4, 1, 8}, {496, 1, 12}, {6114, 1, 16}, {37, 29, 14}, {1, 1, 1}} {
		cs, err := Encode(smoothSamples(tc.w, tc.h, tc.prec), tc.w, tc.h, tc.prec)
		if err != nil {
			t.Fatalf("%v: Encode: %v", tc, err)
		}
		sot, psot := sotPsot(t, cs)
		if !bytes.HasSuffix(cs, []byte{0xFF, 0xD9}) {
			t.Fatalf("%v: no EOC", tc)
		}
		if want := len(cs) - 2 - sot; psot != want {
			t.Errorf("%v: Psot = %d, want tile-part length %d", tc, psot, want)
		}
	}
}

// TestEncodePrecisionSweep covers audit H19: at every precision the
// encoder must either produce a stream that decodes sample-exact or
// return an error. It must succeed up to 28 bits for any samples.
func TestEncodePrecisionSweep(t *testing.T) {
	const w, h = 37, 29
	for prec := 1; prec <= 31; prec++ {
		for _, kind := range []string{"random", "smooth"} {
			prec, kind := prec, kind
			t.Run(fmt.Sprintf("p%d_%s", prec, kind), func(t *testing.T) {
				samples := fullRangeSamples(w*h, prec)
				if kind == "smooth" {
					samples = smoothSamples(w, h, prec)
				}
				cs, err := Encode(samples, w, h, prec)
				if err != nil {
					if prec <= 28 {
						t.Fatalf("Encode: %v", err)
					}
					t.Logf("Encode refused: %v", err)
					return
				}
				got, gw, gh, err := Decode(cs)
				if err != nil {
					t.Fatalf("Decode: %v", err)
				}
				if gw != w || gh != h {
					t.Fatalf("dimensions %dx%d", gw, gh)
				}
				compareSamples(t, got, samples)
			})
		}
	}
}

func TestEncodeRejectsInvalidInput(t *testing.T) {
	s := []int32{0, 1, 2, 3}
	for _, prec := range []int{0, 32, 33, -1} {
		if _, err := Encode(s, 4, 1, prec); err == nil {
			t.Errorf("prec %d: want error", prec)
		}
	}
	if _, err := Encode([]int32{0, 256, 1, 2}, 4, 1, 8); err == nil {
		t.Error("sample 256 at prec 8: want error")
	}
	if _, err := Encode([]int32{0, -1, 1, 2}, 4, 1, 8); err == nil {
		t.Error("negative sample: want error")
	}
	if _, err := EncodeWithOptions(s, 4, 1, 8, 33); err == nil {
		t.Error("33 decomposition levels: want error")
	}
}

// readPGX reads an opj_decompress PGX file ("PG ML [+|-] prec w h").
func readPGX(t *testing.T, path string) ([]int32, int, int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	nl := bytes.IndexByte(data, '\n')
	hdr := strings.Fields(string(data[:nl]))
	if len(hdr) != 6 || hdr[0] != "PG" || hdr[1] != "ML" {
		t.Fatalf("unexpected PGX header %q", data[:nl])
	}
	prec, _ := strconv.Atoi(hdr[3])
	w, _ := strconv.Atoi(hdr[4])
	h, _ := strconv.Atoi(hdr[5])
	signed := hdr[2] == "-"
	_ = prec
	body := data[nl+1:]
	// opj_decompress uses 1, 2 or 4 bytes per sample, Jasper 1..4.
	if w*h == 0 || len(body)%(w*h) != 0 || len(body)/(w*h) > 4 {
		t.Fatalf("PGX body %d bytes for %dx%d", len(body), w, h)
	}
	bps := len(body) / (w * h)
	out := make([]int32, w*h)
	for i := range out {
		var v uint32
		for k := 0; k < bps; k++ {
			v = v<<8 | uint32(body[i*bps+k])
		}
		if signed && bps < 4 {
			sh := uint(32 - 8*bps)
			out[i] = int32(v<<sh) >> sh
		} else {
			out[i] = int32(v)
		}
	}
	return out, w, h
}

// externalDecode runs opj_decompress or jasper on a codestream and
// returns its samples.
func externalDecode(t *testing.T, tool string, cs []byte) ([]int32, int, int, error) {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.j2k")
	if err := os.WriteFile(in, cs, 0o644); err != nil {
		t.Fatal(err)
	}
	outBase := filepath.Join(dir, "out.pgx")
	var cmd *exec.Cmd
	if tool == "jasper" {
		cmd = exec.Command("jasper", "--input", in, "--input-format", "jpc",
			"--output", outBase, "--output-format", "pgx")
	} else {
		cmd = exec.Command("opj_decompress", "-i", in, "-o", outBase)
	}
	msg, err := cmd.CombinedOutput()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("%s: %v: %s", tool, err, msg)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "out*.pgx"))
	if len(matches) != 1 {
		return nil, 0, 0, errors.New(tool + " wrote no PGX file: " + string(msg))
	}
	s, w, h := readPGX(t, matches[0])
	return s, w, h, nil
}

// TestEncodeExternalDecoders decodes j2k.Encode output with OpenJPEG and
// Jasper (audit H18/H19). Each tool is skipped when not installed.
func TestEncodeExternalDecoders(t *testing.T) {
	for _, tool := range []string{"opj_decompress", "jasper"} {
		tool := tool
		t.Run(tool, func(t *testing.T) {
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s not installed", tool)
			}
			testExternalDecoder(t, tool)
		})
	}
}

type externalCase struct {
	w, h, prec int
	kind       string
}

func (c externalCase) name() string { return fmt.Sprintf("%dx%d_p%d_%s", c.w, c.h, c.prec, c.kind) }

func (c externalCase) samples() []int32 {
	if c.kind == "smooth" {
		return smoothSamples(c.w, c.h, c.prec)
	}
	return fullRangeSamples(c.w*c.h, c.prec)
}

var externalCases = []externalCase{
	{1, 1, 8, "smooth"}, {2, 1, 8, "smooth"}, {7, 1, 8, "smooth"}, {3, 2, 12, "smooth"},
	{496, 1, 12, "smooth"}, {6114, 1, 16, "random"}, {37, 29, 1, "random"},
	{37, 29, 16, "random"}, {37, 29, 24, "random"}, {37, 29, 28, "random"},
	{37, 29, 30, "smooth"}, {37, 29, 31, "smooth"},
}

// goldenStreamDir holds Encode's output for externalCases. When written
// (GRIB2_WRITE_J2K_GOLDEN=1), each stream is decoded by opj_decompress (and
// by the jasper CLI up to 16 bits) and must give back the samples exactly.
var goldenStreamDir = filepath.Join("testdata", "encoded")

// TestEncode_GoldenStreams: Encode still writes the streams OpenJPEG and
// Jasper were checked against, and our decoder reads them back exactly. It
// needs no external tools; TestEncodeExternalDecoders re-checks with them.
func TestEncode_GoldenStreams(t *testing.T) {
	write := os.Getenv("GRIB2_WRITE_J2K_GOLDEN") != ""
	if write {
		if err := os.MkdirAll(goldenStreamDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range externalCases {
		t.Run(tc.name(), func(t *testing.T) {
			samples := tc.samples()
			cs, err := Encode(samples, tc.w, tc.h, tc.prec)
			path := filepath.Join(goldenStreamDir, tc.name()+".j2k")
			if err != nil {
				if _, statErr := os.Stat(path); statErr == nil {
					t.Fatalf("Encode now refuses a case it used to encode: %v", err)
				}
				return // refused at write time too (prec > 28 with nasty data)
			}
			if write {
				for _, tool := range []string{"opj_decompress", "jasper"} {
					if tool == "jasper" && tc.prec > 16 {
						continue // the jasper CLI writes PGX up to 16 bits only
					}
					got, w, h, err := externalDecode(t, tool, cs)
					if err != nil || w != tc.w || h != tc.h {
						t.Fatalf("%s: %v (%dx%d)", tool, err, w, h)
					}
					compareSamples(t, got, samples)
				}
				if err := os.WriteFile(path, cs, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(cs, want) {
				t.Fatalf("Encode output changed (%d bytes, golden %d); regenerate with GRIB2_WRITE_J2K_GOLDEN=1", len(cs), len(want))
			}
			got, w, h, err := Decode(want)
			if err != nil || w != tc.w || h != tc.h {
				t.Fatalf("Decode: %v (%dx%d)", err, w, h)
			}
			compareSamples(t, got, samples)
		})
	}
}

func testExternalDecoder(t *testing.T, tool string) {
	for _, tc := range externalCases {
		tc := tc
		if tool == "jasper" && tc.prec > 16 {
			// The jasper CLI cannot write PGX above 16 bits; eccodes decodes
			// our deeper output with Jasper (golden files in the root package).
			continue
		}
		t.Run(tc.name(), func(t *testing.T) {
			samples := tc.samples()
			cs, err := Encode(samples, tc.w, tc.h, tc.prec)
			if err != nil {
				if tc.prec <= 28 {
					t.Fatalf("Encode: %v", err)
				}
				t.Skipf("Encode refused: %v", err)
			}
			got, w, h, err := externalDecode(t, tool, cs)
			if err != nil {
				t.Fatal(err)
			}
			if w != tc.w || h != tc.h {
				t.Fatalf("%s dimensions %dx%d", tool, w, h)
			}
			compareSamples(t, got, samples)
		})
	}
}
