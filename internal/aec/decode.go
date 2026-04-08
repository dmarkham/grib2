// Package aec implements the Adaptive Entropy Coding (AEC) decoder
// per CCSDS 121.0-B-3 recommended standard for lossless data compression.
// This is a pure Go port of the libaec decoder.
//
// Copyright 2026 Mathis Rosenhauer, Moritz Hanke, Joerg Behrens, Luis Kornblueh
// (original C implementation)
// Go port by Dan Markham.
package aec

import (
	"errors"
	"fmt"
	"math/bits"
)

// Flag constants matching the AEC/CCSDS standard.
const (
	DataSigned     uint32 = 1  // AEC_DATA_SIGNED: samples are signed
	Data3Byte      uint32 = 2  // AEC_DATA_3BYTE: 24-bit samples in 3 bytes
	DataMSB        uint32 = 4  // AEC_DATA_MSB: most significant bit first
	DataPreprocess uint32 = 8  // AEC_DATA_PREPROCESS: use preprocessor
	Restricted     uint32 = 16 // AEC_RESTRICTED: restricted code options
	PadRSI         uint32 = 32 // AEC_PAD_RSI: pad RSI to byte boundary
)

// Options configures the AEC decoder.
type Options struct {
	BitsPerSample int    // Resolution in bits per sample (1..32)
	BlockSize     int    // Block size in samples (must be even, > 0)
	RSI           int    // Reference sample interval (number of blocks between reference samples)
	Flags         uint32 // Combination of flag constants
}

// Errors returned by the decoder.
var (
	ErrConfig = errors.New("aec: configuration error")
	ErrData   = errors.New("aec: data error")
)

const (
	seTableSize = 90
	ros         = 5

	mContinue = 1
	mExit     = 0
	mError    = -1
)

// decoder holds all internal state for decoding a single AEC stream.
type decoder struct {
	// Options
	bitsPerSample int
	blockSize     int
	rsi           int
	flags         uint32

	// Derived
	idLen            int
	bytesPerSample   int
	outBlkLen        int
	inBlkLen         int
	xmin             uint32
	xmax             uint32
	pp               bool // preprocessor enabled
	rsiSize          int  // rsi * blockSize
	encodedBlockSize int
	ref              int // 1 if current block has reference sample

	// Bit I/O state
	acc  uint64
	bitp int

	// Mode state
	id            int
	fs            uint32
	sampleCounter int

	// RSI buffer
	rsiBuf    []uint32
	rsip      int // write position in rsiBuf
	flushPos  int // first not-yet-flushed position in rsiBuf
	lastOut   int32

	// SE table
	seTable [2 * (seTableSize + 1)]int

	// ID dispatch table (function indices)
	idTable []int // maps id -> mode constant

	// Input
	input []byte
	inPos int

	// Output
	output   []uint32
	availOut int // tracks remaining output capacity in bytes (for slow-path compat)
}

// Mode constants for the id table dispatch.
const (
	modeLowEntropy = 0
	modeSplit      = 1
	modeUncomp     = 2
)

// Decode decodes AEC/CCSDS-121.0-B-3 compressed data.
// It returns the decoded samples as int32 values.
func Decode(input []byte, opts Options) ([]int32, error) {
	if opts.BitsPerSample == 0 || opts.BitsPerSample > 32 {
		return nil, fmt.Errorf("%w: bits_per_sample must be 1..32, got %d", ErrConfig, opts.BitsPerSample)
	}
	if opts.BlockSize == 0 || opts.BlockSize&1 != 0 {
		return nil, fmt.Errorf("%w: block_size must be even and > 0, got %d", ErrConfig, opts.BlockSize)
	}
	if opts.RSI == 0 {
		return nil, fmt.Errorf("%w: RSI must be > 0", ErrConfig)
	}

	d := &decoder{
		bitsPerSample: opts.BitsPerSample,
		blockSize:     opts.BlockSize,
		rsi:           opts.RSI,
		flags:         opts.Flags,
		input:         input,
	}

	if err := d.init(); err != nil {
		return nil, err
	}

	if err := d.decode(); err != nil {
		return nil, err
	}

	return d.collectOutput(), nil
}

