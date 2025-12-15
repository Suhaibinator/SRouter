// Package main demonstrates WebSocket support in SRouter.
//
// This example creates a server with two endpoints:
//   - REST endpoint: /api/health - returns a JSON health check
//   - WebSocket endpoint: /ws/echo - echoes messages back to the client
//
// The server starts in a goroutine and the main function tests both endpoints,
// printing results to stdout to prove the implementation is working.
package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

const (
	serverAddr     = "127.0.0.1:8765"
	serverTimeout  = 60 * time.Second
	websocketMagic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

func main() {
	fmt.Println("=== SRouter WebSocket Example ===")
	fmt.Println()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), serverTimeout)
	defer cancel()

	// Start the server
	server, err := startServer(ctx)
	if err != nil {
		fmt.Printf("FAILED: Could not start server: %v\n", err)
		return
	}

	// Wait for server to be ready
	waitForServer(serverAddr, 5*time.Second)
	fmt.Println()

	// Test the REST endpoint
	fmt.Println("--- Testing REST Endpoint ---")
	testRESTEndpoint()
	fmt.Println()

	// Test the WebSocket endpoint
	fmt.Println("--- Testing WebSocket Endpoint ---")
	testWebSocketEndpoint()
	fmt.Println()

	// Shutdown server
	fmt.Println("--- Shutting Down ---")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("Server shutdown error: %v\n", err)
	} else {
		fmt.Println("Server shutdown gracefully")
	}

	fmt.Println()
	fmt.Println("=== Example Complete ===")
}

// startServer creates and starts the HTTP server with REST and WebSocket routes
func startServer(ctx context.Context) (*http.Server, error) {
	logger, _ := zap.NewDevelopment()

	routerConfig := router.RouterConfig{
		ServiceName:   "websocket-example",
		Logger:        logger,
		GlobalTimeout: 5 * time.Second, // Regular routes have a timeout
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes: []router.RouteDefinition{
					router.RouteConfigBase{
						Path:      "/health",
						Methods:   []router.HttpMethod{router.MethodGet},
						AuthLevel: router.Ptr(router.NoAuth),
						Handler:   healthHandler,
					},
				},
			},
			{
				PathPrefix: "/ws",
				Routes: []router.RouteDefinition{
					router.RouteConfigBase{
						Path:        "/echo",
						Methods:     []router.HttpMethod{router.MethodGet},
						AuthLevel:   router.Ptr(router.NoAuth),
						IsWebSocket: true, // This disables timeout and enables hijacking
						Handler:     websocketEchoHandler,
					},
				},
			},
		},
	}

	r := router.NewRouter[string, string](routerConfig, nil, nil)

	server := &http.Server{
		Addr:    serverAddr,
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		fmt.Printf("Starting server on %s...\n", serverAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	// Monitor context for cancellation
	go func() {
		<-ctx.Done()
		fmt.Println("Context timeout reached, shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	return server, nil
}

// waitForServer polls until the server is ready or timeout
func waitForServer(addr string, timeout time.Duration) {
	fmt.Printf("Waiting for server to be ready...")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			fmt.Println(" ready!")
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println(" timeout!")
}

// healthHandler handles the REST health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy","service":"websocket-example"}`))
}

// websocketEchoHandler handles WebSocket connections and echoes messages
func websocketEchoHandler(w http.ResponseWriter, r *http.Request) {
	// Verify this is a WebSocket upgrade request
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Expected WebSocket upgrade", http.StatusBadRequest)
		return
	}

	// Get the Sec-WebSocket-Key for the handshake
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Perform WebSocket handshake
	acceptKey := computeAcceptKey(key)
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

	if _, err := bufrw.WriteString(response); err != nil {
		return
	}
	if err := bufrw.Flush(); err != nil {
		return
	}

	fmt.Println("[Server] WebSocket connection established")

	// Simple echo loop - read frames and echo them back
	for {
		// Set read deadline to detect disconnection
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		// Read a WebSocket frame
		frame, err := readWebSocketFrame(bufrw.Reader)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("[Server] Read error: %v\n", err)
			}
			break
		}

		// Handle close frame
		if frame.opcode == 0x8 {
			fmt.Println("[Server] Received close frame")
			// Send close frame back
			writeWebSocketFrame(conn, 0x8, []byte{})
			break
		}

		// Echo text frames
		if frame.opcode == 0x1 {
			message := string(frame.payload)
			fmt.Printf("[Server] Received: %q\n", message)

			// Echo the message back
			echoMessage := "Echo: " + message
			if err := writeWebSocketFrame(conn, 0x1, []byte(echoMessage)); err != nil {
				fmt.Printf("[Server] Write error: %v\n", err)
				break
			}
			fmt.Printf("[Server] Sent: %q\n", echoMessage)
		}
	}

	fmt.Println("[Server] WebSocket connection closed")
}

// computeAcceptKey computes the Sec-WebSocket-Accept header value
func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + websocketMagic))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// wsFrame represents a minimal WebSocket frame
type wsFrame struct {
	opcode  byte
	payload []byte
}

