package aec

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Golden vectors made by libaec's graec.
//
// Each case's input is generated deterministically by genSamples. The files
// in testdata/golden/<name>.rz were produced from that input with
//
//	AEC_WRITE_DAT=$DIR go test -run TestGoldenVectors ./internal/aec/
//	graec <graecArgs> $DIR/<name>.dat testdata/golden/<name>.rz
//
// using libaec built from reference/libaec. Cases with PadRSI need a libaec
// built with -DENABLE_RSI_PADDING (the stock build ignores AEC_PAD_RSI when
// encoding but honours it when decoding). Signed inputs are written as the
// low BitsPerSample bits of the two's complement value, which is what
// libaec's signed preprocessor expects.

type goldenCase struct {
	name  string
	bps   int
	bs    int
	rsi   int
	flags uint32
	gen   string
	n     int
}

var goldenCases = []goldenCase{
	// H21: a run of exactly five zero blocks must not be coded as ROS.
	{"pp_u8_bs16_zero5", 8, 16, 64, DataPreprocess, "zerorun5", 1000},
	{"pp_u16_bs32_zero5", 16, 32, 16, DataPreprocess, "zerorun5", 1000},
	{"pp_u8_bs8_zero64", 8, 8, 128, DataPreprocess, "zerorun64", 1100},
	{"nopp_u8_bs8_zero5", 8, 8, 16, 0, "zerorun5", 300},
	// L29: no preprocessor (no reference sample).
	{"nopp_u8_bs8_ramp", 8, 8, 16, 0, "ramp", 300},
	{"nopp_u12_bs16_random", 12, 16, 8, 0, "random", 700},
	{"nopp_u16_bs64_lowent", 16, 64, 4, 0, "lowent", 900},
	// L29: signed samples, with and without the preprocessor.
	{"pp_s5_bs16_walk", 5, 16, 8, DataPreprocess | DataSigned, "walk", 500},
	{"pp_s12_bs32_walk", 12, 32, 8, DataPreprocess | DataSigned, "walk", 900},
	{"pp_s16_bs16_walk", 16, 16, 16, DataPreprocess | DataSigned, "walk", 700},
	{"pp_s24_bs32_walk", 24, 32, 8, DataPreprocess | DataSigned, "walk", 600},
	{"pp_s32_bs16_extremes", 32, 16, 4, DataPreprocess | DataSigned, "extremes", 300},
	{"nopp_s12_bs16_walk", 12, 16, 8, DataSigned, "walk", 500},
	// L29: PadRSI.
	{"pad_pp_u14_bs32_random", 14, 32, 3, DataPreprocess | PadRSI, "random", 1000},
	{"pad_pp_u14_bs32_zero5", 14, 32, 8, DataPreprocess | PadRSI, "zerorun5", 1000},
	{"pad_nopp_u9_bs8_lowent", 9, 8, 5, PadRSI, "lowent", 500},
	{"pad_pp_s20_bs16_walk", 20, 16, 7, DataPreprocess | DataSigned | PadRSI, "walk", 600},
	// Restricted code options.
	{"restricted_pp_u2_bs16_lowent", 2, 16, 32, DataPreprocess | Restricted, "lowent", 1000},
	{"restricted_pp_u4_bs8_random", 4, 8, 16, DataPreprocess | Restricted, "random", 700},
	{"restricted_nopp_u3_bs32_lowent", 3, 32, 4, Restricted, "lowent", 500},
	// Boundary sample widths.
	{"pp_u1_bs8_random", 1, 8, 32, DataPreprocess, "random", 600},
	{"pp_u7_bs16_random", 7, 16, 8, DataPreprocess, "random", 500},
	{"pp_u9_bs16_walk", 9, 16, 8, DataPreprocess, "walk", 500},
	{"pp_u17_bs32_walk", 17, 32, 4, DataPreprocess, "walk", 500},
	{"pp_u25_bs16_random", 25, 16, 8, DataPreprocess, "random", 400},
	{"pp_u31_bs64_walk", 31, 64, 2, DataPreprocess, "walk", 400},
	{"pp_u32_bs16_extremes", 32, 16, 4, DataPreprocess, "extremes", 300},
	{"nopp_u32_bs8_random", 32, 8, 8, 0, "random", 200},
	// Long fundamental sequences ([0]*32 + [65000]*32, check_long_fs.c).
	{"pp_u16_bs16_longfs", 16, 16, 1, DataPreprocess, "longfs", 64},
	{"pp_u16_bs64_longfs", 16, 64, 1, DataPreprocess, "longfs", 64},
}

