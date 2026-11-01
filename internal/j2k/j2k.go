// Package j2k implements a pure-Go JPEG 2000 codestream decoder for the
// subset used by GRIB2. The decoder lives in decode.go (single component,
// reversible 5/3 wavelet, raw J2K codestream; see its doc comment).
//
// The implementation follows ISO 15444-1 and is modelled after OpenJPEG.
//
// Sibling files provide:
//   - mqc.go  -- MQ arithmetic coder
//   - t1.go   -- Tier-1 codeblock decoder
//   - dwt.go  -- Inverse discrete wavelet transform
package j2k

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// ---------------------------------------------------------------------------
// JPEG 2000 marker codes (ISO 15444-1 Table A.1)
// ---------------------------------------------------------------------------

const (
	markerSOC uint16 = 0xFF4F // Start of codestream
	markerSIZ uint16 = 0xFF51 // Image and tile size
	markerCOD uint16 = 0xFF52 // Coding style default
	markerQCD uint16 = 0xFF5C // Quantization default
	markerCOM uint16 = 0xFF64 // Comment
	markerSOT uint16 = 0xFF90 // Start of tile-part
	markerSOD uint16 = 0xFF93 // Start of data
	markerEOC uint16 = 0xFFD9 // End of codestream
)

// Progression orders (ISO 15444-1 Table A.16).
const (
	progLRCP = 0
	progRLCP = 1
	progRPCL = 2
	progPCRL = 3
	progCPRL = 4
)

// Quantization styles (ISO 15444-1 Table A.28).
const (
	qntstyNone  = 0 // no quantization
	qntstySIQNT = 1 // scalar derived
	qntstySQNT  = 2 // scalar expounded
)

// ---------------------------------------------------------------------------
// Parsed header structures
// ---------------------------------------------------------------------------

// sizMarker holds data extracted from the SIZ marker (Table A.9).
type sizMarker struct {
	rsiz  uint16 // capabilities
	xsiz  uint32 // image width (reference grid)
	ysiz  uint32 // image height
	x0    uint32 // horizontal offset of origin
	y0    uint32 // vertical offset of origin
	xtSiz uint32 // tile width
	ytSiz uint32 // tile height
	xt0   uint32 // horizontal offset of first tile origin
	yt0   uint32 // vertical offset of first tile origin

	ncomp uint16 // number of components (expected 1 for GRIB2)
	prec  uint8  // bit depth (Ssiz bits 0..6 + 1)
	sgnd  bool   // true if component values are signed (Ssiz bit 7)
	xrsiz uint8  // horizontal sub-sampling (expected 1)
	yrsiz uint8  // vertical sub-sampling (expected 1)
}

// codMarker holds data extracted from the COD marker (Table A.12).
type codMarker struct {
	csty      uint8  // coding style
	progOrder uint8  // progression order (SGcod A)
	numLayers uint16 // number of layers (SGcod B)
	mct       uint8  // multiple component transform (SGcod C; 0 for GRIB2)

	// SPcod parameters (Table A.13)
	numResolutions uint8   // number of decomposition levels + 1
	cblkExpnW      uint8   // code-block width exponent (value + 2)
	cblkExpnH      uint8   // code-block height exponent (value + 2)
	cblkSty        uint8   // code-block style (SPcod G)
	qmfbid         uint8   // wavelet transform: 0 = 9/7 irreversible, 1 = 5/3 reversible
	precincts      bool    // true if custom precinct sizes present
	prcW           []uint8 // per-resolution precinct width exponents
	prcH           []uint8 // per-resolution precinct height exponents
}

// qcdMarker holds data extracted from the QCD marker (Table A.27).
type qcdMarker struct {
	qntsty    uint8 // quantization style (bits 0..4)
	numGB     uint8 // number of guard bits (bits 5..7)
	stepsizes []stepSize
}

type stepSize struct {
	expn int32
	mant int32
}

// sotMarker holds data extracted from the SOT marker (Table A.14).
type sotMarker struct {
	isot  uint16 // tile index
	psot  uint32 // length of tile-part (0 means till EOC)
	tpsot uint8  // tile-part index
	tnsot uint8  // number of tile-parts
}

// ---------------------------------------------------------------------------
// Codestream state
// ---------------------------------------------------------------------------

// codestream holds the complete parsed state of a J2K codestream.
type codestream struct {
	siz sizMarker
	cod codMarker
	qcd qcdMarker
	sot sotMarker

	// tileData points to the raw data between SOD and EOC/next SOT.
	tileData []byte

	// Derived image dimensions.
	width  int
	height int
}

// ---------------------------------------------------------------------------
// Tag tree (ISO 15444-1 Annex B.10.2)
// ---------------------------------------------------------------------------

// tagNode is one node of the tag tree.
type tagNode struct {
	parent int   // index of parent (-1 for root)
	value  int32 // current known/assumed value
	low    int32 // lower bound
	known  bool  // true if value is definitively known
}

// tagTree implements the tag-tree structure used for inclusion and
// zero-bitplane coding in T2 packet headers.
type tagTree struct {
	leafsW   int
	leafsH   int
	nodes    []tagNode
	numLeafs int
}