// readWebSocketFrame reads a WebSocket frame from the connection
func readWebSocketFrame(r *bufio.Reader) (*wsFrame, error) {
	// Read first two bytes
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	opcode := header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	length := uint64(header[1] & 0x7F)

	// Extended payload length
	if length == 126 {
		ext := make([]byte, 2)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		length = uint64(ext[0])<<8 | uint64(ext[1])
	} else if length == 127 {
		ext := make([]byte, 8)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		length = uint64(ext[0])<<56 | uint64(ext[1])<<48 | uint64(ext[2])<<40 | uint64(ext[3])<<32 |
			uint64(ext[4])<<24 | uint64(ext[5])<<16 | uint64(ext[6])<<8 | uint64(ext[7])
	}

	// Read masking key if present
	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(r, maskKey); err != nil {
			return nil, err
		}
	}

	// Read payload
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	// Unmask payload if masked
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return &wsFrame{opcode: opcode, payload: payload}, nil
}

// writeWebSocketFrame writes a WebSocket frame to the connection
func writeWebSocketFrame(w io.Writer, opcode byte, payload []byte) error {
	// FIN bit set, opcode
	frame := []byte{0x80 | opcode}

	// Payload length (no masking from server)
	length := len(payload)
	if length < 126 {
		frame = append(frame, byte(length))
	} else if length < 65536 {
		frame = append(frame, 126, byte(length>>8), byte(length))
	} else {
		frame = append(frame, 127,
			byte(length>>56), byte(length>>48), byte(length>>40), byte(length>>32),
			byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}

	// Payload
	frame = append(frame, payload...)

	_, err := w.Write(frame)
	return err
}

// testRESTEndpoint tests the REST health check endpoint
func testRESTEndpoint() {
	url := "http://" + serverAddr + "/api/health"
	fmt.Printf("Testing: GET %s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	fmt.Printf("Status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Printf("Body: %s\n", string(body))

	if resp.StatusCode == http.StatusOK && strings.Contains(string(body), "healthy") {
		fmt.Println("PASSED: REST endpoint working correctly")
	} else {
		fmt.Println("FAILED: Unexpected response")
	}
}

// testWebSocketEndpoint tests the WebSocket echo endpoint
func testWebSocketEndpoint() {
	url := "ws://" + serverAddr + "/ws/echo"
	fmt.Printf("Testing: WebSocket %s\n", url)

	// Connect to server
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		fmt.Printf("FAILED: Could not connect: %v\n", err)
		return
	}
	defer conn.Close()

	// Generate a WebSocket key
	key := base64.StdEncoding.EncodeToString([]byte("test-websocket-key"))

	// Send WebSocket upgrade request
	request := "GET /ws/echo HTTP/1.1\r\n" +
		"Host: " + serverAddr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	if _, err := conn.Write([]byte(request)); err != nil {
		fmt.Printf("FAILED: Could not send upgrade request: %v\n", err)
		return
	}

	// Read response
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("FAILED: Could not read response: %v\n", err)
		return
	}

	fmt.Printf("Response: %s", statusLine)

	if !strings.Contains(statusLine, "101") {
		fmt.Println("FAILED: Expected 101 Switching Protocols")
		// Read and print the rest of the response for debugging
		for {
			line, err := reader.ReadString('\n')
			if err != nil || line == "\r\n" {
				break
			}
			fmt.Printf("  %s", line)
		}
		return
	}

	// Read rest of headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
		fmt.Printf("  %s", line)
	}

	fmt.Println("WebSocket handshake complete!")

	// Send a test message
	testMessage := "Hello, WebSocket!"
	fmt.Printf("Sending: %q\n", testMessage)
	if err := writeClientWebSocketFrame(conn, 0x1, []byte(testMessage)); err != nil {
		fmt.Printf("FAILED: Could not send message: %v\n", err)
		return
	}

	// Read echo response
	frame, err := readWebSocketFrame(reader)
	if err != nil {
		fmt.Printf("FAILED: Could not read response: %v\n", err)
		return
	}

	echoResponse := string(frame.payload)
	fmt.Printf("Received: %q\n", echoResponse)

	expectedResponse := "Echo: " + testMessage
	if echoResponse == expectedResponse {
		fmt.Println("PASSED: WebSocket echo working correctly")
	} else {
		fmt.Printf("FAILED: Expected %q, got %q\n", expectedResponse, echoResponse)
	}

	// Send close frame
	fmt.Println("Sending close frame...")
	writeClientWebSocketFrame(conn, 0x8, []byte{})
}

// writeClientWebSocketFrame writes a masked WebSocket frame (client frames must be masked)
func writeClientWebSocketFrame(w io.Writer, opcode byte, payload []byte) error {
	// FIN bit set, opcode
	frame := []byte{0x80 | opcode}

	// Payload length with mask bit set
	length := len(payload)
	if length < 126 {
		frame = append(frame, 0x80|byte(length)) // Mask bit set
	} else if length < 65536 {
		frame = append(frame, 0x80|126, byte(length>>8), byte(length))
	} else {
		frame = append(frame, 0x80|127,
			byte(length>>56), byte(length>>48), byte(length>>40), byte(length>>32),
			byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}

	// Masking key (using fixed key for simplicity)
	maskKey := []byte{0x12, 0x34, 0x56, 0x78}
	frame = append(frame, maskKey...)

	// Masked payload
	maskedPayload := make([]byte, len(payload))
	for i := range payload {
		maskedPayload[i] = payload[i] ^ maskKey[i%4]
	}
	frame = append(frame, maskedPayload...)

	_, err := w.Write(frame)
	return err
}