// graecArgs returns the graec command-line options for a case.
func (c goldenCase) graecArgs() string {
	s := fmt.Sprintf("-n%d -j%d -r%d", c.bps, c.bs, c.rsi)
	if c.flags&DataPreprocess == 0 {
		s += " -N"
	}
	if c.flags&DataSigned != 0 {
		s += " -s"
	}
	if c.flags&PadRSI != 0 {
		s += " -p"
	}
	if c.flags&Restricted != 0 {
		s += " -t"
	}
	return s
}

func (c goldenCase) opts() Options {
	return Options{BitsPerSample: c.bps, BlockSize: c.bs, RSI: c.rsi, Flags: c.flags}
}

// lcg is a small deterministic generator so the inputs never depend on
// math/rand's implementation.
type lcg uint64

func (l *lcg) next() uint32 {
	*l = *l*6364136223846793005 + 1442695040888963407
	return uint32(*l >> 32)
}

// genSamples builds n samples for the given generator, valid for bps bits
// (signed when signed is true).
func genSamples(gen string, n, bps int, signed bool, blockSize int) []int32 {
	r := lcg(uint64(n)*1000003 + uint64(bps)*7919 + uint64(len(gen)))
	umax := uint64(1)<<bps - 1
	lo := -(int64(1) << (bps - 1))
	hi := (int64(1) << (bps - 1)) - 1
	if !signed {
		lo, hi = 0, int64(umax)
	}
	clamp := func(v int64) int32 {
		if v < lo {
			v = lo
		}
		if v > hi {
			v = hi
		}
		return int32(v)
	}
	rnd := func() int64 {
		v := int64(uint64(r.next()) & umax)
		if signed {
			v += lo
		}
		return v
	}
	out := make([]int32, n)
	switch gen {
	case "ramp":
		for i := range out {
			out[i] = clamp(lo + int64(uint64(i+1)&umax))
		}
	case "random":
		for i := range out {
			out[i] = int32(rnd())
		}
	case "lowent":
		for i := range out {
			v := int64(r.next() % 4)
			if r.next()%7 == 0 {
				v = 0
			}
			out[i] = clamp(lo + v)
		}
	case "walk":
		v := (lo + hi) / 2
		step := int64(1) << (bps / 3)
		for i := range out {
			v += int64(r.next()%uint32(2*step+1)) - step
			out[i] = clamp(v)
			v = int64(out[i])
		}
	case "extremes":
		for i := range out {
			switch i % 4 {
			case 0:
				out[i] = clamp(lo)
			case 1:
				out[i] = clamp(hi)
			case 2:
				out[i] = clamp(hi - 1)
			default:
				out[i] = int32(rnd())
			}
		}
	case "zerorun5", "zerorun64":
		run := 5
		if gen == "zerorun64" {
			run = 64
		}
		for i := range out {
			out[i] = int32(rnd())
		}
		// Blocks 1..run of each 2*run+3 blocks hold the minimum sample, so
		// they are zero blocks with or without the preprocessor, and varying
		// data follows them.
		period := (2*run + 3) * blockSize
		for i := range out {
			p := i % period
			// The last sample of block 0 is zero too, so the preprocessed
			// differences of blocks 1..run are all zero as well.
			if p >= blockSize-1 && p < (run+1)*blockSize {
				out[i] = clamp(lo)
			}
		}
	case "longfs":
		for i := range out {
			if i >= 32 {
				out[i] = clamp(65000)
			}
		}
	default:
		panic("unknown generator " + gen)
	}
	return out
}

// libaecBytesPerSample is the input sample width libaec uses without
// AEC_DATA_3BYTE.
func libaecBytesPerSample(bps int) int {
	switch {
	case bps > 16:
		return 4
	case bps > 8:
		return 2
	default:
		return 1
	}
}

