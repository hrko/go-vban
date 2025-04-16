# go-vban

[![Go Reference](https://pkg.go.dev/badge/github.com/hrko/go-vban/vban.svg)](https://pkg.go.dev/github.com/hrko/go-vban/vban)

## Overview

`go-vban` provides a native Go implementation of the **VBAN protocol** (VB-Audio Network Protocol), designed for real-time transport of digital audio streams and other data over IP-based network environments using UDP.

This package allows Go applications to easily send and receive VBAN packets. It is implemented purely in Go, with no external library dependencies (no Cgo required).

**Current Version (v0.0.1):** This initial version focuses on the primitive aspects of the protocol, providing the core functionality to:
* Encode and decode the VBAN packet header (28 bytes).
* Send and receive raw VBAN packets (header + data payload) over UDP sockets.

## Status

**Early Development:** This package is in its early stages (v0.0.1). The API might change in future versions as more features are added. Detailed documentation and examples will be expanded upon in upcoming releases.

## Installation

```bash
go get github.com/hrko/go-vban/vban
```

## Roadmap

This outlines the planned features and improvements for the `go-vban` package:

* [x] **v0.0.1: Basic Packet Handling**
    * [x] Header Encoding/Decoding (Little Endian)
    * [x] Basic UDP Packet Sending/Receiving (`Conn` type)
    * [x] Constant definitions (SubProtocols, Data Types, etc.)
* [ ] **Sub-Protocol Implementations**
    * [ ] **AUDIO:** Helpers for common PCM formats (e.g., decoding `Packet.Data` into audio buffers).
    * [ ] **SERIAL:** Support for generic serial data and MIDI streams.
    * [ ] **TEXT:** Support for ASCII, UTF-8 text streams.
    * [ ] **SERVICE:** Implementation for PINGO (Discovery) and RT-Packet services.
* [ ] **Extended Data Types:** Support for encoding/decoding less common formats (INT24, FLOAT32, FLOAT64, 12/10BIT if feasible).
* [ ] **Stream Abstractions**
    * [ ] Implement `io.Reader` wrapper for specific incoming VBAN streams (e.g., reading audio data from a stream).
    * [ ] Implement `io.Writer` wrapper for specific outgoing VBAN streams (e.g., writing audio data to a stream).
* [ ] **Documentation & Examples:** Provide comprehensive usage examples and API documentation.
* [ ] **Testing:** Increase unit test coverage for all features.
* [ ] **API Refinement:** Improve the API based on usage and feedback.