func (d *decoder) init() error {
	createSETable(d.seTable[:])

	if d.bitsPerSample > 16 {
		d.idLen = 5
		if d.bitsPerSample <= 24 && d.flags&Data3Byte != 0 {
			d.bytesPerSample = 3
		} else {
			d.bytesPerSample = 4
		}
	} else if d.bitsPerSample > 8 {
		d.bytesPerSample = 2
		d.idLen = 4
	} else {
		if d.flags&Restricted != 0 {
			if d.bitsPerSample <= 4 {
				if d.bitsPerSample <= 2 {
					d.idLen = 1
				} else {
					d.idLen = 2
				}
			} else {
				return fmt.Errorf("%w: restricted mode requires bits_per_sample <= 4", ErrConfig)
			}
		} else {
			d.idLen = 3
		}
		d.bytesPerSample = 1
	}

	d.outBlkLen = d.blockSize * d.bytesPerSample

	if d.flags&DataSigned != 0 {
		d.xmax = uint32((int64(1) << (d.bitsPerSample - 1)) - 1)
		d.xmin = ^d.xmax
	} else {
		d.xmin = 0
		if d.bitsPerSample == 32 {
			d.xmax = 0xFFFFFFFF
		} else {
			d.xmax = uint32((uint64(1) << d.bitsPerSample) - 1)
		}
	}

	d.inBlkLen = (d.blockSize*d.bitsPerSample + d.idLen) / 8 + 16

	modi := 1 << d.idLen
	d.idTable = make([]int, modi)
	d.idTable[0] = modeLowEntropy
	for i := 1; i < modi-1; i++ {
		d.idTable[i] = modeSplit
	}
	d.idTable[modi-1] = modeUncomp

	d.rsiSize = d.rsi * d.blockSize
	// Guard against integer overflow and excessive allocation.
	if d.rsi > 0 && d.blockSize > 0 && d.rsi > (1<<30)/d.blockSize {
		return fmt.Errorf("%w: RSI*blockSize overflow: %d * %d", ErrConfig, d.rsi, d.blockSize)
	}
	const maxRSISize = 100_000_000
	if d.rsiSize > maxRSISize {
		return fmt.Errorf("%w: RSI*blockSize=%d exceeds maximum %d", ErrConfig, d.rsiSize, maxRSISize)
	}
	d.rsiBuf = make([]uint32, d.rsiSize)

	d.pp = d.flags&DataPreprocess != 0
	if d.pp {
		d.ref = 1
		d.encodedBlockSize = d.blockSize - 1
	} else {
		d.ref = 0
		d.encodedBlockSize = d.blockSize
	}

	d.rsip = 0
	d.flushPos = 0
	d.bitp = 0
	d.fs = 0

	return nil
}

func (d *decoder) availIn() int {
	return len(d.input) - d.inPos
}

// decode runs the state machine until all input is consumed.
func (d *decoder) decode() error {
	// We set availOut to a large value since we grow output dynamically.
	d.availOut = d.rsiSize * d.bytesPerSample

	// Cap the total number of output samples to prevent unbounded growth
	// on crafted input that produces massive output from tiny input.
	const maxOutputSamples = 100_000_000

	for {
		status := d.mID()
		if status == mError {
			return ErrData
		}
		if status == mExit {
			// Flush remaining
			d.flushOutput()
			return nil
		}
		// Check output growth
		if len(d.output) > maxOutputSamples {
			return fmt.Errorf("%w: output exceeds maximum %d samples", ErrData, maxOutputSamples)
		}
		// mContinue -> loop
	}
}

// collectOutput returns the accumulated output samples as int32.
func (d *decoder) collectOutput() []int32 {
	result := make([]int32, len(d.output))
	for i, v := range d.output {
		result[i] = int32(v)
	}
	return result
}

// -------------------------------------------------------
// Bit I/O
// -------------------------------------------------------

func (d *decoder) bitsAsk(n int) bool {
	for d.bitp < n {
		if d.availIn() == 0 {
			return false
		}
		d.acc <<= 8
		d.acc |= uint64(d.input[d.inPos])
		d.inPos++
		d.bitp += 8
	}
	return true
}

func (d *decoder) bitsGet(n int) uint32 {
	return uint32((d.acc >> (d.bitp - n)) & (^uint64(0) >> (64 - n)))
}

func (d *decoder) bitsDrop(n int) {
	d.bitp -= n
}

// directGet reads n bits from input when we know there's enough data.
// If insufficient input remains, the missing bytes are treated as zero.
func (d *decoder) directGet(n int) uint32 {
	if d.bitp < n {
		b := (63 - d.bitp) >> 3
		// Clamp to available input to prevent index-out-of-range.
		avail := d.availIn()
		if b > avail {
			b = avail
		}
		for i := 0; i < b; i++ {
			d.acc = (d.acc << 8) | uint64(d.input[d.inPos+i])
		}
		d.inPos += b
		d.bitp += b << 3
	}
	d.bitp -= n
	if n == 0 {
		return 0
	}
	return uint32((d.acc >> d.bitp) & (^uint64(0) >> (64 - n)))
}

