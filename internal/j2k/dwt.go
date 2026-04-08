// Package j2k provides a pure-Go JPEG 2000 decoder for use in GRIB2.
//
// This file implements the inverse Discrete Wavelet Transform (DWT) for the
// reversible 5/3 wavelet (lossless, integer arithmetic). It is a port of the
// relevant routines from OpenJPEG (dwt.c), stripped of SIMD and threading
// paths to produce a clear, correct reference implementation.
//
// Lifting steps for the inverse 5/3 wavelet (cas == 0, even-aligned):
//
//	even[i] -= (odd[clamp(i-1)] + odd[clamp(i)] + 2) >> 2   (undo predict)
//	odd[i]  += (even[i] + even[i+1]) >> 1                     (undo update)
//
// The 2-D transform is separable: each resolution level is reconstructed by
// first applying the inverse 1-D DWT to every row (horizontal pass), then to
// every column (vertical pass), working from the lowest resolution upward.
//
// Copyright (c) 2002-2014 UCL / OpenJPEG contributors (original C).
// BSD 2-Clause — see the OpenJPEG license for redistribution terms.
package j2k

// InverseDWT53 performs the 2-D inverse discrete wavelet transform using the
// reversible 5/3 wavelet on coefficient data arranged in a single contiguous
// buffer.
//
// Parameters:
//
//   - coeffs: the coefficient array of length width*height. On entry it holds
//     the wavelet-domain coefficients arranged in the standard JPEG 2000
//     subband layout (LL at top-left, then HL/LH/HH bands for each
//     resolution level). On return it holds the reconstructed spatial-domain
//     samples.
//   - width:  image (tile-component) width in pixels.
//   - height: image (tile-component) height in pixels.
//   - numResolutions: total number of resolution levels (>= 1). A value of 1
//     means no transform is applied.
//
// The function returns an error only if the inputs are invalid.
func InverseDWT53(coeffs []int32, width, height, numResolutions int) error {
	if numResolutions <= 1 || width == 0 || height == 0 {
		return nil
	}
	if len(coeffs) < width*height {
		return errShortCoeffs
	}

	// Compute the (x0,y0)-(x1,y1) rectangle for each resolution level.
	// In the simple whole-tile case every resolution starts at (0,0) and
	// the dimensions are derived by the standard JPEG 2000 ceil formulae:
	//   resW[r] = ceil_div_pow2(width, numRes-1-r)
	//   resH[r] = ceil_div_pow2(height, numRes-1-r)
	type rect struct{ x0, y0, x1, y1 int }
	nRes := numResolutions
	res := make([]rect, nRes)
	for r := 0; r < nRes; r++ {
		shift := nRes - 1 - r
		res[r] = rect{
			x0: 0,
			y0: 0,
			x1: ceilDivPow2(width, shift),
			y1: ceilDivPow2(height, shift),
		}
	}

	// Allocate a scratch buffer large enough for the longest row or column
	// we will ever encounter.
	maxDim := 0
	for r := 1; r < nRes; r++ {
		if w := res[r].x1 - res[r].x0; w > maxDim {
			maxDim = w
		}
		if h := res[r].y1 - res[r].y0; h > maxDim {
			maxDim = h
		}
	}
	tmp := make([]int32, maxDim)

	// The full tile-component buffer stride (number of int32 values per row).
	// This is always the width of the highest resolution.
	stride := res[nRes-1].x1 - res[nRes-1].x0

	// Starting dimensions (lowest resolution, level 0).
	rw := res[0].x1 - res[0].x0
	rh := res[0].y1 - res[0].y0

	// Iterate from the second-lowest resolution (level 1) up to the highest.
	for r := 1; r < nRes; r++ {
		tr := res[r]
		newRW := tr.x1 - tr.x0
		newRH := tr.y1 - tr.y0

		// -- Horizontal pass: for each row of the new resolution ------
		hSN := rw                // number of low-pass coefficients (previous width)
		hDN := newRW - hSN       // number of high-pass coefficients
		hCas := tr.x0 & 1        // parity of left edge

		for j := 0; j < newRH; j++ {
			row := coeffs[j*stride:]
			idwt53H(tmp, row, hSN, hDN, hCas)
		}

		// -- Vertical pass: for each column of the new resolution -----
		vSN := rh                // number of low-pass coefficients (previous height)
		vDN := newRH - vSN       // number of high-pass coefficients
		vCas := tr.y0 & 1        // parity of top edge

		for j := 0; j < newRW; j++ {
			idwt53V(tmp, coeffs, j, stride, vSN, vDN, vCas)
		}

		rw = newRW
		rh = newRH
	}
	return nil
}

