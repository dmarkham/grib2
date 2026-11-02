// Package aec — Adaptive Entropy Coding encoder
// per CCSDS 121.0-B-3 recommended standard for lossless data compression.
// This is a pure Go port of the libaec encoder.
//
// Copyright 2026 Mathis Rosenhauer, Moritz Hanke, Joerg Behrens, Luis Kornblueh
// (original C implementation)
// Go port by Dan Markham.
package aec

import (
	"fmt"
	"math"
)

const (
	// rosEncoder marks a zero-block run that is coded as "Remainder Of
	// Segment". It must not collide with a real run length: libaec uses -1
	// (encode.h). A value of 5 made every run of exactly five zero blocks
	// come out as ROS, which the decoder expands to the end of the segment.
	rosEncoder = -1

	// maxEncoderRSI is libaec's upper limit on the reference sample interval.
	maxEncoderRSI = 4096
	// maxNotEnforcedBlockSize is libaec's upper limit on the block size when
	// NotEnforce is set.
	maxNotEnforcedBlockSize = 256
)

// encoder holds all internal state for encoding a single AEC stream.
type encoder struct {
	// Options
	bitsPerSample int
	blockSize     int
	rsi           int
	flags         uint32

	// Derived
	idLen     int
	xmin      uint32
	xmax      uint32
	pp        bool // preprocessor enabled
	rsiSize   int  // rsi * blockSize
	kmax      int
	uncompLen uint32 // uncompressed block length in bits
	ref       int    // 1 if the current block carries the reference sample
	refSample uint32

	// Current splitting position (persists between blocks for fast search)
	k int

	// Zero block tracking
	zeroBlocks    int
	zeroRef       int
	zeroRefSample uint32

	// RSI data buffers
	dataRaw []uint32
	dataPP  []uint32
	block   []uint32

	// Output bit accumulator
	out  []byte
	bits int // free bits remaining in current byte (0-8)

	// SE table
	seTable [2 * (seTableSize + 1)]int
}

// Encode encodes samples using AEC/CCSDS-121.0-B-3 compression.
// It returns the compressed data.
//
// The options follow libaec's aec_encode_init: BitsPerSample 1..32, RSI
// 1..4096, and BlockSize 8, 16, 32 or 64 (any even size up to 256 when
// NotEnforce is set). Data3Byte and DataMSB only describe a byte layout and
// are ignored, because samples are passed as int32.
//
// Unsigned samples must lie in [0, 2^BitsPerSample-1] (at 32 bits every int32
// is taken as its uint32 bit pattern). With DataSigned, samples must lie in
// [-2^(BitsPerSample-1), 2^(BitsPerSample-1)-1]. Out-of-range samples return
// ErrData instead of being silently truncated.
//
// With PadRSI, each RSI is padded with zero bits to a byte boundary, as
// libaec does when it is built with ENABLE_RSI_PADDING. The output is
// byte-identical to libaec's aec_buffer_encode for the same options.
func Encode(samples []int32, opts Options) ([]byte, error) {
	if opts.BitsPerSample <= 0 || opts.BitsPerSample > 32 {
		return nil, fmt.Errorf("%w: bits_per_sample must be 1..32, got %d", ErrConfig, opts.BitsPerSample)
	}
	if opts.RSI <= 0 || opts.RSI > maxEncoderRSI {
		return nil, fmt.Errorf("%w: RSI must be 1..%d, got %d", ErrConfig, maxEncoderRSI, opts.RSI)
	}
	if opts.Flags&NotEnforce != 0 {
		if opts.BlockSize <= 0 || opts.BlockSize&1 != 0 || opts.BlockSize > maxNotEnforcedBlockSize {
			return nil, fmt.Errorf("%w: block_size must be even and 2..%d, got %d", ErrConfig, maxNotEnforcedBlockSize, opts.BlockSize)
		}
	} else {
		switch opts.BlockSize {
		case 8, 16, 32, 64:
		default:
			return nil, fmt.Errorf("%w: block_size must be 8, 16, 32 or 64, got %d", ErrConfig, opts.BlockSize)
		}
	}

	e := &encoder{
		bitsPerSample: opts.BitsPerSample,
		blockSize:     opts.BlockSize,
		rsi:           opts.RSI,
		flags:         opts.Flags,
	}

	if err := e.init(); err != nil {
		return nil, err
	}

	raw, err := e.toRaw(samples)
	if err != nil {
		return nil, err
	}

	e.encode(raw)
	return e.out, nil
}

