// Package codec provides encoding and decoding functionality for different data formats.
package codec

import (
	"encoding/base64"
	"fmt"
	"math/big"
)

// DecodeBase64 decodes a base64-encoded string to bytes.
// It uses the standard base64 encoding as defined in RFC 4648.
// This function is used by the router when processing requests with Base64QueryParameter
// or Base64PathParameter source types.
//
// Parameters:
//   - encoded: The base64-encoded string to decode
//
// Returns:
//   - []byte: The decoded bytes
//   - error: An error if the input is not valid base64
func DecodeBase64(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

// DecodeBase62 decodes a base62-encoded string and returns the corresponding bytes.
//
// The base62 encoding uses the characters [0-9, A-Z, a-z], corresponding to
// values [0..61]. This function treats the first 10 digits ('0'–'9') as values
// 0–9, the next 26 letters ('A'–'Z') as values 10–35, and the final 26 letters
// ('a'–'z') as values 36–61.
//
// Leading '0' characters are significant: like base58's leading '1's, each
// leading '0' in the input represents one leading zero byte in the output.
// This preserves binary payloads (e.g. protobuf) whose first bytes are 0x00,
// which a plain big-integer round trip would silently drop.
//
// An error is returned if the input string contains invalid characters.
//
// Example usage:
//
//	decoded, err := DecodeBase62("0A1B")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Decoded bytes: %x\n", decoded)
func DecodeBase62(s string) ([]byte, error) {
	if len(s) == 0 {
		// Decide if you want to treat empty string as zero-length bytes or return an error.
		// Here we'll just return an empty slice.
		return []byte{}, nil
	}

	// Build a character -> value map
	charMap := make(map[rune]int)
	for i := 0; i < 10; i++ {
		charMap[rune('0'+i)] = i
	}
	for i := 0; i < 26; i++ {
		charMap[rune('A'+i)] = 10 + i
		charMap[rune('a'+i)] = 36 + i
	}

	var result big.Int

	for _, c := range s {
		val, ok := charMap[c]
		if !ok {
			return nil, fmt.Errorf("invalid base62 character: %q", c)
		}
		result.Mul(&result, big.NewInt(62))
		result.Add(&result, big.NewInt(int64(val)))
	}

	// Count leading '0' characters: each one encodes a leading zero byte that
	// the big.Int representation cannot carry.
	leadingZeros := 0
	for _, c := range s {
		if c != '0' {
			break
		}
		leadingZeros++
	}

	decoded := result.Bytes()
	if leadingZeros > 0 {
		withZeros := make([]byte, leadingZeros+len(decoded))
		copy(withZeros[leadingZeros:], decoded)
		decoded = withZeros
	}
	return decoded, nil
}

// EncodeBase62 encodes bytes to a base62 string using the same alphabet as
// DecodeBase62 ([0-9A-Za-z]). Leading zero bytes are encoded as leading '0'
// characters so that DecodeBase62(EncodeBase62(b)) round-trips exactly.
func EncodeBase62(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	leadingZeros := 0
	for _, b := range data {
		if b != 0 {
			break
		}
		leadingZeros++
	}

	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var num big.Int
	num.SetBytes(data)

	var digits []byte
	base := big.NewInt(62)
	mod := new(big.Int)
	for num.Sign() > 0 {
		num.DivMod(&num, base, mod)
		digits = append(digits, alphabet[mod.Int64()])
	}

	encoded := make([]byte, leadingZeros+len(digits))
	for i := range leadingZeros {
		encoded[i] = '0'
	}
	for i, d := range digits {
		encoded[leadingZeros+len(digits)-1-i] = d
	}
	return string(encoded)
}