// newTagTree creates a tag tree for a grid of leafsW x leafsH leaves.
func newTagTree(leafsW, leafsH int) *tagTree {
	if leafsW <= 0 || leafsH <= 0 {
		return &tagTree{leafsW: leafsW, leafsH: leafsH}
	}

	// Count nodes per level.
	var nplh, nplv [32]int
	nplh[0] = leafsW
	nplv[0] = leafsH
	numLevels := 0
	totalNodes := 0
	for {
		n := nplh[numLevels] * nplv[numLevels]
		totalNodes += n
		nplh[numLevels+1] = (nplh[numLevels] + 1) / 2
		nplv[numLevels+1] = (nplv[numLevels] + 1) / 2
		numLevels++
		if n <= 1 {
			break
		}
	}

	nodes := make([]tagNode, totalNodes)
	for i := range nodes {
		nodes[i].value = math.MaxInt32
		nodes[i].parent = -1
	}

	// Wire up parent pointers level by level.
	nodeIdx := 0
	parentBase := leafsW * leafsH
	for lvl := 0; lvl < numLevels-1; lvl++ {
		parentIdx := parentBase
		parentIdx0 := parentBase
		for j := 0; j < nplv[lvl]; j++ {
			k := nplh[lvl]
			for k > 0 {
				k--
				nodes[nodeIdx].parent = parentIdx
				nodeIdx++
				if k > 0 {
					k--
					nodes[nodeIdx].parent = parentIdx
					nodeIdx++
				}
				parentIdx++
			}
			if (j&1) != 0 || j == nplv[lvl]-1 {
				parentIdx0 = parentIdx
			} else {
				parentIdx = parentIdx0
				parentIdx0 += nplh[lvl+1]
			}
		}
		parentBase += nplh[lvl+1] * nplv[lvl+1]
	}

	return &tagTree{
		leafsW:   leafsW,
		leafsH:   leafsH,
		nodes:    nodes,
		numLeafs: leafsW * leafsH,
	}
}

// reset sets all nodes to their initial state.
func (t *tagTree) reset() {
	for i := range t.nodes {
		t.nodes[i].value = math.MaxInt32
		t.nodes[i].low = 0
		t.nodes[i].known = false
	}
}

// decode decodes one leaf of the tag tree up to threshold using
// the bit reader. Returns true if the leaf value < threshold.
func (t *tagTree) decode(br *bitReader, leafno int, threshold int32) bool {
	if leafno >= len(t.nodes) || leafno < 0 {
		return false
	}
	// Build stack from leaf to root.
	var stk [32]int
	stkLen := 0
	idx := leafno
	for t.nodes[idx].parent >= 0 {
		stk[stkLen] = idx
		stkLen++
		if stkLen >= len(stk) {
			break
		}
		parent := t.nodes[idx].parent
		if parent < 0 || parent >= len(t.nodes) {
			break
		}
		idx = parent
	}

	low := int32(0)
	// Limit total bit reads to prevent infinite loops on crafted streams.
	const maxIter = 100_000
	iter := 0
	for {
		node := &t.nodes[idx]
		if low > node.low {
			node.low = low
		} else {
			low = node.low
		}
		for low < threshold && low < node.value {
			if br.readBit() != 0 {
				node.value = low
			} else {
				low++
			}
			iter++
			if iter > maxIter {
				return false
			}
		}
		node.low = low
		if stkLen == 0 {
			break
		}
		stkLen--
		idx = stk[stkLen]
	}
	return t.nodes[leafno].value < threshold
}

// ---------------------------------------------------------------------------
// T2 packet header structures
// ---------------------------------------------------------------------------

// cblkInfo describes one codeblock's data within a tile.
type cblkInfo struct {
	// Position of this codeblock in the subband.
	x0, y0, x1, y1 int

	// Number of significant bitplanes (Mb - zeroBitPlanes).
	numbps int

	// Data segments for this codeblock.
	segments []cblkSegment

	// Accumulated across layers.
	numSegs       int
	numNewPasses  int
	numLenBits    int
	zeroBitPlanes int
	included      bool // ever included in any layer
}

// cblkSegment holds one coding pass segment of a code block.
type cblkSegment struct {
	newLen    int
	numPasses int
	maxPasses int
	data      []byte
}

// bandInfo describes one subband within a resolution.
type bandInfo struct {
	orient         int // 0=LL/LH, 1=HL, 2=HH (or for res 0: 0=LL)
	x0, y0, x1, y1 int
	numbps         int // number of magnitude bit planes

	// Precinct info.
	precincts []precinctInfo
}

// precinctInfo describes one precinct within a band.
type precinctInfo struct {
	cw, ch   int // codeblock grid size
	cblks    []cblkInfo
	inclTree *tagTree
	imsbTree *tagTree
}

// resolutionInfo describes one resolution level.
type resolutionInfo struct {
	// Bounds on reference grid.
	x0, y0, x1, y1 int
	// Precinct grid.
	pw, ph int
	bands  []bandInfo
}

// tileComp holds the decoded tile component.
type tileComp struct {
	x0, y0, x1, y1 int
	resolutions    []resolutionInfo
	data           []int32 // decoded coefficient/sample buffer
}

// ---------------------------------------------------------------------------
// Binary helpers
// ---------------------------------------------------------------------------

func readUint16(data []byte, off int) uint16 {
	return binary.BigEndian.Uint16(data[off:])
}

func readUint32(data []byte, off int) uint32 {
	return binary.BigEndian.Uint32(data[off:])
}

// ---------------------------------------------------------------------------
// Resolution / subband / codeblock geometry helpers
// ---------------------------------------------------------------------------

func ceildivpow2(a, b int) int {
	return (a + (1 << uint(b)) - 1) >> uint(b)
}

func ceildiv(a, b int) int {
	if b == 0 {
		return 0
	}
	return (a + b - 1) / b
}