// errShortCoeffs is returned when the coefficient slice is too small.
var errShortCoeffs = shortCoeffsError{}

type shortCoeffsError struct{}

func (shortCoeffsError) Error() string {
	return "j2k: coefficient buffer shorter than width*height"
}

// ---------------------------------------------------------------------------
// 1-D horizontal inverse 5/3 DWT for one row.
// ---------------------------------------------------------------------------

// idwt53H performs the inverse horizontal 5/3 wavelet transform of one row.
//
// On entry the row is laid out as:
//
//	row[0..sn-1]  = low-pass coefficients
//	row[sn..sn+dn-1] = high-pass coefficients
//
// On exit the row contains the reconstructed spatial-domain samples. tmp must
// have length >= sn+dn.
func idwt53H(tmp []int32, row []int32, sn, dn, cas int) {
	length := sn + dn
	if length == 0 {
		return
	}

	if cas == 0 { // left-most sample on even coordinate
		if length == 1 {
			// Single coefficient — nothing to do.
			return
		}
		idwt53HCas0(tmp, row, sn, length)
	} else { // left-most sample on odd coordinate
		if length == 1 {
			row[0] = row[0] / 2 // integer division truncates toward zero
			return
		}
		if length == 2 {
			// Special short-length case.
			inEven := row[sn:]
			inOdd := row[0:]
			tmp[1] = inOdd[0] - ((inEven[0] + 1) >> 1)
			tmp[0] = inEven[0] + tmp[1]
			copy(row, tmp[:length])
			return
		}
		idwt53HCas1(tmp, row, sn, length)
	}
}

// idwt53HCas0 performs the horizontal inverse 5/3 DWT for cas==0 (even start)
// with len > 1, writing the result through tmp and then copying back to row.
//
// This follows the optimised single-pass lifting from OpenJPEG that avoids an
// explicit interleaving step.
func idwt53HCas0(tmp []int32, row []int32, sn, length int) {
	inEven := row[0:]
	inOdd := row[sn:]

	// Bootstrap: first even sample.
	s1n := inEven[0]
	d1n := inOdd[0]
	s0n := s1n - ((d1n + 1) >> 1)

	i := 0
	j := 1
	for ; i < length-3; i, j = i+2, j+1 {
		d1c := d1n
		s0c := s0n

		s1n = inEven[j]
		d1n = inOdd[j]

		s0n = s1n - ((d1c + d1n + 2) >> 2)

		tmp[i] = s0c
		tmp[i+1] = d1c + ((s0c + s0n) >> 1)
	}

	tmp[i] = s0n

	if length&1 != 0 { // odd length
		tmp[length-1] = inEven[(length-1)/2] - ((d1n + 1) >> 1)
		tmp[length-2] = d1n + ((s0n + tmp[length-1]) >> 1)
	} else { // even length
		tmp[length-1] = d1n + s0n
	}

	copy(row, tmp[:length])
}