// toLibaecLSB serialises samples the way graec reads them (LSB first).
func toLibaecLSB(s []int32, bps int) []byte {
	nb := libaecBytesPerSample(bps)
	mask := uint32(uint64(1)<<bps - 1)
	out := make([]byte, 0, len(s)*nb)
	for _, v := range s {
		u := uint32(v) & mask
		for b := 0; b < nb; b++ {
			out = append(out, byte(u>>(8*b)))
		}
	}
	return out
}

func checkDecodedPrefix(t *testing.T, got, want []int32) {
	t.Helper()
	if len(got) < len(want) {
		t.Fatalf("decoded %d samples, want at least %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestGoldenVectors(t *testing.T) {
	writeDir := os.Getenv("AEC_WRITE_DAT")
	for _, c := range goldenCases {
		t.Run(c.name, func(t *testing.T) {
			in := genSamples(c.gen, c.n, c.bps, c.flags&DataSigned != 0, c.bs)
			if writeDir != "" {
				if err := os.WriteFile(filepath.Join(writeDir, c.name+".dat"), toLibaecLSB(in, c.bps), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(writeDir, c.name+".args"), []byte(c.graecArgs()+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(filepath.Join("testdata", "golden", c.name+".rz"))
			if err != nil {
				t.Fatalf("golden vector: %v", err)
			}

			// libaec's stream decodes to the input.
			dec, err := Decode(want, c.opts())
			if err != nil {
				t.Fatalf("Decode(libaec stream): %v", err)
			}
			checkDecodedPrefix(t, dec, in)

			// Our encoder is byte-identical to libaec.
			got, err := Encode(in, c.opts())
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if !bytes.Equal(got, want) {
				i := 0
				for i < len(got) && i < len(want) && got[i] == want[i] {
					i++
				}
				t.Fatalf("Encode differs from libaec (graec %s): len %d vs %d, first difference at byte %d", c.graecArgs(), len(got), len(want), i)
			}
		})
	}
}

// TestEncodeZeroRuns is the H21 table: a constant stretch of runLen blocks
// starting at block 1, followed by varying data.
func TestEncodeZeroRuns(t *testing.T) {
	runs := []int{1, 2, 3, 4, 5, 6, 7, 8, 63, 64, 65}
	for _, bs := range []int{8, 16, 32, 64} {
		for _, runLen := range runs {
			for _, pp := range []bool{true, false} {
				name := fmt.Sprintf("bs%d/run%d/pp=%v", bs, runLen, pp)
				t.Run(name, func(t *testing.T) {
					n := (runLen + 4) * bs
					r := lcg(uint64(bs*1000 + runLen))
					in := make([]int32, n)
					for i := range in {
						in[i] = int32(r.next() & 0xFFF)
					}
					for i := bs; i < (runLen+1)*bs; i++ {
						if pp {
							in[i] = in[bs-1]
						} else {
							in[i] = 0
						}
					}
					var flags uint32
					if pp {
						flags = DataPreprocess
					}
					opts := Options{BitsPerSample: 12, BlockSize: bs, RSI: 128, Flags: flags}
					enc, err := Encode(in, opts)
					if err != nil {
						t.Fatalf("Encode: %v", err)
					}
					dec, err := Decode(enc, opts)
					if err != nil {
						t.Fatalf("Decode: %v", err)
					}
					checkDecodedPrefix(t, dec, in)
				})
			}
		}
	}
}

// TestEncodeNoPreprocessSmall pins the L29 no-preprocessor bug: {1..8}
// decoded as [33 2 4 7 ...] because a spurious reference sample was written.
func TestEncodeNoPreprocessSmall(t *testing.T) {
	in := []int32{1, 2, 3, 4, 5, 6, 7, 8}
	opts := Options{BitsPerSample: 8, BlockSize: 8, RSI: 16}
	enc, err := Encode(in, opts)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, opts)
	if err != nil {
		t.Fatal(err)
	}
	checkDecodedPrefix(t, dec, in)
}

// TestEncodeSigned covers signed samples at widths below 32 bits, where a
// negative int32 used to reach the preprocessor without being masked.
func TestEncodeSigned(t *testing.T) {
	for _, bps := range []int{2, 5, 8, 9, 12, 16, 17, 24, 31, 32} {
		for _, pp := range []bool{true, false} {
			t.Run(fmt.Sprintf("bps%d/pp=%v", bps, pp), func(t *testing.T) {
				in := genSamples("walk", 777, bps, true, 16)
				ext := genSamples("extremes", 64, bps, true, 16)
				in = append(in, ext...)
				flags := DataSigned
				if pp {
					flags |= DataPreprocess
				}
				opts := Options{BitsPerSample: bps, BlockSize: 16, RSI: 8, Flags: flags}
				enc, err := Encode(in, opts)
				if err != nil {
					t.Fatalf("Encode: %v", err)
				}
				dec, err := Decode(enc, opts)
				if err != nil {
					t.Fatalf("Decode: %v", err)
				}
				checkDecodedPrefix(t, dec, in)
			})
		}
	}
}

// TestDecodeSignedNoPreprocessSignExtends pins the decoder convention:
// signed samples come back sign-extended with or without the preprocessor.
func TestDecodeSignedNoPreprocessSignExtends(t *testing.T) {
	in := []int32{-1, -2048, 2047, 0, 5, -5, 100, -100}
	opts := Options{BitsPerSample: 12, BlockSize: 8, RSI: 1, Flags: DataSigned}
	enc, err := Encode(in, opts)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, opts)
	if err != nil {
		t.Fatal(err)
	}
	checkDecodedPrefix(t, dec, in)
}

func TestEncodeConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{"bps0", Options{BitsPerSample: 0, BlockSize: 16, RSI: 8}},
		{"bps33", Options{BitsPerSample: 33, BlockSize: 16, RSI: 8}},
		{"bpsNegative", Options{BitsPerSample: -1, BlockSize: 16, RSI: 8}},
		{"restricted_bps5", Options{BitsPerSample: 5, BlockSize: 16, RSI: 8, Flags: Restricted}},
		{"bs0", Options{BitsPerSample: 8, BlockSize: 0, RSI: 8}},
		{"bsOdd", Options{BitsPerSample: 8, BlockSize: 15, RSI: 8}},
		{"bs12_not_standard", Options{BitsPerSample: 8, BlockSize: 12, RSI: 8}},
		{"bs128_not_standard", Options{BitsPerSample: 8, BlockSize: 128, RSI: 8}},
		{"bsOdd_notenforce", Options{BitsPerSample: 8, BlockSize: 7, RSI: 8, Flags: NotEnforce}},
		{"bs258_notenforce", Options{BitsPerSample: 8, BlockSize: 258, RSI: 8, Flags: NotEnforce}},
		{"rsi0", Options{BitsPerSample: 8, BlockSize: 16, RSI: 0}},
		{"rsi4097", Options{BitsPerSample: 8, BlockSize: 16, RSI: 4097}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Encode([]int32{1, 2, 3}, c.opts)
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("Encode err = %v, want ErrConfig", err)
			}
		})
	}

	// Accepted edge configurations.
	ok := []Options{
		{BitsPerSample: 4, BlockSize: 8, RSI: 1, Flags: Restricted},
		{BitsPerSample: 8, BlockSize: 64, RSI: 4096},
		{BitsPerSample: 8, BlockSize: 2, RSI: 3, Flags: NotEnforce | DataPreprocess},
		{BitsPerSample: 8, BlockSize: 256, RSI: 3, Flags: NotEnforce | DataPreprocess},
	}
	for _, o := range ok {
		in := genSamples("random", 1000, o.BitsPerSample, false, o.BlockSize)
		enc, err := Encode(in, o)
		if err != nil {
			t.Fatalf("Encode(%+v): %v", o, err)
		}
		dec, err := Decode(enc, o)
		if err != nil {
			t.Fatalf("Decode(%+v): %v", o, err)
		}
		checkDecodedPrefix(t, dec, in)
	}
}

