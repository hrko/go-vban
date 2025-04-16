package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/hrko/go-vban/vban"
)

const (
	defaultStreamName = "WavStream"
	defaultDestAddr   = "127.0.0.1:6980"
	samplesPerPacket  = 128 // Number of audio samples per VBAN packet (adjust as needed)
)

// findSRIndex finds the VBAN Sample Rate Index for a given sample rate.
func findSRIndex(rate uint32) (vban.SRIndex, error) {
	for index, sr := range vban.SRList {
		if sr == rate {
			// Ensure index is within the valid 5-bit range (0-31)
			// Although map keys are already SRIndex type, double check mask logic.
			return index & vban.SRMask, nil
		}
	}
	return 0, fmt.Errorf("unsupported sample rate for VBAN: %d Hz", rate)
}

// intBufferToBytes converts audio.IntBuffer data (assumed int16) to little-endian bytes.
func intBufferToBytes(buf *audio.IntBuffer, bitDepth int) ([]byte, error) {
	if bitDepth != 16 {
		return nil, fmt.Errorf("unsupported bit depth for conversion: %d", bitDepth)
	}
	if buf == nil || buf.Data == nil {
		return nil, errors.New("input buffer is nil or has no data")
	}

	byteBuf := new(bytes.Buffer)
	// Expected size: numSamples * numBytesPerSample
	byteBuf.Grow(len(buf.Data) * (bitDepth / 8))

	for _, sample := range buf.Data {
		// Convert int sample to int16 before writing
		sample16 := int16(sample)
		err := binary.Write(byteBuf, binary.LittleEndian, sample16) // Use vban's byteOrder (LittleEndian)
		if err != nil {
			return nil, fmt.Errorf("failed to write sample to byte buffer: %w", err)
		}
	}
	return byteBuf.Bytes(), nil
}

