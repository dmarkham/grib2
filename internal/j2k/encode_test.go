package j2k

import (
	"fmt"
	"testing"
)

// TestEncodeDecodeRoundTrip verifies that Encode followed by Decode
// produces the original samples (lossless round-trip).
func TestEncodeDecodeRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		w, h, prec int
	}{
		{4, 4, 8},
		{8, 8, 8},
		{16, 16, 8},
		{16, 16, 12},
		{15, 17, 10},
		{64, 64, 16},
		{100, 50, 14},
		{1000, 1, 14},
		{65160, 1, 14},
	} {
		name := fmt.Sprintf("%dx%d_prec%d", tc.w, tc.h, tc.prec)
		t.Run(name, func(t *testing.T) {
			n := tc.w * tc.h
			samples := make([]int32, n)
			maxVal := int32(1)<<tc.prec - 1
			for i := range samples {
				samples[i] = int32(i*17+3) % (maxVal + 1)
			}

			encoded, err := Encode(samples, tc.w, tc.h, tc.prec)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			decoded, w, h, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if w != tc.w || h != tc.h {
				t.Fatalf("dimensions: got %dx%d, want %dx%d", w, h, tc.w, tc.h)
			}

			mismatches := 0
			for i := 0; i < n; i++ {
				if decoded[i] != samples[i] {
					if mismatches < 5 {
						t.Errorf("sample[%d] = %d, want %d", i, decoded[i], samples[i])
					}
					mismatches++
				}
			}
			if mismatches > 0 {
				t.Fatalf("total mismatches: %d out of %d", mismatches, n)
			}
			t.Logf("encoded %d samples into %d bytes (%.1f%% ratio)",
				n, len(encoded), 100*float64(len(encoded))/float64(n*2))
		})
	}
}

// TestEncodeDecodeConstant verifies encoding a constant-value image.
func TestEncodeDecodeConstant(t *testing.T) {
	const w, h, prec = 16, 16, 8
	n := w * h
	samples := make([]int32, n)
	for i := range samples {
		samples[i] = 42
	}

	encoded, err := Encode(samples, w, h, prec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, dw, dh, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if dw != w || dh != h {
		t.Fatalf("dimensions: got %dx%d, want %dx%d", dw, dh, w, h)
	}
	for i := 0; i < n; i++ {
		if decoded[i] != 42 {
			t.Fatalf("sample[%d] = %d, want 42", i, decoded[i])
		}
	}
}

// TestT1RoundTrip tests that T1 encode followed by T1 decode gives the
// original coefficients.
func TestT1RoundTrip(t *testing.T) {
	for _, tc := range []struct {
		w, h, orient, numbps int
	}{
		{1, 1, 0, 9},
		{2, 2, 0, 9},
		{4, 4, 0, 9},
		{4, 4, 1, 10},
		{4, 4, 2, 10},
		{4, 4, 3, 11},
		{8, 8, 0, 9},
	} {
		name := fmt.Sprintf("%dx%d_o%d_bps%d", tc.w, tc.h, tc.orient, tc.numbps)
		t.Run(name, func(t *testing.T) {
			n := tc.w * tc.h
			coeffs := make([]int32, n)
			for i := range coeffs {
				coeffs[i] = int32((i*31 + 7) % 200) - 100
			}

			// Encode
			enc := NewT1Encoder()
			result := enc.EncodeCodeblock(coeffs, uint32(tc.w), uint32(tc.h),
				uint32(tc.orient), uint32(tc.numbps))

			if len(result.Data) == 0 {
				for _, c := range coeffs {
					if c != 0 {
						t.Fatalf("encoder produced no data but coefficients are non-zero")
					}
				}
				return
			}

			// Decode
			dec := NewT1()
			segs := []T1Seg{{
				Data:          result.Data,
				Len:           uint32(len(result.Data)),
				RealNumPasses: uint32(result.NumPasses),
			}}

			cblkNumbps := result.NumBPS

			decoded, err := dec.DecodeCodeblock(
				uint32(tc.w), uint32(tc.h),
				uint32(tc.orient), 0,
				0,
				uint32(cblkNumbps),
				segs)
			if err != nil {
				t.Fatalf("DecodeCodeblock: %v", err)
			}

			for di := range decoded {
				decoded[di] /= 2
			}

			mismatches := 0
			for i := 0; i < n; i++ {
				if decoded[i] != coeffs[i] {
					if mismatches < 5 {
						t.Logf("coeff[%d] = %d, want %d", i, decoded[i], coeffs[i])
					}
					mismatches++
				}
			}
			if mismatches > 0 {
				t.Errorf("mismatches: %d out of %d", mismatches, n)
			}
		})
	}
}
