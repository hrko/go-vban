package vban

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// Header represents the 28-byte VBAN header structure.
// Fields correspond to the specification (Spec Rev 11).
type Header struct {
	VBAN       uint32                 // 4 bytes: Magic number 'VBAN' (Little Endian: 0x4E414256)
	FormatSR   uint8                  // 1 byte: Lower 5 bits SR/BPS index, Upper 3 bits Sub-protocol
	FormatNbs  uint8                  // 1 byte: SamplesPerFrame-1 (Audio), BitMode (Serial), Unused (Text), Service Function (Service)
	FormatNbc  uint8                  // 1 byte: Channels-1 (Audio), ChannelIdent (Serial/Text), Service Type (Service)
	FormatBit  uint8                  // 1 byte: Lower 3 bits DataType, Bit 3 Reserved, Upper 4 bits Codec/Type/Format
	StreamName [MaxStreamNameLen]byte // 16 bytes: Null-terminated ASCII Stream Name
	NuFrame    uint32                 // 4 bytes: Frame counter (Audio/Serial/Text), Request ID (Service)
}

// NewHeader creates a new Header with basic information.
// It initializes the magic number, sub-protocol, and stream name.
// Other fields default to zero and should be set explicitly based on the protocol.
func NewHeader(subProto SubProtocol, streamName string) Header {
	h := Header{
		VBAN:     HeaderMagic,
		FormatSR: uint8(subProto), // Sets sub-protocol, SR/BPS index is 0 initially
	}
	h.SetStreamName(streamName)
	return h
}

// --- General Field Accessors / Interpreters ---

// SubProtocol returns the SubProtocol identifier from the FormatSR field.
func (h *Header) SubProtocol() SubProtocol {
	return SubProtocol(h.FormatSR & uint8(ProtocolMask))
}

// SRIndex returns the SRIndex (Sample Rate or BPS index) from the FormatSR field.
func (h *Header) SRIndex() SRIndex {
	return SRIndex(h.FormatSR & uint8(SRMask))
}

// DataType returns the DataType identifier from the FormatBit field.
func (h *Header) DataType() DataType {
	return DataType(h.FormatBit & uint8(DataTypeMask))
}

// CodecType returns the CodecType identifier from the FormatBit field.
// Note: The interpretation of this value depends on the SubProtocol.
func (h *Header) CodecType() CodecType {
	return CodecType(h.FormatBit & uint8(CodecMask))
}

// GetStreamName returns the stream name as a Go string.
// It reads up to the first null terminator or the maximum length.
func (h *Header) GetStreamName() string {
	n := bytes.IndexByte(h.StreamName[:], 0)
	if n == -1 {
		n = MaxStreamNameLen // Not null-terminated
	}
	return string(h.StreamName[:n])
}

// SetStreamName sets the stream name in the header.
// It copies the name and ensures null-termination if space permits.
// Excess bytes in the header field are zeroed.
func (h *Header) SetStreamName(name string) {
	l := copy(h.StreamName[:], name)
	// Zero out remaining bytes including potential null terminator
	for i := l; i < MaxStreamNameLen; i++ {
		h.StreamName[i] = 0
	}
}

// --- Protocol Specific Helpers (Examples for Audio) ---
// These provide a more convenient API but require knowledge of the protocol context.

// SamplesPerFrame returns the number of samples (valid for Audio protocol).
func (h *Header) SamplesPerFrame() uint8 {
	// According to Spec p.9, value is nbSample - 1
	return h.FormatNbs + 1
}

// SetSamplesPerFrame sets the number of samples (valid for Audio protocol).
// nbSamples must be between 1 and 256.
func (h *Header) SetSamplesPerFrame(nbSamples uint8) error {
	if nbSamples == 0 { // Samples range from 1 to 256
		return errors.New("number of samples must be between 1 and 256")
	}
	// Value stored is nbSample - 1
	h.FormatNbs = nbSamples - 1
	return nil
}

// Channels returns the number of channels (valid for Audio protocol).
func (h *Header) Channels() uint8 {
	// According to Spec p.9, value is nbChannel - 1
	return h.FormatNbc + 1
}

// SetChannels sets the number of channels (valid for Audio protocol).
// nbChannels must be between 1 and 256.
func (h *Header) SetChannels(nbChannels uint8) error {
	if nbChannels == 0 { // Channels range from 1 to 256
		return errors.New("number of channels must be between 1 and 256")
	}
	// Value stored is nbChannel - 1
	h.FormatNbc = nbChannels - 1
	return nil
}

