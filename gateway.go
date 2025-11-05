package main

import (
	"bytes"
	"encoding/json" // New import for JSON handling
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// --- CONFIGURATION ---

// Define the API endpoints for the cheap and expensive models.
const cheapModelURL = "http://localhost:8081/v1/chat/completions"     // Target: Cheap Mock Server
const expensiveModelURL = "http://localhost:8082/v1/chat/completions" // Target: Expensive Mock Server

// Define your API Key environment variable name
const apiKeyEnv = "LLM_GATEWAY_API_KEY"

// --- REQUEST BODY STRUCTURES ---

// Message mirrors the structure of a single message object in the chat request.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RequestPayload mirrors the top-level structure of the incoming chat completion request.
type RequestPayload struct {
	Messages []Message `json:"messages"`
}

// --- INTELLIGENCE CORE ---

// analyzePrompt inspects the request body and selects the appropriate LLM target.
// It uses prompt length as a proxy for complexity.
func analyzePrompt(bodyBytes []byte) (targetURL, modelName string) {
	// Default to the cheap, fast model
	targetURL = cheapModelURL
	modelName = "CheapModel"

	// Unmarshal the JSON request body
	var payload RequestPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		// Log a warning if unmarshalling fails, but proceed with the default cheap model
		log.Printf("! WARN: Failed to unmarshal request body: %v. Defaulting to CheapModel.", err)
		return targetURL, modelName
	}

	// Calculate total prompt length from all messages
	totalLength := 0
	for _, msg := range payload.Messages {
		totalLength += len(msg.Content)
	}

	// Rule-Based Routing Logic:
	// Prompts longer than 150 characters are deemed 'complex' and routed to the
	// slower, more capable (mocked) ExpensiveModel.
	const complexityThreshold = 150

	if totalLength > complexityThreshold {
		targetURL = expensiveModelURL
		modelName = "ExpensiveModel"
	}

	return targetURL, modelName
}

// --- CORE HANDLER ---

// LLMProxyHandler is the main entry point for all client requests.
func LLMProxyHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Basic Setup & Validation
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	// Read the original request body once. Essential for routing and forwarding.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	// Restore the body (r.Body is consumed by io.ReadAll)
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 2. Initial Logging
	log.Printf("-> REQ: Received request from %s | Body Size: %d bytes\n", r.RemoteAddr, len(bodyBytes))

	// 3. MODEL SELECTION (Intelligent Routing Logic)
	// The core logic is here: inspect the body and choose the destination.
	targetURL, modelName := analyzePrompt(bodyBytes)

	log.Printf("-> ROUTE: Routing to %s at %s\n", modelName, targetURL)

	// 4. Create the Upstream Request
	// The body is read from the buffer created in step 1, ensuring we can read it again.
	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("! ERR: Failed to create proxy request: %v\n", err)
		http.Error(w, "Error creating upstream request", http.StatusInternalServerError)
		return
	}

	// Copy essential headers
	proxyReq.Header = make(http.Header)
	// Handle API Key authorization
	if r.Header.Get("Authorization") != "" {
		proxyReq.Header.Set("Authorization", r.Header.Get("Authorization"))
	} else if apiKey := os.Getenv(apiKeyEnv); apiKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	proxyReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	// 5. Execute the Request
	client := &http.Client{Timeout: 30 * time.Second} // Client with a timeout
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("! ERR: Failed to execute upstream request to %s: %v\n", targetURL, err)
		http.Error(w, "Error connecting to LLM endpoint", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 6. Return Response to Client
	// Copy upstream status and headers
	w.WriteHeader(resp.StatusCode)
	for name, values := range resp.Header {
		w.Header()[name] = values
	}

	// Stream the body back to the original client
	responseSize, err := io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("! ERR: Failed to copy response body back: %v\n", err)
		// Logging internal error only
	}

	// 7. Final Logging & Performance Metric
	log.Printf("<- RSP: Model: %s | Status: %d | Time: %s | Size: %d bytes\n",
		modelName, resp.StatusCode, time.Since(start), responseSize)
}

func main() {
	// Set up the router
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", LLMProxyHandler)

	port := ":8080"
	log.Printf("✅ LLM Gateway (Intelligent Router) starting on http://localhost%s\n", port)

	// Start the server
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
