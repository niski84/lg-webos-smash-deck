package lgdeck

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// Protocol constants reverse-engineered from lgtv-ip-control (WesSouza/lgtv-ip-control).
const (
	msgTerminator      = '\r'
	respTerminator     = '\n'
	msgBlockSize       = 16
	encKeyLength       = 16
	encIVLength        = 16
	encKeyIterations   = 1 << 14 // 16384
)

// hardcoded salt from DefaultSettings.ts
var encKeySalt = []byte{
	0x63, 0x61, 0xb8, 0x0e, 0x9b, 0xdc, 0xa6, 0x63,
	0x8d, 0x07, 0x20, 0xf2, 0xcc, 0x56, 0x8f, 0xb9,
}

// Encoder handles plain (unencrypted) framing.
type Encoder struct{}

func (e *Encoder) Encode(msg string) []byte {
	return append([]byte(msg), msgTerminator)
}

func (e *Encoder) Decode(data []byte) string {
	s := string(data)
	if i := indexOf(s, respTerminator); i >= 0 {
		return s[:i]
	}
	return s
}

// Encryption handles AES-128-ECB/CBC framing for 2018+ TVs.
type Encryption struct {
	derivedKey []byte
}

func NewEncryption(keycode string) (*Encryption, error) {
	if len(keycode) != 8 {
		return nil, fmt.Errorf("keycode must be 8 characters, got %d", len(keycode))
	}
	// Derive key from keycode as-is. LG keycode format is typically [A-Z0-9]{8}
	// but some TV firmware versions generate lowercase. Try the keycode exactly
	// as provided; the caller can also try strings.ToUpper if this fails.
	key := pbkdf2.Key([]byte(keycode), encKeySalt, encKeyIterations, encKeyLength, sha256.New)
	log.Printf("[lgdeck-enc] keycode=%q  derived_key=%s", keycode, hex.EncodeToString(key))
	return &Encryption{derivedKey: key}, nil
}

// NewEncryptionUpper is like NewEncryption but forces the keycode to uppercase,
// which matches the original TS library's keycodeFormat /[A-Z0-9]{8}/.
func NewEncryptionUpper(keycode string) (*Encryption, error) {
	return NewEncryption(strings.ToUpper(keycode))
}

// Encode encrypts a command string into the wire format:
//   ivEnc (16 bytes, IV encrypted via AES-ECB) || dataEnc (message encrypted via AES-CBC)
func (enc *Encryption) Encode(msg string) ([]byte, error) {
	iv := make([]byte, encIVLength)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("generating IV: %w", err)
	}

	padded := padMessage(string(append([]byte(msg), msgTerminator)))

	ivEnc, err := ecbEncrypt(enc.derivedKey, iv)
	if err != nil {
		return nil, fmt.Errorf("ECB-encrypt IV: %w", err)
	}

	dataEnc, err := cbcEncrypt(enc.derivedKey, iv, []byte(padded))
	if err != nil {
		return nil, fmt.Errorf("CBC-encrypt data: %w", err)
	}

	return append(ivEnc, dataEnc...), nil
}

// Decode decrypts a response from the TV.
func (enc *Encryption) Decode(data []byte) (string, error) {
	log.Printf("[lgdeck-enc] decode  total_bytes=%d  hex=%s", len(data), hex.EncodeToString(data))

	if len(data) < encKeyLength*2 {
		return "", fmt.Errorf("response too short for decryption: %d bytes (need ≥ %d)", len(data), encKeyLength*2)
	}

	ivEnc := data[:encKeyLength]
	ciphertext := data[encKeyLength:]

	log.Printf("[lgdeck-enc] ivEnc=%s  ciphertext=%s", hex.EncodeToString(ivEnc), hex.EncodeToString(ciphertext))

	iv, err := ecbDecrypt(enc.derivedKey, ivEnc)
	if err != nil {
		return "", fmt.Errorf("ECB-decrypt IV: %w", err)
	}
	log.Printf("[lgdeck-enc] recovered_iv=%s", hex.EncodeToString(iv))

	if len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("ciphertext length %d not block-aligned", len(ciphertext))
	}
	plaintext, err := cbcDecrypt(enc.derivedKey, iv, ciphertext)
	if err != nil {
		return "", fmt.Errorf("CBC-decrypt data: %w", err)
	}
	log.Printf("[lgdeck-enc] plaintext_hex=%s  plaintext_str=%q", hex.EncodeToString(plaintext), plaintext)

	s := string(plaintext)
	if i := indexOf(s, respTerminator); i >= 0 {
		return s[:i], nil
	}
	return s, nil
}

// padMessage pads message to a multiple of msgBlockSize.
// If length is already a multiple, a space is prepended first (per TS impl).
func padMessage(msg string) string {
	if len(msg)%msgBlockSize == 0 {
		msg = " " + msg
	}
	rem := len(msg) % msgBlockSize
	if rem != 0 {
		pad := msgBlockSize - rem
		for i := 0; i < pad; i++ {
			msg += string(rune(pad))
		}
	}
	return msg
}

// ecbEncrypt encrypts a single block (or multiple full blocks) using AES-ECB.
func ecbEncrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ecbEncrypt: data length %d not block-aligned", len(data))
	}
	out := make([]byte, len(data))
	for i := 0; i < len(data); i += aes.BlockSize {
		block.Encrypt(out[i:i+aes.BlockSize], data[i:i+aes.BlockSize])
	}
	return out, nil
}

// ecbDecrypt decrypts using AES-ECB (no padding removal).
func ecbDecrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ecbDecrypt: data length %d not block-aligned", len(data))
	}
	out := make([]byte, len(data))
	for i := 0; i < len(data); i += aes.BlockSize {
		block.Decrypt(out[i:i+aes.BlockSize], data[i:i+aes.BlockSize])
	}
	return out, nil
}

func cbcEncrypt(key, iv, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, data)
	return out, nil
}

func cbcDecrypt(key, iv, data []byte) ([]byte, error) {
	if len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("cbcDecrypt: data length %d not block-aligned", len(data))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	return out, nil
}

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
