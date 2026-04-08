// MQ arithmetic coder (decoder only) for JPEG 2000.
//
// Ported from OpenJPEG (openjpeg/src/lib/openjp2/mqc.c, mqc_inl.h, mqc.h).
// Implements the MQ decoder specified in ISO 15444-1 Annex C.
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
//
// 2-clause BSD License. See LICENSE in the OpenJPEG source tree.
package j2k

// NumContexts is the number of MQ coding contexts used by the JPEG 2000
// tier-1 coder (ISO 15444-1 Table D.2).
const NumContexts = 19

// cblkDataExtra is the number of sentinel bytes (0xFF 0xFF) appended past
// the end of the code-block data so that the byte-in routine can always
// look ahead without bounds checking (see OPJ_COMMON_CBLK_DATA_EXTRA).
const cblkDataExtra = 2

// mqcState represents one row of the MQ state-transition table
// (ISO 15444-1 Table C.3). Indices refer to positions in the global
// mqcStates slice rather than C-style pointers.
type mqcState struct {
	qeval uint32 // probability of the LPS in Q-value representation
	mps   uint32 // most probable symbol (0 or 1)
	nmps  uint8  // index of next state after MPS
	nlps  uint8  // index of next state after LPS
}

// mqcStates contains all 47*2 = 94 states (each probability level has two
// entries: one for MPS=0, one for MPS=1). This table is a direct
// transcription of the openjpeg mqc_states[] array which itself encodes
// ISO 15444-1 Table C.3.
var mqcStates = [94]mqcState{
	//  idx  qeval   mps  nmps nlps
	{0x5601, 0, 2, 3},   // 0
	{0x5601, 1, 3, 2},   // 1
	{0x3401, 0, 4, 12},  // 2
	{0x3401, 1, 5, 13},  // 3
	{0x1801, 0, 6, 18},  // 4
	{0x1801, 1, 7, 19},  // 5
	{0x0ac1, 0, 8, 24},  // 6
	{0x0ac1, 1, 9, 25},  // 7
	{0x0521, 0, 10, 58}, // 8
	{0x0521, 1, 11, 59}, // 9
	{0x0221, 0, 76, 66}, // 10
	{0x0221, 1, 77, 67}, // 11
	{0x5601, 0, 14, 13}, // 12
	{0x5601, 1, 15, 12}, // 13
	{0x5401, 0, 16, 28}, // 14
	{0x5401, 1, 17, 29}, // 15
	{0x4801, 0, 18, 28}, // 16
	{0x4801, 1, 19, 29}, // 17
	{0x3801, 0, 20, 28}, // 18
	{0x3801, 1, 21, 29}, // 19
	{0x3001, 0, 22, 34}, // 20
	{0x3001, 1, 23, 35}, // 21
	{0x2401, 0, 24, 36}, // 22
	{0x2401, 1, 25, 37}, // 23
	{0x1c01, 0, 26, 40}, // 24
	{0x1c01, 1, 27, 41}, // 25
	{0x1601, 0, 58, 42}, // 26
	{0x1601, 1, 59, 43}, // 27
	{0x5601, 0, 30, 29}, // 28
	{0x5601, 1, 31, 28}, // 29
	{0x5401, 0, 32, 28}, // 30
	{0x5401, 1, 33, 29}, // 31
	{0x5101, 0, 34, 30}, // 32
	{0x5101, 1, 35, 31}, // 33
	{0x4801, 0, 36, 32}, // 34
	{0x4801, 1, 37, 33}, // 35
	{0x3801, 0, 38, 34}, // 36
	{0x3801, 1, 39, 35}, // 37
	{0x3401, 0, 40, 36}, // 38
	{0x3401, 1, 41, 37}, // 39
	{0x3001, 0, 42, 38}, // 40
	{0x3001, 1, 43, 39}, // 41
	{0x2801, 0, 44, 38}, // 42
	{0x2801, 1, 45, 39}, // 43
	{0x2401, 0, 46, 40}, // 44
	{0x2401, 1, 47, 41}, // 45
	{0x2201, 0, 48, 42}, // 46
	{0x2201, 1, 49, 43}, // 47
	{0x1c01, 0, 50, 44}, // 48
	{0x1c01, 1, 51, 45}, // 49
	{0x1801, 0, 52, 46}, // 50
	{0x1801, 1, 53, 47}, // 51
	{0x1601, 0, 54, 48}, // 52
	{0x1601, 1, 55, 49}, // 53
	{0x1401, 0, 56, 50}, // 54
	{0x1401, 1, 57, 51}, // 55
	{0x1201, 0, 58, 52}, // 56
	{0x1201, 1, 59, 53}, // 57
	{0x1101, 0, 60, 54}, // 58
	{0x1101, 1, 61, 55}, // 59
	{0x0ac1, 0, 62, 56}, // 60
	{0x0ac1, 1, 63, 57}, // 61
	{0x09c1, 0, 64, 58}, // 62
	{0x09c1, 1, 65, 59}, // 63
	{0x08a1, 0, 66, 60}, // 64
	{0x08a1, 1, 67, 61}, // 65
	{0x0521, 0, 68, 62}, // 66
	{0x0521, 1, 69, 63}, // 67
	{0x0441, 0, 70, 64}, // 68
	{0x0441, 1, 71, 65}, // 69
	{0x02a1, 0, 72, 66}, // 70
	{0x02a1, 1, 73, 67}, // 71
	{0x0221, 0, 74, 68}, // 72
	{0x0221, 1, 75, 69}, // 73
	{0x0141, 0, 76, 70}, // 74
	{0x0141, 1, 77, 71}, // 75
	{0x0111, 0, 78, 72}, // 76
	{0x0111, 1, 79, 73}, // 77
	{0x0085, 0, 80, 74}, // 78
	{0x0085, 1, 81, 75}, // 79
	{0x0049, 0, 82, 76}, // 80
	{0x0049, 1, 83, 77}, // 81
	{0x0025, 0, 84, 78}, // 82
	{0x0025, 1, 85, 79}, // 83
	{0x0015, 0, 86, 80}, // 84
	{0x0015, 1, 87, 81}, // 85
	{0x0009, 0, 88, 82}, // 86
	{0x0009, 1, 89, 83}, // 87
	{0x0005, 0, 90, 84}, // 88
	{0x0005, 1, 91, 85}, // 89
	{0x0001, 0, 90, 86}, // 90
	{0x0001, 1, 91, 87}, // 91
	{0x5601, 0, 92, 92}, // 92
	{0x5601, 1, 93, 93}, // 93
}

