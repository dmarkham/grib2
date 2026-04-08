// Tier-1 (T1) code-block decoder for JPEG 2000.
//
// Ported from OpenJPEG (openjpeg/src/lib/openjp2/t1.c, t1.h, t1_luts.h).
// Implements the three coding passes: significance propagation, magnitude
// refinement, and cleanup.  Only the DECODER side is ported; only the
// lossless (reversible, qmfbid=1) path is needed.
//
// Original copyright:
//
//	Copyright (c) 2002-2014, Universite catholique de Louvain (UCL), Belgium
//	Copyright (c) 2002-2014, Professor Benoit Macq
//	Copyright (c) 2001-2003, David Janssens
//	Copyright (c) 2002-2003, Yannick Verschueren
//	Copyright (c) 2003-2007, Francois-Olivier Devaux
//	Copyright (c) 2003-2014, Antonin Descampe
//	Copyright (c) 2005, Herve Drolon, FreeImage Team
//	Copyright (c) 2007, Callum Lerwick
//	Copyright (c) 2012, Carl Hetherington
//	Copyright (c) 2017, IntoPIX SA
//
// 2-clause BSD License. See LICENSE in the OpenJPEG source tree.
package j2k

// ---------------------------------------------------------------------------
// Context-number constants (ISO 15444-1 Table D.2)
// ---------------------------------------------------------------------------

const (
	t1NumCtxsZC  = 9
	t1NumCtxsSC  = 5
	t1NumCtxsMAG = 3
	t1NumCtxsAGG = 1
	t1NumCtxsUNI = 1

	T1CtxnoZC  = 0
	T1CtxnoSC  = T1CtxnoZC + t1NumCtxsZC   // 9
	T1CtxnoMAG = T1CtxnoSC + t1NumCtxsSC    // 14
	T1CtxnoAGG = T1CtxnoMAG + t1NumCtxsMAG  // 17
	T1CtxnoUNI = T1CtxnoAGG + t1NumCtxsAGG  // 18
	t1NumCtxs  = T1CtxnoUNI + t1NumCtxsUNI  // 19
)

// Pass type constants.
const (
	t1TypeMQ  = 0
	t1TypeRAW = 1
)

// Code-block style bits (J2K_CCP_CBLKSTY_*).
const (
	CblkstyLAZY    = 0x01 // Selective arithmetic coding bypass
	CblkstyRESET   = 0x02 // Reset context probabilities on coding pass boundaries
	CblkstyTERMALL = 0x04 // Termination on each coding pass
	CblkstyVSC     = 0x08 // Vertically stripe causal context
	CblkstyPTERM   = 0x10 // Predictable termination
	CblkstySEGSYM  = 0x20 // Segmentation symbols are used
	CblkstyHT      = 0x40 // HT codeblocks
)

// ---------------------------------------------------------------------------
// Flag bits for the per-column (4-row) flag words.
//
// Each 32-bit flag word encodes significance (SIGMA), sign (CHI),
// refinement-visited (MU) and significance-pass-visited (PI) for
// a 4-row column and its 8 neighbours.
//
// Layout (from OpenJPEG t1.h):
//
//	SIGMA: significance  (3 cols x 6 rows = 18 bits)
//	CHI:   sign negative (1 col  x 6 rows =  6 bits)
//	MU:    refinement    (1 col  x 4 rows =  4 bits)
//	PI:    sig-pass      (1 col  x 4 rows =  4 bits)
//
// ---------------------------------------------------------------------------

const (
	t1Sigma0  uint32 = 1 << 0
	t1Sigma1  uint32 = 1 << 1
	t1Sigma2  uint32 = 1 << 2
	t1Sigma3  uint32 = 1 << 3
	t1Sigma4  uint32 = 1 << 4
	t1Sigma5  uint32 = 1 << 5
	t1Sigma6  uint32 = 1 << 6
	t1Sigma7  uint32 = 1 << 7
	t1Sigma8  uint32 = 1 << 8
	t1Sigma9  uint32 = 1 << 9
	t1Sigma10 uint32 = 1 << 10
	t1Sigma11 uint32 = 1 << 11
	t1Sigma12 uint32 = 1 << 12
	t1Sigma13 uint32 = 1 << 13
	t1Sigma14 uint32 = 1 << 14
	t1Sigma15 uint32 = 1 << 15
	t1Sigma16 uint32 = 1 << 16
	t1Sigma17 uint32 = 1 << 17

	t1Chi0   uint32 = 1 << 18
	t1Chi0I         = 18
	t1Chi1   uint32 = 1 << 19
	t1Chi1I         = 19
	t1Mu0    uint32 = 1 << 20
	t1Pi0    uint32 = 1 << 21
	t1Chi2   uint32 = 1 << 22
	t1Chi2I         = 22
	t1Mu1    uint32 = 1 << 23
	t1Pi1    uint32 = 1 << 24
	t1Chi3   uint32 = 1 << 25
	t1Mu2    uint32 = 1 << 26
	t1Pi2    uint32 = 1 << 27
	t1Chi4   uint32 = 1 << 28
	t1Mu3    uint32 = 1 << 29
	t1Pi3    uint32 = 1 << 30
	t1Chi5   uint32 = 1 << 31
	t1Chi5I         = 31
)

// Convenience aliases used when referring to the "current" row within
// a 4-row stripe.  Shift by 3*ci to get the bits for row ci.
const (
	t1SigmaNW         = t1Sigma0
	t1SigmaN          = t1Sigma1
	t1SigmaNE         = t1Sigma2
	t1SigmaW          = t1Sigma3
	t1SigmaThis       = t1Sigma4
	t1SigmaE          = t1Sigma5
	t1SigmaSW         = t1Sigma6
	t1SigmaS          = t1Sigma7
	t1SigmaSE         = t1Sigma8
	t1SigmaNeighbours = t1SigmaNW | t1SigmaN | t1SigmaNE |
		t1SigmaW | t1SigmaE |
		t1SigmaSW | t1SigmaS | t1SigmaSE

	t1ChiThis  = t1Chi1
	t1ChiThisI = t1Chi1I
	t1MuThis   = t1Mu0
	t1PiThis   = t1Pi0
	t1ChiS     = t1Chi2
)

// LUT index helpers.
const (
	t1LutSgnW uint32 = 1 << 0
	t1LutSigN uint32 = 1 << 1
	t1LutSgnE uint32 = 1 << 2
	t1LutSigW uint32 = 1 << 3
	t1LutSgnN uint32 = 1 << 4
	t1LutSigE uint32 = 1 << 5
	t1LutSgnS uint32 = 1 << 6
	t1LutSigS uint32 = 1 << 7
)

// ---------------------------------------------------------------------------
// T1 decoder state
// ---------------------------------------------------------------------------

// T1 holds the state for tier-1 (code-block) decoding.
type T1 struct {
	mqc MQDecoder

	// data holds the decoded coefficients in row-major order, i.e.
	// data[y*w + x].  Values are in signed-magnitude representation
	// (sign in MSB, magnitude in lower bits).
	data []int32

	// flags is the significance/state array.  It is organised as
	// columns of 4-row stripes.  flags[flagsStride + 1 + x + stripeRow*flagsStride]
	// corresponds to column x, stripe starting at row stripeRow*4.
	// An extra row of guard entries exists at the top and bottom, and
	// one extra column on each side, hence flagsStride = w+2.
	flags []uint32

	w           uint32
	h           uint32
	flagsStride uint32

	// lutCtxnoZC points to the orient-specific slice of lutCtxnoZCAll.
	lutCtxnoZC []byte
}