// idwt53HCas1 performs the horizontal inverse 5/3 DWT for cas==1 (odd start)
// with len > 2.
func idwt53HCas1(tmp []int32, row []int32, sn, length int) {
	inEven := row[sn:] // high-index half is the even samples for cas==1
	inOdd := row[0:]   // low-index half is the odd samples for cas==1

	s1 := inEven[1]
	dc := inOdd[0] - ((inEven[0] + s1 + 2) >> 2)
	tmp[0] = inEven[0] + dc

	i := 1
	j := 1
	limit := length - 2
	if length&1 == 0 {
		limit-- // length - 3 when even
	}
	for ; i < limit; i, j = i+2, j+1 {
		s2 := inEven[j+1]
		dn := inOdd[j] - ((s1 + s2 + 2) >> 2)
		tmp[i] = dc
		tmp[i+1] = s1 + ((dn + dc) >> 1)
		dc = dn
		s1 = s2
	}

	tmp[i] = dc

	if length&1 == 0 { // even length
		dn := inOdd[length/2-1] - ((s1 + 1) >> 1)
		tmp[length-2] = s1 + ((dn + dc) >> 1)
		tmp[length-1] = dn
	} else { // odd length
		tmp[length-1] = s1 + dc
	}

	copy(row, tmp[:length])
}

// ---------------------------------------------------------------------------
// 1-D vertical inverse 5/3 DWT for one column.
// ---------------------------------------------------------------------------

// idwt53V performs the inverse vertical 5/3 wavelet transform for a single
// column within a 2-D buffer.
//
// col     — column index within the buffer.
// stride  — number of int32 values per row.
// sn / dn — number of low-pass / high-pass coefficients.
// cas     — parity of the top edge.
func idwt53V(tmp []int32, buf []int32, col, stride, sn, dn, cas int) {
	length := sn + dn
	if length == 0 {
		return
	}

	if cas == 0 {
		if length == 1 {
			return
		}
		idwt53VCas0(tmp, buf, col, stride, sn, length)
	} else {
		if length == 1 {
			buf[col] = buf[col] / 2
			return
		}
		if length == 2 {
			inEven0 := buf[sn*stride+col]
			inOdd0 := buf[col]
			tmp[1] = inOdd0 - ((inEven0 + 1) >> 1)
			tmp[0] = inEven0 + tmp[1]
			buf[col] = tmp[0]
			buf[stride+col] = tmp[1]
			return
		}
		idwt53VCas1(tmp, buf, col, stride, sn, length)
	}
}

// idwt53VCas0 — vertical cas==0, length > 1.
func idwt53VCas0(tmp []int32, buf []int32, col, stride, sn, length int) {
	// in_even starts at row 0 (low-pass); in_odd starts at row sn (high-pass).
	evenOff := col            // row 0, column col
	oddOff := sn*stride + col // row sn, column col

	s1n := buf[evenOff]
	d1n := buf[oddOff]
	s0n := s1n - ((d1n + 1) >> 1)

	i := 0
	j := 0
	for ; i < length-3; i, j = i+2, j+1 {
		d1c := d1n
		s0c := s0n

		s1n = buf[evenOff+(j+1)*stride]
		d1n = buf[oddOff+(j+1)*stride]

		s0n = s1n - ((d1c + d1n + 2) >> 2)

		tmp[i] = s0c
		tmp[i+1] = d1c + ((s0c + s0n) >> 1)
	}

	tmp[i] = s0n

	if length&1 != 0 {
		tmp[length-1] = buf[evenOff+((length-1)/2)*stride] - ((d1n + 1) >> 1)
		tmp[length-2] = d1n + ((s0n + tmp[length-1]) >> 1)
	} else {
		tmp[length-1] = d1n + s0n
	}

	for k := 0; k < length; k++ {
		buf[k*stride+col] = tmp[k]
	}
}