// MQDecoder is the MQ arithmetic decoder defined in ISO 15444-1 Annex C.
type MQDecoder struct {
	// C is the code register (ISO 15444-1 C.2.2).
	c uint32
	// A is the interval/probability interval register.
	a uint32
	// ct is the count of remaining bits in the current byte.
	ct uint32
	// endOfByteStreamCounter counts how many times a terminating
	// 0xFF >0x8F marker is read -- used to limit overread.
	endOfByteStreamCounter uint32

	// data is the underlying compressed byte stream, extended with
	// cblkDataExtra sentinel bytes so byte-in never reads out of bounds.
	data []byte
	// bp is the current read position within data.
	bp int
	// start is the offset of the first real data byte.
	start int
	// end is len(original data) -- everything from data[end:] is sentinel.
	end int
	// backup stores the original bytes that were overwritten by sentinels.
	backup [cblkDataExtra]byte

	// ctxs holds the state-table index for each of the 19 contexts.
	ctxs [NumContexts]uint8
	// curCtx is the index into ctxs[] of the active context.
	curCtx uint8
}

// --------------------------------------------------------------------
// Public API
// --------------------------------------------------------------------

// InitDec initialises the MQ decoder for arithmetic decoding.
//
// Implements ISO 15444-1 C.3.5 Initialization of the decoder (INITDEC).
//
// The caller must supply a byte slice with at least cblkDataExtra (2)
// extra writable bytes beyond len(data). After decoding, call FinishDec
// to restore those bytes.
func (m *MQDecoder) InitDec(data []byte) {
	m.initDecCommon(data)
	m.curCtx = 0
	m.endOfByteStreamCounter = 0

	if len(data) == 0 {
		m.c = 0xff << 16
	} else {
		m.c = uint32(m.data[m.bp]) << 16
	}

	m.byteIn()
	m.c <<= 7
	m.ct -= 7
	m.a = 0x8000
}