// NewT1 creates a new T1 decoder.
func NewT1() *T1 {
	return &T1{}
}

// ---------------------------------------------------------------------------
// Buffer allocation
// ---------------------------------------------------------------------------

func (t *T1) allocateBuffers(w, h uint32) {
	datasize := w * h
	if uint32(len(t.data)) < datasize {
		t.data = make([]int32, datasize)
	} else {
		t.data = t.data[:datasize]
		for i := range t.data {
			t.data[i] = 0
		}
	}

	flagsStride := w + 2
	flagsHeight := (h + 3) / 4
	flagsSize := (flagsHeight + 2) * flagsStride

	if uint32(len(t.flags)) < flagsSize {
		t.flags = make([]uint32, flagsSize)
	} else {
		t.flags = t.flags[:flagsSize]
		for i := range t.flags {
			t.flags[i] = 0
		}
	}

	// First row of flags: guard with PI bits set so no pass touches them.
	for x := uint32(0); x < flagsStride; x++ {
		t.flags[x] = t1Pi0 | t1Pi1 | t1Pi2 | t1Pi3
	}
	// Last row of flags: same guard.
	base := (flagsHeight + 1) * flagsStride
	for x := uint32(0); x < flagsStride; x++ {
		t.flags[base+x] = t1Pi0 | t1Pi1 | t1Pi2 | t1Pi3
	}
	// If h is not a multiple of 4, mark the unused rows in the last
	// real stripe so that no pass processes them.
	if h%4 != 0 {
		var v uint32
		switch h % 4 {
		case 1:
			v = t1Pi1 | t1Pi2 | t1Pi3
		case 2:
			v = t1Pi2 | t1Pi3
		case 3:
			v = t1Pi3
		}
		p := flagsHeight * flagsStride
		for x := uint32(0); x < flagsStride; x++ {
			t.flags[p+x] = v
		}
	}

	t.w = w
	t.h = h
	t.flagsStride = flagsStride
}

// ---------------------------------------------------------------------------
// Look-up tables (from t1_luts.h)
// ---------------------------------------------------------------------------

// lutCtxnoZCAll contains the zero-coding context numbers for all four
// sub-band orientations (HL, LH, HH, LL).  The orient-specific slice
// begins at orient*512.
//
// This is the full 2048-entry table from openjpeg t1_luts.h.
var lutCtxnoZCAll = [2048]byte{
	// orient 0 (LL/LH)  -- indices 0..511
	0, 1, 3, 3, 1, 2, 3, 3, 5, 6, 7, 7, 6, 6, 7, 7, 0, 1, 3, 3, 1, 2, 3, 3, 5, 6, 7, 7, 6, 6, 7, 7,
	5, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 5, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	1, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7, 1, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7,
	6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7, 3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7, 3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	1, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7, 1, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7,
	6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	2, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7, 2, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7,
	6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7, 3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7, 3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	// orient 1 (HL)  -- indices 512..1023
	0, 1, 5, 6, 1, 2, 6, 6, 3, 3, 7, 7, 3, 3, 7, 7, 0, 1, 5, 6, 1, 2, 6, 6, 3, 3, 7, 7, 3, 3, 7, 7,
	3, 3, 7, 7, 3, 3, 7, 7, 4, 4, 7, 7, 4, 4, 7, 7, 3, 3, 7, 7, 3, 3, 7, 7, 4, 4, 7, 7, 4, 4, 7, 7,
	1, 2, 6, 6, 2, 2, 6, 6, 3, 3, 7, 7, 3, 3, 7, 7, 1, 2, 6, 6, 2, 2, 6, 6, 3, 3, 7, 7, 3, 3, 7, 7,
	3, 3, 7, 7, 3, 3, 7, 7, 4, 4, 7, 7, 4, 4, 7, 7, 3, 3, 7, 7, 3, 3, 7, 7, 4, 4, 7, 7, 4, 4, 7, 7,
	5, 6, 8, 8, 6, 6, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 5, 6, 8, 8, 6, 6, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8,
	7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8,
	6, 6, 8, 8, 6, 6, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 6, 6, 8, 8, 6, 6, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8,
	7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8,
	1, 2, 6, 6, 2, 2, 6, 6, 3, 3, 7, 7, 3, 3, 7, 7, 1, 2, 6, 6, 2, 2, 6, 6, 3, 3, 7, 7, 3, 3, 7, 7,
	3, 3, 7, 7, 3, 3, 7, 7, 4, 4, 7, 7, 4, 4, 7, 7, 3, 3, 7, 7, 3, 3, 7, 7, 4, 4, 7, 7, 4, 4, 7, 7,
	2, 2, 6, 6, 2, 2, 6, 6, 3, 3, 7, 7, 3, 3, 7, 7, 2, 2, 6, 6, 2, 2, 6, 6, 3, 3, 7, 7, 3, 3, 7, 7,
	3, 3, 7, 7, 3, 3, 7, 7, 4, 4, 7, 7, 4, 4, 7, 7, 3, 3, 7, 7, 3, 3, 7, 7, 4, 4, 7, 7, 4, 4, 7, 7,
	6, 6, 8, 8, 6, 6, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 6, 6, 8, 8, 6, 6, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8,
	7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8,
	6, 6, 8, 8, 6, 6, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 6, 6, 8, 8, 6, 6, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8,
	7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8, 7, 7, 8, 8,
	// orient 2 (LL for resno>0 -- same as orient 0 in practice)
	0, 1, 3, 3, 1, 2, 3, 3, 5, 6, 7, 7, 6, 6, 7, 7, 0, 1, 3, 3, 1, 2, 3, 3, 5, 6, 7, 7, 6, 6, 7, 7,
	5, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 5, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	1, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7, 1, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7,
	6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7, 3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7, 3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	1, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7, 1, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7,
	6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	2, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7, 2, 2, 3, 3, 2, 2, 3, 3, 6, 6, 7, 7, 6, 6, 7, 7,
	6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 6, 6, 7, 7, 6, 6, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7, 3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7, 3, 3, 4, 4, 3, 3, 4, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8, 7, 7, 7, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 8, 8, 8,
	// orient 3 (HH)  -- indices 1536..2047
	0, 3, 1, 4, 3, 6, 4, 7, 1, 4, 2, 5, 4, 7, 5, 7, 0, 3, 1, 4, 3, 6, 4, 7, 1, 4, 2, 5, 4, 7, 5, 7,
	1, 4, 2, 5, 4, 7, 5, 7, 2, 5, 2, 5, 5, 7, 5, 7, 1, 4, 2, 5, 4, 7, 5, 7, 2, 5, 2, 5, 5, 7, 5, 7,
	3, 6, 4, 7, 6, 8, 7, 8, 4, 7, 5, 7, 7, 8, 7, 8, 3, 6, 4, 7, 6, 8, 7, 8, 4, 7, 5, 7, 7, 8, 7, 8,
	4, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8, 4, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8,
	1, 4, 2, 5, 4, 7, 5, 7, 2, 5, 2, 5, 5, 7, 5, 7, 1, 4, 2, 5, 4, 7, 5, 7, 2, 5, 2, 5, 5, 7, 5, 7,
	2, 5, 2, 5, 5, 7, 5, 7, 2, 5, 2, 5, 5, 7, 5, 7, 2, 5, 2, 5, 5, 7, 5, 7, 2, 5, 2, 5, 5, 7, 5, 7,
	4, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8, 4, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8,
	5, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8,
	3, 6, 4, 7, 6, 8, 7, 8, 4, 7, 5, 7, 7, 8, 7, 8, 3, 6, 4, 7, 6, 8, 7, 8, 4, 7, 5, 7, 7, 8, 7, 8,
	4, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8, 4, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8,
	6, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8, 6, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8,
	7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8,
	4, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8, 4, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8,
	5, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8, 5, 7, 5, 7, 7, 8, 7, 8,
	7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8,
	7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8, 7, 8, 7, 8, 8, 8, 8, 8,
}

