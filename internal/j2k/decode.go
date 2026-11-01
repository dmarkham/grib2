package j2k

// JPEG 2000 Part 1 codestream decoder.
//
// Supported: one component (any precision 1..31, signed or unsigned,
// component sub-sampling), the reversible 5/3 wavelet, any number of
// tiles and tile-parts (Psot honoured, tile-parts of different tiles may
// be interleaved), image and tile origin offsets, any number of quality
// layers, all five progression orders, progression order changes (POC),
// custom precinct sizes, SOP and EPH markers, every Part 1 code-block
// style (BYPASS, RESET, TERMALL, VSC, PTERM, SEGSYM), COC/QCC overrides
// in the main and tile-part headers, and RGN (ROI max-shift).
//
// Explicitly rejected with an error wrapping ErrUnsupported: more than one
// component, the irreversible 9/7 wavelet, packed packet headers
// (PPM/PPT), HT (Part 15) code-blocks and precision above 31 bits.
//
// The structure follows OpenJPEG (j2k.c, tcd.c, pi.c, t2.c) so that the
// output is sample-identical to opj_decompress, including the clamp of
// reconstructed samples to the nominal range after the DC level shift.

import (
	"errors"
	"fmt"
	"sort"
)

// Additional marker codes (ISO 15444-1 Table A.1).
const (
	markerCAP uint16 = 0xFF50
	markerCOC uint16 = 0xFF53
	markerTLM uint16 = 0xFF55
	markerPLM uint16 = 0xFF57
	markerPLT uint16 = 0xFF58
	markerCPF uint16 = 0xFF59
	markerQCC uint16 = 0xFF5D
	markerRGN uint16 = 0xFF5E
	markerPOC uint16 = 0xFF5F
	markerPPM uint16 = 0xFF60
	markerPPT uint16 = 0xFF61
	markerCRG uint16 = 0xFF63
	markerSOP uint16 = 0xFF91
	markerEPH uint16 = 0xFF92
)

// Scod flags (Table A.13).
const (
	cstyPrecincts = 0x01
	cstySOP       = 0x02
	cstyEPH       = 0x04
)

// ErrUnsupported is wrapped by every error that reports a valid
// codestream feature this decoder does not implement.
var ErrUnsupported = errors.New("j2k: unsupported codestream feature")

func errUnsupported(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, fmt.Sprintf(format, args...))
}

// maxPrec is the highest component precision the decoder supports. The
// output is []int32, and OpenJPEG refuses code-blocks with 31 or more
// magnitude bit-planes, so 31 is also the practical interoperable limit.
const maxPrec = 31

// Limits that keep crafted codestreams from exhausting memory or CPU.
const (
	maxPixels  = 100_000_000
	maxPackets = 1 << 20
)

// ---------------------------------------------------------------------------
// Header parameters
// ---------------------------------------------------------------------------

// compParams holds the SPcod/SPcoc parameters of one tile-component.
type compParams struct {
	numRes       int
	cblkw, cblkh int // code-block size exponents (xcb, ycb)
	cblkSty      uint8
	qmfbid       uint8
	prcw, prch   [33]int // precinct size exponents per resolution
}

type codParams struct {
	csty      uint8
	prog      uint8
	numLayers int
	comp      compParams
}

type quantParams struct {
	qntsty uint8
	numGB  int
	steps  []stepSize
}

type pocEntry struct {
	resS, compS, layE, resE, compE int
	prog                           uint8
}

// headerParams holds the marker segments seen in the main header or in
// the tile-part headers of one tile. A nil pointer means "not present".
type headerParams struct {
	cod  *codParams
	coc  *compParams
	qcd  *quantParams
	qcc  *quantParams
	roi  *int
	pocs []pocEntry
}

type tileState struct {
	hdr    headerParams
	data   []byte
	nextTP int
}

type decoder struct {
	siz   sizMarker
	main  headerParams
	tiles []tileState
	numTX int
	numTY int
}

// ---------------------------------------------------------------------------
// Marker segment parsing
// ---------------------------------------------------------------------------

// readSegment reads the marker segment at pos and returns the marker, its
// body (without the length field) and the position after it.
func readSegment(data []byte, pos int) (uint16, []byte, int, error) {
	if pos+4 > len(data) {
		return 0, nil, 0, fmt.Errorf("j2k: truncated marker segment at offset %d", pos)
	}
	m := readUint16(data, pos)
	if m>>8 != 0xFF {
		return 0, nil, 0, fmt.Errorf("j2k: expected a marker at offset %d, found 0x%04X", pos, m)
	}
	l := int(readUint16(data, pos+2))
	if l < 2 {
		return 0, nil, 0, fmt.Errorf("j2k: invalid length %d for marker 0x%04X", l, m)
	}
	end := pos + 2 + l
	if end > len(data) {
		return 0, nil, 0, fmt.Errorf("j2k: truncated body for marker 0x%04X", m)
	}
	return m, data[pos+4 : end], end, nil
}

