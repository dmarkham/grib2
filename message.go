package grib2

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// Message represents a complete GRIB2 message.
// A single GRIB2 message can contain multiple data fields (repeated sections 3-7).
type Message struct {
	Section0 Section0
	Section1 Section1
	Section2 *Section2 // optional, may be nil
	Fields   []Field   // one or more data fields
}

// Field is one data field within a GRIB2 message.
// Each field consists of sections 3 through 7.
//
// After the first field, a message may repeat from Section 2, 3 or 4. A
// field that starts at Section 4 reuses the previous field's grid: its
// Section3 is a copy of that grid and SharesSection3 is true, so WriteMessage
// does not write it again. A field preceded by its own Section 2 carries it in
// Section2 (the first field's Section 2 is Message.Section2).
type Field struct {
	Section2       *Section2 // repeated local use section before this field (fields after the first only)
	SharesSection3 bool      // field started at Section 4 and reuses the previous Section 3
	Section3 Section3
	Section4 Section4
	Section5 Section5
	Section6 Section6
	Section7 Section7
}

// Decoder reads GRIB2 messages from a stream.
// Use NewDecoder to create one, then call Decode repeatedly.
//
// Like eccodes' readers, the Decoder skips anything between messages that
// does not start with "GRIB" (padding, headers, junk). A GRIB edition 1
// message is skipped whole and reported with an error wrapping
// ErrNotEdition2, after which Decode can be called again.
type Decoder struct {
	br *bufio.Reader
}

// NewDecoder returns a Decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &Decoder{br: br}
}

// countingReader counts the bytes read through it.
type countingReader struct {
	r io.Reader
	n uint64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += uint64(n)
	return n, err
}

// syncToMagic discards bytes up to the next "GRIB". It returns io.EOF if the
// stream ends first.
func (d *Decoder) syncToMagic() error {
	for {
		b, err := d.br.Peek(4)
		if string(b) == gribMagic {
			return nil
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.EOF // trailing bytes that are not a message
			}
			return err
		}
		if _, err := d.br.Discard(1); err != nil {
			return err
		}
	}
}

// skipEdition1 discards a GRIB edition 1 message (3-octet length at octets 5-7).
func (d *Decoder) skipEdition1() error {
	hdr, err := d.br.Peek(8)
	if err != nil {
		return ErrTruncated
	}
	n := int(hdr[4])<<16 | int(hdr[5])<<8 | int(hdr[6])
	if n < 8 {
		n = 8
	}
	if _, err := d.br.Discard(n); err != nil {
		return ErrTruncated
	}
	return fmt.Errorf("%w: skipped a GRIB edition %d message of %d bytes", ErrNotEdition2, hdr[7], n)
}

// Decode reads the next GRIB2 message.
// Returns io.EOF (unwrapped) when no more messages are available. A
// message cut short by the end of the stream gives an error wrapping
// ErrTruncated, never io.EOF, so a "read until io.EOF" loop cannot mistake
// a partial message for the end of the file.
func (d *Decoder) Decode() (*Message, error) {
	if err := d.syncToMagic(); err != nil {
		return nil, err
	}
	if hdr, err := d.br.Peek(8); err == nil && hdr[7] != grib2Edition {
		return nil, d.skipEdition1()
	}
	cr := &countingReader{r: d.br}
	msg, err := d.decode(cr)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: %v", ErrTruncated, err)
		}
		return nil, err
	}
	if cr.n != msg.Section0.TotalLength {
		return nil, fmt.Errorf("grib2: message is %d bytes but section 0 declares %d", cr.n, msg.Section0.TotalLength)
	}
	return msg, nil
}