// SetAudioFormat configures FormatSR and FormatBit fields specifically for the Audio protocol.
// It combines the sub-protocol, sample rate index, data type, and codec.
// Ensures the reserved bit in FormatBit is cleared.
func (h *Header) SetAudioFormat(srIndex SRIndex, dataType DataType, codec CodecType) {
	// Combine SubProtocol (Audio) and SRIndex
	h.FormatSR = (uint8(ProtocolAudio) & uint8(ProtocolMask)) | (uint8(srIndex) & uint8(SRMask))
	// Combine DataType and Codec, ensuring reserved bit (3) is 0
	h.FormatBit = (uint8(codec) & uint8(CodecMask)) | (uint8(dataType) & uint8(DataTypeMask))
	h.FormatBit &^= 0x08 // Clear reserved bit 3 (mask 0x08)
}

// --- Marshaling / Unmarshaling ---

// MarshalBinary converts the Header struct into its 28-byte representation (Little Endian).
func (h *Header) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.Grow(HeaderSize)

	// Write fields in the exact order defined by the VBAN specification.
	if err := binary.Write(buf, byteOrder, h.VBAN); err != nil {
		return nil, fmt.Errorf("failed to write VBAN magic: %w", err)
	}
	if err := binary.Write(buf, byteOrder, h.FormatSR); err != nil {
		return nil, fmt.Errorf("failed to write FormatSR: %w", err)
	}
	if err := binary.Write(buf, byteOrder, h.FormatNbs); err != nil {
		return nil, fmt.Errorf("failed to write FormatNbs: %w", err)
	}
	if err := binary.Write(buf, byteOrder, h.FormatNbc); err != nil {
		return nil, fmt.Errorf("failed to write FormatNbc: %w", err)
	}
	if err := binary.Write(buf, byteOrder, h.FormatBit); err != nil {
		return nil, fmt.Errorf("failed to write FormatBit: %w", err)
	}
	if _, err := buf.Write(h.StreamName[:]); err != nil { // Write StreamName bytes directly
		return nil, fmt.Errorf("failed to write StreamName: %w", err)
	}
	if err := binary.Write(buf, byteOrder, h.NuFrame); err != nil {
		return nil, fmt.Errorf("failed to write NuFrame: %w", err)
	}

	// Final check of buffer length
	if buf.Len() != HeaderSize {
		return nil, fmt.Errorf("internal error: marshaled header size is %d, expected %d", buf.Len(), HeaderSize)
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary parses a 28-byte slice (Little Endian) into the Header struct.
// It performs basic validation of the VBAN magic number.
func (h *Header) UnmarshalBinary(data []byte) error {
	if len(data) < HeaderSize {
		return fmt.Errorf("insufficient data for header: expected %d bytes, got %d", HeaderSize, len(data))
	}
	// Use a reader for structured reading
	buf := bytes.NewReader(data[:HeaderSize]) // Ensure reading only header size

	// Read fields in the exact order defined by the VBAN specification.
	if err := binary.Read(buf, byteOrder, &h.VBAN); err != nil {
		return fmt.Errorf("failed to read VBAN magic: %w", err)
	}
	// Validate Magic Number immediately
	if h.VBAN != HeaderMagic {
		return fmt.Errorf("invalid VBAN magic number: expected %X, got %X", HeaderMagic, h.VBAN)
	}

	if err := binary.Read(buf, byteOrder, &h.FormatSR); err != nil {
		return fmt.Errorf("failed to read FormatSR: %w", err)
	}
	if err := binary.Read(buf, byteOrder, &h.FormatNbs); err != nil {
		return fmt.Errorf("failed to read FormatNbs: %w", err)
	}
	if err := binary.Read(buf, byteOrder, &h.FormatNbc); err != nil {
		return fmt.Errorf("failed to read FormatNbc: %w", err)
	}
	if err := binary.Read(buf, byteOrder, &h.FormatBit); err != nil {
		return fmt.Errorf("failed to read FormatBit: %w", err)
	}
	if _, err := buf.Read(h.StreamName[:]); err != nil { // Read directly into StreamName byte array
		return fmt.Errorf("failed to read StreamName: %w", err)
	}
	if err := binary.Read(buf, byteOrder, &h.NuFrame); err != nil {
		return fmt.Errorf("failed to read NuFrame: %w", err)
	}

	return nil
}