func parseSIZBody(b []byte) (sizMarker, error) {
	var s sizMarker
	if len(b) < 36 {
		return s, errors.New("j2k: SIZ marker too short")
	}
	s.rsiz = readUint16(b, 0)
	s.xsiz = readUint32(b, 2)
	s.ysiz = readUint32(b, 6)
	s.x0 = readUint32(b, 10)
	s.y0 = readUint32(b, 14)
	s.xtSiz = readUint32(b, 18)
	s.ytSiz = readUint32(b, 22)
	s.xt0 = readUint32(b, 26)
	s.yt0 = readUint32(b, 30)
	s.ncomp = readUint16(b, 34)
	if s.ncomp == 0 {
		return s, errors.New("j2k: SIZ has zero components")
	}
	if 36+3*int(s.ncomp) > len(b) {
		return s, errors.New("j2k: SIZ marker component data truncated")
	}
	if s.ncomp != 1 {
		return s, errUnsupported("%d components (only single-component codestreams are supported)", s.ncomp)
	}
	ssiz := b[36]
	s.sgnd = ssiz&0x80 != 0
	s.prec = (ssiz & 0x7F) + 1
	s.xrsiz = b[37]
	s.yrsiz = b[38]

	if s.prec > 38 {
		return s, fmt.Errorf("j2k: invalid component precision %d", s.prec)
	}
	if s.prec > maxPrec {
		return s, errUnsupported("component precision %d (maximum %d)", s.prec, maxPrec)
	}
	if s.xrsiz == 0 || s.yrsiz == 0 {
		return s, errors.New("j2k: zero component sub-sampling factor")
	}
	if s.xsiz <= s.x0 || s.ysiz <= s.y0 {
		return s, fmt.Errorf("j2k: zero or negative image size (%d-%d x %d-%d)", s.xsiz, s.x0, s.ysiz, s.y0)
	}
	if s.xtSiz == 0 || s.ytSiz == 0 {
		return s, errors.New("j2k: zero tile size")
	}
	if s.xt0 > s.x0 || s.yt0 > s.y0 ||
		uint64(s.xt0)+uint64(s.xtSiz) <= uint64(s.x0) ||
		uint64(s.yt0)+uint64(s.ytSiz) <= uint64(s.y0) {
		return s, errors.New("j2k: invalid tile grid origin")
	}
	return s, nil
}

// parseSPcod parses SPcod/SPcoc. scod carries the precinct flag.
func parseSPcod(b []byte, scod uint8) (compParams, error) {
	var c compParams
	if len(b) < 5 {
		return c, errors.New("j2k: COD/COC parameters too short")
	}
	if b[0] > 32 {
		return c, fmt.Errorf("j2k: %d decomposition levels (maximum 32)", b[0])
	}
	c.numRes = int(b[0]) + 1
	if b[1] > 8 || b[2] > 8 || int(b[1])+int(b[2]) > 8 {
		return c, fmt.Errorf("j2k: invalid code-block size exponents %d,%d", b[1]+2, b[2]+2)
	}
	c.cblkw = int(b[1]) + 2
	c.cblkh = int(b[2]) + 2
	c.cblkSty = b[3]
	if c.cblkSty&0x40 != 0 {
		return c, errUnsupported("HT (high-throughput) code-blocks, code-block style 0x%02X", c.cblkSty)
	}
	if c.cblkSty&0x80 != 0 {
		return c, fmt.Errorf("j2k: invalid code-block style 0x%02X", c.cblkSty)
	}
	c.qmfbid = b[4]
	if c.qmfbid > 1 {
		return c, fmt.Errorf("j2k: invalid wavelet transform qmfbid=%d", c.qmfbid)
	}
	for r := range c.prcw {
		c.prcw[r] = 15
		c.prch[r] = 15
	}
	if scod&cstyPrecincts != 0 {
		if len(b) < 5+c.numRes {
			return c, errors.New("j2k: COD/COC precinct sizes truncated")
		}
		for r := 0; r < c.numRes; r++ {
			v := b[5+r]
			c.prcw[r] = int(v & 0x0F)
			c.prch[r] = int(v >> 4)
			if r > 0 && (c.prcw[r] == 0 || c.prch[r] == 0) {
				return c, fmt.Errorf("j2k: invalid precinct size exponent 0 at resolution %d", r)
			}
		}
	}
	return c, nil
}

func parseCODBody(b []byte) (*codParams, error) {
	if len(b) < 5 {
		return nil, errors.New("j2k: COD marker too short")
	}
	c := &codParams{
		csty:      b[0],
		prog:      b[1],
		numLayers: int(readUint16(b, 2)),
	}
	if c.prog > progCPRL {
		return nil, fmt.Errorf("j2k: invalid progression order %d", c.prog)
	}
	if c.numLayers == 0 {
		return nil, errors.New("j2k: zero quality layers")
	}
	comp, err := parseSPcod(b[5:], c.csty)
	if err != nil {
		return nil, err
	}
	c.comp = comp
	return c, nil
}

