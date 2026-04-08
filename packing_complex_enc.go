package grib2

import (
	"fmt"
	"math"
	"math/bits"
)

// ============================================================================
// Complex packing encoder (Template 5.2 / 5.3)
// ============================================================================

const missingIntSentinel = int(math.MaxInt32)

// findNbits returns the number of bits needed to represent the unsigned integer i.
func findNbits(i int) int {
	if i <= 0 {
		return 0
	}
	return bits.Len(uint(i))
}

// packSection represents a group of values during the group splitting process.
type packSection struct {
	mn, mx, missing int // stats: min, max, has-missing flag
	i0, i1          int // index range (inclusive) into v[]
	head, tail      *packSection
}

// sizeOfSection computes the number of bits needed to encode a section.
func sizeOfSection(s *packSection, refBits, widthBits, hasUndef int) int {
	if s.mn == missingIntSentinel { // all undefined
		return refBits + widthBits
	}
	if s.mn == s.mx {
		if s.missing == 0 { // constant and no missings
			return refBits + widthBits
		}
		return (s.i1-s.i0+1)*hasUndef + refBits + widthBits
	}
	return findNbits(s.mx-s.mn+hasUndef)*(s.i1-s.i0+1) + refBits + widthBits
}

// sizeOfSection2 computes the size for a hypothetical merged section.
func sizeOfSection2(mn, mx, n, refBits, widthBits, hasUndefSec, hasUndef int) int {
	if mn == missingIntSentinel {
		return refBits + widthBits
	}
	if mn == mx {
		if hasUndefSec == 0 {
			return refBits + widthBits
		}
		return n*hasUndef + refBits + widthBits
	}
	return findNbits(mx-mn+hasUndef)*n + refBits + widthBits
}

// sizeAll computes the total bytes needed for all sections in the linked list.
func sizeAll(s *packSection, refBits, widthBits, hasUndef int) int {
	totalBits := 0
	for s != nil {
		totalBits += sizeOfSection(s, refBits, widthBits, hasUndef)
		s = s.tail
	}
	return (totalBits + 7) / 8
}

// mergeJ performs the group merging optimization pass, following eccodes merge_j.
func mergeJ(h *packSection, refBits, widthBits, hasUndef, param, lenSecMax int) {
	sizeHead, sizeMid, sizeTail := 0, 0, 0

	for h != nil {
		m := h.tail
		if m == nil {
			break
		}
		t := m.tail

		// find savings of merged h - m
		savingHM := -1
		var min0, max0, min1, max1 int

		if m.i1-h.i0 < lenSecMax {
			if m.mn == missingIntSentinel {
				max0 = h.mx
				min0 = h.mn
			} else if h.mn == missingIntSentinel {
				max0 = m.mx
				min0 = m.mn
			} else {
				if h.mn < m.mn {
					min0 = h.mn
				} else {
					min0 = m.mn
				}
				if h.mx > m.mx {
					max0 = h.mx
				} else {
					max0 = m.mx
				}
			}
			if max0-min0 <= param {
				if sizeHead == 0 {
					sizeHead = sizeOfSection(h, refBits, widthBits, hasUndef)
				}
				if sizeMid == 0 {
					sizeMid = sizeOfSection(m, refBits, widthBits, hasUndef)
				}
				missingFlag := 0
				if h.missing != 0 || m.missing != 0 {
					missingFlag = 1
				}
				savingHM = sizeHead + sizeMid -
					sizeOfSection2(min0, max0, m.i1-h.i0+1, refBits, widthBits, missingFlag, hasUndef)
			}
		}

		// find savings of merged m - t
		savingMT := -1
		if t != nil && t.i1-m.i0 < lenSecMax {
			if m.mn == missingIntSentinel {
				max1 = t.mx
				min1 = t.mn
			} else if t.mn == missingIntSentinel {
				max1 = m.mx
				min1 = m.mn
			} else {
				if m.mn < t.mn {
					min1 = m.mn
				} else {
					min1 = t.mn
				}
				if m.mx > t.mx {
					max1 = m.mx
				} else {
					max1 = t.mx
				}
			}
			if max1-min1 <= param {
				if sizeMid == 0 {
					sizeMid = sizeOfSection(m, refBits, widthBits, hasUndef)
				}
				if sizeTail == 0 {
					sizeTail = sizeOfSection(t, refBits, widthBits, hasUndef)
				}
				missingFlag := 0
				if m.missing != 0 || t.missing != 0 {
					missingFlag = 1
				}
				savingMT = sizeMid + sizeTail -
					sizeOfSection2(min1, max1, t.i1-m.i0+1, refBits, widthBits, missingFlag, hasUndef)
			}
		}

		if savingHM >= savingMT && savingHM >= 0 {
			// merge h and m
			h.i1 = m.i1
			h.tail = m.tail
			h.mn = min0
			h.mx = max0
			if h.missing == 0 && m.missing != 0 {
				h.missing = 1
			}
			m = h.tail
			if m != nil {
				m.head = h
			}
			if h.head != nil {
				h = h.head
			}
			sizeHead, sizeMid, sizeTail = 0, 0, 0
		} else if savingMT >= savingHM && savingMT >= 0 {
			// merge m and t
			m.i1 = t.i1
			m.tail = t.tail
			m.mn = min1
			m.mx = max1
			if m.missing == 0 && t.missing != 0 {
				m.missing = 1
			}
			t = m.tail
			if t != nil {
				t.head = m
			}
			sizeHead, sizeMid, sizeTail = 0, 0, 0
		} else {
			h = h.tail
			sizeHead = sizeMid
			sizeMid = sizeTail
			sizeTail = 0
		}
	}
}