// RawInitDec initialises the MQ decoder for raw (bypass) decoding.
//
// After decoding, call FinishDec to restore the sentinel bytes.
func (m *MQDecoder) RawInitDec(data []byte) {
	m.initDecCommon(data)
	m.c = 0
	m.ct = 0
}

// FinishDec restores the sentinel bytes that were temporarily
// overwritten by InitDec / RawInitDec.
func (m *MQDecoder) FinishDec() {
	copy(m.data[m.end:m.end+cblkDataExtra], m.backup[:])
}

// Decode decodes a single binary decision from the MQ code stream.
//
// Implements ISO 15444-1 C.3.2 Decoding a decision (DECODE).
func (m *MQDecoder) Decode() uint32 {
	st := &mqcStates[m.ctxs[m.curCtx]]
	m.a -= st.qeval
	if (m.c >> 16) < st.qeval {
		d := m.lpsExchange(st)
		m.renormD()
		return d
	}
	m.c -= st.qeval << 16
	if (m.a & 0x8000) == 0 {
		d := m.mpsExchange(st)
		m.renormD()
		return d
	}
	return st.mps
}

// RawDecode decodes a single bit using the raw (bypass) decoder.
//
// Cf. Taubman, p. 506.
func (m *MQDecoder) RawDecode() uint32 {
	if m.ct == 0 {
		// The sentinel 0xFF 0xFF guarantees we will eventually stop.
		if m.c == 0xff {
			if m.data[m.bp] > 0x8f {
				m.c = 0xff
				m.ct = 8
			} else {
				m.c = uint32(m.data[m.bp])
				m.bp++
				m.ct = 7
			}
		} else {
			m.c = uint32(m.data[m.bp])
			m.bp++
			m.ct = 8
		}
	}
	m.ct--
	return (m.c >> m.ct) & 0x01
}

// ResetStates resets all 19 contexts to the initial (equiprobable) state
// (state index 0, i.e. mqcStates[0]).
//
// See ISO 15444-1 Table D.7.
func (m *MQDecoder) ResetStates() {
	for i := range m.ctxs {
		m.ctxs[i] = 0
	}
}

// SetState sets context ctxno to the state identified by msb (the MPS
// value) and prob (the probability-class index, 0..46).
//
// The state-table index is computed as msb + prob*2, matching the layout
// of mqcStates where even indices have MPS=0 and odd indices have MPS=1.
func (m *MQDecoder) SetState(ctxno uint32, msb uint32, prob int32) {
	m.ctxs[ctxno] = uint8(msb + uint32(prob<<1))
}

// SetCurCtx sets the active context number (0..NumContexts-1).
func (m *MQDecoder) SetCurCtx(ctxno uint8) {
	m.curCtx = ctxno
}

// NumBytes returns the number of bytes consumed from the start of the
// stream so far.
func (m *MQDecoder) NumBytes() uint32 {
	return uint32(m.bp - m.start)
}

// =========================================================================
// MQ Encoder
// =========================================================================

// MQEncoder is the MQ arithmetic encoder defined in ISO 15444-1 Annex C.
type MQEncoder struct {
	// c is the code register.
	c uint32
	// a is the interval register.
	a uint32
	// ct is the count of remaining shifts before a byte-out.
	ct uint32

	// buf is the output byte buffer.
	buf []byte
	// bp is the current write position within buf.
	bp int

	// ctxs holds the state-table index for each of the 19 contexts.
	ctxs [NumContexts]uint8
	// curCtx is the index into ctxs[] of the active context.
	curCtx uint8
}

// InitEnc initialises the MQ encoder.
//
// Implements ISO 15444-1 C.2.8 Initialization of the encoder (INITENC).
// The caller must provide a buffer large enough for the encoded output.
func (m *MQEncoder) InitEnc(buf []byte) {
	m.buf = buf
	m.a = 0x8000
	m.c = 0
	// bp starts at -1 conceptually; we use index 0 but set buf[0] = 0
	// and start writing from 1. We handle this by keeping bp = 0 and
	// writing the "previous byte" at bp, advancing on byte-out.
	// OpenJPEG points bp to start-1. We mimic this by using a sentinel.
	m.bp = 0
	m.buf[0] = 0 // sentinel byte (not 0xff)
	m.ct = 12
	m.curCtx = 0
}