func parseCOCBody(b []byte) (*compParams, error) {
	// Ccoc is one byte because Csiz < 257.
	if len(b) < 2 {
		return nil, errors.New("j2k: COC marker too short")
	}
	if b[0] != 0 {
		return nil, fmt.Errorf("j2k: COC for component %d, codestream has 1 component", b[0])
	}
	c, err := parseSPcod(b[2:], b[1])
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func parseQuant(b []byte) (*quantParams, error) {
	if len(b) < 1 {
		return nil, errors.New("j2k: QCD/QCC marker too short")
	}
	q := &quantParams{qntsty: b[0] & 0x1F, numGB: int(b[0] >> 5)}
	rest := b[1:]
	switch q.qntsty {
	case qntstyNone:
		for _, v := range rest {
			q.steps = append(q.steps, stepSize{expn: int32(v >> 3)})
		}
	case qntstySIQNT:
		if len(rest) < 2 {
			return nil, errors.New("j2k: QCD/QCC derived step size truncated")
		}
		v := readUint16(rest, 0)
		q.steps = append(q.steps, stepSize{expn: int32(v >> 11), mant: int32(v & 0x7FF)})
	case qntstySQNT:
		for i := 0; i+1 < len(rest); i += 2 {
			v := readUint16(rest, i)
			q.steps = append(q.steps, stepSize{expn: int32(v >> 11), mant: int32(v & 0x7FF)})
		}
	default:
		return nil, fmt.Errorf("j2k: unknown quantization style %d", q.qntsty)
	}
	if len(q.steps) == 0 {
		return nil, errors.New("j2k: QCD/QCC has no step sizes")
	}
	return q, nil
}

func parseQCCBody(b []byte) (*quantParams, error) {
	if len(b) < 1 {
		return nil, errors.New("j2k: QCC marker too short")
	}
	if b[0] != 0 {
		return nil, fmt.Errorf("j2k: QCC for component %d, codestream has 1 component", b[0])
	}
	return parseQuant(b[1:])
}

func parseRGNBody(b []byte) (int, error) {
	if len(b) < 3 {
		return 0, errors.New("j2k: RGN marker too short")
	}
	if b[0] != 0 {
		return 0, fmt.Errorf("j2k: RGN for component %d, codestream has 1 component", b[0])
	}
	if b[1] != 0 {
		return 0, errUnsupported("RGN style %d", b[1])
	}
	return int(b[2]), nil
}

func parsePOCBody(b []byte) ([]pocEntry, error) {
	// Each entry is 7 bytes when Csiz < 257.
	if len(b) == 0 || len(b)%7 != 0 {
		return nil, fmt.Errorf("j2k: invalid POC marker length %d", len(b))
	}
	var out []pocEntry
	for i := 0; i < len(b); i += 7 {
		e := pocEntry{
			resS:  int(b[i]),
			compS: int(b[i+1]),
			layE:  int(readUint16(b, i+2)),
			resE:  int(b[i+4]),
			compE: int(b[i+5]),
			prog:  b[i+6],
		}
		if e.compE == 0 {
			e.compE = 256
		}
		if e.prog > progCPRL {
			return nil, fmt.Errorf("j2k: invalid POC progression order %d", e.prog)
		}
		out = append(out, e)
	}
	return out, nil
}

// applySegment records one main or tile-part header marker segment.
func applySegment(m uint16, body []byte, hp *headerParams) error {
	var err error
	switch m {
	case markerCOD:
		hp.cod, err = parseCODBody(body)
	case markerCOC:
		hp.coc, err = parseCOCBody(body)
	case markerQCD:
		hp.qcd, err = parseQuant(body)
	case markerQCC:
		hp.qcc, err = parseQCCBody(body)
	case markerRGN:
		var shift int
		shift, err = parseRGNBody(body)
		hp.roi = &shift
	case markerPOC:
		var e []pocEntry
		e, err = parsePOCBody(body)
		hp.pocs = append(hp.pocs, e...)
	case markerPPM, markerPPT:
		err = errUnsupported("packed packet headers (marker 0x%04X)", m)
	case markerSIZ:
		err = errors.New("j2k: unexpected SIZ marker")
	case markerSOC, markerSOT, markerSOD, markerEOC:
		err = fmt.Errorf("j2k: unexpected marker 0x%04X", m)
	default:
		// COM, TLM, PLM, PLT, CRG, CAP, CPF and unknown segments carry no
		// information needed for decoding.
	}
	return err
}

// parse splits the codestream into its main header and per-tile data.
func parse(data []byte) (*decoder, error) {
	if len(data) < 2 || readUint16(data, 0) != markerSOC {
		return nil, errors.New("j2k: missing SOC marker")
	}
	m, body, pos, err := readSegment(data, 2)
	if err != nil {
		return nil, err
	}
	if m != markerSIZ {
		return nil, fmt.Errorf("j2k: expected SIZ after SOC, found 0x%04X", m)
	}
	d := &decoder{}
	if d.siz, err = parseSIZBody(body); err != nil {
		return nil, err
	}
	s := &d.siz
	d.numTX = int(ceildiv64(uint64(s.xsiz)-uint64(s.xt0), uint64(s.xtSiz)))
	d.numTY = int(ceildiv64(uint64(s.ysiz)-uint64(s.yt0), uint64(s.ytSiz)))
	if d.numTX*d.numTY > 65535 {
		return nil, fmt.Errorf("j2k: %d x %d tiles exceeds the maximum of 65535", d.numTX, d.numTY)
	}
	compW := int(ceildiv64(uint64(s.xsiz), uint64(s.xrsiz)) - ceildiv64(uint64(s.x0), uint64(s.xrsiz)))
	compH := int(ceildiv64(uint64(s.ysiz), uint64(s.yrsiz)) - ceildiv64(uint64(s.y0), uint64(s.yrsiz)))
	if int64(compW)*int64(compH) > maxPixels {
		return nil, fmt.Errorf("j2k: image size %d x %d exceeds maximum %d pixels", compW, compH, maxPixels)
	}

	// Main header.
	for {
		if pos+2 > len(data) {
			return nil, errors.New("j2k: truncated main header")
		}
		m := readUint16(data, pos)
		if m == markerSOT {
			break
		}
		if m == markerEOC {
			return nil, errors.New("j2k: codestream has no tile-parts")
		}
		if m >= 0xFF30 && m <= 0xFF3F { // reserved markers without a segment
			pos += 2
			continue
		}
		m, body, next, err := readSegment(data, pos)
		if err != nil {
			return nil, err
		}
		if err := applySegment(m, body, &d.main); err != nil {
			return nil, err
		}
		pos = next
	}
	if d.main.cod == nil {
		return nil, errors.New("j2k: missing COD marker")
	}
	if d.main.qcd == nil && d.main.qcc == nil {
		return nil, errors.New("j2k: missing QCD marker")
	}

	d.tiles = make([]tileState, d.numTX*d.numTY)

	// Tile-parts.
	for pos+2 <= len(data) {
		m := readUint16(data, pos)
		if m == markerEOC {
			break
		}
		if m != markerSOT {
			return nil, fmt.Errorf("j2k: expected SOT or EOC at offset %d, found 0x%04X", pos, m)
		}
		sotStart := pos
		_, body, next, err := readSegment(data, pos)
		if err != nil {
			return nil, err
		}
		if len(body) != 8 {
			return nil, fmt.Errorf("j2k: invalid SOT length %d", len(body)+2)
		}
		isot := int(readUint16(body, 0))
		psot := int(readUint32(body, 2))
		tpsot := int(body[6])
		if isot >= len(d.tiles) {
			return nil, fmt.Errorf("j2k: tile index %d out of range (%d tiles)", isot, len(d.tiles))
		}
		var end int
		if psot == 0 {
			// The tile-part runs to the EOC marker (or the end of data).
			end = len(data)
			if end-next >= 2 && readUint16(data, end-2) == markerEOC {
				end -= 2
			}
		} else {
			if psot < 14 {
				return nil, fmt.Errorf("j2k: invalid tile-part length Psot=%d", psot)
			}
			end = sotStart + psot
			if end > len(data) {
				return nil, fmt.Errorf("j2k: truncated tile-part: Psot=%d but only %d bytes remain", psot, len(data)-sotStart)
			}
		}
		ts := &d.tiles[isot]
		if tpsot != ts.nextTP {
			return nil, fmt.Errorf("j2k: tile %d: tile-part %d out of order (expected %d)", isot, tpsot, ts.nextTP)
		}
		ts.nextTP++
		pos = next
		for {
			if pos+2 > end {
				return nil, errors.New("j2k: truncated tile-part header")
			}
			m := readUint16(data, pos)
			if m == markerSOD {
				pos += 2
				break
			}
			m, body, next, err := readSegment(data[:end], pos)
			if err != nil {
				return nil, err
			}
			if err := applySegment(m, body, &ts.hdr); err != nil {
				return nil, err
			}
			pos = next
		}
		ts.data = append(ts.data, data[pos:end]...)
		pos = end
	}
	return d, nil
}

func ceildiv64(a, b uint64) uint64 {
	if b == 0 {
		return 0
	}
	return (a + b - 1) / b
}

// ---------------------------------------------------------------------------
// Packet header bit reader (port of OpenJPEG bio.c)
// ---------------------------------------------------------------------------

type bitReader struct {
	data    []byte
	pos     int
	buf     uint32
	ct      int
	overrun bool // a bit was requested past the end of data
}

func newBitReader(data []byte) *bitReader { return &bitReader{data: data} }

func (br *bitReader) byteIn() {
	br.buf = (br.buf << 8) & 0xFFFF
	if br.buf == 0xFF00 {
		br.ct = 7
	} else {
		br.ct = 8
	}
	if br.pos >= len(br.data) {
		br.overrun = true
		return
	}
	br.buf |= uint32(br.data[br.pos])
	br.pos++
}

func (br *bitReader) readBit() uint32 {
	if br.ct == 0 {
		br.byteIn()
	}
	br.ct--
	return (br.buf >> uint(br.ct)) & 1
}

func (br *bitReader) readBits(n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		v = v<<1 | br.readBit()
	}
	return v
}