// lutCtxnoSC: sign-coding context number from the 8-bit neighbour index.
var lutCtxnoSC = [256]byte{
	0x9, 0x9, 0xa, 0xa, 0x9, 0x9, 0xa, 0xa, 0xc, 0xc, 0xd, 0xb, 0xc, 0xc, 0xd, 0xb,
	0x9, 0x9, 0xa, 0xa, 0x9, 0x9, 0xa, 0xa, 0xc, 0xc, 0xb, 0xd, 0xc, 0xc, 0xb, 0xd,
	0xc, 0xc, 0xd, 0xd, 0xc, 0xc, 0xb, 0xb, 0xc, 0x9, 0xd, 0xa, 0x9, 0xc, 0xa, 0xb,
	0xc, 0xc, 0xb, 0xb, 0xc, 0xc, 0xd, 0xd, 0xc, 0x9, 0xb, 0xa, 0x9, 0xc, 0xa, 0xd,
	0x9, 0x9, 0xa, 0xa, 0x9, 0x9, 0xa, 0xa, 0xc, 0xc, 0xd, 0xb, 0xc, 0xc, 0xd, 0xb,
	0x9, 0x9, 0xa, 0xa, 0x9, 0x9, 0xa, 0xa, 0xc, 0xc, 0xb, 0xd, 0xc, 0xc, 0xb, 0xd,
	0xc, 0xc, 0xd, 0xd, 0xc, 0xc, 0xb, 0xb, 0xc, 0x9, 0xd, 0xa, 0x9, 0xc, 0xa, 0xb,
	0xc, 0xc, 0xb, 0xb, 0xc, 0xc, 0xd, 0xd, 0xc, 0x9, 0xb, 0xa, 0x9, 0xc, 0xa, 0xd,
	0xa, 0xa, 0xa, 0xa, 0xa, 0xa, 0xa, 0xa, 0xd, 0xb, 0xd, 0xb, 0xd, 0xb, 0xd, 0xb,
	0xa, 0xa, 0x9, 0x9, 0xa, 0xa, 0x9, 0x9, 0xd, 0xb, 0xc, 0xc, 0xd, 0xb, 0xc, 0xc,
	0xd, 0xd, 0xd, 0xd, 0xb, 0xb, 0xb, 0xb, 0xd, 0xa, 0xd, 0xa, 0xa, 0xb, 0xa, 0xb,
	0xd, 0xd, 0xc, 0xc, 0xb, 0xb, 0xc, 0xc, 0xd, 0xa, 0xc, 0x9, 0xa, 0xb, 0x9, 0xc,
	0xa, 0xa, 0x9, 0x9, 0xa, 0xa, 0x9, 0x9, 0xb, 0xd, 0xc, 0xc, 0xb, 0xd, 0xc, 0xc,
	0xa, 0xa, 0xa, 0xa, 0xa, 0xa, 0xa, 0xa, 0xb, 0xd, 0xb, 0xd, 0xb, 0xd, 0xb, 0xd,
	0xb, 0xb, 0xc, 0xc, 0xd, 0xd, 0xc, 0xc, 0xb, 0xa, 0xc, 0x9, 0xa, 0xd, 0x9, 0xc,
	0xb, 0xb, 0xb, 0xb, 0xd, 0xd, 0xd, 0xd, 0xb, 0xa, 0xb, 0xa, 0xa, 0xd, 0xa, 0xd,
}

// lutSPB: sign prediction bit from the 8-bit neighbour index.
var lutSPB = [256]byte{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 1, 0, 1, 0, 1, 0, 1,
	0, 0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 1, 0, 1, 1, 1,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 1, 0, 1, 0, 1, 0, 1,
	0, 0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 1, 0, 1, 1, 1,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1,
	0, 0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0, 0, 1, 0, 1,
	1, 1, 0, 0, 1, 1, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 1, 0, 1, 0, 1,
	0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 0, 0, 1, 1, 0, 1, 0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 0, 1, 1, 1, 1, 1,
}

// ---------------------------------------------------------------------------
// Context calculation functions
// ---------------------------------------------------------------------------

// getctxnoZC returns the zero-coding context number for the given flag word
// (shifted to align the current row's bits at position 0).
func (t *T1) getctxnoZC(f uint32) byte {
	return t.lutCtxnoZC[f&t1SigmaNeighbours]
}

// getctxtnoSCorSPBIndex builds the 8-bit LUT index used for both
// sign-coding context and sign prediction.
//
// fX   = flags for the current column
// pfX  = flags for the column to the west  (flagsp[-1])
// nfX  = flags for the column to the east  (flagsp[+1])
// ci   = row within the 4-row stripe (0..3)
func getctxtnoSCorSPBIndex(fX, pfX, nfX, ci uint32) uint32 {
	lu := (fX >> (ci * 3)) & (t1Sigma1 | t1Sigma3 | t1Sigma5 | t1Sigma7)

	lu |= (pfX >> (t1ChiThisI + ci*3)) & (1 << 0)
	lu |= (nfX >> (t1ChiThisI - 2 + ci*3)) & (1 << 2)
	if ci == 0 {
		lu |= (fX >> (t1Chi0I - 4)) & (1 << 4)
	} else {
		lu |= (fX >> (t1Chi1I - 4 + (ci-1)*3)) & (1 << 4)
	}
	lu |= (fX >> (t1Chi2I - 6 + ci*3)) & (1 << 6)
	return lu
}

// getctxnoSC returns the sign-coding context number from an 8-bit index.
func getctxnoSC(lu uint32) byte {
	return lutCtxnoSC[lu]
}

// getctxnoMAG returns the magnitude refinement context number.
func getctxnoMAG(f uint32) uint32 {
	tmp := uint32(T1CtxnoMAG)
	if f&t1SigmaNeighbours != 0 {
		tmp = T1CtxnoMAG + 1
	}
	if f&t1MuThis != 0 {
		return T1CtxnoMAG + 2
	}
	return tmp
}

// getSPB returns the sign prediction bit from an 8-bit index.
func getSPB(lu uint32) uint32 {
	return uint32(lutSPB[lu])
}

// ---------------------------------------------------------------------------
// Flag update
// ---------------------------------------------------------------------------