func TestEncodeSampleRange(t *testing.T) {
	cases := []struct {
		name    string
		opts    Options
		samples []int32
		wantErr bool
	}{
		{"u8_max", Options{BitsPerSample: 8, BlockSize: 8, RSI: 1}, []int32{255}, false},
		{"u8_over", Options{BitsPerSample: 8, BlockSize: 8, RSI: 1}, []int32{256}, true},
		{"u8_negative", Options{BitsPerSample: 8, BlockSize: 8, RSI: 1}, []int32{-1}, true},
		{"u32_negative_is_bit_pattern", Options{BitsPerSample: 32, BlockSize: 8, RSI: 1}, []int32{-1}, false},
		{"s8_min", Options{BitsPerSample: 8, BlockSize: 8, RSI: 1, Flags: DataSigned}, []int32{-128, 127}, false},
		{"s8_under", Options{BitsPerSample: 8, BlockSize: 8, RSI: 1, Flags: DataSigned}, []int32{-129}, true},
		{"s8_over", Options{BitsPerSample: 8, BlockSize: 8, RSI: 1, Flags: DataSigned}, []int32{128}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Encode(c.samples, c.opts)
			if c.wantErr != (err != nil) {
				t.Fatalf("Encode err = %v, wantErr %v", err, c.wantErr)
			}
			if c.wantErr && !errors.Is(err, ErrData) {
				t.Fatalf("Encode err = %v, want ErrData", err)
			}
		})
	}
}