// moveOneLeft moves the first element of s.tail into s.
func moveOneLeft(s *packSection, v []int) {
	t := s.tail
	s.i1++
	t.i0++
	val := v[s.i1]

	// update s statistics
	if val == missingIntSentinel {
		s.missing = 1
	} else {
		if val > s.mx {
			s.mx = val
		}
		if val < s.mn {
			s.mn = val
		}
	}

	// remove t if empty
	if t.i0 > t.i1 {
		s.tail = t.tail
		if s.tail != nil {
			s.tail.head = s
		}
		return
	}

	// update t statistics after removing val
	if val == missingIntSentinel {
		for i := t.i0; i <= t.i1; i++ {
			if v[i] == missingIntSentinel {
				return
			}
		}
		t.missing = 0
		return
	}
	if val == t.mx {
		k := missingIntSentinel
		first := true
		for i := t.i0; i <= t.i1; i++ {
			if v[i] != missingIntSentinel {
				if first {
					k = v[i]
					first = false
				} else if v[i] > k {
					k = v[i]
				}
			}
		}
		t.mx = k
		return
	}
	if val == t.mn {
		k := missingIntSentinel
		first := true
		for i := t.i0; i <= t.i1; i++ {
			if v[i] != missingIntSentinel {
				if first {
					k = v[i]
					first = false
				} else if v[i] < k {
					k = v[i]
				}
			}
		}
		t.mn = k
	}
}

// moveOneRight moves the last element of s into s.tail.
func moveOneRight(s *packSection, v []int) {
	t := s.tail
	s.i1--
	t.i0--
	val := v[t.i0]

	// update t statistics
	if val == missingIntSentinel {
		t.missing = 1
	} else {
		if val > t.mx {
			t.mx = val
		}
		if val < t.mn {
			t.mn = val
		}
	}

	// if s is empty, copy t to s and recalculate
	if s.i0 > s.i1 {
		s.i0 = t.i0
		s.i1 = t.i1
		s.tail = t.tail

		s.mx = missingIntSentinel
		s.mn = missingIntSentinel
		s.missing = 0
		first := true
		for i := s.i0; i <= s.i1; i++ {
			if v[i] == missingIntSentinel {
				s.missing = 1
			} else if first {
				s.mx = v[i]
				s.mn = v[i]
				first = false
			} else {
				if v[i] > s.mx {
					s.mx = v[i]
				}
				if v[i] < s.mn {
					s.mn = v[i]
				}
			}
		}
		return
	}

	// update s statistics after removing val
	if val == missingIntSentinel {
		for i := s.i0; i <= s.i1; i++ {
			if v[i] == missingIntSentinel {
				return
			}
		}
		s.missing = 0
		return
	}
	if val == s.mx {
		k := missingIntSentinel
		first := true
		for i := s.i0; i <= s.i1; i++ {
			if v[i] != missingIntSentinel {
				if first {
					k = v[i]
					first = false
				} else if v[i] > k {
					k = v[i]
				}
			}
		}
		s.mx = k
		return
	}
	if val == s.mn {
		k := missingIntSentinel
		first := true
		for i := s.i0; i <= s.i1; i++ {
			if v[i] != missingIntSentinel {
				if first {
					k = v[i]
					first = false
				} else if v[i] < k {
					k = v[i]
				}
			}
		}
		s.mn = k
	}
}

