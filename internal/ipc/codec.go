package ipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// EncodeMessage encodes a message into wire format
func EncodeMessage(msg *Message) ([]byte, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	if len(payload) > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", len(payload), MaxMessageSize)
	}

	frame := make([]byte, HeaderSize+len(payload))
	binary.BigEndian.PutUint32(frame[:HeaderSize], uint32(len(payload)))
	copy(frame[HeaderSize:], payload)

	return frame, nil
}

// DecodeMessage decodes a message from wire format
func DecodeMessage(data []byte) (*Message, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("data too short: need at least %d bytes, got %d", HeaderSize, len(data))
	}

	length := binary.BigEndian.Uint32(data[:HeaderSize])
	if int(length) > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, MaxMessageSize)
	}

	if len(data) < HeaderSize+int(length) {
		return nil, fmt.Errorf("incomplete message: expected %d bytes, got %d", HeaderSize+int(length), len(data))
	}

	var msg Message
	if err := json.Unmarshal(data[HeaderSize:HeaderSize+int(length)], &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// Encoder writes messages to an io.Writer
type Encoder struct {
	w io.Writer
}

// NewEncoder creates a new Encoder
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes a message to the underlying writer
func (e *Encoder) Encode(msg *Message) error {
	frame, err := EncodeMessage(msg)
	if err != nil {
		return err
	}

	_, err = e.w.Write(frame)
	return err
}

// Decoder reads messages from an io.Reader
type Decoder struct {
	r      io.Reader
	header [HeaderSize]byte
}

// NewDecoder creates a new Decoder
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

// Decode reads and decodes the next message from the underlying reader
func (d *Decoder) Decode() (*Message, error) {
	if _, err := io.ReadFull(d.r, d.header[:]); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(d.header[:])
	if length > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, MaxMessageSize)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(d.r, payload); err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// FrameReader is a buffered reader that handles partial frame reads
type FrameReader struct {
	buf *bytes.Buffer
}

// NewFrameReader creates a new FrameReader
func NewFrameReader() *FrameReader {
	return &FrameReader{
		buf: new(bytes.Buffer),
	}
}

// Write adds data to the internal buffer
func (fr *FrameReader) Write(data []byte) {
	fr.buf.Write(data)
}

// ReadMessage attempts to read a complete message from the buffer
func (fr *FrameReader) ReadMessage() (*Message, error) {
	if fr.buf.Len() < HeaderSize {
		return nil, nil
	}

	headerBytes := fr.buf.Bytes()[:HeaderSize]
	length := binary.BigEndian.Uint32(headerBytes)

	if length > MaxMessageSize {
		fr.buf.Reset()
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, MaxMessageSize)
	}

	totalLen := HeaderSize + int(length)
	if fr.buf.Len() < totalLen {
		return nil, nil
	}

	frame := make([]byte, totalLen)
	_, _ = fr.buf.Read(frame)

	var msg Message
	if err := json.Unmarshal(frame[HeaderSize:], &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// Pending returns the number of bytes waiting in the buffer
func (fr *FrameReader) Pending() int {
	return fr.buf.Len()
}

// Reset clears the internal buffer
func (fr *FrameReader) Reset() {
	fr.buf.Reset()
}
