package vban

import "encoding/binary"

// Endianness for VBAN protocol (Little Endian)
var byteOrder = binary.LittleEndian

// VBAN Header Magic Number ('VBAN' in Little Endian)
const HeaderMagic uint32 = 0x4E414256 // 'N' 'A' 'B' 'V'

// Max lengths and sizes defined in the specification or common implementations.
const (
	MaxStreamNameLen  = 16   // Spec p.6
	HeaderSize        = 28   // Spec p.5
	MaxPacketDataSize = 1436 // Spec p.7 (Voicemeeter limit: 1500 MTU - IP/UDP Headers - Reserves = 1436)
	MaxVBANPacketSize = HeaderSize + MaxPacketDataSize
	DefaultPort       = 6980 // Common default port for VBAN
)

// --- Sub Protocol (Spec p.8) ---

// SubProtocol identifies the type of data carried in the VBAN packet.
type SubProtocol uint8

const (
	ProtocolAudio   SubProtocol = 0x00 // VBAN Audio Protocol
	ProtocolSerial  SubProtocol = 0x20 // VBAN Serial Protocol (incl. MIDI)
	ProtocolText    SubProtocol = 0x40 // VBAN Text Protocol
	ProtocolService SubProtocol = 0x60 // VBAN Service Protocol (Ping, RT-Packets)
	// ... other undefined/user protocols (0x80, 0xA0, 0xC0, 0xE0)
	ProtocolMask SubProtocol = 0xE0 // Mask to extract sub protocol bits (5-7)
)

// IsAudio checks if the sub-protocol is Audio.
func (sp SubProtocol) IsAudio() bool { return (sp & ProtocolMask) == ProtocolAudio }

// IsSerial checks if the sub-protocol is Serial.
func (sp SubProtocol) IsSerial() bool { return (sp & ProtocolMask) == ProtocolSerial }

// IsText checks if the sub-protocol is Text.
func (sp SubProtocol) IsText() bool { return (sp & ProtocolMask) == ProtocolText }

// IsService checks if the sub-protocol is Service.
func (sp SubProtocol) IsService() bool { return (sp & ProtocolMask) == ProtocolService }

// --- Sample Rate Index (Audio) / BPS Index (Serial/Text) (Spec p.8, p.14, p.19) ---

// SRIndex represents the Sample Rate index (Audio) or BPS index (Serial/Text).
// The lower 5 bits (0-4) hold the index value.
type SRIndex uint8

const (
	SRMask SRIndex = 0x1F // Mask to extract SR/BPS index bits (0-4)
)

// SRList provides mapping from SRIndex to Sample Rate (Hz) for Audio.
var SRList = map[SRIndex]uint32{
	0: 6000, 1: 12000, 2: 24000, 3: 48000, 4: 96000, 5: 192000, 6: 384000,
	7: 8000, 8: 16000, 9: 32000, 10: 64000, 11: 128000, 12: 256000, 13: 512000,
	14: 11025, 15: 22050, 16: 44100, 17: 88200, 18: 176400, 19: 352800, 20: 705600,
	// 21-31 are Undefined in Spec Rev 11
}

// BPSList provides mapping from SRIndex to Bits Per Second for Serial/Text.
var BPSList = map[SRIndex]uint32{
	0: 0, 1: 110, 2: 150, 3: 300, 4: 600, 5: 1200, 6: 2400, 7: 4800, 8: 9600, 9: 14400,
	10: 19200, 11: 31250, 12: 38400, 13: 57600, 14: 115200, 15: 128000, 16: 230400,
	17: 250000, 18: 256000, 19: 460800, 20: 921600, 21: 1000000, 22: 1500000,
	23: 2000000, 24: 3000000,
	// 25-31 are Undefined in Spec Rev 11
}

// GetRate returns the Sample Rate (for Audio) or BPS (for Serial/Text)
// corresponding to the SRIndex, based on the given SubProtocol.
// Returns 0 if the index or protocol is not defined for rate lookup.
func (sri SRIndex) GetRate(sp SubProtocol) uint32 {
	index := sri & SRMask
	if sp.IsAudio() {
		rate, ok := SRList[index]
		if ok {
			return rate
		}
	} else if sp.IsSerial() || sp.IsText() {
		rate, ok := BPSList[index]
		if ok {
			return rate
		}
	}
	return 0 // Undefined index or protocol for rate lookup
}