// exchange adjusts group boundaries to reduce storage.
func exchange(s *packSection, v []int, hasUndef, lenSecMax int) {
	if s == nil {
		return
	}
	for {
		t := s.tail
		if t == nil {
			break
		}

		var nbitS, nbitT int
		if s.mn == missingIntSentinel {
			nbitS = 0
		} else if s.mn == s.mx {
			nbitS = s.missing
		} else {
			nbitS = findNbits(s.mx - s.mn + hasUndef)
		}

		if t.mn == missingIntSentinel {
			nbitT = 0
		} else if t.mn == t.mx {
			nbitT = t.missing
		} else {
			nbitT = findNbits(t.mx - t.mn + hasUndef)
		}

		if nbitS == nbitT {
			s = t
			continue
		}

		val0 := v[s.i1]
		val1 := v[t.i0]

		if s.missing == 1 || t.missing == 1 {
			s = t
			continue
		}

		if nbitS < nbitT && val1 == missingIntSentinel {
			if (s.i1-s.i0) < lenSecMax && s.mx != s.mn {
				moveOneLeft(s, v)
			} else {
				s = t
			}
			continue
		}

		if nbitS > nbitT && val0 == missingIntSentinel {
			if (t.i1-t.i0) < lenSecMax && t.mn != t.mx {
				moveOneRight(s, v)
			} else {
				s = t
			}
			continue
		}

		if nbitS < nbitT && (s.i1-s.i0) < lenSecMax && val1 >= s.mn && val1 <= s.mx {
			moveOneLeft(s, v)
		} else if nbitS > nbitT && (t.i1-t.i0) < lenSecMax && val0 >= t.mn && val0 <= t.mx {
			moveOneRight(s, v)
		} else {
			s = s.tail
		}
	}
}

// writeSignedMagnitude encodes a signed integer into sign-magnitude format
// and stores it using storeBits.
func writeSignedMagnitude(data []byte, bitOffset uint64, nOctets int, val int) {
	nbits := nOctets * 8
	if val < 0 {
		uval := uint64(-val) | (1 << (nbits - 1))
		storeBits(data, bitOffset, nbits, uval)
	} else {
		storeBits(data, bitOffset, nbits, uint64(val))
	}
}

// PackComplex encodes values using Template 5.2 (complex packing).
// Returns packed data bytes and the completed Template52.
func PackComplex(values []float64, bitsPerValue uint8) ([]byte, Template52, error) {
	data, tmpl52, _, err := packComplexInternal(values, bitsPerValue, 0)
	return data, tmpl52, err
}

// PackComplexSpatialDiff encodes values using Template 5.3
// (complex packing with spatial differencing).
// order must be 1 or 2.
// Returns packed data bytes and the completed Template53.
func PackComplexSpatialDiff(values []float64, bitsPerValue uint8, order uint8) ([]byte, Template53, error) {
	if order != 1 && order != 2 {
		return nil, Template53{}, fmt.Errorf("grib2: spatial differencing order must be 1 or 2, got %d", order)
	}
	data, tmpl52, nOctetsExtra, err := packComplexInternal(values, bitsPerValue, order)
	if err != nil {
		return nil, Template53{}, err
	}
	tmpl53 := Template53{
		Template52:                     tmpl52,
		OrderOfSpatialDifferencing:     order,
		NumberOfOctetsExtraDescriptors: nOctetsExtra,
	}
	return data, tmpl53, nil
}

// savedSection stores the state of a packSection for backup/restore.
type savedSection struct {
	mn, mx, missing int
	i0, i1          int
	headIdx, tailIdx int // -1 means nil
}

// saveList creates a snapshot of the linked list state.
func saveList(list []packSection) []savedSection {
	saved := make([]savedSection, len(list))
	for i := range list {
		saved[i] = savedSection{
			mn:      list[i].mn,
			mx:      list[i].mx,
			missing: list[i].missing,
			i0:      list[i].i0,
			i1:      list[i].i1,
			headIdx: -1,
			tailIdx: -1,
		}
		if list[i].head != nil {
			for j := range list {
				if &list[j] == list[i].head {
					saved[i].headIdx = j
					break
				}
			}
		}
		if list[i].tail != nil {
			for j := range list {
				if &list[j] == list[i].tail {
					saved[i].tailIdx = j
					break
				}
			}
		}
	}
	return saved
}

