package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Suhaibinator/SRouter/pkg/codec"
)

const (
	runeScrollVersion = "SROUTER-RUNE-SCROLL/1"
	runeScrollType    = "text/x-srouter-rune-scroll; charset=utf-8"
)

// OracleRequest is the typed value produced from an incoming rune scroll.
type OracleRequest struct {
	Seeker   string
	Question string
	Vibe     string
}

// OracleResponse is the typed value the handler gives back to the codec.
type OracleResponse struct {
	Seeker    string
	Question  string
	Vibe      string
	Omen      string
	LuckyRune string
}

// RuneScrollCodec implements SRouter's codec contract for a deliberately
// whimsical line-based protocol. A real custom codec could use XML, YAML,
// MessagePack, encryption, or an existing company-specific wire format.
type RuneScrollCodec struct{}

// Keep the interface relationship visible and checked by the compiler.
var _ codec.Codec[OracleRequest, OracleResponse] = (*RuneScrollCodec)(nil)

func NewRuneScrollCodec() *RuneScrollCodec {
	return &RuneScrollCodec{}
}

func (c *RuneScrollCodec) NewRequest() OracleRequest {
	return OracleRequest{}
}

// Decode handles the normal request-body source.
func (c *RuneScrollCodec) Decode(r *http.Request) (OracleRequest, error) {
	defer func() { _ = r.Body.Close() }()

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		// Preserve wrapped errors such as http.MaxBytesError so SRouter can map
		// them to the appropriate status code.
		return c.NewRequest(), fmt.Errorf("read rune scroll: %w", err)
	}

	return c.DecodeBytes(payload)
}

// DecodeBytes lets the same codec work with SRouter's encoded query and path
// sources after the router has removed the base64/base62 transport encoding.
func (c *RuneScrollCodec) DecodeBytes(payload []byte) (OracleRequest, error) {
	request := c.NewRequest()
	if !utf8.Valid(payload) {
		return request, fmt.Errorf("rune scroll must be valid UTF-8")
	}

	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimSpace(payload)))
	if !scanner.Scan() {
		return request, fmt.Errorf("rune scroll is empty")
	}
	if header := strings.TrimSuffix(scanner.Text(), "\r"); header != runeScrollVersion {
		return request, fmt.Errorf("expected %q header", runeScrollVersion)
	}

	seen := make(map[string]bool, 3)
	for lineNumber := 2; scanner.Scan(); lineNumber++ {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		key, value, ok := strings.Cut(line, " :: ")
		if !ok {
			return request, fmt.Errorf("line %d must use 'key :: value'", lineNumber)
		}

		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if seen[key] {
			return request, fmt.Errorf("line %d repeats field %q", lineNumber, key)
		}
		seen[key] = true

		switch key {
		case "seeker":
			request.Seeker = value
		case "question":
			request.Question = value
		case "vibe":
			request.Vibe = value
		default:
			return request, fmt.Errorf("line %d contains unknown field %q", lineNumber, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return request, fmt.Errorf("scan rune scroll: %w", err)
	}

	return request, nil
}

// Encode turns the typed handler response into a decorative rune scroll.
func (c *RuneScrollCodec) Encode(w http.ResponseWriter, response OracleResponse) error {
	w.Header().Set("Content-Type", runeScrollType)

	var scroll strings.Builder
	fmt.Fprintf(&scroll, "%s\n", runeScrollVersion)
	fmt.Fprintf(&scroll, "✦ seeker :: %s\n", response.Seeker)
	fmt.Fprintf(&scroll, "✦ omen :: %s\n", response.Omen)
	fmt.Fprintf(&scroll, "✦ aura :: %s\n", response.Vibe)
	fmt.Fprintf(&scroll, "✦ lucky-rune :: %s\n", response.LuckyRune)
	fmt.Fprintf(&scroll, "✦ echoes :: %s\n", response.Question)

	if _, err := io.WriteString(w, scroll.String()); err != nil {
		return fmt.Errorf("write rune scroll: %w", err)
	}
	return nil
}