// directGetFS reads a Fundamental Sequence: counts 0-bits until a 1-bit.
func (d *decoder) directGetFS() uint32 {
	var fs uint32

	if d.bitp > 0 {
		d.acc &= ^uint64(0) >> (64 - d.bitp)
	} else {
		d.acc = 0
	}

	// Limit iterations to prevent infinite loops on crafted all-zero input.
	const maxIter = 10_000
	iter := 0
	for d.acc == 0 {
		if d.availIn() < 7 {
			return 0
		}
		d.acc = (d.acc << 56) |
			(uint64(d.input[d.inPos]) << 48) |
			(uint64(d.input[d.inPos+1]) << 40) |
			(uint64(d.input[d.inPos+2]) << 32) |
			(uint64(d.input[d.inPos+3]) << 24) |
			(uint64(d.input[d.inPos+4]) << 16) |
			(uint64(d.input[d.inPos+5]) << 8) |
			uint64(d.input[d.inPos+6])
		d.inPos += 7
		fs += uint32(d.bitp)
		d.bitp = 56
		iter++
		if iter > maxIter {
			return 0
		}
	}

	// Find position of highest set bit
	i := 63 - bits.LeadingZeros64(d.acc)
	fs += uint32(d.bitp - i - 1)
	d.bitp = i
	return fs
}

// fsAsk ensures we can read a fundamental sequence (slow path).
func (d *decoder) fsAsk() bool {
	if !d.bitsAsk(1) {
		return false
	}
	const maxFS = 10_000_000 // prevent infinite loop on crafted all-zero input
	for (d.acc & (uint64(1) << (d.bitp - 1))) == 0 {
		if d.bitp == 1 {
			if d.availIn() == 0 {
				return false
			}
			d.acc <<= 8
			d.acc |= uint64(d.input[d.inPos])
			d.inPos++
			d.bitp += 8
		}
		d.fs++
		if d.fs > maxFS {
			return false
		}
		d.bitp--
	}
	return true
}

func (d *decoder) fsDrop() {
	d.fs = 0
	d.bitp--
}

func (d *decoder) putSample(s uint32) {
	if d.rsip >= len(d.rsiBuf) {
		return // prevent index-out-of-range on malformed data
	}
	d.rsiBuf[d.rsip] = s
	d.rsip++
	d.availOut -= d.bytesPerSample
}

func (d *decoder) copySample() bool {
	if !d.bitsAsk(d.bitsPerSample) || d.availOut < d.bytesPerSample {
		return false
	}
	d.putSample(d.bitsGet(d.bitsPerSample))
	d.bitsDrop(d.bitsPerSample)
	return true
}

func (d *decoder) bufferSpace() bool {
	return d.availIn() >= d.inBlkLen && d.availOut >= d.outBlkLen
}

func (d *decoder) rsiUsedSize() int {
	return d.rsip
}

// -------------------------------------------------------
// Flush / post-processing
// -------------------------------------------------------

func (d *decoder) flushOutput() {
	flushEnd := d.rsip
	if d.pp {
		if d.flushPos == 0 && d.rsip > 0 {
			d.lastOut = int32(d.rsiBuf[0])
			if d.flags&DataSigned != 0 {
				m := uint32(1) << (d.bitsPerSample - 1)
				d.lastOut = int32((uint32(d.lastOut) ^ m) - m)
			}
			d.output = append(d.output, uint32(d.lastOut))
			d.flushPos++
		}

		data := uint32(d.lastOut)
		xmax := d.xmax

		if d.xmin == 0 {
			// Unsigned case
			med := xmax/2 + 1
			for bp := d.flushPos; bp < flushEnd; bp++ {
				dd := d.rsiBuf[bp]
				halfD := (dd >> 1) + (dd & 1)
				var mask uint32
				if data&med != 0 {
					mask = xmax
				}
				if halfD <= mask^data {
					// d>>1 XOR ~((d&1) - 1)
					// When d&1==1: ~0 = 0xFFFFFFFF, so d>>1 ^ 0xFFFFFFFF = -(d>>1)-1
					// When d&1==0: ~(0xFFFFFFFF) = 0, so d>>1 ^ 0 = d>>1
					data += (dd >> 1) ^ (^((dd & 1) - 1))
				} else {
					data = mask ^ dd
				}
				d.output = append(d.output, data)
			}
			d.lastOut = int32(data)
		} else {
			// Signed case
			for bp := d.flushPos; bp < flushEnd; bp++ {
				dd := d.rsiBuf[bp]
				halfD := (dd >> 1) + (dd & 1)

				if int32(data) < 0 {
					if halfD <= xmax+data+1 {
						data += (dd >> 1) ^ (^((dd & 1) - 1))
					} else {
						data = dd - xmax - 1
					}
				} else {
					if halfD <= xmax-data {
						data += (dd >> 1) ^ (^((dd & 1) - 1))
					} else {
						data = xmax - dd
					}
				}
				d.output = append(d.output, data)
			}
			d.lastOut = int32(data)
		}
	} else {
		for bp := d.flushPos; bp < flushEnd; bp++ {
			d.output = append(d.output, d.rsiBuf[bp])
		}
	}
	d.flushPos = d.rsip
}