func (e *encoder) init() error {
	createSETable(e.seTable[:])

	if e.bitsPerSample > 16 {
		e.idLen = 5
	} else if e.bitsPerSample > 8 {
		e.idLen = 4
	} else {
		if e.flags&Restricted != 0 {
			if e.bitsPerSample <= 4 {
				if e.bitsPerSample <= 2 {
					e.idLen = 1
				} else {
					e.idLen = 2
				}
			} else {
				return fmt.Errorf("%w: restricted mode requires bits_per_sample <= 4", ErrConfig)
			}
		} else {
			e.idLen = 3
		}
	}

	if e.flags&DataSigned != 0 {
		e.xmax = uint32((int64(1) << (e.bitsPerSample - 1)) - 1)
		e.xmin = ^e.xmax
	} else {
		e.xmin = 0
		e.xmax = uint32((uint64(1) << e.bitsPerSample) - 1)
	}

	e.kmax = (1 << e.idLen) - 3
	e.rsiSize = e.rsi * e.blockSize
	e.uncompLen = uint32(e.blockSize * e.bitsPerSample)
	e.pp = e.flags&DataPreprocess != 0

	// Allocate buffers for one RSI
	e.dataPP = make([]uint32, e.rsiSize)
	if e.pp {
		e.dataRaw = make([]uint32, e.rsiSize)
	} else {
		e.dataRaw = e.dataPP
	}

	e.out = make([]byte, 0, e.rsiSize*4)
	e.bits = 8
	e.out = append(e.out, 0) // current (partially filled) output byte

	return nil
}

// toRaw validates the samples and converts them to the raw bit patterns
// libaec reads from its input buffer: the low BitsPerSample bits of each
// sample. Signed samples are therefore stored in two's complement truncated
// to BitsPerSample bits; the signed preprocessor sign-extends them again.
func (e *encoder) toRaw(samples []int32) ([]uint32, error) {
	raw := make([]uint32, len(samples))
	signed := e.flags&DataSigned != 0
	mask := uint32((uint64(1) << e.bitsPerSample) - 1)
	lo := -(int64(1) << (e.bitsPerSample - 1))
	hi := (int64(1) << (e.bitsPerSample - 1)) - 1
	for i, s := range samples {
		if signed {
			if int64(s) < lo || int64(s) > hi {
				return nil, fmt.Errorf("%w: sample %d = %d out of signed %d-bit range [%d, %d]", ErrData, i, s, e.bitsPerSample, lo, hi)
			}
			raw[i] = uint32(s) & mask
		} else {
			u := uint32(s)
			if u > e.xmax {
				return nil, fmt.Errorf("%w: sample %d = %d out of unsigned %d-bit range [0, %d]", ErrData, i, u, e.bitsPerSample, e.xmax)
			}
			raw[i] = u
		}
	}
	return raw, nil
}

// emit writes a sequence of bits to the output stream.
func (e *encoder) emit(data uint32, nbits int) {
	if nbits <= e.bits {
		e.bits -= nbits
		e.out[len(e.out)-1] += byte(data << e.bits)
	} else {
		nbits -= e.bits
		e.out[len(e.out)-1] += byte(uint64(data) >> nbits)

		for nbits > 8 {
			nbits -= 8
			e.out = append(e.out, byte(data>>nbits))
		}

		e.bits = 8 - nbits
		e.out = append(e.out, byte(data<<e.bits))
	}
}

// emitfs emits a fundamental sequence: fs zero bits followed by one 1 bit.
func (e *encoder) emitfs(fs int) {
	for {
		if fs < e.bits {
			e.bits -= fs + 1
			e.out[len(e.out)-1] += 1 << e.bits
			break
		} else {
			fs -= e.bits
			e.out = append(e.out, 0)
			e.bits = 8
		}
	}
}

// emitblock_fs emits the FS (unary) part for a whole block.
// For each sample, it emits (sample >> k) zero bits followed by a 1 bit.
func (e *encoder) emitblock_fs(k int, refOffset int) {
	for i := refOffset; i < e.blockSize; i++ {
		e.emitfs(int(e.block[i] >> k))
	}
}

// emitblock emits k LSBs of all samples in the block.
func (e *encoder) emitblock(k int, refOffset int) {
	if k == 0 {
		return
	}
	mask := uint32((uint64(1) << k) - 1)
	for i := refOffset; i < e.blockSize; i++ {
		e.emit(e.block[i]&mask, k)
	}
}