func floorDiv(a, b int) int {
	if b == 0 {
		return 0
	}
	return a / b
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// setupTileComponent computes the resolution/band/precinct/codeblock
// geometry for a single tile-component (the only one in GRIB2).
func setupTileComponent(cs *codestream) *tileComp {
	tc := &tileComp{}
	siz := &cs.siz
	cod := &cs.cod
	qcd := &cs.qcd

	// Tile bounds (single tile = entire image for GRIB2).
	tc.x0 = int(siz.x0)
	tc.y0 = int(siz.y0)
	tc.x1 = int(siz.xsiz)
	tc.y1 = int(siz.ysiz)

	numRes := int(cod.numResolutions)
	numDecomp := numRes - 1

	tc.resolutions = make([]resolutionInfo, numRes)

	cbW := int(cod.cblkExpnW) // code-block width exponent
	cbH := int(cod.cblkExpnH) // code-block height exponent

	for r := 0; r < numRes; r++ {
		res := &tc.resolutions[r]
		pdx := 15 // default precinct width exponent
		pdy := 15 // default precinct height exponent
		if cod.precincts && r < len(cod.prcW) {
			pdx = int(cod.prcW[r])
			pdy = int(cod.prcH[r])
		}

		// Resolution bounds (ISO 15444-1 eq. B-11).
		level := numDecomp - r
		res.x0 = ceildivpow2(tc.x0, level)
		res.y0 = ceildivpow2(tc.y0, level)
		res.x1 = ceildivpow2(tc.x1, level)
		res.y1 = ceildivpow2(tc.y1, level)

		// Precinct grid.
		if res.x1 > res.x0 {
			res.pw = ceildiv(res.x1, 1<<uint(pdx)) - floorDiv(res.x0, 1<<uint(pdx))
		}
		if res.y1 > res.y0 {
			res.ph = ceildiv(res.y1, 1<<uint(pdy)) - floorDiv(res.y0, 1<<uint(pdy))
		}

		// Bands.
		if r == 0 {
			// Resolution 0: single LL band.
			res.bands = make([]bandInfo, 1)
			b := &res.bands[0]
			b.orient = 0
			b.x0 = res.x0
			b.y0 = res.y0
			b.x1 = res.x1
			b.y1 = res.y1
			b.numbps = computeBandBitPlanes(qcd, 0, int(siz.prec))
			setupPrecincts(b, res, cbW, cbH, pdx, pdy)
		} else {
			// Resolutions >= 1: three subbands HL, LH, HH.
			res.bands = make([]bandInfo, 3)
			for orient := 0; orient < 3; orient++ {
				b := &res.bands[orient]
				b.orient = orient + 1 // 1=HL, 2=LH, 3=HH

				// Band index in the global step-size array.
				bandIdx := 1 + (r-1)*3 + orient

				b.numbps = computeBandBitPlanes(qcd, bandIdx, int(siz.prec))

				// Band bounds (ISO 15444-1 eq. B-15 through B-18).
				// bandno = orient+1 (1=HL, 2=LH, 3=HH)
				// x0b = bandno & 1  (1 for HL, 0 for LH, 1 for HH)
				// y0b = bandno >> 1 (0 for HL, 1 for LH, 1 for HH)
				bandno := orient + 1
				x0b := bandno & 1
				y0b := bandno >> 1
				b.x0 = ceildivpow2(tc.x0-(x0b<<uint(level)), level+1)
				b.y0 = ceildivpow2(tc.y0-(y0b<<uint(level)), level+1)
				b.x1 = ceildivpow2(tc.x1-(x0b<<uint(level)), level+1)
				b.y1 = ceildivpow2(tc.y1-(y0b<<uint(level)), level+1)
				setupPrecincts(b, res, cbW, cbH, pdx, pdy)
			}
		}
	}

	return tc
}

// computeBandBitPlanes computes the number of magnitude bit planes for a
// subband given the quantization step-size exponent and guard bits.
// This is the Mb value from Equation E-2 in ISO 15444-1:
//
//	Mb = epsilon_b + numgbits - 1
//
// where epsilon_b is the step-size exponent for the band.
func computeBandBitPlanes(qcd *qcdMarker, bandIdx int, prec int) int {
	if bandIdx < len(qcd.stepsizes) {
		expn := int(qcd.stepsizes[bandIdx].expn)
		return expn + int(qcd.numGB) - 1
	}
	// Fallback: derive from precision.
	return prec
}

// setupPrecincts creates the precinct and codeblock grid for one band.
func setupPrecincts(b *bandInfo, res *resolutionInfo, cbW, cbH, pdx, pdy int) {
	nprc := res.pw * res.ph
	if nprc <= 0 {
		return
	}
	b.precincts = make([]precinctInfo, nprc)

	// Effective codeblock size exponents (cannot exceed precinct).
	cbExpW := imin(cbW, pdx)
	cbExpH := imin(cbH, pdy)
	if b.orient != 0 {
		// For sub-bands at resolution > 0, precinct is half as big.
		cbExpW = imin(cbW, imax(pdx-1, 0))
		cbExpH = imin(cbH, imax(pdy-1, 0))
	}

	bandW := b.x1 - b.x0
	bandH := b.y1 - b.y0
	if bandW <= 0 || bandH <= 0 {
		return
	}

	// Codeblock grid over the band.
	totalCBW := ceildiv(bandW, 1<<uint(cbExpW))
	totalCBH := ceildiv(bandH, 1<<uint(cbExpH))

	for prcIdx := 0; prcIdx < nprc; prcIdx++ {
		prc := &b.precincts[prcIdx]

		// For GRIB2 with large (default 2^15) precincts, typically there
		// is only one precinct per band.
		if nprc == 1 {
			prc.cw = totalCBW
			prc.ch = totalCBH
		} else {
			// Compute precinct col/row.
			prcX := prcIdx % res.pw
			prcY := prcIdx / res.pw

			// Precinct boundaries in band coordinates.
			prcSize := 1 << uint(pdx)
			if b.orient != 0 {
				prcSize = 1 << uint(pdx-1)
			}
			prcSizeH := 1 << uint(pdy)
			if b.orient != 0 {
				prcSizeH = 1 << uint(pdy-1)
			}

			px0 := prcX * prcSize
			py0 := prcY * prcSizeH
			px1 := imin((prcX+1)*prcSize, bandW)
			py1 := imin((prcY+1)*prcSizeH, bandH)

			if px1 <= px0 || py1 <= py0 {
				continue
			}
			prc.cw = ceildiv(px1, 1<<uint(cbExpW)) - floorDiv(px0, 1<<uint(cbExpW))
			prc.ch = ceildiv(py1, 1<<uint(cbExpH)) - floorDiv(py0, 1<<uint(cbExpH))
		}

		nblocks := prc.cw * prc.ch
		prc.cblks = make([]cblkInfo, nblocks)
		prc.inclTree = newTagTree(prc.cw, prc.ch)
		prc.imsbTree = newTagTree(prc.cw, prc.ch)

		// Fill in codeblock bounds.
		cbSize := 1 << uint(cbExpW)
		cbSizeH := 1 << uint(cbExpH)

		// Compute the precinct's starting codeblock index in the band's
		// codeblock grid so that bounds are relative to the band origin.
		prcCBX0 := 0
		prcCBY0 := 0
		if nprc > 1 {
			prcX := prcIdx % res.pw
			prcY := prcIdx / res.pw
			prcSize := 1 << uint(pdx)
			prcSizeH := 1 << uint(pdy)
			if b.orient != 0 {
				prcSize = 1 << uint(pdx-1)
				prcSizeH = 1 << uint(pdy-1)
			}
			prcCBX0 = floorDiv(prcX*prcSize, cbSize)
			prcCBY0 = floorDiv(prcY*prcSizeH, cbSizeH)
		}

		for cbIdx := 0; cbIdx < nblocks; cbIdx++ {
			cx := prcCBX0 + cbIdx%prc.cw
			cy := prcCBY0 + cbIdx/prc.cw
			cb := &prc.cblks[cbIdx]
			cb.x0 = b.x0 + cx*cbSize
			cb.y0 = b.y0 + cy*cbSizeH
			cb.x1 = imin(cb.x0+cbSize, b.x1)
			cb.y1 = imin(cb.y0+cbSizeH, b.y1)
			cb.numLenBits = 3
		}
	}
}

// ---------------------------------------------------------------------------
// T2: Tier-2 packet parsing (ISO 15444-1 Annex B.10)
// ---------------------------------------------------------------------------

// getNumPasses reads the variable-length code for the number of coding
// passes (ISO 15444-1 Table B.3).
func getNumPasses(br *bitReader) int {
	if br.readBit() == 0 {
		return 1
	}
	if br.readBit() == 0 {
		return 2
	}
	v := br.readBits(2)
	if v != 3 {
		return int(3 + v)
	}
	v = br.readBits(5)
	if v != 31 {
		return int(6 + v)
	}
	return int(37 + br.readBits(7))
}

// getCommaCode reads a comma code (unary code of 1-bits terminated by a 0).
// The iteration is capped to prevent infinite loops on crafted input.
func getCommaCode(br *bitReader) int {
	n := 0
	// Cap at 64 to prevent infinite loops on malformed streams that
	// consist entirely of 1-bits (the bit reader returns 0xFF padding
	// once data is exhausted, so readBit never returns 0).
	const maxCommaLen = 64
	for br.readBit() != 0 {
		n++
		if n >= maxCommaLen {
			break
		}
	}
	return n
}

// floorLog2 returns the floor of log2(n) for n > 0.
func floorLog2(n int) int {
	if n <= 0 {
		return 0
	}
	r := 0
	v := n
	for v > 1 {
		v >>= 1
		r++
	}
	return r
}

// =========================================================================
// Encoder
// =========================================================================

// encodedCblk holds the result of encoding one code-block.
type encodedCblk struct {
	result EncodeCodeblockResult
	zbp    int // zero bit planes
}

// Encode encodes samples into a raw JPEG 2000 codestream (the GRIB2 subset).
//
// Parameters:
//   - samples: integer sample values (row-major, width*height elements)
//   - width, height: image dimensions
//   - prec: bit depth (precision) of the samples
//
// Returns the encoded J2K codestream bytes.
//
// This encoder supports: single component, single tile, lossless only,
// reversible 5/3 wavelet, LRCP progression, single layer, no MCT.
func Encode(samples []int32, width, height, prec int) ([]byte, error) {
	// Choose number of decomposition levels based on image size.
	numDecomp := 5
	minDim := width
	if height < minDim {
		minDim = height
	}
	for numDecomp > 0 && (1<<numDecomp) > minDim {
		numDecomp--
	}
	return EncodeWithOptions(samples, width, height, prec, numDecomp)
}

// EncodeWithOptions encodes samples into a raw JPEG 2000 codestream
// with explicit control over the number of decomposition levels.
//
// Samples must lie in [0, 2^prec-1] and prec must be in 1..31. An error is
// returned (rather than a corrupt codestream) when the wavelet
// coefficients would need 31 or more magnitude bit-planes, which no
// conforming decoder (OpenJPEG included) can reconstruct; this can only
// happen at precisions of 29 bits and above.
func EncodeWithOptions(samples []int32, width, height, prec, numDecomp int) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("j2k: zero or negative image dimensions")
	}
	if prec < 1 || prec > maxPrec {
		return nil, fmt.Errorf("j2k: precision %d out of range 1..%d", prec, maxPrec)
	}
	if numDecomp < 0 || numDecomp > 32 {
		return nil, fmt.Errorf("j2k: %d decomposition levels out of range 0..32", numDecomp)
	}
	if int64(width)*int64(height) > maxPixels {
		return nil, fmt.Errorf("j2k: image size %d x %d exceeds maximum %d pixels", width, height, maxPixels)
	}
	if len(samples) < width*height {
		return nil, errors.New("j2k: samples buffer shorter than width*height")
	}
	n := width * height
	numRes := numDecomp + 1

	// Code-block size exponents.
	cbExpW := 6 // 64
	cbExpH := 6 // 64

	// DC level shift (unsigned component), then the forward DWT in int64
	// so that the lifting steps cannot overflow.
	maxVal := int64(1)<<uint(prec) - 1
	dcShift := int64(1) << uint(prec-1)
	c64 := make([]int64, n)
	for i, v := range samples[:n] {
		if v < 0 || int64(v) > maxVal {
			return nil, fmt.Errorf("j2k: sample %d = %d outside [0, %d] for precision %d", i, v, maxVal, prec)
		}
		c64[i] = int64(v) - dcShift
	}
	if err := forwardDWT53(c64, width, height, numRes); err != nil {
		return nil, fmt.Errorf("j2k: forward DWT failed: %w", err)
	}
	// The T1 coder (here and in OpenJPEG) holds magnitude plus one
	// fractional bit in an int32 and decoders reject 31+ bit-planes.
	const maxMag = int64(1)<<30 - 1
	coeffs := make([]int32, n)
	for i, v := range c64 {
		if v > maxMag || v < -maxMag {
			return nil, fmt.Errorf("j2k: precision %d: wavelet coefficient %d needs more than 30 magnitude bit-planes", prec, v)
		}
		coeffs[i] = int32(v)
	}

	// Build a codestream structure to reuse setupTileComponent.
	cs := &codestream{
		siz: sizMarker{
			rsiz:  0,
			xsiz:  uint32(width),
			ysiz:  uint32(height),
			x0:    0,
			y0:    0,
			xtSiz: uint32(width),
			ytSiz: uint32(height),
			xt0:   0,
			yt0:   0,
			ncomp: 1,
			prec:  uint8(prec),
			sgnd:  false,
			xrsiz: 1,
			yrsiz: 1,
		},
		cod: codMarker{
			csty:           0,
			progOrder:      progLRCP,
			numLayers:      1,
			mct:            0,
			numResolutions: uint8(numRes),
			cblkExpnW:      uint8(cbExpW),
			cblkExpnH:      uint8(cbExpH),
			cblkSty:        0,
			qmfbid:         1,
			precincts:      false,
		},
		width:  width,
		height: height,
	}

	// Build QCD: reversible (no quantization). For the reversible 5/3
	// transform the epsilon_b values follow OpenJPEG: prec + gain, with
	// gain 0 for LL, 1 for HL/LH and 2 for HH. The guard bits are chosen
	// below so that every code-block's bit-planes fit in Mb.
	numBands := 1 + 3*numDecomp
	cs.qcd.qntsty = qntstyNone
	cs.qcd.numGB = 2
	cs.qcd.stepsizes = make([]stepSize, numBands)
	for i := 0; i < numBands; i++ {
		var gain int
		if i > 0 {
			switch (i-1)%3 + 1 { // 1=HL, 2=LH, 3=HH
			case 1, 2:
				gain = 1
			case 3:
				gain = 2
			}
		}
		// The exponent field is 5 bits; any shortfall is made up by the
		// guard bits.
		cs.qcd.stepsizes[i] = stepSize{expn: int32(imin(prec+gain, 31))}
	}

	tc := setupTileComponent(cs)

	// Extract coefficients for each codeblock and encode.
	t1enc := NewT1Encoder()
	cblkResults := make(map[*cblkInfo]encodedCblk)
	type cblkBand struct {
		cb   *cblkInfo
		expn int
	}
	var order []cblkBand
	for r := 0; r < numRes; r++ {
		res := &tc.resolutions[r]
		for bandno := 0; bandno < len(res.bands); bandno++ {
			band := &res.bands[bandno]
			if band.x1-band.x0 <= 0 || band.y1-band.y0 <= 0 {
				continue
			}
			bandIdx := 0
			if r > 0 {
				bandIdx = 1 + (r-1)*3 + bandno
			}
			expn := int(cs.qcd.stepsizes[bandIdx].expn)
			for prcIdx := range band.precincts {
				prc := &band.precincts[prcIdx]
				for cblkIdx := range prc.cblks {
					cb := &prc.cblks[cblkIdx]
					cbW := cb.x1 - cb.x0
					cbH := cb.y1 - cb.y0
					if cbW <= 0 || cbH <= 0 {
						continue
					}
					cbCoeffs := extractCoefficients(coeffs, width, height,
						cb, band, numDecomp, r)
					result := t1enc.EncodeCodeblock(cbCoeffs,
						uint32(cbW), uint32(cbH),
						uint32(band.orient), 30)
					cblkResults[cb] = encodedCblk{result: result}
					order = append(order, cblkBand{cb, expn})
				}
			}
		}
	}

	// Guard bits: Mb = expn + G - 1 must cover every code-block.
	for _, o := range order {
		if need := cblkResults[o.cb].result.NumBPS - o.expn + 1; need > int(cs.qcd.numGB) {
			cs.qcd.numGB = uint8(need)
		}
	}
	if cs.qcd.numGB > 7 {
		return nil, fmt.Errorf("j2k: precision %d: coefficients need %d guard bits (maximum 7)", prec, cs.qcd.numGB)
	}
	for _, o := range order {
		e := cblkResults[o.cb]
		e.zbp = o.expn + int(cs.qcd.numGB) - 1 - e.result.NumBPS
		cblkResults[o.cb] = e
	}

	// Now write the codestream.
	out := newBitWriter(width*height*4 + 4096) // generous initial capacity

	out.writeUint16(markerSOC)
	writeSIZMarker(out, cs)
	writeCODMarker(out, cs)
	writeQCDMarker(out, cs)

	// SOT. Psot is patched once the tile-part length is known: Jasper
	// (wgrib2, g2clib and the default eccodes build) rejects Psot=0.
	sotStart := out.pos()
	out.writeUint16(markerSOT)
	out.writeUint16(10) // Lsot
	out.writeUint16(0)  // Isot (tile index)
	psotPos := out.pos()
	out.writeUint32(0) // Psot, patched below
	out.writeByte(0)   // TPsot
	out.writeByte(1)   // TNsot

	out.writeUint16(markerSOD)

	// T2: write packets (single layer, LRCP).
	for r := 0; r < numRes; r++ {
		res := &tc.resolutions[r]
		nprc := res.pw * res.ph
		for prcno := 0; prcno < nprc; prcno++ {
			writePacket(out, res, prcno, cblkResults)
		}
	}
	binary.BigEndian.PutUint32(out.data[psotPos:], uint32(out.pos()-sotStart))

	out.writeUint16(markerEOC)

	return out.bytes(), nil
}