// updateFlags is called after a sample at row ci within the stripe at
// flagsp becomes significant with sign s.  It updates the significance
// state of all 8 neighbours.
//
// flagsp is the index into t.flags for the column.
// ci is 0..3 for the row within the stripe.
// s is the sign (0 or 1).
// stride is the flags stride (t.w + 2).
// vsc is true if vertical stripe causal context applies.
func (t *T1) updateFlags(flagsp uint32, ci, s, stride uint32, vsc bool) {
	// east neighbour
	t.flags[flagsp-1] |= t1SigmaE << (3 * ci)

	// mark target as significant + record sign
	t.flags[flagsp] |= ((s << t1ChiThisI) | t1SigmaThis) << (3 * ci)

	// west neighbour
	t.flags[flagsp+1] |= t1SigmaW << (3 * ci)

	// north-west, north, north-east
	if ci == 0 && !vsc {
		north := flagsp - stride
		t.flags[north] |= (s << t1Chi5I) | t1Sigma16
		t.flags[north-1] |= t1Sigma17
		t.flags[north+1] |= t1Sigma15
	}

	// south-west, south, south-east
	if ci == 3 {
		south := flagsp + stride
		t.flags[south] |= (s << t1Chi0I) | t1Sigma1
		t.flags[south-1] |= t1Sigma2
		t.flags[south+1] |= t1Sigma0
	}
}

// updateFlagsInline is the same as updateFlags but operates on a local
// copy of the flag word (flags) so the caller can batch writes.
// It writes neighbours directly but returns the updated local flags.
func (t *T1) updateFlagsInline(flags uint32, flagsp uint32, ci, s, stride uint32, vsc bool) uint32 {
	// east neighbour
	t.flags[flagsp-1] |= t1SigmaE << (3 * ci)

	// mark target as significant + record sign
	flags |= ((s << t1ChiThisI) | t1SigmaThis) << (3 * ci)

	// west neighbour
	t.flags[flagsp+1] |= t1SigmaW << (3 * ci)

	// north-west, north, north-east
	if ci == 0 && !vsc {
		north := flagsp - stride
		t.flags[north] |= (s << t1Chi5I) | t1Sigma16
		t.flags[north-1] |= t1Sigma17
		t.flags[north+1] |= t1Sigma15
	}

	// south-west, south, south-east
	if ci == 3 {
		south := flagsp + stride
		t.flags[south] |= (s << t1Chi0I) | t1Sigma1
		t.flags[south-1] |= t1Sigma2
		t.flags[south+1] |= t1Sigma0
	}
	return flags
}

// ---------------------------------------------------------------------------
// Significance Propagation Pass  -- RAW decoder
// ---------------------------------------------------------------------------

func (t *T1) decSigpassStepRaw(flagsp uint32, datapIdx int, oneplushalf int32, vsc bool, ci uint32) {
	flags := t.flags[flagsp]

	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) == 0 &&
		(flags&(t1SigmaNeighbours<<(ci*3))) != 0 {
		if t.mqc.RawDecode() != 0 {
			v := t.mqc.RawDecode()
			if v != 0 {
				t.data[datapIdx] = -oneplushalf
			} else {
				t.data[datapIdx] = oneplushalf
			}
			t.updateFlags(flagsp, ci, v, t.w+2, vsc)
		}
		t.flags[flagsp] |= t1PiThis << (ci * 3)
	}
}

func (t *T1) decSigpassRaw(bpno int32, cblksty uint32) {
	one := int32(1) << bpno
	half := one >> 1
	oneplushalf := one | half

	flagsp := t.flagsStride + 1 // &T1_FLAGS(0,0)
	lw := t.w

	var k uint32
	for k = 0; k < (t.h & ^uint32(3)); k += 4 {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			if t.flags[fi] != 0 {
				dataBase := int(k*lw + i)
				t.decSigpassStepRaw(fi, dataBase, oneplushalf,
					cblksty&CblkstyVSC != 0, 0)
				t.decSigpassStepRaw(fi, dataBase+int(lw), oneplushalf,
					false, 1)
				t.decSigpassStepRaw(fi, dataBase+int(2*lw), oneplushalf,
					false, 2)
				t.decSigpassStepRaw(fi, dataBase+int(3*lw), oneplushalf,
					false, 3)
			}
		}
		flagsp += t.flagsStride
	}
	if k < t.h {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			for j := uint32(0); j < t.h-k; j++ {
				t.decSigpassStepRaw(fi, int((k+j)*lw+i), oneplushalf,
					cblksty&CblkstyVSC != 0, j)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Significance Propagation Pass  -- MQ decoder
// ---------------------------------------------------------------------------

// decSigpassStepMQC processes one sample in the significance propagation pass
// using the MQ decoder.  It operates on a local copy of the flags word and
// returns the (possibly updated) flags.
func (t *T1) decSigpassStepMQC(flags uint32, flagsp uint32, flagsStride uint32,
	datapBase int, dataStride uint32, ci uint32, oneplushalf int32, vsc bool) uint32 {

	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) == 0 &&
		(flags&(t1SigmaNeighbours<<(ci*3))) != 0 {

		ctxt1 := t.getctxnoZC(flags >> (ci * 3))
		t.mqc.SetCurCtx(ctxt1)
		v := t.mqc.Decode()
		if v != 0 {
			lu := getctxtnoSCorSPBIndex(flags, t.flags[flagsp-1], t.flags[flagsp+1], ci)
			ctxt2 := getctxnoSC(lu)
			spb := getSPB(lu)
			t.mqc.SetCurCtx(ctxt2)
			v = t.mqc.Decode()
			v ^= spb
			if v != 0 {
				t.data[datapBase+int(ci*dataStride)] = -oneplushalf
			} else {
				t.data[datapBase+int(ci*dataStride)] = oneplushalf
			}
			flags = t.updateFlagsInline(flags, flagsp, ci, v, flagsStride, vsc)
		}
		flags |= t1PiThis << (ci * 3)
	}
	return flags
}

// decSigpassStepMQCSlow is the non-batched version used for the trailing
// rows when h is not a multiple of 4.  It writes flags back directly.
func (t *T1) decSigpassStepMQCSlow(flagsp uint32, datapIdx int, oneplushalf int32, ci uint32, flagsStride uint32, vsc bool) {
	flags := t.flags[flagsp]

	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) == 0 &&
		(flags&(t1SigmaNeighbours<<(ci*3))) != 0 {

		ctxt1 := t.getctxnoZC(flags >> (ci * 3))
		t.mqc.SetCurCtx(ctxt1)
		v := t.mqc.Decode()
		if v != 0 {
			lu := getctxtnoSCorSPBIndex(t.flags[flagsp], t.flags[flagsp-1], t.flags[flagsp+1], ci)
			ctxt2 := getctxnoSC(lu)
			spb := getSPB(lu)
			t.mqc.SetCurCtx(ctxt2)
			v = t.mqc.Decode()
			v ^= spb
			if v != 0 {
				t.data[datapIdx] = -oneplushalf
			} else {
				t.data[datapIdx] = oneplushalf
			}
			t.updateFlags(flagsp, ci, v, flagsStride, vsc)
		}
		t.flags[flagsp] |= t1PiThis << (ci * 3)
	}
}