// inalign skips the stuffing byte that follows a 0xFF header byte.
func (br *bitReader) inalign() {
	if br.buf&0xFF == 0xFF {
		br.byteIn()
	}
	br.ct = 0
}

func (br *bitReader) numBytes() int { return br.pos }

// ---------------------------------------------------------------------------
// Tile geometry
// ---------------------------------------------------------------------------

type dSeg struct {
	data      []byte
	numPasses int
	maxPasses int
}

type dPending struct {
	seg, passes, length int
}

type dCblk struct {
	x0, y0, x1, y1 int
	numbps         int
	numLenBits     int
	segs           []dSeg
	pending        []dPending
}

type dPrc struct {
	cw, ch   int
	cblks    []dCblk
	inclTree *tagTree
	imsbTree *tagTree
}

type dBand struct {
	bandno         int // 0=LL, 1=HL, 2=LH, 3=HH
	x0, y0, x1, y1 int
	numbps         int
	prcs           []dPrc
}

func (b *dBand) empty() bool { return b.x1 <= b.x0 || b.y1 <= b.y0 }

type dRes struct {
	x0, y0, x1, y1 int
	pdx, pdy       int
	pw, ph         int
	prcX0, prcY0   int // absolute index of the first precinct
	bands          []dBand
}

type dTile struct {
	tx0, ty0, tx1, ty1     int // tile on the reference grid
	tcx0, tcy0, tcx1, tcy1 int // tile-component
	res                    []dRes
}

