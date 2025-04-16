package vban

import (
	"errors"
	"fmt"
	"net"
)

// Packet represents a complete VBAN packet, including its header and data payload.
type Packet struct {
	Header Header // The 28-byte VBAN header.
	Data   []byte // The raw data payload following the header.
}

// NewPacket creates a new VBAN Packet struct from a header and data payload.
// It returns an error if the data payload size exceeds the maximum allowed limit.
func NewPacket(header Header, data []byte) (*Packet, error) {
	if len(data) > MaxPacketDataSize {
		return nil, fmt.Errorf("data size (%d bytes) exceeds VBAN maximum (%d bytes)", len(data), MaxPacketDataSize)
	}
	// Note: The data slice is assigned directly. If the caller modifies the slice
	// after calling NewPacket, the Packet's Data field will reflect the change.
	// Consider making a copy if independent data is required:
	// dataCopy := make([]byte, len(data))
	// copy(dataCopy, data)
	// p.Data = dataCopy
	p := &Packet{
		Header: header,
		Data:   data,
	}
	return p, nil
}

// MarshalBinary converts the entire VBAN packet (Header + Data) into a single byte slice
// suitable for sending over UDP.
func (p *Packet) MarshalBinary() ([]byte, error) {
	headerBytes, err := p.Header.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal packet header: %w", err)
	}
	// Re-validate data size before concatenating
	if len(p.Data) > MaxPacketDataSize {
		return nil, fmt.Errorf("data size (%d bytes) exceeds VBAN maximum (%d bytes)", len(p.Data), MaxPacketDataSize)
	}

	// Allocate buffer for header + data and concatenate
	fullPacket := make([]byte, 0, len(headerBytes)+len(p.Data))
	fullPacket = append(fullPacket, headerBytes...)
	fullPacket = append(fullPacket, p.Data...)
	return fullPacket, nil
}

// UnmarshalBinary parses a byte slice representing a full VBAN packet into a Packet struct.
// It assumes the input `data` contains one complete VBAN packet.
// It copies the data payload to prevent issues with buffer reuse.
func UnmarshalBinary(data []byte) (*Packet, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("insufficient data for VBAN packet: got %d bytes, need at least %d", len(data), HeaderSize)
	}
	// Packet size cannot exceed the maximum defined size
	if len(data) > MaxVBANPacketSize {
		// This check might be redundant if the read buffer is already limited,
		// but provides an explicit validation against the protocol limit.
		return nil, fmt.Errorf("packet size (%d bytes) exceeds VBAN maximum (%d bytes)", len(data), MaxVBANPacketSize)
	}

	p := &Packet{}
	// First, unmarshal the header part
	err := p.Header.UnmarshalBinary(data[:HeaderSize]) // Pass only the header bytes
	if err != nil {
		// Error during header parsing (e.g., bad magic number)
		return nil, fmt.Errorf("failed to unmarshal VBAN header: %w", err)
	}

	// The rest of the data is the payload.
	// Create a copy of the data payload to ensure the Packet owns its data.
	dataPayload := data[HeaderSize:]
	p.Data = make([]byte, len(dataPayload))
	copy(p.Data, dataPayload)

	// v0.0.1: No validation of Data length against header fields yet.

	return p, nil
}

// --- Connection Handling ---

// Conn provides methods for sending and receiving VBAN packets over UDP.
type Conn struct {
	udpConn *net.UDPConn
	// readBuffer is allocated once per connection to minimize allocations during receive operations.
	readBuffer []byte
}

// Listen creates a VBAN Conn that listens for incoming UDP packets
// on the specified local address and port.
// If localAddr is nil, it listens on all available interfaces using the DefaultPort.
func Listen(localAddr *net.UDPAddr) (*Conn, error) {
	addr := localAddr
	// Default address if nil
	if addr == nil {
		addr = &net.UDPAddr{IP: net.IPv4zero, Port: DefaultPort}
	}
	// Default port if 0
	if addr.Port == 0 {
		addr.Port = DefaultPort
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP %s: %w", addr.String(), err)
	}
	return &Conn{
		udpConn: conn,
		// Allocate buffer slightly larger than max packet size to detect overflow.
		readBuffer: make([]byte, MaxVBANPacketSize+1),
	}, nil
}

// Dial creates a VBAN Conn configured to send packets to a specific remote UDP address.
// It can also receive packets (typically replies) from any source on the bound local port.
// If localAddr is nil, the OS chooses the source IP and port.
func Dial(localAddr, remoteAddr *net.UDPAddr) (*Conn, error) {
	if remoteAddr == nil {
		return nil, errors.New("remote address cannot be nil for Dial")
	}
	conn, err := net.DialUDP("udp", localAddr, remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial UDP from %v to %s: %w", localAddr, remoteAddr.String(), err)
	}
	return &Conn{
		udpConn:    conn,
		readBuffer: make([]byte, MaxVBANPacketSize+1),
	}, nil
}