// idwt53VCas1 — vertical cas==1, length > 2.
func idwt53VCas1(tmp []int32, buf []int32, col, stride, sn, length int) {
	evenOff := sn * stride + col // even (low-frequency) samples start at row sn
	oddOff := col                // odd (high-frequency) samples start at row 0

	s1 := buf[evenOff+stride]
	dc := buf[oddOff] - ((buf[evenOff] + s1 + 2) >> 2)
	tmp[0] = buf[evenOff] + dc

	i := 1
	j := 1
	limit := length - 2
	if length&1 == 0 {
		limit--
	}
	for ; i < limit; i, j = i+2, j+1 {
		s2 := buf[evenOff+(j+1)*stride]
		dn := buf[oddOff+j*stride] - ((s1 + s2 + 2) >> 2)
		tmp[i] = dc
		tmp[i+1] = s1 + ((dn + dc) >> 1)
		dc = dn
		s1 = s2
	}

	tmp[i] = dc

	if length&1 == 0 {
		dn := buf[oddOff+(length/2-1)*stride] - ((s1 + 1) >> 1)
		tmp[length-2] = s1 + ((dn + dc) >> 1)
		tmp[length-1] = dn
	} else {
		tmp[length-1] = s1 + dc
	}

	for k := 0; k < length; k++ {
		buf[k*stride+col] = tmp[k]
	}
}

// ---------------------------------------------------------------------------
// Forward 5/3 DWT (encoder)
// ---------------------------------------------------------------------------

// ForwardDWT53 performs the 2-D forward discrete wavelet transform using
// the reversible 5/3 wavelet. This is the inverse of InverseDWT53.
//
// Parameters:
//
//   - coeffs: on entry holds the spatial-domain samples. On return it
//     holds the wavelet-domain coefficients in the standard JPEG 2000
//     subband layout (LL at top-left, then HL/LH/HH bands for each
//     resolution level).
//   - width, height: image dimensions.
//   - numResolutions: total number of resolution levels (>= 1). A value
//     of 1 means no transform is applied. The number of decomposition
//     levels is numResolutions - 1.
func ForwardDWT53(coeffs []int32, width, height, numResolutions int) error {
	if numResolutions <= 1 || width == 0 || height == 0 {
		return nil
	}
	if len(coeffs) < width*height {
		return errShortCoeffs
	}

	nRes := numResolutions
	numDecomp := nRes - 1

	// Compute the resolution dimensions at each level.
	type dimPair struct{ w, h int }
	dims := make([]dimPair, nRes)
	for r := 0; r < nRes; r++ {
		shift := numDecomp - r
		dims[r] = dimPair{
			w: ceilDivPow2(width, shift),
			h: ceilDivPow2(height, shift),
		}
	}

	stride := width

	// Allocate scratch buffer.
	maxDim := 0
	for r := 1; r < nRes; r++ {
		if dims[r].w > maxDim {
			maxDim = dims[r].w
		}
		if dims[r].h > maxDim {
			maxDim = dims[r].h
		}
	}
	tmp := make([]int32, maxDim)

	// Forward DWT works from the highest resolution downward (the
	// reverse of the inverse). For each level l (from numDecomp-1 down
	// to 0), we transform the region at resolution l+1 and produce
	// the subband layout for that level.
	for l := numDecomp; l >= 1; l-- {
		rw := dims[l].w // current resolution width
		rh := dims[l].h // current resolution height

		sn_h := dims[l-1].w // low-pass count (horizontal)
		dn_h := rw - sn_h   // high-pass count (horizontal)

		sn_v := dims[l-1].h // low-pass count (vertical)
		dn_v := rh - sn_v   // high-pass count (vertical)

		// Forward order: vertical first, then horizontal
		// (reverse of decoder's horizontal-then-vertical).

		// -- Vertical pass: for each column --
		for j := 0; j < rw; j++ {
			fwdDWT53V(tmp, coeffs, j, stride, sn_v, dn_v, rh)
		}

		// -- Horizontal pass: for each row --
		for j := 0; j < rh; j++ {
			row := coeffs[j*stride:]
			fwdDWT53H(tmp, row, sn_h, dn_h, rw)
		}
	}
	return nil
}