// restoreList restores the linked list state from a snapshot.
func restoreList(list []packSection, saved []savedSection) {
	for i := range saved {
		list[i].mn = saved[i].mn
		list[i].mx = saved[i].mx
		list[i].missing = saved[i].missing
		list[i].i0 = saved[i].i0
		list[i].i1 = saved[i].i1
		if saved[i].headIdx >= 0 {
			list[i].head = &list[saved[i].headIdx]
		} else {
			list[i].head = nil
		}
		if saved[i].tailIdx >= 0 {
			list[i].tail = &list[saved[i].tailIdx]
		} else {
			list[i].tail = nil
		}
	}
}

// packComplexInternal is the main implementation for both Template 5.2 and 5.3.
// Returns (packedData, template52, nOctetsExtra, error).
func packComplexInternal(values []float64, wantedBitsPerValue uint8, orderOfSpatialDiff uint8) ([]byte, Template52, uint8, error) {
	n := len(values)
	if n == 0 {
		return nil, Template52{}, 0, nil
	}

	// Clamp bits per value to 23 (eccodes ECC-1968 limit)
	wantedBits := int(wantedBitsPerValue)
	if wantedBits > 23 {
		wantedBits = 23
	}

	// Find min and max of defined values
	hasUndef := 0
	mn, mx := math.Inf(1), math.Inf(-1)
	ndef := 0
	for _, val := range values {
		if !math.IsNaN(val) {
			if val < mn {
				mn = val
			}
			if val > mx {
				mx = val
			}
			ndef++
		}
	}

	if ndef == 0 {
		// All NaN / missing: return a minimal constant-zero field
		tmpl := Template52{
			Template50: Template50{
				ReferenceValue:            0,
				BinaryScaleFactor:         0,
				DecimalScaleFactor:        0,
				BitsPerValue:              0,
				TypeOfOriginalFieldValues: 0,
			},
			GroupSplittingMethod:              1,
			MissingValueManagement:            0,
			NumberOfGroups:                    0,
			ReferenceForGroupWidths:           0,
			NumberOfBitsForGroupWidths:        0,
			ReferenceForGroupLengths:          0,
			LengthIncrementForGroupLengths:    1,
			TrueLengthOfLastGroup:             0,
			NumberOfBitsForScaledGroupLengths: 0,
		}
		return nil, tmpl, 0, nil
	}

	// NaN values become missingIntSentinel in the integer array
	nndata := n
	for _, val := range values {
		if math.IsNaN(val) {
			hasUndef = 1
			break
		}
	}

	// Compute scaling: ECMWF style
	ref := mn
	frange := mx - ref
	var binaryScale int

	if frange != 0.0 {
		_, exp := math.Frexp(frange)
		binaryScale = exp - wantedBits
		scl := math.Ldexp(1.0, -binaryScale)
		frange2 := math.Floor((mx-ref)*scl + 0.5)
		_, exp2 := math.Frexp(frange2)
		if exp2 != wantedBits {
			binaryScale++
		}
	}

	// Convert to integer domain
	v := make([]int, nndata)
	scl := math.Ldexp(1.0, -binaryScale)
	for i, val := range values {
		if math.IsNaN(val) {
			v[i] = missingIntSentinel
		} else {
			vi := int(math.Floor((val-ref)*scl + 0.5))
			if vi < 0 {
				vi = 0
			}
			v[i] = vi
		}
	}

	// Spatial differencing
	var extra0, extra1 int
	vmn, vmx := 0, 0

	packingMode := 0
	if orderOfSpatialDiff == 0 {
		packingMode = 1
	} else if orderOfSpatialDiff == 1 {
		packingMode = 2
	} else if orderOfSpatialDiff == 2 {
		packingMode = 3
	}

	if packingMode == 3 {
		// Second order differencing
		var last, last0, penultimate int
		i := 0
		for ; i < nndata; i++ {
			if v[i] != missingIntSentinel {
				extra0 = v[i]
				penultimate = v[i]
				v[i] = 0
				i++
				break
			}
		}
		for ; i < nndata; i++ {
			if v[i] != missingIntSentinel {
				extra1 = v[i]
				last = v[i]
				v[i] = 0
				i++
				break
			}
		}
		for ; i < nndata; i++ {
			if v[i] != missingIntSentinel {
				last0 = v[i]
				v[i] = v[i] - 2*last + penultimate
				penultimate = last
				last = last0
				if v[i] < vmn {
					vmn = v[i]
				}
				if v[i] > vmx {
					vmx = v[i]
				}
			}
		}
	} else if packingMode == 2 {
		// First order differencing
		var last, last0 int
		i := 0
		for ; i < nndata; i++ {
			if v[i] != missingIntSentinel {
				extra0 = v[i]
				last = v[i]
				v[i] = 0
				i++
				break
			}
		}
		for ; i < nndata; i++ {
			if v[i] != missingIntSentinel {
				last0 = v[i]
				v[i] = v[i] - last
				last = last0
				if v[i] < vmn {
					vmn = v[i]
				}
				if v[i] > vmx {
					vmx = v[i]
				}
			}
		}
	} else {
		// No spatial differencing: find min/max
		first := true
		for i := 0; i < nndata; i++ {
			if v[i] != missingIntSentinel {
				if first {
					vmn = v[i]
					vmx = v[i]
					first = false
				} else {
					if v[i] < vmn {
						vmn = v[i]
					}
					if v[i] > vmx {
						vmx = v[i]
					}
				}
			}
		}
	}

	// Subtract global minimum (bias) so all values are non-negative
	for i := 0; i < nndata; i++ {
		if v[i] != missingIntSentinel {
			v[i] = v[i] - vmn
		}
	}
	vmx = vmx - vmn
	vbits := findNbits(vmx + hasUndef)

	// Build initial sections (run-length of equal values)
	lenSecMax := 127
	lenBits := 7
	estGroupWidth := 6

	// Count how many initial sections we need
	nstruct := 1
	ii := 0
	for i := 1; i < nndata; i++ {
		if (i-ii+1) > lenSecMax || v[i] != v[ii] {
			nstruct++
			ii = i
		}
	}

	// Initialize linked list of sections
	list := make([]packSection, nstruct)
	ii = 0
	list[0].mn = v[0]
	list[0].mx = v[0]
	if v[0] == missingIntSentinel {
		list[0].missing = 1
	}
	list[0].i0 = 0
	list[0].i1 = 0

	for i := 1; i < nndata; i++ {
		if (i-list[ii].i0) < lenSecMax && v[i] == list[ii].mn {
			list[ii].i1 = i
		} else {
			ii++
			list[ii].mn = v[i]
			list[ii].mx = v[i]
			if v[i] == missingIntSentinel {
				list[ii].missing = 1
			}
			list[ii].i0 = i
			list[ii].i1 = i
		}
	}

	actualNstruct := ii + 1
	if nstruct != actualNstruct {
		return nil, Template52{}, 0, fmt.Errorf("grib2: packComplex: nstruct mismatch: %d vs %d", nstruct, actualNstruct)
	}

	// Set up linked list pointers
	list[0].head = nil
	list[actualNstruct-1].tail = nil
	for i := 1; i < actualNstruct; i++ {
		list[i].head = &list[i-1]
		list[i-1].tail = &list[i]
	}

	// Create a sentinel start node
	start := &packSection{tail: &list[0]}

	// Merging passes: sequence 1, 3, 7, ... (or 2, 6, 14, ... if has_undef)
	k := 1
	if hasUndef != 0 {
		k = 2
	}
	for k < vmx/2 {
		mergeJ(start.tail, vbits, lenBits+estGroupWidth, hasUndef, k, lenSecMax)
		k = 2*k + 1 + hasUndef
	}

	// Try making segment sizes larger.
	// Use index-based backup/restore to correctly preserve linked list pointers.
	j := sizeAll(start.tail, vbits, lenBits+estGroupWidth, hasUndef)
	j0 := j + 1
	for j < j0 && lenBits < 25 {
		j0 = j
		lenBits++
		lenSecMax = lenSecMax + lenSecMax + 1
		listBackup := saveList(list)
		mergeJ(start.tail, vbits, lenBits+estGroupWidth, hasUndef, k, lenSecMax)
		j = sizeAll(start.tail, vbits, lenBits+estGroupWidth, hasUndef)
		if j > j0 {
			restoreList(list, listBackup)
			lenBits--
			lenSecMax = (lenSecMax - 1) / 2
		}
	}

	exchange(start.tail, v, hasUndef, lenSecMax)
	mergeJ(start.tail, vbits, lenBits+estGroupWidth, hasUndef, vmx, lenSecMax)

	// FIX: Ensure the last section ends exactly at nndata-1.
	// After merging/exchange the last group's i1 can drift beyond the
	// actual data range. Clamp it so the total group lengths equal nndata.
	if start.tail != nil {
		last := start.tail
		for last.tail != nil {
			last = last.tail
		}
		if last.i1 != nndata-1 {
			last.i1 = nndata - 1
		}
	}

	// Compute number of bytes for extra info (spatial differencing)
	var nOctetsExtra int
	if packingMode != 1 {
		k2 := 0
		if vmn >= 0 {
			k2 = findNbits(vmn) + 1
		} else {
			k2 = findNbits(-vmn) + 1
		}
		j2 := findNbits(extra0) + 1
		if j2 > k2 {
			k2 = j2
		}
		if packingMode == 3 {
			j2 = findNbits(extra1) + 1
			if j2 > k2 {
				k2 = j2
			}
		}
		nOctetsExtra = (k2 + 7) / 8
		if nOctetsExtra == 0 {
			nOctetsExtra = 1
		}
	}

	// Count groups and extract arrays
	ngroups := 0
	for s := start.tail; s != nil; s = s.tail {
		ngroups++
	}

	if ngroups == 0 {
		return nil, Template52{}, 0, fmt.Errorf("grib2: packComplex: no groups generated")
	}

	lens := make([]int, ngroups)
	widths := make([]int, ngroups)
	refs := make([]int, ngroups)
	groupMx := make([]int, ngroups)
	groupMissing := make([]int, ngroups)
	groupStarts := make([]int, ngroups)

	{
		total := 0
		idx := 0
		for s := start.tail; s != nil; s, idx = s.tail, idx+1 {
			lens[idx] = s.i1 - s.i0 + 1
			total += lens[idx]
			refs[idx] = s.mn
			groupMx[idx] = s.mx
			groupMissing[idx] = s.missing
			groupStarts[idx] = s.i0
		}
		if total != nndata {
			return nil, Template52{}, 0, fmt.Errorf("grib2: packComplex: group lengths sum %d != ndata %d", total, nndata)
		}
	}

	// Compute group widths (bits per value within each group)
	for i := 0; i < ngroups; i++ {
		if refs[i] == missingIntSentinel {
			widths[i] = 0
		} else if refs[i] == groupMx[i] {
			widths[i] = groupMissing[i]
		} else {
			widths[i] = findNbits(groupMx[i] - refs[i] + hasUndef)
		}
	}

	// Compute template parameters
	lenLast := lens[ngroups-1]

	glenmn, glenmx := lens[0], lens[0]
	gwidmn, gwidmx := widths[0], widths[0]
	grefmx := 0
	if refs[0] != missingIntSentinel {
		grefmx = refs[0]
	}

	for i := 1; i < ngroups; i++ {
		if lens[i] > glenmx {
			glenmx = lens[i]
		}
		if lens[i] < glenmn {
			glenmn = lens[i]
		}
		if widths[i] > gwidmx {
			gwidmx = widths[i]
		}
		if widths[i] < gwidmn {
			gwidmn = widths[i]
		}
		if refs[i] != missingIntSentinel && refs[i] > grefmx {
			grefmx = refs[i]
		}
	}

	bitsPerValueActual := findNbits(grefmx + hasUndef)
	nBitsGroupWidths := findNbits(gwidmx - gwidmn + hasUndef)
	nBitsScaledGroupLens := findNbits(glenmx - glenmn)

	// Prepare arrays for bitstream writing:
	// refs: replace missingIntSentinel with all-ones
	// scaledWidths: widths[i] - gwidmn
	// scaledLens: lens[i] - glenmn
	ones := int(^uint32(0))
	scaledWidths := make([]int, ngroups)
	scaledLens := make([]int, ngroups)
	groupRefs := make([]int, ngroups) // save original refs for per-value packing
	for i := 0; i < ngroups; i++ {
		groupRefs[i] = refs[i]
		if refs[i] == missingIntSentinel {
			refs[i] = ones
		}
		scaledWidths[i] = widths[i] - gwidmn
		scaledLens[i] = lens[i] - glenmn
	}

	// Compute data size
	dataSize := 0

	// Extra octets for spatial differencing
	if packingMode == 2 {
		dataSize += 2 * nOctetsExtra
	} else if packingMode == 3 {
		dataSize += 3 * nOctetsExtra
	}

	// Group reference values
	dataSize += (ngroups*bitsPerValueActual + 7) / 8
	// Group widths
	dataSize += (ngroups*nBitsGroupWidths + 7) / 8
	// Group lengths
	dataSize += (ngroups*nBitsScaledGroupLens + 7) / 8
	// Per-group packed values
	totalValBits := 0
	for i := 0; i < ngroups; i++ {
		totalValBits += lens[i] * widths[i]
	}
	dataSize += (totalValBits + 7) / 8

	// Allocate and pack
	data := make([]byte, dataSize)
	bitPos := uint64(0)

	// Write extra octets (spatial differencing)
	if packingMode == 2 || packingMode == 3 {
		storeBits(data, bitPos, nOctetsExtra*8, uint64(extra0))
		bitPos += uint64(nOctetsExtra * 8)
		if packingMode == 3 {
			storeBits(data, bitPos, nOctetsExtra*8, uint64(extra1))
			bitPos += uint64(nOctetsExtra * 8)
		}
		writeSignedMagnitude(data, bitPos, nOctetsExtra, vmn)
		bitPos += uint64(nOctetsExtra * 8)
		// Align to byte boundary
		if bitPos%8 != 0 {
			bitPos = ((bitPos + 7) / 8) * 8
		}
	}

	// Write group reference values
	for i := 0; i < ngroups; i++ {
		if bitsPerValueActual > 0 {
			storeBits(data, bitPos, bitsPerValueActual, uint64(uint32(refs[i])))
			bitPos += uint64(bitsPerValueActual)
		}
	}
	if bitPos%8 != 0 {
		bitPos = ((bitPos + 7) / 8) * 8
	}

	// Write group widths (scaled)
	for i := 0; i < ngroups; i++ {
		if nBitsGroupWidths > 0 {
			storeBits(data, bitPos, nBitsGroupWidths, uint64(scaledWidths[i]))
			bitPos += uint64(nBitsGroupWidths)
		}
	}
	if bitPos%8 != 0 {
		bitPos = ((bitPos + 7) / 8) * 8
	}

	// Write group lengths (scaled)
	for i := 0; i < ngroups; i++ {
		if nBitsScaledGroupLens > 0 {
			storeBits(data, bitPos, nBitsScaledGroupLens, uint64(scaledLens[i]))
			bitPos += uint64(nBitsScaledGroupLens)
		}
	}
	if bitPos%8 != 0 {
		bitPos = ((bitPos + 7) / 8) * 8
	}

	// Write per-group values (value - group_ref)
	for i := 0; i < ngroups; i++ {
		if widths[i] > 0 {
			for j := 0; j < lens[i]; j++ {
				vi := v[groupStarts[i]+j]
				var packed uint64
				if vi == missingIntSentinel {
					packed = (1 << widths[i]) - 1
				} else {
					packed = uint64(vi - groupRefs[i])
				}
				storeBits(data, bitPos, widths[i], packed)
				bitPos += uint64(widths[i])
			}
		}
	}

	// Truncate data to actual used bytes
	usedBytes := int((bitPos + 7) / 8)
	if usedBytes < len(data) {
		data = data[:usedBytes]
	}

	tmpl := Template52{
		Template50: Template50{
			ReferenceValue:            float32(ref),
			BinaryScaleFactor:         int16(binaryScale),
			DecimalScaleFactor:        0,
			BitsPerValue:              uint8(bitsPerValueActual),
			TypeOfOriginalFieldValues: 0,
		},
		GroupSplittingMethod:              1,
		MissingValueManagement:            uint8(hasUndef),
		NumberOfGroups:                    uint32(ngroups),
		ReferenceForGroupWidths:           uint8(gwidmn),
		NumberOfBitsForGroupWidths:        uint8(nBitsGroupWidths),
		ReferenceForGroupLengths:          uint32(glenmn),
		LengthIncrementForGroupLengths:    1,
		TrueLengthOfLastGroup:             uint32(lenLast),
		NumberOfBitsForScaledGroupLengths: uint8(nBitsScaledGroupLens),
	}

	return data, tmpl, uint8(nOctetsExtra), nil
}