// --- Data Type (Spec p.9) ---

// DataType defines the format of individual data samples or elements.
// The lower 3 bits (0-2) hold the type index. Bit 3 is reserved.
type DataType uint8

const (
	DataTypeUINT8   DataType = 0x00 // 8-bit unsigned integer (0 to 255)
	DataTypeINT16   DataType = 0x01 // 16-bit signed integer (-32768 to 32767)
	DataTypeINT24   DataType = 0x02 // 24-bit signed integer (Requires special handling)
	DataTypeINT32   DataType = 0x03 // 32-bit signed integer
	DataTypeFLOAT32 DataType = 0x04 // 32-bit float (-1.0 to +1.0 recommended range)
	DataTypeFLOAT64 DataType = 0x05 // 64-bit float (-1.0 to +1.0 recommended range)
	DataType12BIT   DataType = 0x06 // 12-bit integer (-2048 to +2047) (Requires special handling)
	DataType10BIT   DataType = 0x07 // 10-bit integer (-512 to +511) (Requires special handling)
	DataTypeMask    DataType = 0x07 // Mask to extract data type bits (0-2)
	// Bit 3 (0x08) is reserved, should be 0 for standard types.
)

// Size returns the size in bytes for the DataType.
// Returns 0 for types requiring special handling (INT24, 12BIT, 10BIT)
// as their byte representation isn't straightforward.
func (dt DataType) Size() int {
	switch dt & DataTypeMask {
	case DataTypeUINT8:
		return 1
	case DataTypeINT16:
		return 2
	case DataTypeINT32, DataTypeFLOAT32:
		return 4
	case DataTypeFLOAT64:
		return 8
	case DataTypeINT24, DataType12BIT, DataType10BIT:
		return 0 // Special handling required
	default:
		return 0
	}
}

// --- Codec (Audio) / Serial Type / Text Format (Spec p.10, p.16, p.20) ---

// CodecType defines the encoding format applied to the data.
// The upper 4 bits (4-7) hold the type index.
type CodecType uint8

const (
	// Audio Codec Types (Spec p.10)
	CodecPCM  CodecType = 0x00 // Native PCM (No Codec)
	CodecVBCA CodecType = 0x10 // VB-Audio AOIP Codec (Not Free)
	CodecVBCV CodecType = 0x20 // VB-Audio VOIP Codec (Not Free)
	// ... other undefined/user codecs

	// Serial Stream Types (Spec p.16)
	SerialGeneric CodecType = 0x00 // Generic byte stream
	SerialMIDI    CodecType = 0x10 // MIDI messages
	// ... other undefined/user serial types

	// Text Stream Formats (Spec p.20)
	TextASCII CodecType = 0x00 // Null-terminated ASCII string(s)
	TextUTF8  CodecType = 0x10 // UTF-8 encoded string(s)
	TextWCHAR CodecType = 0x20 // Wide Character (UTF-16/UCS-2?) string(s) (Requires special handling)
	// ... other undefined/user text formats

	CodecMask CodecType = 0xF0 // Mask to extract codec/type/format bits (4-7)
)

// GetType returns the specific codec/type/format masked value.
// The interpretation depends on the SubProtocol context.
func (ct CodecType) GetType(sp SubProtocol) CodecType {
	// Basic implementation returns the masked value.
	// Higher-level logic can provide more specific types based on sp.
	return ct & CodecMask
}

// --- Service Types and Functions (Spec p.23, p.27) ---
// Defined here for completeness, used by Service protocol.

// ServiceType identifies the VBAN service being used.
type ServiceType uint8

// ServiceFunction identifies the specific function within a service.
// Bit 7 is often used as a reply flag.
type ServiceFunction uint8

const (
	ServiceIdentification   ServiceType = 0  // Service: Device Identification (PINGO)
	ServiceChatUTF8         ServiceType = 1  // Service: Simple Chat (UTF8)
	ServiceRTPacketRegister ServiceType = 32 // Service: Register for RT Packets
	ServiceRTPacket         ServiceType = 33 // Service: Real-Time Packet Data

	ServiceFuncPing  ServiceFunction = 0    // Function (for Identification): Send Ping Request
	ServiceFuncReply ServiceFunction = 0x80 // Mask/Flag: Indicates a reply message
	// Function (for RTPacketRegister): Timeout in seconds (0-255) in format_bit field
	// Function (for RTPacket): RT Packet ID (0-127)
)