// fwdDWT53H performs the forward horizontal 5/3 wavelet transform of one row.
// On entry the row holds spatial samples. On exit, row[0:sn] holds low-pass
// and row[sn:sn+dn] holds high-pass coefficients.
func fwdDWT53H(tmp []int32, row []int32, sn, dn, length int) {
	if length < 2 {
		return
	}

	// Convenience accessors for the interleaved view.
	S := func(i int) int32 { return row[i*2] }
	D := func(i int) int32 { return row[1+i*2] }

	// Clamped accessors for boundary extension.
	Sc := func(i int) int32 {
		if i < 0 {
			return row[0]
		}
		if i >= sn {
			return row[(sn-1)*2]
		}
		return row[i*2]
	}
	// High-pass update: D(i) -= (S(i) + S(i+1)) >> 1
	for i := 0; i < dn; i++ {
		tmp[sn+i] = D(i) - ((Sc(i) + Sc(i+1)) >> 1)
	}
	// Low-pass predict: S(i) += (D(i-1) + D(i) + 2) >> 2
	// We must use the updated D values.
	for i := 0; i < sn; i++ {
		dm1 := tmp[sn] // D(0) updated
		if i > 0 {
			dm1 = tmp[sn+i-1]
		}
		di := tmp[sn] // D(0) updated
		if dn > 0 {
			if i < dn {
				di = tmp[sn+i]
			} else {
				di = tmp[sn+dn-1]
			}
		} else {
			di = 0
			dm1 = 0
		}
		if i == 0 && dn > 0 {
			dm1 = tmp[sn] // clamp D(-1) to D(0)
		}
		if dn == 0 {
			tmp[i] = S(i)
		} else {
			tmp[i] = S(i) + ((dm1 + di + 2) >> 2)
		}
	}

	copy(row[:length], tmp[:length])
}

// fwdDWT53V performs the forward vertical 5/3 wavelet transform for a
// single column within a 2-D buffer.
func fwdDWT53V(tmp []int32, buf []int32, col, stride, sn, dn, length int) {
	if length < 2 {
		return
	}

	// Read column into tmp as interleaved: even indices are S, odd are D.
	// buf[0*stride+col] = sample 0, buf[1*stride+col] = sample 1, etc.

	S := func(i int) int32 { return buf[i*2*stride+col] }
	D := func(i int) int32 { return buf[(1+i*2)*stride+col] }
	Sc := func(i int) int32 {
		if i < 0 {
			return buf[col]
		}
		if i >= sn {
			return buf[(sn-1)*2*stride+col]
		}
		return buf[i*2*stride+col]
	}
	// High-pass update: D(i) -= (S(i) + S(i+1)) >> 1
	for i := 0; i < dn; i++ {
		tmp[sn+i] = D(i) - ((Sc(i) + Sc(i+1)) >> 1)
	}
	// Low-pass predict: S(i) += (D(i-1) + D(i) + 2) >> 2
	for i := 0; i < sn; i++ {
		dm1 := tmp[sn]
		if i > 0 {
			dm1 = tmp[sn+i-1]
		}
		di := tmp[sn]
		if dn > 0 {
			if i < dn {
				di = tmp[sn+i]
			} else {
				di = tmp[sn+dn-1]
			}
		} else {
			di = 0
			dm1 = 0
		}
		if i == 0 && dn > 0 {
			dm1 = tmp[sn]
		}
		if dn == 0 {
			tmp[i] = S(i)
		} else {
			tmp[i] = S(i) + ((dm1 + di + 2) >> 2)
		}
	}
	// Write back: low-pass in rows 0..sn-1, high-pass in rows sn..length-1
	for k := 0; k < length; k++ {
		buf[k*stride+col] = tmp[k]
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// ceilDivPow2 returns ceil(value / 2^shift).
func ceilDivPow2(value, shift int) int {
	if shift <= 0 {
		return value
	}
	return (value + (1 << shift) - 1) >> shift
}