// TestEncodeMatrix is the check_rsi_block_access-style round-trip grid.
func TestEncodeMatrix(t *testing.T) {
	ns := []int{1, 255, 256, 2560, 67000}
	rsis := []int{1, 2, 255, 4096}
	bss := []int{8, 16, 32, 64}
	bpss := []int{1, 7, 8, 9, 15, 16, 17, 23, 24, 25, 31, 32}
	gens := []string{"zero", "random", "ramp", "zerorun5"}
	if testing.Short() {
		ns = []int{1, 255, 2560}
		rsis = []int{1, 255}
		bss = []int{8, 64}
		bpss = []int{1, 8, 17, 32}
	}
	for _, bps := range bpss {
		for _, gen := range gens {
			for _, n := range ns {
				for _, bs := range bss {
					var in []int32
					if gen == "zero" {
						in = make([]int32, n)
					} else {
						in = genSamples(gen, n, bps, false, bs)
					}
					for _, rsi := range rsis {
						for _, flags := range []uint32{DataPreprocess, 0} {
							opts := Options{BitsPerSample: bps, BlockSize: bs, RSI: rsi, Flags: flags}
							enc, err := Encode(in, opts)
							if err != nil {
								t.Fatalf("%s n=%d %+v: Encode: %v", gen, n, opts, err)
							}
							dec, err := Decode(enc, opts)
							if err != nil {
								t.Fatalf("%s n=%d %+v: Decode: %v", gen, n, opts, err)
							}
							if len(dec) < n {
								t.Fatalf("%s n=%d %+v: decoded %d samples", gen, n, opts, len(dec))
							}
							for i := range in {
								if dec[i] != in[i] {
									t.Fatalf("%s n=%d %+v: sample %d = %d, want %d", gen, n, opts, i, dec[i], in[i])
								}
							}
						}
					}
				}
			}
		}
	}
}

// TestDecodeAppendedZeroBytes checks that trailing zero bytes after a
// complete stream do not corrupt the decoded samples.
func TestDecodeAppendedZeroBytes(t *testing.T) {
	in := genSamples("random", 1000, 14, false, 32)
	opts := Options{BitsPerSample: 14, BlockSize: 32, RSI: 4, Flags: DataPreprocess}
	enc, err := Encode(in, opts)
	if err != nil {
		t.Fatal(err)
	}
	padded := append(append([]byte(nil), enc...), make([]byte, 64)...)
	dec, err := Decode(padded, opts)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	checkDecodedPrefix(t, dec, in)
}

// TestDecodeShortStreams exercises the slow (byte-at-a-time) decoder paths
// with streams of 5 to 12 bytes.
func TestDecodeShortStreams(t *testing.T) {
	for _, bps := range []int{8, 16, 32} {
		for _, n := range []int{1, 2, 3, 8, 9, 16} {
			for _, flags := range []uint32{DataPreprocess, 0} {
				in := genSamples("random", n, bps, false, 8)
				opts := Options{BitsPerSample: bps, BlockSize: 8, RSI: 1, Flags: flags}
				enc, err := Encode(in, opts)
				if err != nil {
					t.Fatal(err)
				}
				dec, err := Decode(enc, opts)
				if err != nil {
					t.Fatalf("bps %d n %d flags %d (%d bytes): %v", bps, n, flags, len(enc), err)
				}
				checkDecodedPrefix(t, dec, in)
			}
		}
	}
}
