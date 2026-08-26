// Package codec provides encoding and decoding functionality for different data formats.
package codec

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"unicode/utf8"
)

// base62Alphabet is the alphabet used by EncodeBase62 and DecodeBase62:
// '0'-'9' are 0-9, 'A'-'Z' are 10-35, and 'a'-'z' are 36-61.
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// base62DecodeLeafDigits bounds the quadratic work done by math/big.SetString.
// Larger values are assembled from leaves with a balanced multiplication tree.
const base62DecodeLeafDigits = 256

// swapASCIICase maps between this package's base62 alphabet and the one
// math/big uses for bases above 36.
//
// math/big orders its digits '0'-'9', then 'a'-'z' (10-35), then 'A'-'Z'
// (36-61) — the inverse case ordering of base62Alphabet. Swapping the case of
// every letter therefore translates a string from one alphabet to the other,
// in either direction, letting DecodeBase62 and EncodeBase62 use math/big's
// subquadratic conversion routines instead of a digit-at-a-time loop.
func swapASCIICase(s []byte) {
	for i, c := range s {
		switch {
		case c >= 'A' && c <= 'Z':
			s[i] = c + ('a' - 'A')
		case c >= 'a' && c <= 'z':
			s[i] = c - ('a' - 'A')
		}
	}
}

// decodeBase62Digits converts digits written in math/big's base-62 alphabet.
// math/big.SetString parses non-power-of-two bases by repeatedly multiplying a
// growing integer, which is quadratic for large inputs. Keep that work inside
// fixed-size leaves, then concatenate the leaves with a balanced tree:
//
//	left || right = left*62^len(right) + right
//
// The powers are shared by digit length so each tree level computes them once.
func decodeBase62Digits(digits []byte) (*big.Int, bool) {
	base := big.NewInt(62)
	powers := map[int]*big.Int{
		0: big.NewInt(1),
		1: base,
	}

	var power func(int) *big.Int
	power = func(n int) *big.Int {
		if p, ok := powers[n]; ok {
			return p
		}

		half := power(n / 2)
		p := new(big.Int).Mul(half, half)
		if n%2 != 0 {
			p.Mul(p, base)
		}
		powers[n] = p
		return p
	}

	var decode func([]byte) (*big.Int, bool)
	decode = func(part []byte) (*big.Int, bool) {
		if len(part) <= base62DecodeLeafDigits {
			value, ok := new(big.Int).SetString(string(part), 62)
			return value, ok
		}

		mid := len(part) / 2
		left, ok := decode(part[:mid])
		if !ok {
			return nil, false
		}
		right, ok := decode(part[mid:])
		if !ok {
			return nil, false
		}

		left.Mul(left, power(len(part)-mid))
		left.Add(left, right)
		return left, true
	}

	return decode(digits)
}

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

	// Validate every byte and translate into math/big's alphabet in one pass.
	// Doing this up front means an invalid character is rejected in linear time
	// rather than after the arbitrary-precision conversion has already run.
	digits := []byte(s)
	for i, c := range digits {
		if (c < '0' || c > '9') && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			// Report the whole rune, not the leading byte, so multi-byte input
			// produces a readable error.
			r, _ := utf8.DecodeRuneInString(s[i:])
			return nil, fmt.Errorf("invalid base62 character: %q", r)
		}
	}
	// Count leading '0' characters: each one encodes a leading zero byte that
	// the big.Int representation cannot carry.
	leadingZeros := 0
	for _, c := range digits {
		if c != '0' {
			break
		}
		leadingZeros++
	}

	var decoded []byte
	if leadingZeros < len(digits) {
		digits = digits[leadingZeros:]
		swapASCIICase(digits)

		result, ok := decodeBase62Digits(digits)
		if !ok {
			return nil, fmt.Errorf("invalid base62 string")
		}
		decoded = result.Bytes()
	}

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

	var num big.Int
	num.SetBytes(data)

	// Text uses a divide-and-conquer conversion for large values, avoiding the
	// quadratic cost of dividing a shrinking big.Int once per digit. A zero
	// value must stay empty here: its leading zero bytes are already accounted
	// for by leadingZeros, and Text would render it as a spurious extra "0".
	var digits []byte
	if num.Sign() > 0 {
		digits = []byte(num.Text(62))
		swapASCIICase(digits)
	}

	encoded := make([]byte, leadingZeros+len(digits))
	for i := range leadingZeros {
		encoded[i] = '0'
	}
	copy(encoded[leadingZeros:], digits)
	return string(encoded)
}