// encode runs the libaec block loop (m_get_block / m_check_zero_block /
// m_select_code_option) over the raw samples.
func (e *encoder) encode(raw []uint32) {
	n := len(raw)

	for pos := 0; pos < n; pos += e.rsiSize {
		// Load one RSI, padding a short final RSI with its last sample.
		rsiLen := e.rsiSize
		if pos+rsiLen > n {
			rsiLen = n - pos
		}
		copy(e.dataRaw[:rsiLen], raw[pos:pos+rsiLen])
		for i := rsiLen; i < e.rsiSize; i++ {
			e.dataRaw[i] = e.dataRaw[rsiLen-1]
		}

		// Only blocks that hold real samples are coded.
		blocksInRSI := (rsiLen + e.blockSize - 1) / e.blockSize

		if e.pp {
			e.preprocess()
			e.ref = 1
			e.uncompLen = uint32((e.blockSize - 1) * e.bitsPerSample)
		} else {
			e.ref = 0
			e.uncompLen = uint32(e.blockSize * e.bitsPerSample)
		}
		e.zeroBlocks = 0

		for b := 0; b < blocksInRSI; b++ {
			if b > 0 && e.ref == 1 {
				e.ref = 0
				e.uncompLen = uint32(e.blockSize * e.bitsPerSample)
			}
			e.block = e.dataPP[b*e.blockSize : (b+1)*e.blockSize]

			isLastBlock := b == blocksInRSI-1
			blocksDispensed := b + 1

			allZero := true
			for _, v := range e.block {
				if v != 0 {
					allZero = false
					break
				}
			}

			if !allZero {
				if e.zeroBlocks > 0 {
					// Emit the pending zero run first.
					e.encodeZero()
				}
				e.selectCodeOption()
				continue
			}

			e.zeroBlocks++
			if e.zeroBlocks == 1 {
				e.zeroRef = e.ref
				e.zeroRefSample = e.refSample
			}
			// Flush the run at the end of the RSI or a 64-block segment.
			if isLastBlock || blocksDispensed%64 == 0 {
				if e.zeroBlocks > 4 {
					e.zeroBlocks = rosEncoder
				}
				e.encodeZero()
			}
		}

		if e.flags&PadRSI != 0 {
			e.emit(0, e.bits%8)
		}
	}
	// Like libaec, the final partially filled byte is always output; its
	// unused low bits are zero.
}

// preprocess applies the unsigned or signed preprocessor to the current RSI.
func (e *encoder) preprocess() {
	if e.flags&DataSigned != 0 {
		e.preprocessSigned()
	} else {
		e.preprocessUnsigned()
	}
}

// preprocessUnsigned implements the unsigned preprocessor from libaec.
func (e *encoder) preprocessUnsigned() {
	x := e.dataRaw
	d := e.dataPP
	xmax := e.xmax
	rsi := e.rsi*e.blockSize - 1

	e.refSample = x[0]
	d[0] = 0 // the reference sample is coded separately; libaec zeroes d[0]

	for i := 0; i < rsi; i++ {
		if x[i+1] >= x[i] {
			D := x[i+1] - x[i]
			if D <= x[i] {
				d[i+1] = 2 * D
			} else {
				d[i+1] = x[i+1]
			}
		} else {
			D := x[i] - x[i+1]
			if D <= xmax-x[i] {
				d[i+1] = 2*D - 1
			} else {
				d[i+1] = xmax - x[i+1]
			}
		}
	}
}

// preprocessSigned implements the signed preprocessor from libaec.
func (e *encoder) preprocessSigned() {
	x := e.dataRaw
	d := e.dataPP
	xmax := e.xmax
	xmin := e.xmin
	rsi := e.rsi*e.blockSize - 1
	m := uint32(1) << (e.bitsPerSample - 1)

	e.refSample = x[0]
	d[0] = 0
	// Sign extension
	x[0] = (x[0] ^ m) - m

	for i := 0; i < rsi; i++ {
		x[i+1] = (x[i+1] ^ m) - m
		if int32(x[i+1]) < int32(x[i]) {
			D := x[i] - x[i+1]
			if D <= xmax-x[i] {
				d[i+1] = 2*D - 1
			} else {
				d[i+1] = xmax - x[i+1]
			}
		} else {
			D := x[i+1] - x[i]
			if D <= x[i]-xmin {
				d[i+1] = 2 * D
			} else {
				d[i+1] = x[i+1] - xmin
			}
		}
	}
}

