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
	rosEncoder = 5 // Remainder Of Segment marker for zero block encoding
)

// encoder holds all internal state for encoding a single AEC stream.
type encoder struct {
	// Options
	bitsPerSample int
	blockSize     int
	rsi           int
	flags         uint32

	// Derived
	idLen          int
	xmin           uint32
	xmax           uint32
	pp             bool // preprocessor enabled
	rsiSize        int  // rsi * blockSize
	kmax           int
	uncompLen     uint32 // uncompressed block length in bits
	ref            int    // 1 if first block in RSI, 0 otherwise
	refSample      uint32
	encodedBlkSize int

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
	acc  uint64
	bits int // free bits remaining in current byte (1-8)

	// SE table
	seTable [2 * (seTableSize + 1)]int
}

// Encode encodes samples using AEC/CCSDS-121.0-B-3 compression.
// It returns the compressed data.
func Encode(samples []int32, opts Options) ([]byte, error) {
	if opts.BitsPerSample == 0 || opts.BitsPerSample > 32 {
		return nil, fmt.Errorf("%w: bits_per_sample must be 1..32, got %d", ErrConfig, opts.BitsPerSample)
	}
	if opts.BlockSize == 0 || opts.BlockSize&1 != 0 {
		return nil, fmt.Errorf("%w: block_size must be even and > 0, got %d", ErrConfig, opts.BlockSize)
	}
	if opts.RSI == 0 {
		return nil, fmt.Errorf("%w: RSI must be > 0", ErrConfig)
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

	e.encode(samples)
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
		if e.bitsPerSample == 32 {
			e.xmax = 0xFFFFFFFF
		} else {
			e.xmax = uint32((uint64(1) << e.bitsPerSample) - 1)
		}
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

	// Pre-allocate output buffer (worst case: uncompressed + overhead)
	e.out = make([]byte, 0, e.rsiSize*4)
	e.bits = 8
	e.out = append(e.out, 0) // first byte of output

	return nil
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

func (e *encoder) encode(samples []int32) {
	n := len(samples)

	// Convert to uint32
	raw := make([]uint32, n)
	for i, s := range samples {
		raw[i] = uint32(s)
	}

	// Pad to full RSI boundary
	for len(raw)%e.rsiSize != 0 {
		raw = append(raw, raw[len(raw)-1])
	}

	pos := 0
	for pos < n {
		// Load one RSI
		rsiLen := e.rsiSize
		if pos+rsiLen > len(raw) {
			rsiLen = len(raw) - pos
		}
		copy(e.dataRaw[:rsiLen], raw[pos:pos+rsiLen])

		// Pad if needed
		for i := rsiLen; i < e.rsiSize; i++ {
			e.dataRaw[i] = e.dataRaw[rsiLen-1]
		}

		// Preprocess
		if e.pp {
			e.preprocess()
		}

		// Encode blocks in this RSI
		e.ref = 1
		if e.pp {
			e.encodedBlkSize = e.blockSize - 1
			e.uncompLen = uint32((e.blockSize - 1) * e.bitsPerSample)
		} else {
			e.encodedBlkSize = e.blockSize
			e.uncompLen = uint32(e.blockSize * e.bitsPerSample)
		}
		e.zeroBlocks = 0

		blocksInRSI := e.rsi
		// If we don't have enough real samples for all blocks, limit
		remaining := n - pos
		realBlocks := (remaining + e.blockSize - 1) / e.blockSize
		if realBlocks < blocksInRSI {
			blocksInRSI = realBlocks
		}

		for b := 0; b < blocksInRSI; b++ {
			e.block = e.dataPP[b*e.blockSize : (b+1)*e.blockSize]

			isLastBlock := (b == blocksInRSI-1)
			blocksDispensed := b + 1

			// Check if block is all zeros (after preprocessing)
			allZero := true
			for i := 0; i < e.blockSize; i++ {
				if e.block[i] != 0 {
					allZero = false
					break
				}
			}

			if allZero {
				e.zeroBlocks++
				if e.zeroBlocks == 1 {
					e.zeroRef = e.ref
					e.zeroRefSample = e.refSample
				}
				// Flush zero blocks if: end of RSI, or at a 64-block boundary, or last block
				if isLastBlock || blocksDispensed%64 == 0 {
					if e.zeroBlocks > 4 {
						e.zeroBlocks = rosEncoder
					}
					e.encodeZero()
				}
			} else {
				if e.zeroBlocks > 0 {
					// Flush accumulated zero blocks first
					e.encodeZero()
				}
				e.selectCodeOption()
			}

			// After first block, subsequent blocks don't have ref
			if e.ref == 1 {
				e.ref = 0
				e.encodedBlkSize = e.blockSize
				if !e.pp {
					e.uncompLen = uint32(e.blockSize * e.bitsPerSample)
				} else {
					e.uncompLen = uint32(e.blockSize * e.bitsPerSample)
				}
			}
		}

		// Flush any remaining zero blocks
		if e.zeroBlocks > 0 {
			if e.zeroBlocks > 4 {
				e.zeroBlocks = rosEncoder
			}
			e.encodeZero()
		}

		pos += e.rsiSize
	}

	// Pad final byte with zero bits
	if e.bits < 8 {
		// The current byte already has the right content, just ensure it's flushed
	}
	// Trim trailing zero byte if bits == 8 and we appended an empty byte
	if e.bits == 8 && len(e.out) > 1 && e.out[len(e.out)-1] == 0 {
		e.out = e.out[:len(e.out)-1]
	}
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
	d[0] = x[0] // reference sample stored as-is in preprocessed buffer (but not encoded in block)

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
	d[0] = x[0]
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
