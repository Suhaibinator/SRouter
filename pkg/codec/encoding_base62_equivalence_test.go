package codec

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// referenceDecodeBase62 is the original digit-at-a-time implementation, kept
// here as the behavioural oracle for the subquadratic one in encoding.go.
func referenceDecodeBase62(s string) ([]byte, error) {
	if len(s) == 0 {
		return []byte{}, nil
	}

	charMap := make(map[rune]int)
	for i := range 10 {
		charMap[rune('0'+i)] = i
	}
	for i := range 26 {
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

// referenceEncodeBase62 is the original DivMod implementation.
func referenceEncodeBase62(data []byte) string {
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

	var digits []byte
	base := big.NewInt(62)
	mod := new(big.Int)
	for num.Sign() > 0 {
		num.DivMod(&num, base, mod)
		digits = append(digits, base62Alphabet[mod.Int64()])
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

func TestDecodeBase62MatchesReference(t *testing.T) {
	cases := []string{
		"",
		"0",
		"00",
		"000",
		"0A1B",
		"z",
		"zz",
		"ZZ",
		"aA0",
		"0000000000",
		"10",
		"01",
		strings.Repeat("z", 100),
		strings.Repeat("0", 50) + "abcXYZ789",
		// Invalid inputs, including multi-byte runes and sign characters that
		// big.Int.SetString would otherwise accept.
		"!",
		"a!b",
		"-1",
		"+1",
		"1_0",
		"héllo",
		strings.Repeat("z", 64) + "!",
		"日本語",
		strings.Repeat("z", base62DecodeLeafDigits),
		strings.Repeat("z", base62DecodeLeafDigits+1),
		"000" + strings.Repeat("z", 2*base62DecodeLeafDigits+1),
	}

	rng := rand.New(rand.NewSource(1))
	for range 500 {
		n := rng.Intn(300)
		var sb strings.Builder
		for range n {
			sb.WriteByte(base62Alphabet[rng.Intn(len(base62Alphabet))])
		}
		cases = append(cases, sb.String())
	}

	for _, in := range cases {
		wantBytes, wantErr := referenceDecodeBase62(in)
		gotBytes, gotErr := DecodeBase62(in)

		switch {
		case wantErr == nil && gotErr != nil:
			t.Errorf("DecodeBase62(%q) unexpected error: %v", in, gotErr)
		case wantErr != nil && gotErr == nil:
			t.Errorf("DecodeBase62(%q) expected error %v, got none", in, wantErr)
		case wantErr != nil && gotErr != nil:
			if wantErr.Error() != gotErr.Error() {
				t.Errorf("DecodeBase62(%q) error = %q, want %q", in, gotErr, wantErr)
			}
		default:
			if !bytes.Equal(wantBytes, gotBytes) {
				t.Errorf("DecodeBase62(%q) = %x, want %x", in, gotBytes, wantBytes)
			}
		}
	}
}

func TestEncodeBase62MatchesReference(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0},
		{0, 0},
		{0, 0, 0},
		{1},
		{0, 1},
		{255},
		{255, 255, 255},
		{0, 0, 255, 0, 1},
	}

	rng := rand.New(rand.NewSource(2))
	for range 500 {
		b := make([]byte, rng.Intn(300))
		rng.Read(b)
		// Bias toward payloads with leading zero bytes, the tricky case.
		for i := 0; i < rng.Intn(4) && i < len(b); i++ {
			b[i] = 0
		}
		cases = append(cases, b)
	}

	for _, in := range cases {
		want := referenceEncodeBase62(in)
		got := EncodeBase62(in)
		if want != got {
			t.Errorf("EncodeBase62(%x) = %q, want %q", in, got, want)
		}
		// And the pair must still round-trip.
		back, err := DecodeBase62(got)
		if err != nil {
			t.Errorf("DecodeBase62(EncodeBase62(%x)) error: %v", in, err)
			continue
		}
		if len(in) == 0 && len(back) == 0 {
			continue
		}
		if !bytes.Equal(in, back) {
			t.Errorf("round trip of %x produced %x", in, back)
		}
	}
}

// TestDecodeBase62LargeInputScalesSubquadratically guards against returning to
// a growing-integer multiply for every digit. Comparing two input sizes keeps
// the assertion independent of the absolute speed of the test machine.
func TestDecodeBase62LargeInputScalesSubquadratically(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scaling test in short mode")
	}

	fastestDecode := func(size int) time.Duration {
		t.Helper()
		best := time.Duration(1<<63 - 1)
		input := strings.Repeat("z", size)
		for range 3 {
			start := time.Now()
			if _, err := DecodeBase62(input); err != nil {
				t.Fatalf("decoding %d bytes: %v", size, err)
			}
			best = min(best, time.Since(start))
		}
		return best
	}

	const smallSize = 1 << 18
	smallDuration := fastestDecode(smallSize)
	largeDuration := fastestDecode(4 * smallSize)

	// The balanced conversion is bounded by math/big's Karatsuba multiply, so
	// four times the input costs about 4^log2(3) ~ 9x in theory and measures
	// 7.5x-9x on a quiet machine, rising past 11x on shared CI runners once the
	// larger input spills out of cache. The quadratic conversion it replaced
	// costs 16x in theory and measures 15x or more under the same conditions,
	// and noise only pushes it higher. 13x sits between the two with margin on
	// both sides.
	const maxGrowth = 13
	if largeDuration > maxGrowth*smallDuration {
		t.Errorf("decode time grew from %v to %v for 4x input (%.1fx); want less than %dx growth",
			smallDuration, largeDuration, float64(largeDuration)/float64(smallDuration), maxGrowth)
	}
}

// TestDecodeBase62RejectsInvalidBeforeConversion ensures a long valid prefix
// followed by one invalid byte is rejected without doing the conversion work.
func TestDecodeBase62RejectsInvalidBeforeConversion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}

	input := strings.Repeat("z", 1<<20) + "!"
	start := time.Now()
	_, err := DecodeBase62(input)
	if err == nil {
		t.Fatal("expected an error for trailing invalid character")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("rejecting invalid input took %v; validation should precede conversion", elapsed)
	}
}