// Encode encodes a single binary decision.
//
// Implements ISO 15444-1 C.2 Encoding a decision (ENCODE).
func (m *MQEncoder) Encode(d uint32) {
	st := &mqcStates[m.ctxs[m.curCtx]]
	if st.mps == d {
		m.codemps()
	} else {
		m.codelps()
	}
}

// Flush terminates the encoding.
//
// Implements ISO 15444-1 C.2.9 Termination of coding (FLUSH).
func (m *MQEncoder) Flush() {
	m.setbits()
	m.c <<= m.ct
	m.byteout()
	m.c <<= m.ct
	m.byteout()

	// It is forbidden for a coding pass to end with 0xff.
	if m.buf[m.bp] != 0xff {
		m.bp++
	}
}

// Bytes returns the encoded byte stream (excluding the sentinel byte at
// position 0).
func (m *MQEncoder) Bytes() []byte {
	// The encoded data is in buf[1:bp].
	if m.bp <= 1 {
		return nil
	}
	return m.buf[1:m.bp]
}

// NumBytesEncoded returns the number of encoded bytes.
func (m *MQEncoder) NumBytesEncoded() int {
	if m.bp <= 1 {
		return 0
	}
	return m.bp - 1
}

// ResetStates resets all 19 contexts to the initial state.
func (m *MQEncoder) ResetStates() {
	for i := range m.ctxs {
		m.ctxs[i] = 0
	}
}

// SetState sets context ctxno to the given state.
func (m *MQEncoder) SetState(ctxno uint32, msb uint32, prob int32) {
	m.ctxs[ctxno] = uint8(msb + uint32(prob<<1))
}

// SetCurCtx sets the active context number.
func (m *MQEncoder) SetCurCtx(ctxno uint8) {
	m.curCtx = ctxno
}

// byteout outputs a byte, handling the 0xFF bit-stuffing rule.
//
// Implements the encoder BYTEOUT procedure from ISO 15444-1.
func (m *MQEncoder) byteout() {
	if m.buf[m.bp] == 0xff {
		m.bp++
		m.buf[m.bp] = byte(m.c >> 20)
		m.c &= 0xfffff
		m.ct = 7
	} else {
		if (m.c & 0x8000000) == 0 {
			m.bp++
			m.buf[m.bp] = byte(m.c >> 19)
			m.c &= 0x7ffff
			m.ct = 8
		} else {
			m.buf[m.bp]++
			if m.buf[m.bp] == 0xff {
				m.c &= 0x7ffffff
				m.bp++
				m.buf[m.bp] = byte(m.c >> 20)
				m.c &= 0xfffff
				m.ct = 7
			} else {
				m.bp++
				m.buf[m.bp] = byte(m.c >> 19)
				m.c &= 0x7ffff
				m.ct = 8
			}
		}
	}
}

// renorme performs interval renormalization while encoding.
func (m *MQEncoder) renorme() {
	for {
		m.a <<= 1
		m.c <<= 1
		m.ct--
		if m.ct == 0 {
			m.byteout()
		}
		if (m.a & 0x8000) != 0 {
			break
		}
	}
}

// codemps encodes the most probable symbol.
func (m *MQEncoder) codemps() {
	ctx := m.curCtx
	st := &mqcStates[m.ctxs[ctx]]
	m.a -= st.qeval
	if (m.a & 0x8000) == 0 {
		if m.a < st.qeval {
			m.a = st.qeval
		} else {
			m.c += st.qeval
		}
		m.ctxs[ctx] = st.nmps
		m.renorme()
	} else {
		m.c += st.qeval
	}
}

// codelps encodes the least probable symbol.
func (m *MQEncoder) codelps() {
	ctx := m.curCtx
	st := &mqcStates[m.ctxs[ctx]]
	m.a -= st.qeval
	if m.a < st.qeval {
		m.c += st.qeval
	} else {
		m.a = st.qeval
	}
	m.ctxs[ctx] = st.nlps
	m.renorme()
}

// setbits fills c with 1's for flushing.
func (m *MQEncoder) setbits() {
	tempc := m.c + m.a
	m.c |= 0xffff
	if m.c >= tempc {
		m.c -= 0x8000
	}
}