func floordivpow2(a, b int) int { return a >> uint(b) }

// effective returns the coding parameters that apply to one tile, using
// the precedence tile COC > tile COD > main COC > main COD (and likewise
// for QCC/QCD).
func (d *decoder) effective(ts *tileState) (*codParams, compParams, *quantParams, int, []pocEntry) {
	cod := d.main.cod
	if ts.hdr.cod != nil {
		cod = ts.hdr.cod
	}
	var comp compParams
	switch {
	case ts.hdr.coc != nil:
		comp = *ts.hdr.coc
	case ts.hdr.cod != nil:
		comp = ts.hdr.cod.comp
	case d.main.coc != nil:
		comp = *d.main.coc
	default:
		comp = d.main.cod.comp
	}
	var q *quantParams
	switch {
	case ts.hdr.qcc != nil:
		q = ts.hdr.qcc
	case ts.hdr.qcd != nil:
		q = ts.hdr.qcd
	case d.main.qcc != nil:
		q = d.main.qcc
	default:
		q = d.main.qcd
	}
	roi := 0
	if ts.hdr.roi != nil {
		roi = *ts.hdr.roi
	} else if d.main.roi != nil {
		roi = *d.main.roi
	}
	pocs := d.main.pocs
	if len(ts.hdr.pocs) > 0 {
		pocs = ts.hdr.pocs
	}
	return cod, comp, q, roi, pocs
}

// bandSteps expands the quantization parameters to one step size per band.
func bandSteps(q *quantParams, numRes int) ([]stepSize, error) {
	numBands := 3*(numRes-1) + 1
	if q.qntsty == qntstySIQNT {
		out := make([]stepSize, numBands)
		out[0] = q.steps[0]
		for b := 1; b < numBands; b++ {
			e := q.steps[0].expn - int32((b-1)/3)
			if e < 0 {
				e = 0
			}
			out[b] = stepSize{expn: e, mant: q.steps[0].mant}
		}
		return out, nil
	}
	if len(q.steps) < numBands {
		return nil, fmt.Errorf("j2k: quantization marker has %d step sizes, need %d", len(q.steps), numBands)
	}
	return q.steps[:numBands], nil
}

func (d *decoder) setupTile(ti int, comp compParams, steps []stepSize, numGB int) (*dTile, error) {
	s := &d.siz
	p := ti % d.numTX
	q := ti / d.numTX
	t := &dTile{
		tx0: imax(int(s.xt0)+p*int(s.xtSiz), int(s.x0)),
		ty0: imax(int(s.yt0)+q*int(s.ytSiz), int(s.y0)),
		tx1: imin(int(s.xt0)+(p+1)*int(s.xtSiz), int(s.xsiz)),
		ty1: imin(int(s.yt0)+(q+1)*int(s.ytSiz), int(s.ysiz)),
	}
	dx, dy := int(s.xrsiz), int(s.yrsiz)
	t.tcx0 = ceildiv(t.tx0, dx)
	t.tcy0 = ceildiv(t.ty0, dy)
	t.tcx1 = ceildiv(t.tx1, dx)
	t.tcy1 = ceildiv(t.ty1, dy)

	numRes := comp.numRes
	t.res = make([]dRes, numRes)
	stepIdx := 0
	for r := 0; r < numRes; r++ {
		res := &t.res[r]
		level := numRes - 1 - r
		res.x0 = ceildivpow2(t.tcx0, level)
		res.y0 = ceildivpow2(t.tcy0, level)
		res.x1 = ceildivpow2(t.tcx1, level)
		res.y1 = ceildivpow2(t.tcy1, level)
		res.pdx = comp.prcw[r]
		res.pdy = comp.prch[r]

		prcXStart := floordivpow2(res.x0, res.pdx) << uint(res.pdx)
		prcYStart := floordivpow2(res.y0, res.pdy) << uint(res.pdy)
		prcXEnd := ceildivpow2(res.x1, res.pdx) << uint(res.pdx)
		prcYEnd := ceildivpow2(res.y1, res.pdy) << uint(res.pdy)
		if res.x0 != res.x1 {
			res.pw = (prcXEnd - prcXStart) >> uint(res.pdx)
		}
		if res.y0 != res.y1 {
			res.ph = (prcYEnd - prcYStart) >> uint(res.pdy)
		}
		res.prcX0 = floordivpow2(res.x0, res.pdx)
		res.prcY0 = floordivpow2(res.y0, res.pdy)
		if int64(res.pw)*int64(res.ph) > maxPackets {
			return nil, fmt.Errorf("j2k: %d x %d precincts exceeds the limit", res.pw, res.ph)
		}

		var cbgX0, cbgY0, cbgwExp, cbghExp int
		if r == 0 {
			cbgX0, cbgY0 = prcXStart, prcYStart
			cbgwExp, cbghExp = res.pdx, res.pdy
		} else {
			cbgX0, cbgY0 = ceildivpow2(prcXStart, 1), ceildivpow2(prcYStart, 1)
			cbgwExp, cbghExp = res.pdx-1, res.pdy-1
		}
		cblkwExp := imin(comp.cblkw, cbgwExp)
		cblkhExp := imin(comp.cblkh, cbghExp)

		nb := 3
		if r == 0 {
			nb = 1
		}
		res.bands = make([]dBand, nb)
		for i := 0; i < nb; i++ {
			b := &res.bands[i]
			if r == 0 {
				b.bandno = 0
				b.x0, b.y0, b.x1, b.y1 = res.x0, res.y0, res.x1, res.y1
			} else {
				b.bandno = i + 1
				x0b := b.bandno & 1
				y0b := b.bandno >> 1
				b.x0 = ceildivpow2(t.tcx0-(x0b<<uint(level)), level+1)
				b.y0 = ceildivpow2(t.tcy0-(y0b<<uint(level)), level+1)
				b.x1 = ceildivpow2(t.tcx1-(x0b<<uint(level)), level+1)
				b.y1 = ceildivpow2(t.tcy1-(y0b<<uint(level)), level+1)
			}
			b.numbps = int(steps[stepIdx].expn) + numGB - 1
			stepIdx++
			if b.empty() {
				continue
			}
			b.prcs = make([]dPrc, res.pw*res.ph)
			for pi := range b.prcs {
				prc := &b.prcs[pi]
				cbgxs := cbgX0 + (pi%res.pw)<<uint(cbgwExp)
				cbgys := cbgY0 + (pi/res.pw)<<uint(cbghExp)
				px0 := imax(cbgxs, b.x0)
				py0 := imax(cbgys, b.y0)
				px1 := imin(cbgxs+1<<uint(cbgwExp), b.x1)
				py1 := imin(cbgys+1<<uint(cbghExp), b.y1)
				if px1 <= px0 || py1 <= py0 {
					continue
				}
				tlx := floordivpow2(px0, cblkwExp) << uint(cblkwExp)
				tly := floordivpow2(py0, cblkhExp) << uint(cblkhExp)
				brx := ceildivpow2(px1, cblkwExp) << uint(cblkwExp)
				bry := ceildivpow2(py1, cblkhExp) << uint(cblkhExp)
				prc.cw = (brx - tlx) >> uint(cblkwExp)
				prc.ch = (bry - tly) >> uint(cblkhExp)
				prc.cblks = make([]dCblk, prc.cw*prc.ch)
				for ci := range prc.cblks {
					cb := &prc.cblks[ci]
					cxs := tlx + (ci%prc.cw)<<uint(cblkwExp)
					cys := tly + (ci/prc.cw)<<uint(cblkhExp)
					cb.x0 = imax(cxs, px0)
					cb.y0 = imax(cys, py0)
					cb.x1 = imin(cxs+1<<uint(cblkwExp), px1)
					cb.y1 = imin(cys+1<<uint(cblkhExp), py1)
				}
				prc.inclTree = newTagTree(prc.cw, prc.ch)
				prc.imsbTree = newTagTree(prc.cw, prc.ch)
			}
		}
	}
	return t, nil
}