// -------------------------------------------------------
// State machine modes
// -------------------------------------------------------

func (d *decoder) mID() int {
	if d.availIn() >= d.inBlkLen {
		d.id = int(d.directGet(d.idLen))
	} else {
		if !d.bitsAsk(d.idLen) {
			return mExit
		}
		d.id = int(d.bitsGet(d.idLen))
		d.bitsDrop(d.idLen)
	}

	mode := d.idTable[d.id]
	switch mode {
	case modeLowEntropy:
		return d.mLowEntropy()
	case modeSplit:
		return d.mSplit()
	case modeUncomp:
		return d.mUncomp()
	}
	return mError
}

func (d *decoder) mNextCDS() int {
	if d.rsiSize == d.rsiUsedSize() {
		d.flushOutput()
		d.flushPos = 0
		d.rsip = 0
		if d.pp {
			d.ref = 1
			d.encodedBlockSize = d.blockSize - 1
		}
		if d.flags&PadRSI != 0 {
			d.bitp -= d.bitp % 8
		}
		// Reset availOut for next RSI
		d.availOut = d.rsiSize * d.bytesPerSample
	} else {
		d.ref = 0
		d.encodedBlockSize = d.blockSize
	}
	return d.mID()
}

// -------------------------------------------------------
// Split mode (Golomb-Rice coding)
// -------------------------------------------------------

func (d *decoder) mSplit() int {
	if d.bufferSpace() {
		k := d.id - 1

		if d.ref != 0 {
			if d.rsip >= len(d.rsiBuf) {
				return mError
			}
			d.rsiBuf[d.rsip] = d.directGet(d.bitsPerSample)
			d.rsip++
		}

		base := d.rsip
		if base+d.encodedBlockSize > len(d.rsiBuf) {
			return mError
		}
		for i := 0; i < d.encodedBlockSize; i++ {
			d.rsiBuf[base+i] = d.directGetFS() << k
		}

		if k != 0 {
			binaryPart := (k*d.encodedBlockSize)/8 + 9
			if d.availIn() < binaryPart {
				return mError
			}
			for i := 0; i < d.encodedBlockSize; i++ {
				d.rsiBuf[base+i] += d.directGet(k)
				d.rsip++
			}
		} else {
			d.rsip += d.encodedBlockSize
		}

		d.availOut -= d.outBlkLen
		return d.mNextCDS()
	}

	// Slow path
	if d.ref != 0 {
		if !d.copySample() {
			return mExit
		}
	}

	// Phase 1: read fundamental sequences
	d.sampleCounter = 0
	return d.mSplitFS()
}

func (d *decoder) mSplitFS() int {
	k := d.id - 1
	base := d.rsip

	for d.sampleCounter < d.encodedBlockSize {
		if !d.fsAsk() {
			return mExit
		}
		idx := base + d.sampleCounter
		if idx >= len(d.rsiBuf) {
			return mError
		}
		d.rsiBuf[idx] = d.fs << k
		d.fsDrop()
		d.sampleCounter++
	}

	d.sampleCounter = 0
	return d.mSplitOutput()
}

func (d *decoder) mSplitOutput() int {
	k := d.id - 1

	for d.sampleCounter < d.encodedBlockSize {
		if !d.bitsAsk(k) || d.availOut < d.bytesPerSample {
			return mExit
		}
		if k != 0 {
			d.rsiBuf[d.rsip] += d.bitsGet(k)
		}
		d.rsip++
		d.availOut -= d.bytesPerSample
		d.bitsDrop(k)
		d.sampleCounter++
	}

	return d.mNextCDS()
}