// NewConn wraps an existing *net.UDPConn into a vban.Conn.
// Useful if the UDP connection is managed externally.
func NewConn(udpConn *net.UDPConn) *Conn {
	if udpConn == nil {
		return nil // Or panic, depending on desired behavior
	}
	return &Conn{
		udpConn:    udpConn,
		readBuffer: make([]byte, MaxVBANPacketSize+1),
	}
}

// Close closes the underlying UDP connection.
// It's safe to call Close multiple times.
func (c *Conn) Close() error {
	if c.udpConn == nil {
		return nil // Already closed or not initialized
	}
	err := c.udpConn.Close()
	c.udpConn = nil // Mark as closed
	return err
}

// Send marshals the given VBAN Packet and sends it over the UDP connection.
// If the Conn was created using Dial, `addr` can be nil to send to the dialed address.
// Otherwise, `addr` must specify the destination UDP address.
func (c *Conn) Send(packet *Packet, addr *net.UDPAddr) error {
	if c.udpConn == nil {
		return errors.New("connection is closed")
	}
	if packet == nil {
		return errors.New("cannot send a nil packet")
	}

	// Marshal the packet into bytes
	packetBytes, err := packet.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal packet for sending: %w", err)
	}

	var n int
	if addr != nil {
		// Send to a specific address using WriteToUDP
		n, err = c.udpConn.WriteToUDP(packetBytes, addr)
	} else {
		// If addr is nil, assume sending to the dialed address (or fail if not dialed)
		if c.udpConn.RemoteAddr() == nil {
			// Not a dialed connection, and no destination address provided
			return errors.New("destination address (addr) must be provided for non-dialed connections")
		}
		// Use Write() for dialed connections
		n, err = c.udpConn.Write(packetBytes)
	}

	// Check for UDP write errors
	if err != nil {
		// Consider specific error handling, e.g., for network issues
		return fmt.Errorf("UDP write error: %w", err)
	}
	// Check if the entire packet was written
	if n != len(packetBytes) {
		return fmt.Errorf("incomplete UDP write: wrote %d bytes, expected %d", n, len(packetBytes))
	}
	return nil
}

// Receive blocks until a UDP packet is received, attempts to parse it as a VBAN packet,
// and returns the parsed Packet, the sender's address, and any error encountered.
func (c *Conn) Receive() (*Packet, *net.UDPAddr, error) {
	if c.udpConn == nil {
		return nil, nil, errors.New("connection is closed")
	}

	// Read data from the UDP connection into the reusable buffer
	// ReadFromUDP waits for a packet.
	n, remoteAddr, err := c.udpConn.ReadFromUDP(c.readBuffer)

	// Handle read errors
	if err != nil {
		// Check if the error is due to the connection being closed.
		if errors.Is(err, net.ErrClosed) {
			return nil, nil, fmt.Errorf("connection closed: %w", err) // Return specific error?
		}
		// Other potential errors (network issues, etc.)
		return nil, nil, fmt.Errorf("UDP read error: %w", err)
	}

	// Basic validation of received data length
	if n == 0 {
		// Theoretically possible to receive empty UDP datagrams, though unlikely for VBAN
		return nil, remoteAddr, errors.New("received empty UDP packet")
	}
	if n > MaxVBANPacketSize {
		// Packet larger than our buffer + overflow byte could handle, or larger than protocol max.
		// This indicates an issue, possibly fragmentation or non-VBAN traffic.
		return nil, remoteAddr, fmt.Errorf("received oversized packet: %d bytes (max allowed %d)", n, MaxVBANPacketSize)
	}

	// Attempt to unmarshal the received bytes into a VBAN Packet struct
	// Pass only the slice containing the actual received data ([:n]).
	packet, err := UnmarshalBinary(c.readBuffer[:n])
	if err != nil {
		// Data was received, but it wasn't a valid VBAN packet (e.g., bad magic number)
		return nil, remoteAddr, fmt.Errorf("failed to unmarshal received data as VBAN packet: %w", err)
	}

	// Successfully received and parsed a VBAN packet
	return packet, remoteAddr, nil
}

// LocalAddr returns the local network address of the underlying UDP connection.
func (c *Conn) LocalAddr() net.Addr {
	if c.udpConn == nil {
		return nil
	}
	return c.udpConn.LocalAddr()
}

// RemoteAddr returns the remote network address (only meaningful if Conn was created using Dial).
func (c *Conn) RemoteAddr() net.Addr {
	if c.udpConn == nil {
		return nil
	}
	return c.udpConn.RemoteAddr()
}
