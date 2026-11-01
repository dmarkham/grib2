package aec

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// libaecDataDir holds the CCSDS 121.0-B-2 test vectors shipped with libaec.
// reference/ is not committed, so the test skips when the files are absent.
var libaecDataDir = filepath.Join("..", "..", "reference", "libaec", "data", "121B2TestData")

// sampleVector is one row of libaec's tests/sampledata.sh.
type sampleVector struct {
	rz, dat string
	opts    Options
	// mode: "codec" requires a byte-identical encode, "cosdec" an encode of
	// the same size, "decode" only decodes.
	mode string
}

func libaecSampleVectors() []sampleVector {
	var v []sampleVector
	allo := filepath.Join(libaecDataDir, "AllOptions")
	lowe := filepath.Join(libaecDataDir, "LowEntropyOptions")
	extp := filepath.Join(libaecDataDir, "ExtendedParameters")
	pp := DataPreprocess

	for i := 1; i <= 4; i++ {
		dat := filepath.Join(allo, fmt.Sprintf("test_p256n%02d.dat", i))
		v = append(v,
			sampleVector{filepath.Join(allo, fmt.Sprintf("test_p256n%02d-basic.rz", i)), dat,
				Options{BitsPerSample: i, BlockSize: 16, RSI: 16, Flags: pp}, "codec"},
			sampleVector{filepath.Join(allo, fmt.Sprintf("test_p256n%02d-restricted.rz", i)), dat,
				Options{BitsPerSample: i, BlockSize: 16, RSI: 16, Flags: pp | Restricted}, "codec"},
		)
	}
	for i := 5; i <= 16; i++ {
		v = append(v, sampleVector{
			filepath.Join(allo, fmt.Sprintf("test_p256n%02d.rz", i)),
			filepath.Join(allo, fmt.Sprintf("test_p256n%02d.dat", i)),
			Options{BitsPerSample: i, BlockSize: 16, RSI: 16, Flags: pp}, "cosdec"})
	}
	for i := 17; i <= 24; i++ {
		v = append(v, sampleVector{
			filepath.Join(allo, fmt.Sprintf("test_p512n%02d.rz", i)),
			filepath.Join(allo, fmt.Sprintf("test_p512n%02d.dat", i)),
			Options{BitsPerSample: i, BlockSize: 16, RSI: 32, Flags: pp}, "cosdec"})
	}
	for set := 1; set <= 3; set++ {
		dat := filepath.Join(lowe, fmt.Sprintf("Lowset%d_8bit.dat", set))
		for j := 1; j <= 4; j++ {
			v = append(v,
				sampleVector{filepath.Join(lowe, fmt.Sprintf("Lowset%d_8bit.n%02d-basic.rz", set, j)), dat,
					Options{BitsPerSample: j, BlockSize: 16, RSI: 64, Flags: pp}, "codec"},
				sampleVector{filepath.Join(lowe, fmt.Sprintf("Lowset%d_8bit.n%02d-restricted.rz", set, j)), dat,
					Options{BitsPerSample: j, BlockSize: 16, RSI: 64, Flags: pp | Restricted}, "codec"},
			)
		}
		for j := 5; j <= 8; j++ {
			v = append(v, sampleVector{filepath.Join(lowe, fmt.Sprintf("Lowset%d_8bit.n%02d.rz", set, j)), dat,
				Options{BitsPerSample: j, BlockSize: 16, RSI: 64, Flags: pp}, "codec"})
		}
	}
	sar := filepath.Join(extp, "sar32bit.dat")
	v = append(v,
		sampleVector{filepath.Join(extp, "sar32bit.j16.r256.rz"), sar,
			Options{BitsPerSample: 32, BlockSize: 16, RSI: 256, Flags: pp | PadRSI}, "decode"},
		sampleVector{filepath.Join(extp, "sar32bit.j64.r4096.rz"), sar,
			Options{BitsPerSample: 32, BlockSize: 64, RSI: 4096, Flags: pp | PadRSI}, "decode"},
	)
	return v
}

// readLibaecLSB parses a little-endian sample file at libaec's input width.
func readLibaecLSB(b []byte, bps int) []int32 {
	nb := libaecBytesPerSample(bps)
	out := make([]int32, len(b)/nb)
	for i := range out {
		switch nb {
		case 1:
			out[i] = int32(b[i])
		case 2:
			out[i] = int32(binary.LittleEndian.Uint16(b[2*i:]))
		default:
			out[i] = int32(binary.LittleEndian.Uint32(b[4*i:]))
		}
	}
	return out
}

// TestLibaecSampleData ports libaec's tests/sampledata.sh: every 121B2 vector
// decodes to its .dat, and the "codec" rows encode byte-identically.
func TestLibaecSampleData(t *testing.T) {
	if _, err := os.Stat(libaecDataDir); err != nil {
		t.Skipf("libaec sample data not present: %v", err)
	}
	for _, sv := range libaecSampleVectors() {
		name := filepath.Base(sv.rz)
		if sv.opts.Flags&Restricted != 0 {
			name += "/restricted"
		}
		t.Run(name, func(t *testing.T) {
			rz, err := os.ReadFile(sv.rz)
			if err != nil {
				t.Fatal(err)
			}
			datRaw, err := os.ReadFile(sv.dat)
			if err != nil {
				t.Fatal(err)
			}
			// Values in the .dat are only meaningful in the low bps bits.
			in := readLibaecLSB(datRaw, sv.opts.BitsPerSample)
			if sv.opts.BitsPerSample < 32 {
				mask := int32(uint32(1)<<sv.opts.BitsPerSample - 1)
				for i := range in {
					in[i] &= mask
				}
			}

			dec, err := Decode(rz, sv.opts)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			checkDecodedPrefix(t, dec, in)

			if sv.mode == "decode" {
				return
			}
			enc, err := Encode(in, sv.opts)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			switch sv.mode {
			case "codec":
				if !bytes.Equal(enc, rz) {
					t.Fatalf("Encode: %d bytes, reference %d bytes, contents differ", len(enc), len(rz))
				}
			case "cosdec":
				if len(enc) != len(rz) {
					t.Fatalf("Encode: %d bytes, reference %d bytes", len(enc), len(rz))
				}
			}
		})
	}
}
