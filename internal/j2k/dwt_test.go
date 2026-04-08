package j2k

import (
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// Reference forward 5/3 DWT (encoder side) used to generate test vectors.
// The inverse must exactly undo the forward transform for lossless coding.
// ---------------------------------------------------------------------------

// fwd53_1d performs the forward 5/3 wavelet on a 1-D signal of length n
// stored in a[0:n]. On exit, the even-indexed samples (low-pass) are in
// a[0], a[2], a[4], ... and the odd-indexed (high-pass) in a[1], a[3], ...
// These are then de-interleaved into a so that a[0:sn] holds low-pass and
// a[sn:n] holds high-pass, matching the JPEG 2000 subband layout.
//
// Forward lifting (cas == 0):
//
//	D(i) -= (S_(i) + S_(i+1)) >> 1        (update / high-pass)
//	S(i) += (D_(i-1) + D_(i) + 2) >> 2    (predict / low-pass)
//
// where S(i) = a[2*i], D(i) = a[2*i+1], and S_ / D_ apply mirror boundary
// extension.
func fwd53_1d(a []int32, n int) {
	if n < 2 {
		return
	}
	sn := (n + 1) / 2 // number of low-pass (even) samples
	dn := n / 2        // number of high-pass (odd) samples

	// Convenience accessors with boundary mirroring.
	S := func(i int) int32 { return a[i*2] }
	D := func(i int) int32 { return a[1+i*2] }
	setS := func(i int, v int32) { a[i*2] = v }
	setD := func(i int, v int32) { a[1+i*2] = v }
	Sc := func(i int) int32 {
		if i < 0 {
			return S(0)
		}
		if i >= sn {
			return S(sn - 1)
		}
		return S(i)
	}
	Dc := func(i int) int32 {
		if i < 0 {
			return D(0)
		}
		if i >= dn {
			return D(dn - 1)
		}
		return D(i)
	}

	// High-pass update.
	for i := 0; i < dn; i++ {
		setD(i, D(i)-((Sc(i)+Sc(i+1))>>1))
	}
	// Low-pass predict.
	for i := 0; i < sn; i++ {
		setS(i, S(i)+((Dc(i-1)+Dc(i)+2)>>2))
	}

	// De-interleave: put even samples first, then odd.
	tmp := make([]int32, n)
	for i := 0; i < sn; i++ {
		tmp[i] = a[i*2]
	}
	for i := 0; i < dn; i++ {
		tmp[sn+i] = a[1+i*2]
	}
	copy(a[:n], tmp)
}

// fwd53_2d applies the forward 2-D 5/3 DWT for one resolution level.
// data is the image stored row-major with the given stride. rw x rh is the
// region to transform.
//
// The standard JPEG 2000 forward order is: vertical pass first, then
// horizontal. The inverse (decoder) reverses this: horizontal then vertical.
func fwd53_2d(data []int32, stride, rw, rh int) {
	// Vertical pass first (to match OpenJPEG encoder order).
	colBuf := make([]int32, rh)
	for i := 0; i < rw; i++ {
		for j := 0; j < rh; j++ {
			colBuf[j] = data[j*stride+i]
		}
		fwd53_1d(colBuf, rh)
		for j := 0; j < rh; j++ {
			data[j*stride+i] = colBuf[j]
		}
	}
	// Then horizontal pass.
	rowBuf := make([]int32, rw)
	for j := 0; j < rh; j++ {
		copy(rowBuf, data[j*stride:j*stride+rw])
		fwd53_1d(rowBuf, rw)
		copy(data[j*stride:j*stride+rw], rowBuf)
	}
}

// fwd53_multilevel applies numLevels levels of the forward 2-D 5/3 DWT.
func fwd53_multilevel(data []int32, width, height, numLevels int) {
	stride := width
	rw := width
	rh := height
	for l := 0; l < numLevels; l++ {
		fwd53_2d(data, stride, rw, rh)
		rw = (rw + 1) / 2
		rh = (rh + 1) / 2
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestRoundTrip_Small verifies lossless round-trip for small images at
// various sizes and resolution levels.
func TestRoundTrip_Small(t *testing.T) {
	for _, tc := range []struct {
		w, h, levels int
	}{
		{1, 1, 1},
		{2, 1, 1},
		{1, 2, 1},
		{2, 2, 1},
		{3, 3, 1},
		{3, 3, 2},
		{4, 4, 1},
		{4, 4, 2},
		{5, 5, 1},
		{5, 5, 2},
		{5, 5, 3},
		{7, 7, 1},
		{7, 7, 2},
		{7, 7, 3},
		{8, 8, 1},
		{8, 8, 2},
		{8, 8, 3},
		{8, 4, 2},
		{4, 8, 2},
		{16, 16, 1},
		{16, 16, 2},
		{16, 16, 3},
		{16, 16, 4},
		{15, 17, 3},
		{17, 15, 3},
		{1, 16, 2},
		{16, 1, 2},
		{3, 1, 1},
		{1, 3, 1},
		{2, 3, 1},
		{3, 2, 1},
	} {
		name := fmt.Sprintf("%dx%d_L%d", tc.w, tc.h, tc.levels)
		t.Run(name, func(t *testing.T) {
			n := tc.w * tc.h
			orig := make([]int32, n)
			for i := range orig {
				orig[i] = int32(i*17 + 3)
			}
			data := make([]int32, n)
			copy(data, orig)

			fwd53_multilevel(data, tc.w, tc.h, tc.levels)

			err := InverseDWT53(data, tc.w, tc.h, tc.levels+1)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < n; i++ {
				if data[i] != orig[i] {
					t.Fatalf("mismatch at index %d: got %d, want %d", i, data[i], orig[i])
				}
			}
		})
	}
}

// TestRoundTrip_Ramp tests a 64x64 ramp image through several levels.
func TestRoundTrip_Ramp(t *testing.T) {
	const w, h = 64, 64
	for levels := 1; levels <= 5; levels++ {
		t.Run(fmt.Sprintf("levels=%d", levels), func(t *testing.T) {
			orig := make([]int32, w*h)
			for i := range orig {
				orig[i] = int32(i)
			}
			data := make([]int32, w*h)
			copy(data, orig)

			fwd53_multilevel(data, w, h, levels)

			err := InverseDWT53(data, w, h, levels+1)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < w*h; i++ {
				if data[i] != orig[i] {
					t.Fatalf("level %d: mismatch at %d: got %d want %d", levels, i, data[i], orig[i])
				}
			}
		})
	}
}

// TestRoundTrip_NonPowerOfTwo tests odd-sized images.
func TestRoundTrip_NonPowerOfTwo(t *testing.T) {
	for _, tc := range []struct {
		w, h, levels int
	}{
		{13, 7, 2},
		{7, 13, 2},
		{31, 17, 3},
		{17, 31, 3},
		{100, 50, 4},
		{50, 100, 4},
		{127, 127, 5},
	} {
		name := fmt.Sprintf("%dx%d_L%d", tc.w, tc.h, tc.levels)
		t.Run(name, func(t *testing.T) {
			n := tc.w * tc.h
			orig := make([]int32, n)
			for i := range orig {
				orig[i] = int32((i * 31) % 256)
			}
			data := make([]int32, n)
			copy(data, orig)

			fwd53_multilevel(data, tc.w, tc.h, tc.levels)

			err := InverseDWT53(data, tc.w, tc.h, tc.levels+1)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < n; i++ {
				if data[i] != orig[i] {
					t.Fatalf("mismatch at %d: got %d want %d", i, data[i], orig[i])
				}
			}
		})
	}
}

// TestRoundTrip_NegativeValues verifies that negative coefficients are
// handled correctly.
func TestRoundTrip_NegativeValues(t *testing.T) {
	const w, h, levels = 16, 16, 3
	n := w * h
	orig := make([]int32, n)
	for i := range orig {
		orig[i] = int32(i*41-128) % 500
	}
	data := make([]int32, n)
	copy(data, orig)

	fwd53_multilevel(data, w, h, levels)

	err := InverseDWT53(data, w, h, levels+1)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < n; i++ {
		if data[i] != orig[i] {
			t.Fatalf("mismatch at %d: got %d want %d", i, data[i], orig[i])
		}
	}
}

// TestInverseDWT53_NumRes1_Noop verifies that numResolutions==1 is a no-op.
func TestInverseDWT53_NumRes1_Noop(t *testing.T) {
	data := []int32{1, 2, 3, 4}
	orig := make([]int32, len(data))
	copy(orig, data)

	err := InverseDWT53(data, 2, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range data {
		if data[i] != orig[i] {
			t.Fatalf("should be no-op but index %d changed", i)
		}
	}
}

// TestInverseDWT53_ShortBuffer verifies error on too-short buffer.
func TestInverseDWT53_ShortBuffer(t *testing.T) {
	data := []int32{1, 2, 3}
	err := InverseDWT53(data, 2, 2, 2)
	if err == nil {
		t.Fatal("expected error for short buffer")
	}
}

// TestCeilDivPow2 verifies the ceiling division helper.
func TestCeilDivPow2(t *testing.T) {
	tests := [][3]int{
		{0, 0, 0},
		{1, 0, 1},
		{7, 0, 7},
		{8, 1, 4},
		{9, 1, 5},
		{8, 3, 1},
		{9, 3, 2},
		{15, 3, 2},
		{16, 3, 2},
		{17, 3, 3},
		{1, 5, 1},
		{0, 5, 0},
	}
	for _, tc := range tests {
		got := ceilDivPow2(tc[0], tc[1])
		if got != tc[2] {
			t.Errorf("ceilDivPow2(%d, %d) = %d, want %d", tc[0], tc[1], got, tc[2])
		}
	}
}

// TestRoundTrip_1x2 specific tiny case.
func TestRoundTrip_1x2(t *testing.T) {
	orig := []int32{10, 20}
	data := make([]int32, 2)
	copy(data, orig)

	fwd53_multilevel(data, 2, 1, 1)

	err := InverseDWT53(data, 2, 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	for i := range data {
		if data[i] != orig[i] {
			t.Fatalf("mismatch at %d: got %d want %d", i, data[i], orig[i])
		}
	}
}

// TestRoundTrip_2x1 specific tiny case (2 rows, 1 column).
func TestRoundTrip_2x1(t *testing.T) {
	orig := []int32{10, 20}
	data := make([]int32, 2)
	copy(data, orig)

	// stride = width = 1
	fwd53_multilevel(data, 1, 2, 1)

	err := InverseDWT53(data, 1, 2, 2)
	if err != nil {
		t.Fatal(err)
	}

	for i := range data {
		if data[i] != orig[i] {
			t.Fatalf("mismatch at %d: got %d want %d", i, data[i], orig[i])
		}
	}
}

// TestKnownVector_1D_4 tests a known 4-element 1-D forward+inverse.
func TestKnownVector_1D_4(t *testing.T) {
	// Forward 5/3 on [10, 20, 30, 40]:
	// Interleaved: S0=10, D0=20, S1=30, D1=40
	// Update (high-pass): D(i) -= (S(i) + S(i+1)) >> 1
	//   D0 = 20 - (10+30)/2 = 20 - 20 = 0
	//   D1 = 40 - (30+30)/2 = 40 - 30 = 10   (S(2) clamped to S(1)=30)
	// Predict (low-pass): S(i) += (D(i-1) + D(i) + 2) >> 2
	//   S0 = 10 + (0 + 0 + 2)/4 = 10 + 0 = 10   (D(-1) clamped to D(0)=0)
	//   S1 = 30 + (0 + 10 + 2)/4 = 30 + 3 = 33
	// De-interleaved: [10, 33, 0, 10]
	data := []int32{10, 20, 30, 40}
	fwd53_1d(data, 4)

	expected := []int32{10, 33, 0, 10}
	for i, v := range expected {
		if data[i] != v {
			t.Errorf("forward[%d] = %d, want %d", i, data[i], v)
		}
	}

	// Now apply inverse via the 2-D interface (1 row, 4 cols, 2 resolutions).
	err := InverseDWT53(data, 4, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	orig := []int32{10, 20, 30, 40}
	for i, v := range orig {
		if data[i] != v {
			t.Errorf("inverse[%d] = %d, want %d", i, data[i], v)
		}
	}
}

// TestForwardDWT53_MatchesReference verifies that ForwardDWT53 produces
// the same coefficients as the reference fwd53_multilevel.
func TestForwardDWT53_MatchesReference(t *testing.T) {
	for _, tc := range []struct {
		w, h, levels int
	}{
		{4, 4, 1},
		{4, 4, 2},
		{8, 8, 3},
		{16, 16, 4},
		{15, 17, 3},
		{64, 64, 5},
		{100, 50, 4},
	} {
		name := fmt.Sprintf("%dx%d_L%d", tc.w, tc.h, tc.levels)
		t.Run(name, func(t *testing.T) {
			n := tc.w * tc.h
			orig := make([]int32, n)
			for i := range orig {
				orig[i] = int32((i*17 + 3) % 256)
			}

			// Reference forward DWT (from test helpers above).
			refData := make([]int32, n)
			copy(refData, orig)
			fwd53_multilevel(refData, tc.w, tc.h, tc.levels)

			// ForwardDWT53 (the new production code).
			newData := make([]int32, n)
			copy(newData, orig)
			err := ForwardDWT53(newData, tc.w, tc.h, tc.levels+1)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < n; i++ {
				if newData[i] != refData[i] {
					t.Fatalf("mismatch at index %d: ForwardDWT53=%d, reference=%d",
						i, newData[i], refData[i])
				}
			}
		})
	}
}

// TestForwardDWT53_InverseDWT53_RoundTrip verifies that ForwardDWT53 followed
// by InverseDWT53 is a perfect round-trip.
func TestForwardDWT53_InverseDWT53_RoundTrip(t *testing.T) {
	for _, tc := range []struct {
		w, h, levels int
	}{
		{4, 4, 2},
		{8, 8, 3},
		{16, 16, 4},
		{15, 17, 3},
		{64, 64, 5},
	} {
		name := fmt.Sprintf("%dx%d_L%d", tc.w, tc.h, tc.levels)
		t.Run(name, func(t *testing.T) {
			n := tc.w * tc.h
			orig := make([]int32, n)
			for i := range orig {
				orig[i] = int32((i*31 + 7) % 500)
			}

			data := make([]int32, n)
			copy(data, orig)

			numRes := tc.levels + 1
			err := ForwardDWT53(data, tc.w, tc.h, numRes)
			if err != nil {
				t.Fatal(err)
			}
			err = InverseDWT53(data, tc.w, tc.h, numRes)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < n; i++ {
				if data[i] != orig[i] {
					t.Fatalf("mismatch at index %d: got %d, want %d", i, data[i], orig[i])
				}
			}
		})
	}
}