// SegmarkEnc encodes the segmentation marker symbol (0xa = 1010).
func (m *MQEncoder) SegmarkEnc() {
	m.SetCurCtx(T1CtxnoUNI)
	for i := uint32(1); i < 5; i++ {
		m.Encode(i % 2)
	}
}

// ResetEncStates resets states and sets the standard encoder initial
// contexts (same as decoder).
func (m *MQEncoder) ResetEncStates() {
	m.ResetStates()
	m.SetState(T1CtxnoUNI, 0, 46)
	m.SetState(T1CtxnoAGG, 0, 3)
	m.SetState(T1CtxnoZC, 0, 4)
}

// --------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------

// initDecCommon is shared setup for both MQ and raw decoder initialisation.
// It appends sentinel 0xFF 0xFF bytes past the real data end so that the
// byte-in routine can always read ahead without bounds checking.
func (m *MQDecoder) initDecCommon(data []byte) {
	// We need at least cblkDataExtra extra bytes beyond data.
	// The caller guarantees the backing array is large enough.
	// We work on a slice that includes the sentinel region.
	total := len(data) + cblkDataExtra
	if cap(data) < total {
		// If the caller didn't provide enough capacity we must
		// allocate. This path is a safety net; callers should
		// pre-allocate.
		buf := make([]byte, total)
		copy(buf, data)
		data = buf
	} else {
		data = data[:total]
	}

	m.data = data
	m.start = 0
	m.end = total - cblkDataExtra // == original len(data)
	m.bp = 0

	// Backup the bytes we are about to overwrite with sentinels.
	copy(m.backup[:], m.data[m.end:m.end+cblkDataExtra])
	m.data[m.end] = 0xFF
	m.data[m.end+1] = 0xFF
}

// byteIn reads the next byte from the code stream into the C register.
//
// Implements ISO 15444-1 C.3.4 Compressed data input (BYTEIN).
// After a 0xFF byte, if the following byte is > 0x8F (i.e. a marker
// prefix), we do not advance bp -- this is the end-of-stream handling.
func (m *MQDecoder) byteIn() {
	// The sentinel 0xFF 0xFF at the end guarantees bp+1 is always valid.
	nextByte := uint32(m.data[m.bp+1])
	if m.data[m.bp] == 0xff {
		if nextByte > 0x8f {
			m.c += 0xff00
			m.ct = 8
			m.endOfByteStreamCounter++
		} else {
			m.bp++
			m.c += nextByte << 9
			m.ct = 7
		}
	} else {
		m.bp++
		m.c += nextByte << 8
		m.ct = 8
	}
}

// renormD performs interval renormalisation while decoding.
//
// Implements ISO 15444-1 C.3.3 Renormalization in the decoder (RENORMD).
func (m *MQDecoder) renormD() {
	for {
		if m.ct == 0 {
			m.byteIn()
		}
		m.a <<= 1
		m.c <<= 1
		m.ct--
		if m.a >= 0x8000 {
			break
		}
	}
}

// mpsExchange performs the conditional MPS/LPS exchange when the MPS
// sub-interval is selected but the interval is not yet normalised.
//
// Implements ISO 15444-1 C.3.2, MPS_EXCHANGE procedure.
func (m *MQDecoder) mpsExchange(st *mqcState) uint32 {
	ctx := m.curCtx
	if m.a < st.qeval {
		// Interval inversion: LPS is actually more probable here.
		d := st.mps ^ 1
		m.ctxs[ctx] = st.nlps
		return d
	}
	d := st.mps
	m.ctxs[ctx] = st.nmps
	return d
}

// lpsExchange performs the conditional MPS/LPS exchange when the LPS
// sub-interval is selected.
//
// Implements ISO 15444-1 C.3.2, LPS_EXCHANGE procedure.
func (m *MQDecoder) lpsExchange(st *mqcState) uint32 {
	ctx := m.curCtx
	if m.a < st.qeval {
		m.a = st.qeval
		d := st.mps
		m.ctxs[ctx] = st.nmps
		return d
	}
	m.a = st.qeval
	d := st.mps ^ 1
	m.ctxs[ctx] = st.nlps
	return d
}