// extractCoefficients extracts a codeblock's coefficients from the full
// coefficient buffer. This is the reverse of placeCoefficients.
func extractCoefficients(coeffs []int32, stride, totalH int,
	cb *cblkInfo, band *bandInfo, numDecomp, resno int) []int32 {

	cbW := cb.x1 - cb.x0
	cbH := cb.y1 - cb.y0

	var offX, offY int
	if resno == 0 {
		offX = 0
		offY = 0
	} else {
		level := numDecomp - resno
		halfW := ceildivpow2(stride, level+1)
		halfH := ceildivpow2(totalH, level+1)
		switch band.orient {
		case 1: // HL
			offX = halfW
			offY = 0
		case 2: // LH
			offX = 0
			offY = halfH
		case 3: // HH
			offX = halfW
			offY = halfH
		}
	}

	relX := cb.x0 - band.x0
	relY := cb.y0 - band.y0

	result := make([]int32, cbW*cbH)
	for y := 0; y < cbH; y++ {
		dy := offY + relY + y
		if dy >= totalH {
			break
		}
		for x := 0; x < cbW; x++ {
			dx := offX + relX + x
			if dx >= stride {
				break
			}
			result[y*cbW+x] = coeffs[dy*stride+dx]
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Bit writer for T2 packet headers and codestream construction
// ---------------------------------------------------------------------------

type bitWriter struct {
	data   []byte
	wpos   int    // byte write position
	bitBuf uint32 // bit accumulator
	bitCt  int    // bits in accumulator (0..8)
}

func newBitWriter(initialCap int) *bitWriter {
	return &bitWriter{data: make([]byte, 0, initialCap)}
}

func (bw *bitWriter) pos() int {
	return bw.wpos
}

func (bw *bitWriter) bytes() []byte {
	return bw.data[:bw.wpos]
}

func (bw *bitWriter) grow(n int) {
	need := bw.wpos + n
	if need <= len(bw.data) {
		return
	}
	if need <= cap(bw.data) {
		bw.data = bw.data[:need]
		return
	}
	newCap := cap(bw.data) * 2
	if newCap < need {
		newCap = need
	}
	nd := make([]byte, newCap)
	copy(nd, bw.data[:bw.wpos])
	bw.data = nd
}

func (bw *bitWriter) writeByte(b byte) {
	bw.grow(1)
	bw.data[bw.wpos] = b
	bw.wpos++
}

func (bw *bitWriter) writeUint16(v uint16) {
	bw.grow(2)
	binary.BigEndian.PutUint16(bw.data[bw.wpos:], v)
	bw.wpos += 2
}

func (bw *bitWriter) writeUint32(v uint32) {
	bw.grow(4)
	binary.BigEndian.PutUint32(bw.data[bw.wpos:], v)
	bw.wpos += 4
}

func (bw *bitWriter) writeBytes(b []byte) {
	bw.grow(len(b))
	copy(bw.data[bw.wpos:], b)
	bw.wpos += len(b)
}

// writeBit writes a single bit to the bit-oriented output.
func (bw *bitWriter) writeBit(bit uint32) {
	if bw.bitCt == 0 {
		bw.bitCt = 8
		bw.bitBuf = 0
	}
	bw.bitCt--
	bw.bitBuf |= (bit & 1) << uint(bw.bitCt)
	if bw.bitCt == 0 {
		bw.writeByte(byte(bw.bitBuf))
		// 0xFF bit-stuffing: after writing 0xFF, next byte uses only 7 bits.
		if byte(bw.bitBuf) == 0xFF {
			bw.bitCt = 7
		} else {
			bw.bitCt = 8
		}
		bw.bitBuf = 0
	}
}

// writeBits writes n bits (MSB first).
func (bw *bitWriter) writeBits(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		bw.writeBit((v >> uint(i)) & 1)
	}
}

// flushBits byte-aligns the bit writer.
func (bw *bitWriter) flushBits() {
	if bw.bitCt > 0 && bw.bitCt < 8 {
		bw.writeByte(byte(bw.bitBuf))
		// If the last flushed byte is 0xFF, write a stuffing byte.
		if byte(bw.bitBuf) == 0xFF {
			bw.writeByte(0)
		}
	}
	bw.bitCt = 0
	bw.bitBuf = 0
}

// ---------------------------------------------------------------------------
// Marker writing helpers
// ---------------------------------------------------------------------------

func writeSIZMarker(out *bitWriter, cs *codestream) {
	siz := &cs.siz
	lsiz := 38 + int(siz.ncomp)*3 // Lsiz includes itself (2 bytes)
	out.writeUint16(markerSIZ)
	out.writeUint16(uint16(lsiz)) // Lsiz
	out.writeUint16(siz.rsiz)     // Rsiz
	out.writeUint32(siz.xsiz)     // Xsiz
	out.writeUint32(siz.ysiz)     // Ysiz
	out.writeUint32(siz.x0)       // XOsiz
	out.writeUint32(siz.y0)       // YOsiz
	out.writeUint32(siz.xtSiz)    // XTsiz
	out.writeUint32(siz.ytSiz)    // YTsiz
	out.writeUint32(siz.xt0)      // XTOsiz
	out.writeUint32(siz.yt0)      // YTOsiz
	out.writeUint16(siz.ncomp)    // Csiz

	// Component parameters.
	ssiz := uint8(siz.prec - 1)
	if siz.sgnd {
		ssiz |= 0x80
	}
	out.writeByte(ssiz)      // Ssiz
	out.writeByte(siz.xrsiz) // XRsiz
	out.writeByte(siz.yrsiz) // YRsiz
}

func writeCODMarker(out *bitWriter, cs *codestream) {
	cod := &cs.cod
	bodyLen := 12 // Lcod(2) + Scod(1) + SGcod(4) + SPcod(5)
	out.writeUint16(markerCOD)
	out.writeUint16(uint16(bodyLen))      // Lcod
	out.writeByte(cod.csty)               // Scod
	out.writeByte(cod.progOrder)          // SGcod A: progression order
	out.writeUint16(cod.numLayers)        // SGcod B: number of layers
	out.writeByte(cod.mct)                // SGcod C: MCT
	out.writeByte(cod.numResolutions - 1) // SPcod A: number of decomposition levels
	out.writeByte(cod.cblkExpnW - 2)      // SPcod B: code-block width exponent
	out.writeByte(cod.cblkExpnH - 2)      // SPcod C: code-block height exponent
	out.writeByte(cod.cblkSty)            // SPcod D: code-block style
	out.writeByte(cod.qmfbid)             // SPcod E: wavelet transform
}

func writeQCDMarker(out *bitWriter, cs *codestream) {
	qcd := &cs.qcd
	var bodyLen int
	switch qcd.qntsty {
	case qntstyNone:
		bodyLen = 3 + len(qcd.stepsizes) // Lqcd(2) + Sqcd(1) + 1 byte per step
	default:
		bodyLen = 3 + len(qcd.stepsizes)*2
	}
	out.writeUint16(markerQCD)
	out.writeUint16(uint16(bodyLen))

	sqcd := qcd.qntsty | (qcd.numGB << 5)
	out.writeByte(sqcd)

	for _, ss := range qcd.stepsizes {
		switch qcd.qntsty {
		case qntstyNone:
			out.writeByte(byte(ss.expn << 3))
		default:
			val := uint16(ss.expn<<11) | uint16(ss.mant&0x7FF)
			out.writeUint16(val)
		}
	}
}

// ---------------------------------------------------------------------------
// T2: Packet writing
// ---------------------------------------------------------------------------

// tagTreeEncode encodes a tag tree leaf value up to a threshold.
// This must produce bits that the decoder's decode() method will
// interpret correctly.
//
// The decoder's algorithm (from tagTree.decode):
//   - Walk from leaf to root, build stack
//   - From root to leaf:
//   - Sync low with node.low
//   - While low < threshold && low < node.value: read bit
//   - If bit == 1: node.value = low (value found at this level)
//   - If bit == 0: low++ (value is higher)
//   - Update node.low = low
//
// The encoder writes the bits that the decoder expects to read.
func tagTreeEncode(bw *bitWriter, tt *tagTree, leafno int, threshold int32) {
	if leafno >= len(tt.nodes) {
		return
	}

	// Build stack from leaf to root (same as decoder).
	var stk [32]int
	stkLen := 0
	idx := leafno
	for tt.nodes[idx].parent >= 0 {
		stk[stkLen] = idx
		stkLen++
		idx = tt.nodes[idx].parent
	}

	low := int32(0)
	for {
		node := &tt.nodes[idx]
		if low > node.low {
			node.low = low
		} else {
			low = node.low
		}

		for low < threshold {
			if low >= node.value {
				if !node.known {
					bw.writeBit(1)
					node.known = true
				}
				break
			}
			bw.writeBit(0)
			low++
		}
		node.low = low

		if stkLen == 0 {
			break
		}
		stkLen--
		idx = stk[stkLen]
	}
}

// writeNumPasses writes the variable-length code for the number of passes.
func writeNumPasses(bw *bitWriter, numPasses int) {
	if numPasses == 1 {
		bw.writeBit(0)
	} else if numPasses == 2 {
		bw.writeBit(1)
		bw.writeBit(0)
	} else if numPasses <= 5 {
		bw.writeBit(1)
		bw.writeBit(1)
		bw.writeBits(uint32(numPasses-3), 2)
	} else if numPasses <= 36 {
		bw.writeBit(1)
		bw.writeBit(1)
		bw.writeBits(3, 2)
		bw.writeBits(uint32(numPasses-6), 5)
	} else {
		bw.writeBit(1)
		bw.writeBit(1)
		bw.writeBits(3, 2)
		bw.writeBits(31, 5)
		bw.writeBits(uint32(numPasses-37), 7)
	}
}

// writeCommaCode writes a comma code (unary) for the given value.
func writeCommaCode(bw *bitWriter, n int) {
	for i := 0; i < n; i++ {
		bw.writeBit(1)
	}
	bw.writeBit(0)
}

func writePacket(out *bitWriter, res *resolutionInfo, prcno int,
	cblkResults map[*cblkInfo]encodedCblk) {

	// Start bit-oriented header.
	out.bitCt = 0
	out.bitBuf = 0

	// Packet present bit = 1.
	out.writeBit(1)

	// First pass: set up tag tree values for all codeblocks in all bands
	// for this precinct.
	for bandno := 0; bandno < len(res.bands); bandno++ {
		band := &res.bands[bandno]
		if prcno >= len(band.precincts) {
			continue
		}
		prc := &band.precincts[prcno]
		if band.x1-band.x0 <= 0 || band.y1-band.y0 <= 0 {
			continue
		}

		nblocks := prc.cw * prc.ch
		for cblkno := 0; cblkno < nblocks; cblkno++ {
			cb := &prc.cblks[cblkno]
			enc, ok := cblkResults[cb]
			if !ok || len(enc.result.Data) == 0 {
				// Not included in this (only) layer.
				prc.inclTree.nodes[cblkno].value = math.MaxInt32
				prc.imsbTree.nodes[cblkno].value = 0
			} else {
				// Included in layer 0.
				prc.inclTree.nodes[cblkno].value = 0
				prc.imsbTree.nodes[cblkno].value = int32(enc.zbp)
			}
		}

		// Propagate tag tree values from leaves to internal nodes.
		propagateTagTree(prc.inclTree)
		propagateTagTree(prc.imsbTree)
	}

	// Collect refs for the body.
	type cblkRef struct {
		cb  *cblkInfo
		enc encodedCblk
	}
	var refs []cblkRef

	// Second pass: encode the packet header for each band/codeblock.
	for bandno := 0; bandno < len(res.bands); bandno++ {
		band := &res.bands[bandno]
		if prcno >= len(band.precincts) {
			continue
		}
		prc := &band.precincts[prcno]
		if band.x1-band.x0 <= 0 || band.y1-band.y0 <= 0 {
			continue
		}

		nblocks := prc.cw * prc.ch
		for cblkno := 0; cblkno < nblocks; cblkno++ {
			cb := &prc.cblks[cblkno]
			enc, ok := cblkResults[cb]
			included := ok && len(enc.result.Data) > 0

			// Inclusion tag tree: encode whether this codeblock is included
			// for the first time in layer <= 0. The decoder calls
			// prc.inclTree.decode(br, cblkno, layno+1) with layno=0,
			// so threshold=1. We need to encode the value so that
			// decode returns true (value < threshold) for included blocks.
			tagTreeEncode(out, prc.inclTree, cblkno, 1)

			if !included {
				continue
			}

			// Zero bit-planes via imsb tag tree.
			// The decoder reads: zeroBP starts at 0, increments until
			// prc.imsbTree.decode(br, cblkno, zeroBP) returns true.
			// So we need to encode so that decode returns false for
			// thresholds 0..zbp-1 and true for threshold zbp.
			zbp := int32(enc.zbp)
			tagTreeEncode(out, prc.imsbTree, cblkno, zbp+1)

			// Number of coding passes.
			writeNumPasses(out, enc.result.NumPasses)

			// Length indicator increment (comma code).
			dataLen := len(enc.result.Data)
			numLenBits := 3
			for (1 << numLenBits) <= dataLen {
				numLenBits++
			}
			increment := numLenBits - 3
			writeCommaCode(out, increment)

			// Segment length.
			// The decoder reads bitsToRead = numLenBits + floorLog2(passesInSeg).
			// For single segment: passesInSeg = numPasses, maxPasses = 109.
			bitsToRead := numLenBits + floorLog2(enc.result.NumPasses)
			out.writeBits(uint32(dataLen), bitsToRead)

			refs = append(refs, cblkRef{cb: cb, enc: enc})
		}
	}

	// Byte-align the header.
	out.flushBits()

	// Write packet body: raw codeblock data.
	for _, ref := range refs {
		out.writeBytes(ref.enc.result.Data)
	}
}

// propagateTagTree sets internal node values to the minimum of their
// children's values.
func propagateTagTree(tt *tagTree) {
	if len(tt.nodes) == 0 {
		return
	}
	// Process from leaves toward root: each node's parent gets
	// the minimum of itself and the child.
	for i := 0; i < len(tt.nodes); i++ {
		if tt.nodes[i].parent >= 0 {
			p := tt.nodes[i].parent
			if tt.nodes[i].value < tt.nodes[p].value {
				tt.nodes[p].value = tt.nodes[i].value
			}
		}
	}
}