// -------------------------------------------------------
// Low entropy mode
// -------------------------------------------------------

func (d *decoder) mLowEntropy() int {
	if !d.bitsAsk(1) {
		return mExit
	}
	d.id = int(d.bitsGet(1))
	d.bitsDrop(1)
	return d.mLowEntropyRef()
}

func (d *decoder) mLowEntropyRef() int {
	if d.ref != 0 {
		if !d.copySample() {
			return mExit
		}
	}
	if d.id == 1 {
		return d.mSE()
	}
	return d.mZeroBlock()
}

// -------------------------------------------------------
// Zero block mode
// -------------------------------------------------------

func (d *decoder) mZeroBlock() int {
	if !d.fsAsk() {
		return mExit
	}

	zeroBlocks := int(d.fs) + 1
	d.fsDrop()

	if zeroBlocks == ros {
		b := d.rsiUsedSize() / d.blockSize
		rem := d.rsi - b
		align := 64 - (b % 64)
		if rem < align {
			zeroBlocks = rem
		} else {
			zeroBlocks = align
		}
	} else if zeroBlocks > ros {
		zeroBlocks--
	}

	zeroSamples := zeroBlocks*d.blockSize - d.ref
	if d.rsiSize-d.rsiUsedSize() < zeroSamples {
		return mError
	}

	zeroBytes := zeroSamples * d.bytesPerSample
	if d.availOut >= zeroBytes {
		if d.rsip+zeroSamples > len(d.rsiBuf) {
			return mError
		}
		for i := 0; i < zeroSamples; i++ {
			d.rsiBuf[d.rsip+i] = 0
		}
		d.rsip += zeroSamples
		d.availOut -= zeroBytes
		return d.mNextCDS()
	}

	// Slow path
	d.sampleCounter = zeroSamples
	return d.mZeroOutput()
}

func (d *decoder) mZeroOutput() int {
	for d.sampleCounter > 0 {
		if d.availOut < d.bytesPerSample {
			return mExit
		}
		d.putSample(0)
		d.sampleCounter--
	}
	return d.mNextCDS()
}

// -------------------------------------------------------
// Second Extension mode
// -------------------------------------------------------

func (d *decoder) mSE() int {
	if d.bufferSpace() {
		i := d.ref
		for i < d.blockSize {
			m := d.directGetFS()
			if m > seTableSize {
				return mError
			}
			d1 := int32(m) - int32(d.seTable[2*m+1])

			if i&1 == 0 {
				d.putSample(uint32(int32(d.seTable[2*m]) - d1))
				i++
			}
			d.putSample(uint32(d1))
			i++
		}
		return d.mNextCDS()
	}

	// Slow path
	d.sampleCounter = d.ref
	return d.mSEDecode()
}

func (d *decoder) mSEDecode() int {
	for d.sampleCounter < d.blockSize {
		if !d.fsAsk() {
			return mExit
		}
		m := d.fs
		if m > seTableSize {
			return mError
		}
		d1 := int32(m) - int32(d.seTable[2*m+1])

		if d.sampleCounter&1 == 0 {
			if d.availOut < d.bytesPerSample {
				return mExit
			}
			d.putSample(uint32(int32(d.seTable[2*m]) - d1))
			d.sampleCounter++
		}

		if d.availOut < d.bytesPerSample {
			return mExit
		}
		d.putSample(uint32(d1))
		d.sampleCounter++
		d.fsDrop()
	}

	return d.mNextCDS()
}

// -------------------------------------------------------
// Uncompressed mode
// -------------------------------------------------------

func (d *decoder) mUncomp() int {
	if d.bufferSpace() {
		if d.rsip+d.blockSize > len(d.rsiBuf) {
			return mError
		}
		for i := 0; i < d.blockSize; i++ {
			d.rsiBuf[d.rsip] = d.directGet(d.bitsPerSample)
			d.rsip++
		}
		d.availOut -= d.outBlkLen
		return d.mNextCDS()
	}

	// Slow path
	d.sampleCounter = d.blockSize
	return d.mUncompCopy()
}

func (d *decoder) mUncompCopy() int {
	for d.sampleCounter > 0 {
		if !d.copySample() {
			return mExit
		}
		d.sampleCounter--
	}
	return d.mNextCDS()
}

// -------------------------------------------------------
// SE table construction
// -------------------------------------------------------

func createSETable(table []int) {
	k := 0
	for i := 0; i < 13; i++ {
		ms := k
		for j := 0; j <= i; j++ {
			table[2*k] = i
			table[2*k+1] = ms
			k++
		}
	}
}