func main() {
	// --- Argument Parsing ---
	wavFilePath := flag.String("wavfile", "", "Path to the WAV file (required)")
	streamName := flag.String("stream", defaultStreamName, "VBAN stream name")
	destAddrStr := flag.String("dest", defaultDestAddr, "Destination address (e.g., 127.0.0.1:6980)")
	flag.Parse()

	if *wavFilePath == "" {
		fmt.Println("Error: --wavfile flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// --- WAV File Reading ---
	wavFile, err := os.Open(*wavFilePath)
	if err != nil {
		log.Fatalf("Failed to open WAV file '%s': %v", *wavFilePath, err)
	}
	defer wavFile.Close()

	// Create a WAV decoder
	decoder := wav.NewDecoder(wavFile)
	if !decoder.IsValidFile() {
		log.Fatalf("'%s' is not a valid WAV file", *wavFilePath)
	}

	// Check audio format - MUST be 16-bit PCM for this example
	format := decoder.Format()
	if format == nil {
		log.Fatalf("Failed to get audio format from WAV file")
	}
	bitDepth := decoder.BitDepth // Use decoder's BitDepth method
	if decoder.BitDepth != 16 {
		log.Fatalf("Unsupported WAV format: only 16-bit PCM is supported (file has %d bits)", bitDepth)
	}
	// Check WavAudioFormat field as well (PCM = 1)
	if decoder.WavAudioFormat != 1 {
		log.Fatalf("Unsupported WAV audio format code: %d (expected 1 for PCM)", decoder.WavAudioFormat)
	}

	sampleRate := format.SampleRate
	numChannels := format.NumChannels
	log.Printf("WAV Info: %s, Sample Rate: %d Hz, Channels: %d, Bit Depth: %d",
		*wavFilePath, sampleRate, numChannels, bitDepth)

	// Find the corresponding VBAN sample rate index
	vbanSRIndex, err := findSRIndex(uint32(sampleRate))
	if err != nil {
		log.Fatalf("Failed to map WAV sample rate to VBAN: %v", err)
	}
	log.Printf("VBAN Info: Using SR Index %d for %d Hz", vbanSRIndex, sampleRate)

	// --- VBAN Connection Setup ---
	destAddr, err := net.ResolveUDPAddr("udp", *destAddrStr)
	if err != nil {
		log.Fatalf("Failed to resolve destination address '%s': %v", *destAddrStr, err)
	}

	// Use Dial for sending primarily
	conn, err := vban.Dial(nil, destAddr) // Use nil for local address (OS chooses)
	if err != nil {
		log.Fatalf("Failed to dial VBAN destination '%s': %v", destAddr, err)
	}
	defer conn.Close()
	log.Printf("Sending VBAN stream '%s' to %s from %s", *streamName, conn.RemoteAddr(), conn.LocalAddr())

	// --- VBAN Header Preparation ---
	vbanHeader := vban.NewHeader(vban.ProtocolAudio, *streamName)
	vbanHeader.SetAudioFormat(vbanSRIndex, vban.DataTypeINT16, vban.CodecPCM)
	err = vbanHeader.SetChannels(uint8(numChannels))
	if err != nil {
		log.Fatalf("Failed to set VBAN channels: %v", err) // Should not happen if numChannels > 0
	}
	// SamplesPerFrame will be set in the loop

	// --- Transmission Loop ---
	// Create a buffer to read PCM data from the WAV file
	pcmBuf := &audio.IntBuffer{
		Format:         format,
		Data:           make([]int, samplesPerPacket*numChannels), // Buffer size
		SourceBitDepth: int(bitDepth),
	}

	var frameCounter uint32 = 0
	startTime := time.Now()
	totalSamplesSent := 0

	for {
		// Read a chunk of audio data from the WAV decoder
		samplesRead, err := decoder.PCMBuffer(pcmBuf) // Fills pcmBuf.Data

		if err == io.EOF {
			log.Println("Reached end of WAV file.")
			break // End of file
		}
		if err != nil {
			log.Fatalf("Error reading PCM data from WAV file: %v", err)
		}

		if samplesRead == 0 {
			fmt.Printf("\033[2K") // Clear line
			log.Println("Read 0 samples, stopping.")
			break // No more samples
		}

		// Note: samplesRead is the total number of integer samples read (samples * channels)
		// We need the number of samples *per channel* for the VBAN header.
		samplesReadPerChannel := samplesRead / numChannels
		if samplesReadPerChannel == 0 {
			log.Printf("Warning: Read %d total samples, which is less than num channels (%d). Skipping.", samplesRead, numChannels)
			continue
		}

		// Convert the read integer samples (pcmBuf.Data[:samplesRead]) to byte data for VBAN
		// Important: Slice the Data field to only include the samples actually read!
		dataToSend, err := intBufferToBytes(&audio.IntBuffer{Data: pcmBuf.Data[:samplesRead], Format: format}, int(bitDepth))
		if err != nil {
			log.Fatalf("Failed to convert PCM buffer to bytes: %v", err)
		}

		// Update VBAN header for this packet
		vbanHeader.NuFrame = frameCounter
		err = vbanHeader.SetSamplesPerFrame(uint8(samplesReadPerChannel)) // Use samples *per channel*
		if err != nil {
			log.Fatalf("Failed to set samples per frame (%d): %v", samplesReadPerChannel, err)
		}

		// Create the VBAN packet
		packet, err := vban.NewPacket(vbanHeader, dataToSend)
		if err != nil {
			log.Fatalf("Failed to create VBAN packet: %v", err)
		}

		// Send the packet
		err = conn.Send(packet, nil) // nil address because we used Dial
		if err != nil {
			log.Printf("Warning: Failed to send VBAN packet %d: %v", frameCounter, err)
			// Decide whether to continue or stop on send errors
		}

		// --- Rate Control ---
		totalSamplesSent += samplesReadPerChannel
		frameCounter++

		// Calculate the expected time elapsed since start
		expectedElapsedTime := time.Duration(float64(totalSamplesSent) / float64(sampleRate) * float64(time.Second))
		// Calculate the actual time elapsed
		actualElapsedTime := time.Since(startTime)

		// Calculate sleep duration to match the expected rate
		sleepDuration := max(expectedElapsedTime-actualElapsedTime, 0)
		time.Sleep(sleepDuration)

		// Print progress
		if frameCounter%100 == 0 { // Print every 100 packets
			fmt.Printf("Sent packet %d (Total Samples: %d, Elapsed: %v)\r", frameCounter, totalSamplesSent, actualElapsedTime.Round(time.Millisecond))
		}
	}

	log.Println("Finished sending.")
}
