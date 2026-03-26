package lgdeck

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

const (
	tvPort    = 9761
	tvTimeout = 8 * time.Second

	// readIdleTimeout: how long we wait for more bytes after the first chunk.
	// LG encrypted responses are always exactly 32 bytes in one TCP segment on
	// LAN, so the first Read drains the full response and the next Read hits
	// the deadline. Larger = more reliable on bad networks, smaller = faster.
	readIdleTimeout     = 500 * time.Millisecond
	readIdleTimeoutFast = 80 * time.Millisecond // safe on LAN for KEY_ACTION
)

// TVClient manages a single TCP connection to the LG TV IP control port.
type TVClient struct {
	host         string
	enc          *Encryption   // nil = unencrypted mode
	keyIdleTimeout time.Duration // 0 = use readIdleTimeout default
}

// NewTVClient creates a client. If keycode is empty, plain (unencrypted) mode is used.
// The keycode is normalised to uppercase to match the LG TV's key derivation
// (the original TS library uses format /[A-Z0-9]{8}/).
func NewTVClient(host, keycode string) (*TVClient, error) {
	c := &TVClient{host: host}
	if keycode != "" {
		// Always normalise to uppercase — LG derives the shared secret from the
		// uppercase form regardless of what the Settings UI displays.
		enc, err := NewEncryptionUpper(keycode)
		if err != nil {
			return nil, fmt.Errorf("creating encryption: %w", err)
		}
		c.enc = enc
	}
	return c, nil
}

// SendCommand opens a TCP connection, sends the command, reads the full response,
// and closes. One command per connection (mirrors the TS library behaviour).
func (c *TVClient) SendCommand(command string) (string, error) {
	idle := readIdleTimeout
	if c.keyIdleTimeout > 0 {
		idle = c.keyIdleTimeout
	}
	return c.sendWithIdle(command, idle)
}

// sendWithIdle is the internal implementation; idle controls how long we wait
// for more bytes after the first chunk arrives (smaller = faster on LAN).
func (c *TVClient) sendWithIdle(command string, idle time.Duration) (string, error) {
	addr := fmt.Sprintf("%s:%d", c.host, tvPort)
	conn, err := net.DialTimeout("tcp", addr, tvTimeout)
	if err != nil {
		return "", fmt.Errorf("connect to TV: %w", err)
	}
	defer conn.Close()

	// Encode
	var payload []byte
	if c.enc != nil {
		payload, err = c.enc.Encode(command)
		if err != nil {
			return "", fmt.Errorf("encode command: %w", err)
		}
	} else {
		e := &Encoder{}
		payload = e.Encode(command)
	}

	log.Printf("[lgdeck-wire] cmd=%q  tx_bytes=%d  tx_hex=%s",
		command, len(payload), hex.EncodeToString(payload))

	_ = conn.SetDeadline(time.Now().Add(tvTimeout))
	if _, err = conn.Write(payload); err != nil {
		return "", fmt.Errorf("write command: %w", err)
	}

	// Accumulate the full response — TCP can fragment the encrypted blobs.
	raw, err := readAll(conn, idle)
	if err != nil && len(raw) == 0 {
		return "", fmt.Errorf("read response: %w", err)
	}

	log.Printf("[lgdeck-wire] cmd=%q  rx_bytes=%d  rx_hex=%s",
		command, len(raw), hex.EncodeToString(raw))

	// Decode
	if c.enc != nil {
		result, decErr := c.enc.Decode(raw)
		if decErr != nil {
			log.Printf("[lgdeck-wire] decrypt FAILED cmd=%q  err=%v", command, decErr)
			return "", decErr
		}
		log.Printf("[lgdeck-wire] decrypted cmd=%q  plain=%q", command, result)
		return result, nil
	}
	e := &Encoder{}
	result := e.Decode(raw)
	log.Printf("[lgdeck-wire] plain cmd=%q  resp=%q", command, result)
	return result, nil
}

// readAll reads from conn until EOF, connection close, or a brief idle pause,
// accumulating all bytes. This handles fragmented TCP responses correctly.
func readAll(conn net.Conn, idleTimeout time.Duration) ([]byte, error) {
	var buf bytes.Buffer
	chunk := make([]byte, 4096)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		n, err := conn.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
		}
		if err != nil {
			if err == io.EOF {
				return buf.Bytes(), nil
			}
			// A deadline timeout after we already have bytes is fine — we're done.
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && buf.Len() > 0 {
				return buf.Bytes(), nil
			}
			if buf.Len() > 0 {
				return buf.Bytes(), nil
			}
			return buf.Bytes(), err
		}
		// If we're in plain-text mode, stop at the response terminator.
		if bytes.ContainsRune(buf.Bytes(), respTerminator) {
			return buf.Bytes(), nil
		}
	}
}

// IsReachable returns true if the TCP port is connectable within timeout.
func (c *TVClient) IsReachable() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.host, tvPort), tvTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