// ---------------------------------------------------------------------------
// Packet iteration (ISO 15444-1 B.12)
// ---------------------------------------------------------------------------

type packetID struct{ layer, res, prc int }

// packetOrder lists the packets of a tile in codestream order. Position-
// driven orders sort precincts by the reference-grid point at which
// OpenJPEG's iterator (pi.c) visits them: the precinct's upper-left corner,
// or the tile origin for precincts that start before it.
func (d *decoder) packetOrder(t *dTile, cod *codParams, numRes int, pocs []pocEntry) ([]packetID, error) {
	numLayers := cod.numLayers
	total := int64(0)
	for r := range t.res {
		total += int64(t.res[r].pw) * int64(t.res[r].ph)
	}
	if total*int64(numLayers) > maxPackets {
		return nil, fmt.Errorf("j2k: %d packets exceeds the limit", total*int64(numLayers))
	}
	if len(pocs) == 0 {
		pocs = []pocEntry{{resS: 0, compS: 0, layE: numLayers, resE: numRes, compE: 1, prog: cod.prog}}
	}

	dx, dy := int(d.siz.xrsiz), int(d.siz.yrsiz)
	type cand struct {
		id  packetID
		key [5]int
	}
	// emitted is indexed by layer*total + prcBase[res] + precinct.
	prcBase := make([]int, len(t.res))
	for r := 1; r < len(t.res); r++ {
		prcBase[r] = prcBase[r-1] + t.res[r-1].pw*t.res[r-1].ph
	}
	emitted := make([]bool, int(total)*numLayers)
	idx := func(id packetID) int { return id.layer*int(total) + prcBase[id.res] + id.prc }
	var out []packetID
	for _, poc := range pocs {
		if poc.compS > 0 || poc.compE < 1 {
			continue // only component 0 exists
		}
		layE := imin(poc.layE, numLayers)
		resE := imin(poc.resE, numRes)
		var cands []cand
		for r := poc.resS; r < resE; r++ {
			res := &t.res[r]
			level := numRes - 1 - r
			for pi := 0; pi < res.pw*res.ph; pi++ {
				kx := res.prcX0 + pi%res.pw
				ky := res.prcY0 + pi/res.pw
				xk := imax(t.tx0, (kx<<uint(res.pdx+level))*dx)
				yk := imax(t.ty0, (ky<<uint(res.pdy+level))*dy)
				for l := 0; l < layE; l++ {
					id := packetID{l, r, pi}
					if emitted[idx(id)] {
						continue
					}
					var key [5]int
					switch poc.prog {
					case progLRCP:
						key = [5]int{l, r, 0, pi, 0}
					case progRLCP:
						key = [5]int{r, l, 0, pi, 0}
					case progRPCL:
						key = [5]int{r, yk, xk, 0, l}
					case progPCRL:
						key = [5]int{yk, xk, 0, r, l}
					case progCPRL:
						key = [5]int{0, yk, xk, r, l}
					}
					cands = append(cands, cand{id, key})
				}
			}
		}
		sort.SliceStable(cands, func(i, j int) bool {
			a, b := cands[i].key, cands[j].key
			for k := range a {
				if a[k] != b[k] {
					return a[k] < b[k]
				}
			}
			return false
		})
		for _, c := range cands {
			emitted[idx(c.id)] = true
			out = append(out, c.id)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Tier-2 packet decoding (port of OpenJPEG t2.c)
// ---------------------------------------------------------------------------

func initSeg(cb *dCblk, cblkSty uint8) {
	var maxp int
	switch {
	case cblkSty&CblkstyTERMALL != 0:
		maxp = 1
	case cblkSty&CblkstyLAZY != 0:
		if len(cb.segs) == 0 {
			maxp = 10
		} else if prev := cb.segs[len(cb.segs)-1].maxPasses; prev == 1 || prev == 10 {
			maxp = 2
		} else {
			maxp = 1
		}
	default:
		maxp = 109
	}
	cb.segs = append(cb.segs, dSeg{maxPasses: maxp})
}

// decodePacket reads one packet starting at data[0] and returns the number
// of bytes it occupies.
func decodePacket(data []byte, res *dRes, prcno, layno int, csty, cblkSty uint8) (int, error) {
	pos := 0
	if csty&cstySOP != 0 && len(data) >= 6 && readUint16(data, 0) == markerSOP {
		pos = 6
	}

	br := newBitReader(data[pos:])
	present := br.readBit()
	var included []*dCblk
	if present != 0 {
		for bi := range res.bands {
			band := &res.bands[bi]
			if band.empty() {
				continue
			}
			prc := &band.prcs[prcno]
			for cbi := range prc.cblks {
				cb := &prc.cblks[cbi]
				var in bool
				first := len(cb.segs) == 0
				if first {
					in = prc.inclTree.decode(br, cbi, int32(layno+1))
				} else {
					in = br.readBit() != 0
				}
				if !in {
					continue
				}
				if first {
					zbp := int32(0)
					for !prc.imsbTree.decode(br, cbi, zbp) {
						zbp++
						if zbp > 74 || br.overrun {
							return 0, errors.New("invalid zero bit-plane count")
						}
					}
					cb.numbps = band.numbps + 1 - int(zbp)
					if cb.numbps < 0 {
						return 0, fmt.Errorf("zero bit-plane count %d exceeds the band's %d bit-planes", zbp, band.numbps+1)
					}
					cb.numLenBits = 3
				}
				n := getNumPasses(br)
				cb.numLenBits += getCommaCode(br)
				if first {
					initSeg(cb, cblkSty)
				} else if last := &cb.segs[len(cb.segs)-1]; last.numPasses == last.maxPasses {
					initSeg(cb, cblkSty)
				}
				segno := len(cb.segs) - 1
				for n > 0 {
					seg := &cb.segs[segno]
					np := imin(seg.maxPasses-seg.numPasses, n)
					bits := cb.numLenBits + floorLog2(np)
					if bits > 32 {
						return 0, fmt.Errorf("invalid code-block length field of %d bits", bits)
					}
					cb.pending = append(cb.pending, dPending{seg: segno, passes: np, length: int(br.readBits(bits))})
					n -= np
					if n > 0 {
						initSeg(cb, cblkSty)
						segno++
					}
				}
				included = append(included, cb)
			}
		}
		if br.overrun {
			return 0, errors.New("truncated packet header")
		}
	}
	br.inalign()
	pos += br.numBytes()

	if csty&cstyEPH != 0 {
		if pos+2 > len(data) {
			if present == 0 && pos >= len(data) {
				return pos, nil // packet stream exhausted
			}
			return 0, errors.New("missing EPH marker")
		}
		if readUint16(data, pos) != markerEPH {
			return 0, errors.New("missing EPH marker")
		}
		pos += 2
	}

	for _, cb := range included {
		for _, p := range cb.pending {
			if p.length > len(data)-pos {
				return 0, fmt.Errorf("code-block segment of %d bytes overruns the packet data (%d bytes left)", p.length, len(data)-pos)
			}
			seg := &cb.segs[p.seg]
			seg.data = append(seg.data, data[pos:pos+p.length]...)
			seg.numPasses += p.passes
			pos += p.length
		}
		cb.pending = cb.pending[:0]
	}
	return pos, nil
}

// ---------------------------------------------------------------------------
// Top-level decode
// ---------------------------------------------------------------------------

// Decode decodes a raw JPEG 2000 codestream (not JP2) and returns the
// reconstructed samples of its single component in row-major order, along
// with the component dimensions.
//
// Unsupported features produce an error wrapping ErrUnsupported; the
// decoder never returns silently wrong samples for a valid codestream.
func Decode(data []byte) (samples []int32, width, height int, err error) {
	d, err := parse(data)
	if err != nil {
		return nil, 0, 0, err
	}
	s := &d.siz
	dx, dy := int(s.xrsiz), int(s.yrsiz)
	compX0 := ceildiv(int(s.x0), dx)
	compY0 := ceildiv(int(s.y0), dy)
	width = ceildiv(int(s.xsiz), dx) - compX0
	height = ceildiv(int(s.ysiz), dy) - compY0
	if width <= 0 || height <= 0 {
		return nil, 0, 0, errors.New("j2k: zero or negative image dimensions")
	}
	out := make([]int32, width*height)
	for ti := range d.tiles {
		if err := d.decodeTile(ti, out, compX0, compY0, width); err != nil {
			if len(d.tiles) > 1 {
				return nil, 0, 0, fmt.Errorf("j2k: tile %d: %w", ti, err)
			}
			return nil, 0, 0, err
		}
	}
	return out, width, height, nil
}

func (d *decoder) decodeTile(ti int, out []int32, compX0, compY0, compW int) error {
	ts := &d.tiles[ti]
	if ts.nextTP == 0 {
		return errors.New("j2k: tile has no tile-parts")
	}
	cod, comp, q, roishift, pocs := d.effective(ts)
	if q == nil {
		return errors.New("j2k: missing QCD marker")
	}
	if comp.qmfbid != 1 {
		return errUnsupported("irreversible 9/7 wavelet (qmfbid=%d); only the reversible 5/3 wavelet is supported", comp.qmfbid)
	}
	steps, err := bandSteps(q, comp.numRes)
	if err != nil {
		return err
	}
	t, err := d.setupTile(ti, comp, steps, q.numGB)
	if err != nil {
		return err
	}
	tw := t.tcx1 - t.tcx0
	th := t.tcy1 - t.tcy0
	if tw <= 0 || th <= 0 {
		return nil
	}

	order, err := d.packetOrder(t, cod, comp.numRes, pocs)
	if err != nil {
		return err
	}
	// When the tile data runs out, the remaining packets read as empty
	// (OpenJPEG does the same). opj_compress relies on this: with -POC it
	// omits the packets of later progression volumes. A packet whose
	// header or body is cut short is an error.
	pos := 0
	for _, id := range order {
		n, err := decodePacket(ts.data[pos:], &t.res[id.res], id.prc, id.layer, cod.csty, comp.cblkSty)
		if err != nil {
			return fmt.Errorf("j2k: T2 packet (l=%d r=%d p=%d): %w", id.layer, id.res, id.prc, err)
		}
		pos += n
	}

	// Tier-1 decode and placement into the tile-component buffer.
	coeffs := make([]int32, tw*th)
	t1 := NewT1()
	for r := range t.res {
		res := &t.res[r]
		for bi := range res.bands {
			band := &res.bands[bi]
			if band.empty() {
				continue
			}
			offX, offY := 0, 0
			if r > 0 {
				pres := &t.res[r-1]
				if band.bandno&1 != 0 {
					offX = pres.x1 - pres.x0
				}
				if band.bandno&2 != 0 {
					offY = pres.y1 - pres.y0
				}
			}
			for pi := range band.prcs {
				prc := &band.prcs[pi]
				for ci := range prc.cblks {
					cb := &prc.cblks[ci]
					if len(cb.segs) == 0 {
						continue
					}
					w := cb.x1 - cb.x0
					h := cb.y1 - cb.y0
					segs := make([]T1Seg, 0, len(cb.segs))
					for _, sg := range cb.segs {
						if sg.numPasses == 0 {
							continue
						}
						segs = append(segs, T1Seg{Data: sg.data, Len: uint32(len(sg.data)), RealNumPasses: uint32(sg.numPasses)})
					}
					dec, err := t1.DecodeCodeblock(uint32(w), uint32(h), uint32(band.bandno),
						uint32(roishift), uint32(comp.cblkSty), uint32(cb.numbps), segs)
					if err != nil {
						return fmt.Errorf("j2k: T1 decode failed: %w", err)
					}
					if roishift > 0 {
						if roishift >= 31 {
							for i := range dec {
								dec[i] = 0
							}
						} else {
							thresh := int32(1) << uint(roishift)
							for i, v := range dec {
								mag := v
								if mag < 0 {
									mag = -mag
								}
								if mag >= thresh {
									mag >>= uint(roishift)
									if v < 0 {
										mag = -mag
									}
									dec[i] = mag
								}
							}
						}
					}
					x0 := cb.x0 - band.x0 + offX
					y0 := cb.y0 - band.y0 + offY
					for y := 0; y < h; y++ {
						row := coeffs[(y0+y)*tw+x0:]
						src := dec[y*w : (y+1)*w]
						for x, v := range src {
							row[x] = v / 2
						}
					}
				}
			}
		}
	}

	rects := make([]dwtRect, len(t.res))
	for r := range t.res {
		rects[r] = dwtRect{t.res[r].x0, t.res[r].y0, t.res[r].x1, t.res[r].y1}
	}
	if err := inverseDWT53Rects(coeffs, tw, rects); err != nil {
		return fmt.Errorf("j2k: inverse DWT failed: %w", err)
	}

	// DC level shift and clamp to the nominal range (as OpenJPEG does).
	prec := uint(d.siz.prec)
	var lo, hi, shift int64
	if d.siz.sgnd {
		lo, hi = -(int64(1) << (prec - 1)), int64(1)<<(prec-1)-1
	} else {
		lo, hi, shift = 0, int64(1)<<prec-1, int64(1)<<(prec-1)
	}
	for y := 0; y < th; y++ {
		dst := out[(t.tcy0-compY0+y)*compW+(t.tcx0-compX0):]
		src := coeffs[y*tw : (y+1)*tw]
		for x, v := range src {
			s := int64(v) + shift
			if s < lo {
				s = lo
			} else if s > hi {
				s = hi
			}
			dst[x] = int32(s)
		}
	}
	return nil
}