// blockFS computes the sum of fundamental sequence lengths for all samples
// in the current block at splitting position k.
func (e *encoder) blockFS(k int) uint64 {
	var fs uint64
	for i := 0; i < e.blockSize; i++ {
		fs += uint64(e.block[i] >> k)
	}
	return fs
}

// assessSplitting finds the optimal splitting position k and returns
// the CDS length for the best split.
func (e *encoder) assessSplitting() uint32 {
	thisBS := uint64(e.blockSize - e.ref)

	lenMin := uint64(math.MaxUint64)
	k := e.k
	kMin := k

	noTurn := 0
	if k == 0 {
		noTurn = 1
	}
	dir := 1 // 1 = increasing k, 0 = decreasing k

	for {
		fsLen := e.blockFS(k)
		length := fsLen + thisBS*uint64(k+1)

		if length < lenMin {
			if lenMin < math.MaxUint64 {
				noTurn = 1
			}
			lenMin = length
			kMin = k

			if dir == 1 {
				if fsLen < thisBS || k >= e.kmax {
					if noTurn != 0 {
						break
					}
					k = e.k - 1
					dir = 0
					noTurn = 1
				} else {
					k++
				}
			} else {
				if fsLen >= thisBS || k == 0 {
					break
				}
				k--
			}
		} else {
			if noTurn != 0 {
				break
			}
			k = e.k - 1
			dir = 0
			noTurn = 1
		}
	}
	e.k = kMin
	return uint32(lenMin)
}

// assessSE returns the CDS length for second extension coding.
// Returns math.MaxUint32 if length exceeds uncompressed length.
func (e *encoder) assessSE() uint32 {
	var length uint64 = 1

	for i := 0; i < e.blockSize; i += 2 {
		d := uint64(e.block[i]) + uint64(e.block[i+1])
		length += d*(d+1)/2 + uint64(e.block[i+1]) + 1
		if length > uint64(e.uncompLen) {
			return math.MaxUint32
		}
	}
	return uint32(length)
}

// selectCodeOption chooses the best encoding for the current block.
func (e *encoder) selectCodeOption() {
	var splitLen uint32
	if e.idLen > 1 {
		splitLen = e.assessSplitting()
	} else {
		splitLen = math.MaxUint32
	}
	seLen := e.assessSE()

	if splitLen < e.uncompLen {
		if splitLen < seLen {
			e.encodeSplitting()
		} else {
			e.encodeSE()
		}
	} else {
		if e.uncompLen <= seLen {
			e.encodeUncomp()
		} else {
			e.encodeSE()
		}
	}
}

// encodeSplitting encodes the current block using the splitting (Golomb-Rice) option.
func (e *encoder) encodeSplitting() {
	k := e.k
	e.emit(uint32(k+1), e.idLen)
	if e.ref != 0 {
		e.emit(e.refSample, e.bitsPerSample)
	}

	e.emitblock_fs(k, e.ref)
	if k > 0 {
		e.emitblock(k, e.ref)
	}
}

// encodeUncomp encodes the current block uncompressed.
func (e *encoder) encodeUncomp() {
	e.emit((1<<e.idLen)-1, e.idLen)
	if e.ref != 0 {
		e.block[0] = e.refSample
	}
	e.emitblock(e.bitsPerSample, 0)
}

// encodeSE encodes the current block using the second extension option.
func (e *encoder) encodeSE() {
	e.emit(1, e.idLen+1)
	if e.ref != 0 {
		e.emit(e.refSample, e.bitsPerSample)
	}

	for i := 0; i < e.blockSize; i += 2 {
		d := e.block[i] + e.block[i+1]
		e.emitfs(int(d*(d+1)/2 + e.block[i+1]))
	}
}

// encodeZero encodes accumulated zero blocks.
func (e *encoder) encodeZero() {
	e.emit(0, e.idLen+1)

	if e.zeroRef != 0 {
		e.emit(e.zeroRefSample, e.bitsPerSample)
	}

	if e.zeroBlocks == rosEncoder {
		e.emitfs(4)
	} else if e.zeroBlocks >= 5 {
		e.emitfs(e.zeroBlocks)
	} else {
		e.emitfs(e.zeroBlocks - 1)
	}

	e.zeroBlocks = 0
}
