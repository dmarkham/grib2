package grib2

import (
	"bufio"
	"bytes"
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
type Field struct {
	Section3 Section3
	Section4 Section4
	Section5 Section5
	Section6 Section6
	Section7 Section7
}

// Decoder reads GRIB2 messages from a stream.
// Use NewDecoder to create one, then call Decode repeatedly.
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

// Decode reads the next GRIB2 message.
// Returns io.EOF when no more messages are available.
func (d *Decoder) Decode() (*Message, error) {
	s0, err := ReadSection0(d.br)
	if err != nil {
		return nil, err
	}

	s1, err := ReadSection1(d.br)
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
		s2, err := ReadSection2(d.br)
		if err != nil {
			return nil, err
		}
		msg.Section2 = &s2
	}

	// Read repeated field groups (sections 3-7)
	var lastSection3 Section3
	for {
		secNum, err = peekSectionNumber(d.br)
		if err != nil {
			return nil, fmt.Errorf("grib2: peeking for next field group: %w", err)
		}

		if secNum != 3 && secNum != 4 {
			break
		}

		field, err := readField(d.br, secNum)
		if err != nil {
			return nil, err
		}

		if secNum == 3 {
			lastSection3 = field.Section3
		} else {
			// Section 4 start: inherit the previous field's Section 3
			field.Section3 = lastSection3
		}

		msg.Fields = append(msg.Fields, field)
	}

	if err := ReadSection8(d.br); err != nil {
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
	for _, f := range msg.Fields {
		if err := WriteSection3(&buf, f.Section3); err != nil {
			return err
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