func (t *T1) decSigpassMQC(bpno int32, cblksty uint32) {
	one := int32(1) << bpno
	half := one >> 1
	oneplushalf := one | half
	vsc := cblksty&CblkstyVSC != 0

	lw := t.w
	flagsStride := t.w + 2
	flagsp := flagsStride + 1

	var k uint32
	for k = 0; k < (t.h & ^uint32(3)); k += 4 {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			flags := t.flags[fi]
			if flags != 0 {
				dataBase := int(k*lw + i)
				flags = t.decSigpassStepMQC(flags, fi, flagsStride,
					dataBase, lw, 0, oneplushalf, vsc)
				flags = t.decSigpassStepMQC(flags, fi, flagsStride,
					dataBase, lw, 1, oneplushalf, false)
				flags = t.decSigpassStepMQC(flags, fi, flagsStride,
					dataBase, lw, 2, oneplushalf, false)
				flags = t.decSigpassStepMQC(flags, fi, flagsStride,
					dataBase, lw, 3, oneplushalf, false)
				t.flags[fi] = flags
			}
		}
		flagsp += flagsStride
	}
	if k < t.h {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			for j := uint32(0); j < t.h-k; j++ {
				t.decSigpassStepMQCSlow(fi, int((k+j)*lw+i), oneplushalf, j, flagsStride, vsc)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Magnitude Refinement Pass  -- RAW decoder
// ---------------------------------------------------------------------------

func (t *T1) decRefpassStepRaw(flagsp uint32, datapIdx int, poshalf int32, ci uint32) {
	flags := t.flags[flagsp]
	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) == (t1SigmaThis << (ci * 3)) {
		v := t.mqc.RawDecode()
		cur := t.data[datapIdx]
		// v ^ (cur < 0)  -- if sign matches the decoded bit, add; otherwise subtract
		neg := uint32(0)
		if cur < 0 {
			neg = 1
		}
		if (v ^ neg) != 0 {
			t.data[datapIdx] = cur + poshalf
		} else {
			t.data[datapIdx] = cur - poshalf
		}
		t.flags[flagsp] |= t1MuThis << (ci * 3)
	}
}

func (t *T1) decRefpassRaw(bpno int32) {
	one := int32(1) << bpno
	poshalf := one >> 1

	lw := t.w
	flagsp := t.flagsStride + 1

	var k uint32
	for k = 0; k < (t.h & ^uint32(3)); k += 4 {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			if t.flags[fi] != 0 {
				dataBase := int(k*lw + i)
				t.decRefpassStepRaw(fi, dataBase, poshalf, 0)
				t.decRefpassStepRaw(fi, dataBase+int(lw), poshalf, 1)
				t.decRefpassStepRaw(fi, dataBase+int(2*lw), poshalf, 2)
				t.decRefpassStepRaw(fi, dataBase+int(3*lw), poshalf, 3)
			}
		}
		flagsp += t.flagsStride
	}
	if k < t.h {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			for j := uint32(0); j < t.h-k; j++ {
				t.decRefpassStepRaw(fi, int((k+j)*lw+i), poshalf, j)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Magnitude Refinement Pass  -- MQ decoder
// ---------------------------------------------------------------------------

// decRefpassStepMQC processes one sample in the refinement pass using MQ.
// Operates on a local flags copy and returns the updated flags.
func (t *T1) decRefpassStepMQC(flags uint32, datapBase int, dataStride uint32, ci uint32, poshalf int32) uint32 {
	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) == (t1SigmaThis << (ci * 3)) {
		ctxt := getctxnoMAG(flags >> (ci * 3))
		t.mqc.SetCurCtx(uint8(ctxt))
		v := t.mqc.Decode()
		idx := datapBase + int(ci*dataStride)
		cur := t.data[idx]
		neg := uint32(0)
		if cur < 0 {
			neg = 1
		}
		if (v ^ neg) != 0 {
			t.data[idx] = cur + poshalf
		} else {
			t.data[idx] = cur - poshalf
		}
		flags |= t1MuThis << (ci * 3)
	}
	return flags
}

// decRefpassStepMQCSlow is for trailing rows.
func (t *T1) decRefpassStepMQCSlow(flagsp uint32, datapIdx int, poshalf int32, ci uint32) {
	flags := t.flags[flagsp]
	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) == (t1SigmaThis << (ci * 3)) {
		ctxt := getctxnoMAG(flags >> (ci * 3))
		t.mqc.SetCurCtx(uint8(ctxt))
		v := t.mqc.Decode()
		cur := t.data[datapIdx]
		neg := uint32(0)
		if cur < 0 {
			neg = 1
		}
		if (v ^ neg) != 0 {
			t.data[datapIdx] = cur + poshalf
		} else {
			t.data[datapIdx] = cur - poshalf
		}
		t.flags[flagsp] |= t1MuThis << (ci * 3)
	}
}

func (t *T1) decRefpassMQC(bpno int32) {
	one := int32(1) << bpno
	poshalf := one >> 1

	lw := t.w
	flagsStride := t.w + 2
	flagsp := flagsStride + 1

	var k uint32
	for k = 0; k < (t.h & ^uint32(3)); k += 4 {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			flags := t.flags[fi]
			if flags != 0 {
				dataBase := int(k*lw + i)
				flags = t.decRefpassStepMQC(flags, dataBase, lw, 0, poshalf)
				flags = t.decRefpassStepMQC(flags, dataBase, lw, 1, poshalf)
				flags = t.decRefpassStepMQC(flags, dataBase, lw, 2, poshalf)
				flags = t.decRefpassStepMQC(flags, dataBase, lw, 3, poshalf)
				t.flags[fi] = flags
			}
		}
		flagsp += flagsStride
	}
	if k < t.h {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			for j := uint32(0); j < t.h-k; j++ {
				t.decRefpassStepMQCSlow(fi, int((k+j)*lw+i), poshalf, j)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Cleanup Pass
// ---------------------------------------------------------------------------

// decClnpassStep processes one sample in the cleanup pass.
// checkFlags: if true, skip if already significant or already visited.
// partial: if true, skip the zero-coding test (sample is known significant).
// Returns updated local flags.
func (t *T1) decClnpassStep(flags uint32, flagsp uint32, flagsStride uint32,
	datapBase int, dataStride uint32, ci uint32, oneplushalf int32, vsc bool,
	checkFlags bool, partial bool) uint32 {

	if checkFlags && (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) != 0 {
		return flags
	}

	if !partial {
		ctxt1 := t.getctxnoZC(flags >> (ci * 3))
		t.mqc.SetCurCtx(ctxt1)
		v := t.mqc.Decode()
		if v == 0 {
			return flags
		}
	}

	// The sample is newly significant -- decode its sign.
	lu := getctxtnoSCorSPBIndex(flags, t.flags[flagsp-1], t.flags[flagsp+1], ci)
	t.mqc.SetCurCtx(getctxnoSC(lu))
	v := t.mqc.Decode()
	v ^= getSPB(lu)
	if v != 0 {
		t.data[datapBase+int(ci*dataStride)] = -oneplushalf
	} else {
		t.data[datapBase+int(ci*dataStride)] = oneplushalf
	}
	flags = t.updateFlagsInline(flags, flagsp, ci, v, flagsStride, vsc)
	return flags
}

// decClnpassStepSlow is for the trailing rows (non-batched).
func (t *T1) decClnpassStepSlow(flagsp uint32, datapIdx int, oneplushalf int32, ci uint32, vsc bool) {
	flags := t.flags[flagsp]
	if flags&((t1SigmaThis|t1PiThis)<<(ci*3)) != 0 {
		return
	}

	ctxt1 := t.getctxnoZC(flags >> (ci * 3))
	t.mqc.SetCurCtx(ctxt1)
	v := t.mqc.Decode()
	if v == 0 {
		return
	}

	lu := getctxtnoSCorSPBIndex(t.flags[flagsp], t.flags[flagsp-1], t.flags[flagsp+1], ci)
	t.mqc.SetCurCtx(getctxnoSC(lu))
	v = t.mqc.Decode()
	v ^= getSPB(lu)
	if v != 0 {
		t.data[datapIdx] = -oneplushalf
	} else {
		t.data[datapIdx] = oneplushalf
	}
	t.updateFlags(flagsp, ci, v, t.w+2, vsc)
}

func (t *T1) decClnpass(bpno int32, cblksty uint32) {
	one := int32(1) << bpno
	half := one >> 1
	oneplushalf := one | half
	vsc := cblksty&CblkstyVSC != 0

	lw := t.w
	flagsStride := t.w + 2
	flagsp := flagsStride + 1

	var k uint32
	for k = 0; k < (t.h & ^uint32(3)); k += 4 {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			flags := t.flags[fi]

			if flags == 0 {
				// Aggregation: all 4 rows are insignificant and unvisited.
				t.mqc.SetCurCtx(T1CtxnoAGG)
				v := t.mqc.Decode()
				if v == 0 {
					continue
				}
				// Decode the run length (0..3).
				t.mqc.SetCurCtx(T1CtxnoUNI)
				rlHigh := t.mqc.Decode()
				rlLow := t.mqc.Decode()
				runlen := (rlHigh << 1) | rlLow

				dataBase := int(k*lw + i)
				partial := true

				switch runlen {
				case 0:
					flags = t.decClnpassStep(flags, fi, flagsStride,
						dataBase, lw, 0, oneplushalf, vsc, false, true)
					partial = false
					fallthrough
				case 1:
					flags = t.decClnpassStep(flags, fi, flagsStride,
						dataBase, lw, 1, oneplushalf, false, false, partial)
					partial = false
					fallthrough
				case 2:
					flags = t.decClnpassStep(flags, fi, flagsStride,
						dataBase, lw, 2, oneplushalf, false, false, partial)
					partial = false
					fallthrough
				case 3:
					flags = t.decClnpassStep(flags, fi, flagsStride,
						dataBase, lw, 3, oneplushalf, false, false, partial)
				}
			} else {
				dataBase := int(k*lw + i)
				flags = t.decClnpassStep(flags, fi, flagsStride,
					dataBase, lw, 0, oneplushalf, vsc, true, false)
				flags = t.decClnpassStep(flags, fi, flagsStride,
					dataBase, lw, 1, oneplushalf, false, true, false)
				flags = t.decClnpassStep(flags, fi, flagsStride,
					dataBase, lw, 2, oneplushalf, false, true, false)
				flags = t.decClnpassStep(flags, fi, flagsStride,
					dataBase, lw, 3, oneplushalf, false, true, false)
			}
			t.flags[fi] = flags & ^(t1Pi0 | t1Pi1 | t1Pi2 | t1Pi3)
		}
		flagsp += flagsStride
	}

	// Trailing rows (h not multiple of 4).
	if k < t.h {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			for j := uint32(0); j < t.h-k; j++ {
				t.decClnpassStepSlow(fi, int((k+j)*lw+i), oneplushalf, j, vsc)
			}
			t.flags[fi] &= ^(t1Pi0 | t1Pi1 | t1Pi2 | t1Pi3)
		}
	}

	// SEGSYM check (decode 4-bit segmentation symbol and verify == 0xa).
	if cblksty&CblkstySEGSYM != 0 {
		t.mqc.SetCurCtx(T1CtxnoUNI)
		v := t.mqc.Decode()
		v2 := t.mqc.Decode()
		v = (v << 1) | v2
		v2 = t.mqc.Decode()
		v = (v << 1) | v2
		v2 = t.mqc.Decode()
		_ = (v << 1) | v2
		// Standard says this should be 0xa; we don't error on mismatch
		// (matching openjpeg behaviour).
	}
}

// ---------------------------------------------------------------------------
// Code-block segment descriptor
// ---------------------------------------------------------------------------

// T1Seg describes one coding segment within a code-block.
type T1Seg struct {
	Data          []byte // compressed data for this segment
	Len           uint32 // byte length of compressed data
	RealNumPasses uint32 // number of coding passes in this segment
}

// ---------------------------------------------------------------------------
// Top-level code-block decode
// ---------------------------------------------------------------------------

// DecodeCodeblock decodes a single code-block.
//
// Parameters:
//   - w, h:       code-block width and height
//   - orient:     sub-band orientation (0=LL, 1=HL, 2=LH, 3=HH)
//   - roishift:   region-of-interest shift
//   - cblksty:    code-block style flags (CblkstyLAZY, CblkstyRESET, etc.)
//   - numbps:     number of bit-planes for this code-block
//   - segs:       the coding segments (each with compressed data and pass count)
//
// Returns the decoded coefficient array (row-major, size w*h) in
// sign+magnitude representation.  For lossless (qmfbid=1) decoding the
// caller should divide each sample by 2 (arithmetic right-shift with
// round-towards-zero) to obtain the final wavelet coefficient.
func (t *T1) DecodeCodeblock(w, h, orient, roishift, cblksty, numbps uint32, segs []T1Seg) ([]int32, error) {
	mqc := &t.mqc

	// Set the orient-specific ZC LUT.
	base := orient << 9
	t.lutCtxnoZC = lutCtxnoZCAll[base : base+512]

	t.allocateBuffers(w, h)

	bpnoPlusOne := int32(roishift + numbps)
	if bpnoPlusOne >= 31 {
		// Unsupported -- matches openjpeg guard.
		bpnoPlusOne = 30
	}
	passtype := uint32(2) // first pass of first bitplane is always cleanup

	mqc.ResetStates()
	mqc.SetState(T1CtxnoUNI, 0, 46)
	mqc.SetState(T1CtxnoAGG, 0, 3)
	mqc.SetState(T1CtxnoZC, 0, 4)

	for _, seg := range segs {
		if seg.Len == 0 && seg.RealNumPasses == 0 {
			continue
		}

		// Determine BYPASS mode.
		bypassType := t1TypeMQ
		if bpnoPlusOne <= int32(numbps)-4 && passtype < 2 && cblksty&CblkstyLAZY != 0 {
			bypassType = t1TypeRAW
		}

		// Prepare data buffer with sentinel bytes.
		buf := make([]byte, int(seg.Len)+cblkDataExtra)
		copy(buf, seg.Data[:seg.Len])

		if bypassType == t1TypeRAW {
			mqc.RawInitDec(buf[:seg.Len])
		} else {
			mqc.InitDec(buf[:seg.Len])
		}

		for passno := uint32(0); passno < seg.RealNumPasses && bpnoPlusOne >= 1; passno++ {
			switch passtype {
			case 0: // significance propagation
				if bypassType == t1TypeRAW {
					t.decSigpassRaw(bpnoPlusOne, cblksty)
				} else {
					t.decSigpassMQC(bpnoPlusOne, cblksty)
				}
			case 1: // magnitude refinement
				if bypassType == t1TypeRAW {
					t.decRefpassRaw(bpnoPlusOne)
				} else {
					t.decRefpassMQC(bpnoPlusOne)
				}
			case 2: // cleanup
				t.decClnpass(bpnoPlusOne, cblksty)
			}

			if cblksty&CblkstyRESET != 0 && bypassType == t1TypeMQ {
				mqc.ResetStates()
				mqc.SetState(T1CtxnoUNI, 0, 46)
				mqc.SetState(T1CtxnoAGG, 0, 3)
				mqc.SetState(T1CtxnoZC, 0, 4)
			}

			passtype++
			if passtype == 3 {
				passtype = 0
				bpnoPlusOne--
			}
		}

		mqc.FinishDec()
	}

	// Return a copy of the data so the internal buffer can be reused.
	out := make([]int32, w*h)
	copy(out, t.data)
	return out, nil
}

// =========================================================================
// T1 Encoder
// =========================================================================

// T1Encoder holds the state for tier-1 (code-block) encoding.
type T1Encoder struct {
	mqc MQEncoder

	// data holds the coefficients in row-major order. The encoder
	// stores them in signed-magnitude representation (sign in bit 31,
	// magnitude in lower bits), multiplied by 2 to match the decoder's
	// convention.
	data []int32

	// flags is the significance/state array (same layout as T1).
	flags []uint32

	w           uint32
	h           uint32
	flagsStride uint32

	// lutCtxnoZC points to the orient-specific slice of lutCtxnoZCAll.
	lutCtxnoZC []byte
}

// NewT1Encoder creates a new T1 encoder.
func NewT1Encoder() *T1Encoder {
	return &T1Encoder{}
}

func (t *T1Encoder) allocateBuffers(w, h uint32) {
	datasize := w * h
	if uint32(len(t.data)) < datasize {
		t.data = make([]int32, datasize)
	} else {
		t.data = t.data[:datasize]
		for i := range t.data {
			t.data[i] = 0
		}
	}

	flagsStride := w + 2
	flagsHeight := (h + 3) / 4
	flagsSize := (flagsHeight + 2) * flagsStride

	if uint32(len(t.flags)) < flagsSize {
		t.flags = make([]uint32, flagsSize)
	} else {
		t.flags = t.flags[:flagsSize]
		for i := range t.flags {
			t.flags[i] = 0
		}
	}

	t.w = w
	t.h = h
	t.flagsStride = flagsStride
}

// getctxnoZC returns the zero-coding context number.
func (t *T1Encoder) getctxnoZC(f uint32) byte {
	return t.lutCtxnoZC[f&t1SigmaNeighbours]
}

// updateFlags is called after a sample becomes significant.
func (t *T1Encoder) updateFlags(flagsp uint32, ci, s, stride uint32) {
	// east neighbour
	t.flags[flagsp-1] |= t1SigmaE << (3 * ci)

	// mark target as significant + record sign
	t.flags[flagsp] |= ((s << t1ChiThisI) | t1SigmaThis) << (3 * ci)

	// west neighbour
	t.flags[flagsp+1] |= t1SigmaW << (3 * ci)

	// north neighbours
	if ci == 0 {
		north := flagsp - stride
		t.flags[north] |= (s << t1Chi5I) | t1Sigma16
		t.flags[north-1] |= t1Sigma17
		t.flags[north+1] |= t1Sigma15
	}

	// south neighbours
	if ci == 3 {
		south := flagsp + stride
		t.flags[south] |= (s << t1Chi0I) | t1Sigma1
		t.flags[south-1] |= t1Sigma2
		t.flags[south+1] |= t1Sigma0
	}
}

// ---------------------------------------------------------------------------
// Encoder: Significance Propagation Pass
// ---------------------------------------------------------------------------

func (t *T1Encoder) encSigpassStep(flagsp uint32, ci uint32, datapIdx int, bpno int32) {
	flags := t.flags[flagsp]

	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) != 0 ||
		(flags&(t1SigmaNeighbours<<(ci*3))) == 0 {
		return
	}

	val := t.data[datapIdx]
	mag := val & 0x7FFFFFFF
	sigBit := uint32(0)
	if mag&(1<<uint(bpno)) != 0 {
		sigBit = 1
	}

	ctxt := t.getctxnoZC(flags >> (ci * 3))
	t.mqc.SetCurCtx(ctxt)
	t.mqc.Encode(sigBit)

	if sigBit != 0 {
		// Encode sign.
		s := uint32(0)
		if val < 0 {
			s = 1
		}
		lu := getctxtnoSCorSPBIndex(flags, t.flags[flagsp-1], t.flags[flagsp+1], ci)
		ctxt2 := getctxnoSC(lu)
		spb := getSPB(lu)
		t.mqc.SetCurCtx(ctxt2)
		t.mqc.Encode(s ^ spb)

		t.updateFlags(flagsp, ci, s, t.flagsStride)
	}
	t.flags[flagsp] |= t1PiThis << (ci * 3)
}

