package codec

import (
	"encoding/base64"
	"testing"
)

// TestDecodeBase64 tests the DecodeBase64 function
func TestDecodeBase64(t *testing.T) {
	// Test cases
	testCases := []struct {
		name        string
		input       string
		expected    []byte
		expectError bool
	}{
		{
			name:        "Valid base64",
			input:       "SGVsbG8gV29ybGQ=", // "Hello World"
			expected:    []byte("Hello World"),
			expectError: false,
		},
		{
			name:        "Empty string",
			input:       "",
			expected:    []byte{},
			expectError: false,
		},
		{
			name:        "Invalid base64",
			input:       "Invalid!@#$",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "JSON object",
			input:       base64.StdEncoding.EncodeToString([]byte(`{"name":"John","age":30}`)),
			expected:    []byte(`{"name":"John","age":30}`),
			expectError: false,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function
			result, err := DecodeBase64(tc.input)

			// Check error
			if tc.expectError && err == nil {
				t.Errorf("Expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// If we don't expect an error, check the result
			if !tc.expectError {
				if string(result) != string(tc.expected) {
					t.Errorf("Expected %q but got %q", tc.expected, result)
				}
			}
		})
	}
}

// TestDecodeBase62 tests the DecodeBase62 function
func TestDecodeBase62(t *testing.T) {
	// Test cases
	testCases := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "Valid base62 characters",
			input:       "73XpUgyMwkGr29M", // "Hello World" in base64, which is also valid base62
			expectError: false,
		},
		{
			name:        "Empty string",
			input:       "",
			expectError: false, // Empty string is valid base64
		},
		{
			name:        "Invalid base62 characters",
			input:       "Invalid!@#$",
			expectError: true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function
			_, err := DecodeBase62(tc.input)

			// Check error
			if tc.expectError && err == nil {
				t.Errorf("Expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// Note: We don't check the result for base62 since it's not fully implemented
		})
	}
}

// TestBase62RoundTrip verifies EncodeBase62/DecodeBase62 round-trip exactly,
// including binary payloads with leading zero bytes (regression for the
// big.Int round trip silently dropping leading 0x00 bytes).
func TestBase62RoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{"simple ascii", []byte("Hello World")},
		{"single zero byte", []byte{0x00}},
		{"leading zero bytes", []byte{0x00, 0x00, 0x01, 0x02}},
		{"all zero bytes", []byte{0x00, 0x00, 0x00}},
		{"binary with embedded zeros", []byte{0x0a, 0x00, 0xff, 0x00}},
		{"empty", []byte{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := EncodeBase62(tc.data)
			decoded, err := DecodeBase62(encoded)
			if err != nil {
				t.Fatalf("DecodeBase62(%q) failed: %v", encoded, err)
			}
			if len(decoded) != len(tc.data) {
				t.Fatalf("round trip changed length: %d -> %d (encoded %q, decoded %x, original %x)",
					len(tc.data), len(decoded), encoded, decoded, tc.data)
			}
			for i := range decoded {
				if decoded[i] != tc.data[i] {
					t.Fatalf("round trip mismatch at byte %d: got %x, want %x", i, decoded, tc.data)
				}
			}
		})
	}
}