func (d *Decoder) decode(r io.Reader) (*Message, error) {
	s0, err := ReadSection0(r)
	if err != nil {
		return nil, err
	}

	s1, err := ReadSection1(r)
	if err != nil {
		return nil, err
	}

	msg := &Message{
		Section0: s0,
		Section1: s1,
	}

	secNum, err := peekSectionNumber(d.br)
	if err != nil {
		return nil, fmt.Errorf("grib2: peeking after section 1: %w", err)
	}

	if secNum == 2 {
		s2, err := ReadSection2(r)
		if err != nil {
			return nil, err
		}
		msg.Section2 = &s2
	}

	// Read repeated field groups (sections 3-7). After the first field the
	// message may repeat from Section 2, 3 or 4 (WMO GRIB2 regulation 92.1.3).
	var lastSection3 *Section3
	var lastBitmap []byte
	for {
		secNum, err = peekSectionNumber(d.br)
		if err != nil {
			return nil, fmt.Errorf("grib2: peeking for next field group: %w", err)
		}
		if secNum == 8 {
			break
		}

		var s2 *Section2
		if secNum == 2 && len(msg.Fields) > 0 {
			sec, err := ReadSection2(r)
			if err != nil {
				return nil, err
			}
			s2 = &sec
			if secNum, err = peekSectionNumber(d.br); err != nil {
				return nil, fmt.Errorf("grib2: peeking after repeated section 2: %w", err)
			}
			if secNum != 3 {
				return nil, fmt.Errorf("grib2: repeated section 2 must be followed by section 3, got section %d", secNum)
			}
		}

		switch secNum {
		case 3:
		case 4:
			if lastSection3 == nil {
				return nil, fmt.Errorf("grib2: section 4 before any section 3")
			}
		default:
			if secNum > 8 {
				// Not a section at all: most likely a damaged "7777".
				return nil, fmt.Errorf("%w: found section number %d", ErrInvalidEndSection, secNum)
			}
			return nil, fmt.Errorf("grib2: unexpected section %d in field group", secNum)
		}

		field, err := readField(r, secNum)
		if err != nil {
			return nil, err
		}
		field.Section2 = s2

		if secNum == 3 {
			s3 := field.Section3
			lastSection3 = &s3
		} else {
			field.Section3 = *lastSection3
			field.SharesSection3 = true
		}

		// Bit-map indicator 254 reuses the last bitmap defined in this message.
		switch field.Section6.Indicator {
		case 0:
			lastBitmap = field.Section6.Bitmap
		case 254:
			field.Section6.PreviousBitmap = lastBitmap
		}

		msg.Fields = append(msg.Fields, field)
	}

	if len(msg.Fields) == 0 {
		return nil, fmt.Errorf("grib2: message has no data fields")
	}

	if err := ReadSection8(r); err != nil {
		return nil, err
	}

	return msg, nil
}

// ReadMessage reads a single GRIB2 message from r.
// For reading multiple messages, use a Decoder instead.
func ReadMessage(r io.Reader) (*Message, error) {
	return NewDecoder(r).Decode()
}

func readField(r io.Reader, startSection uint8) (Field, error) {
	var f Field
	var err error

	if startSection == 3 {
		f.Section3, err = ReadSection3(r)
		if err != nil {
			return Field{}, err
		}
	}

	f.Section4, err = ReadSection4(r)
	if err != nil {
		return Field{}, err
	}

	f.Section5, err = ReadSection5(r)
	if err != nil {
		return Field{}, err
	}

	f.Section6, err = ReadSection6(r)
	if err != nil {
		return Field{}, err
	}

	f.Section7, err = ReadSection7(r)
	if err != nil {
		return Field{}, err
	}

	return f, nil
}

// peekSectionNumber peeks ahead to determine the next section number
// without consuming data.
func peekSectionNumber(br *bufio.Reader) (uint8, error) {
	peek, err := br.Peek(4)
	if err != nil {
		return 0, fmt.Errorf("grib2: peeking section number: %w", err)
	}

	if bytes.Equal(peek[:4], []byte(section8Magic)) {
		return 8, nil
	}

	peek, err = br.Peek(5)
	if err != nil {
		if len(peek) == 4 {
			// Four bytes left that are not "7777": a corrupt end section.
			return 0, fmt.Errorf("%w: got %q", ErrInvalidEndSection, string(peek[:4]))
		}
		return 0, fmt.Errorf("grib2: peeking section number: %w", err)
	}

	return peek[4], nil
}

// WriteMessage writes a complete GRIB2 message to w.
func WriteMessage(w io.Writer, msg *Message) error {
	var buf bytes.Buffer

	if err := WriteSection0(&buf, msg.Section0); err != nil {
		return err
	}
	if err := WriteSection1(&buf, msg.Section1); err != nil {
		return err
	}
	if msg.Section2 != nil {
		if err := WriteSection2(&buf, *msg.Section2); err != nil {
			return err
		}
	}
	for i, f := range msg.Fields {
		if i > 0 && f.Section2 != nil {
			if err := WriteSection2(&buf, *f.Section2); err != nil {
				return err
			}
		}
		if i == 0 || !f.SharesSection3 {
			if err := WriteSection3(&buf, f.Section3); err != nil {
				return err
			}
		}
		if err := WriteSection4(&buf, f.Section4); err != nil {
			return err
		}
		if err := WriteSection5(&buf, f.Section5); err != nil {
			return err
		}
		if err := WriteSection6(&buf, f.Section6); err != nil {
			return err
		}
		if err := WriteSection7(&buf, f.Section7); err != nil {
			return err
		}
	}
	if err := WriteSection8(&buf); err != nil {
		return err
	}

	// Patch bytes 8-15 of Section 0 with the actual total length (uint64 big-endian).
	data := buf.Bytes()
	totalLength := uint64(len(data))
	data[8] = byte(totalLength >> 56)
	data[9] = byte(totalLength >> 48)
	data[10] = byte(totalLength >> 40)
	data[11] = byte(totalLength >> 32)
	data[12] = byte(totalLength >> 24)
	data[13] = byte(totalLength >> 16)
	data[14] = byte(totalLength >> 8)
	data[15] = byte(totalLength)

	_, err := buf.WriteTo(w)
	return err
}