func (t *T1Encoder) encSigpass(bpno int32) {
	lw := t.w
	flagsp := t.flagsStride + 1

	var k uint32
	for k = 0; k < (t.h & ^uint32(3)); k += 4 {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			dataBase := int(k*lw + i)
			t.encSigpassStep(fi, 0, dataBase, bpno)
			t.encSigpassStep(fi, 1, dataBase+int(lw), bpno)
			t.encSigpassStep(fi, 2, dataBase+int(2*lw), bpno)
			t.encSigpassStep(fi, 3, dataBase+int(3*lw), bpno)
		}
		flagsp += t.flagsStride
	}
	if k < t.h {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			for j := uint32(0); j < t.h-k; j++ {
				t.encSigpassStep(fi, j, int((k+j)*lw+i), bpno)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Encoder: Magnitude Refinement Pass
// ---------------------------------------------------------------------------

func (t *T1Encoder) encRefpassStep(flagsp uint32, ci uint32, datapIdx int, bpno int32) {
	flags := t.flags[flagsp]

	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) != (t1SigmaThis << (ci * 3)) {
		return
	}

	val := t.data[datapIdx]
	mag := val & 0x7FFFFFFF
	bit := uint32(0)
	if mag&(1<<uint(bpno)) != 0 {
		bit = 1
	}

	ctxt := getctxnoMAG(flags >> (ci * 3))
	t.mqc.SetCurCtx(uint8(ctxt))
	t.mqc.Encode(bit)

	t.flags[flagsp] |= t1MuThis << (ci * 3)
}

func (t *T1Encoder) encRefpass(bpno int32) {
	lw := t.w
	flagsp := t.flagsStride + 1

	var k uint32
	for k = 0; k < (t.h & ^uint32(3)); k += 4 {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			dataBase := int(k*lw + i)
			t.encRefpassStep(fi, 0, dataBase, bpno)
			t.encRefpassStep(fi, 1, dataBase+int(lw), bpno)
			t.encRefpassStep(fi, 2, dataBase+int(2*lw), bpno)
			t.encRefpassStep(fi, 3, dataBase+int(3*lw), bpno)
		}
		flagsp += t.flagsStride
	}
	if k < t.h {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			for j := uint32(0); j < t.h-k; j++ {
				t.encRefpassStep(fi, j, int((k+j)*lw+i), bpno)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Encoder: Cleanup Pass
// ---------------------------------------------------------------------------

func (t *T1Encoder) encClnpassStep(flagsp uint32, ci uint32, datapIdx int,
	bpno int32, doZC bool) {

	flags := t.flags[flagsp]

	if (flags&((t1SigmaThis|t1PiThis)<<(ci*3))) != 0 {
		return
	}

	val := t.data[datapIdx]
	mag := val & 0x7FFFFFFF
	sigBit := uint32(0)
	if mag&(1<<uint(bpno)) != 0 {
		sigBit = 1
	}

	if doZC {
		ctxt := t.getctxnoZC(flags >> (ci * 3))
		t.mqc.SetCurCtx(ctxt)
		t.mqc.Encode(sigBit)
	}

	if sigBit != 0 {
		s := uint32(0)
		if val < 0 {
			s = 1
		}
		lu := getctxtnoSCorSPBIndex(flags, t.flags[flagsp-1], t.flags[flagsp+1], ci)
		ctxt2 := getctxnoSC(lu)
		spb := getSPB(lu)
		t.mqc.SetCurCtx(ctxt2)
		t.mqc.Encode(s ^ spb)

		t.updateFlags(flagsp, ci, s, t.flagsStride)
	}
}

func (t *T1Encoder) encClnpass(bpno int32) {
	lw := t.w
	flagsp := t.flagsStride + 1

	var k uint32
	for k = 0; k < (t.h & ^uint32(3)); k += 4 {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			flags := t.flags[fi]
			dataBase := int(k*lw + i)

			if flags == 0 {
				// Check aggregation: are all 4 samples zero at this bit plane?
				allZero := true
				for ci := uint32(0); ci < 4; ci++ {
					val := t.data[dataBase+int(ci*lw)]
					mag := val & 0x7FFFFFFF
					if mag&(1<<uint(bpno)) != 0 {
						allZero = false
						break
					}
				}
				t.mqc.SetCurCtx(T1CtxnoAGG)
				if allZero {
					t.mqc.Encode(0) // no significant sample in this stripe column
					continue
				}
				t.mqc.Encode(1) // at least one significant sample

				// Encode run length (first significant row).
				runlen := uint32(0)
				for ci := uint32(0); ci < 4; ci++ {
					val := t.data[dataBase+int(ci*lw)]
					mag := val & 0x7FFFFFFF
					if mag&(1<<uint(bpno)) != 0 {
						break
					}
					runlen++
				}
				t.mqc.SetCurCtx(T1CtxnoUNI)
				t.mqc.Encode(runlen >> 1)
				t.mqc.Encode(runlen & 1)

				for ci := runlen; ci < 4; ci++ {
					doZC := ci != runlen
					t.encClnpassStep(fi, ci, dataBase+int(ci*lw), bpno, doZC)
				}
			} else {
				for ci := uint32(0); ci < 4; ci++ {
					t.encClnpassStep(fi, ci, dataBase+int(ci*lw), bpno, true)
				}
			}
			t.flags[fi] &= ^(t1Pi0 | t1Pi1 | t1Pi2 | t1Pi3)
		}
		flagsp += t.flagsStride
	}

	// Trailing rows.
	if k < t.h {
		for i := uint32(0); i < lw; i++ {
			fi := flagsp + i
			for j := uint32(0); j < t.h-k; j++ {
				t.encClnpassStep(fi, j, int((k+j)*lw+i), bpno, true)
			}
			t.flags[fi] &= ^(t1Pi0 | t1Pi1 | t1Pi2 | t1Pi3)
		}
	}
}

// ---------------------------------------------------------------------------
// Top-level code-block encode
// ---------------------------------------------------------------------------

// EncodeCodeblockResult holds the result of encoding one code-block.
type EncodeCodeblockResult struct {
	Data      []byte // encoded byte stream
	NumPasses int    // total number of coding passes
	NumBPS    int    // number of significant bit planes
}

// EncodeCodeblock encodes a single code-block.
//
// Parameters:
//   - coeffs: wavelet coefficients for this code-block (row-major)
//   - w, h: code-block width and height
//   - orient: sub-band orientation (0=LL, 1=HL, 2=LH, 3=HH)
//   - numbps: maximum number of magnitude bit planes (Mb for the band)
//
// The coefficients should be the raw wavelet coefficients (NOT multiplied
// by 2 as in the decoder output). This function handles the *2 internally.
func (t *T1Encoder) EncodeCodeblock(coeffs []int32, w, h, orient, numbps uint32) EncodeCodeblockResult {
	mqc := &t.mqc

	// Set the orient-specific ZC LUT.
	base := orient << 9
	t.lutCtxnoZC = lutCtxnoZCAll[base : base+512]

	t.allocateBuffers(w, h)

	// Convert coefficients to signed-magnitude * 2 representation,
	// matching what the decoder produces before the /2 step.
	for y := uint32(0); y < h; y++ {
		for x := uint32(0); x < w; x++ {
			idx := y*w + x
			v := coeffs[idx]
			if v < 0 {
				// sign bit (MSB) set, magnitude * 2
				t.data[idx] = int32(uint32(1<<31) | uint32((-v)<<1))
			} else {
				t.data[idx] = v << 1
			}
		}
	}

	// Find the actual maximum number of bit planes needed.
	maxMag := int32(0)
	for _, v := range t.data[:w*h] {
		m := v & 0x7FFFFFFF
		if m > maxMag {
			maxMag = m
		}
	}
	actualBPS := uint32(0)
	{
		v := maxMag
		for v > 0 {
			actualBPS++
			v >>= 1
		}
	}
	if actualBPS > numbps {
		actualBPS = numbps
	}

	if actualBPS == 0 {
		return EncodeCodeblockResult{}
	}

	// Allocate encoder buffer -- worst case is a few bytes per sample per bitplane.
	bufSize := int(w*h)*2 + 4096
	buf := make([]byte, bufSize)
	mqc.InitEnc(buf)
	mqc.ResetEncStates()

	// Encode all coding passes. The pass order is: cleanup, then for each
	// subsequent bitplane: sigprop, refine, cleanup.  bpno decrements after
	// the cleanup pass (passtype == 2), matching the decoder convention.
	totalPasses := 0
	passtype := uint32(2) // first pass is always cleanup
	bpno := int32(actualBPS)

	for bpno >= 1 {
		switch passtype {
		case 0: // significance propagation
			t.encSigpass(bpno)
			totalPasses++
		case 1: // magnitude refinement
			t.encRefpass(bpno)
			totalPasses++
		case 2: // cleanup
			t.encClnpass(bpno)
			totalPasses++
		}

		passtype++
		if passtype == 3 {
			passtype = 0
			bpno--
		}
	}

	mqc.Flush()

	encoded := mqc.Bytes()
	result := make([]byte, len(encoded))
	copy(result, encoded)

	return EncodeCodeblockResult{
		Data:      result,
		NumPasses: totalPasses,
		NumBPS:    int(actualBPS),
	}
}
