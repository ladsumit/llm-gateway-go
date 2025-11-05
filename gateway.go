package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// LLMUsageMetrics holds all global, thread-safe metrics for the gateway.
type LLMUsageMetrics struct {
	TotalRequests int            `json:"totalRequests"`
	TotalCost     float64        `json:"totalCostUsd"`
	ModelUsage    map[string]int `json:"modelUsageCount"` // Maps model name to request count
}

var (
	// metricsMutex protects the globalMetrics struct from concurrent access
	metricsMutex sync.Mutex
	// globalMetrics is the single source of truth for all usage data
	globalMetrics = LLMUsageMetrics{
		TotalRequests: 0,
		TotalCost:     0.0,
		ModelUsage:    make(map[string]int),
	}
)

// --- CONFIGURATION ---

// Define the API endpoints for the cheap and expensive models.
const cheapModelURL = "http://localhost:8081/v1/chat/completions"     // Target: Cheap Mock Server
const expensiveModelURL = "http://localhost:8082/v1/chat/completions" // Target: Expensive Mock Server

// Define your API Key environment variable name
const apiKeyEnv = "LLM_GATEWAY_API_KEY"

const promptLengthThreshold = 150 // Prompts longer than 150 characters are routed to the Expensive model.

// Define cost estimates (as a proxy for token usage)
const (
	cheapModelCostPerChar     = 0.0000001 // $0.10 per million characters
	expensiveModelCostPerChar = 0.0000015 // $1.50 per million characters
)

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

// getPromptLength processes the raw request body to determine the total length of the user's prompt.
func getPromptLength(bodyBytes []byte) (int, error) {
	var payload RequestPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		// Using fmt.Errorf allows us to wrap the original error for better debugging
		return 0, fmt.Errorf("failed to unmarshal JSON request body: %w", err)
	}

	totalLength := 0
	// Calculate the total content length from all user messages
	for _, message := range payload.Messages {
		if message.Role == "user" {
			// Using len(message.Content) is a simple character count proxy for token usage.
			totalLength += len(message.Content)
		}
	}
	return totalLength, nil
}

// MetricsHandler serves the current usage and cost metrics in JSON format.
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	// Lock the mutex to ensure we read a consistent snapshot of the data
	metricsMutex.Lock()
	// IMPORTANT: Defer the Unlock call to ensure the lock is always released
	defer metricsMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Encode the global metrics struct into the HTTP response body
	if err := json.NewEncoder(w).Encode(globalMetrics); err != nil {
		log.Printf("! ERR: Failed to encode metrics JSON: %v", err)
		http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
	}
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

	// 2. Analyze Prompt Content to Determine Routing
	totalPromptLength, err := getPromptLength(bodyBytes)
	if err != nil {
		// CRITICAL FIX: If the payload is bad, we must abort here.
		log.Printf("! ERR: Failed to analyze prompt body: %v\n", err)
		http.Error(w, "Invalid request format: must be a valid chat completion JSON payload", http.StatusBadRequest)
		return
	}

	// 3. MODEL SELECTION (Intelligent Routing Logic)
	var targetURL string
	var modelName string
	var costPerChar float64

	if totalPromptLength <= promptLengthThreshold {
		targetURL = cheapModelURL
		modelName = "CheapModel"
		costPerChar = cheapModelCostPerChar
	} else {
		targetURL = expensiveModelURL
		modelName = "ExpensiveModel"
		costPerChar = expensiveModelCostPerChar
	}

	// Calculate Estimated Cost
	estimatedCost := float64(totalPromptLength) * costPerChar

	// 4. Logging & Authorization Setup (Combined for clarity)
	log.Printf("-> REQ: Received request from %s | Prompt Size: %d chars", r.RemoteAddr, totalPromptLength)
	log.Printf("-> ROUTE: Prompt size (%d) <= %d? %t. Routing to %s (Est. Cost: $%.6f)\n",
		totalPromptLength, promptLengthThreshold, totalPromptLength <= promptLengthThreshold, modelName, estimatedCost)

	// Determine Authorization and Content-Type headers for upstream calls
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		if apiKey := os.Getenv(apiKeyEnv); apiKey != "" {
			authHeader = "Bearer " + apiKey
		}
	}
	contentType := r.Header.Get("Content-Type")

	// 5. Create the Upstream Request
	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("! ERR: Failed to create proxy request: %v\n", err)
		http.Error(w, "Error creating upstream request", http.StatusInternalServerError)
		return
	}

	// Copy essential headers
	proxyReq.Header = make(http.Header)
	if authHeader != "" {
		proxyReq.Header.Set("Authorization", authHeader)
	}
	proxyReq.Header.Set("Content-Type", contentType)

	// 6. Execute the Request
	client := &http.Client{Timeout: 30 * time.Second} // Client with a timeout
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("! ERR: Failed to execute upstream request to %s: %v\n", targetURL, err)
		http.Error(w, fmt.Sprintf("Error connecting to %s endpoint", modelName), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 7. Return Response to Client & Update Metrics
	// Copy upstream status and headers
	w.WriteHeader(resp.StatusCode)
	for name, values := range resp.Header {
		w.Header()[name] = values
	}

	// Stream the body back to the original client
	responseSize, err := io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("! ERR: Failed to copy response body back: %v\n", err)
	}

	// Update Metrics (Thread-safe operation)
	metricsMutex.Lock()
	globalMetrics.TotalRequests++
	globalMetrics.TotalCost += estimatedCost
	globalMetrics.ModelUsage[modelName]++
	metricsMutex.Unlock()

	// 8. Final Logging & Performance Metric
	log.Printf("<- RSP: Model: %s | Cost Est: $%.6f | Status: %d | Time: %s | Size: %d bytes\n",
		modelName, estimatedCost, resp.StatusCode, time.Since(start), responseSize)
}

func main() {
	// Initialize map for ModelUsage
	globalMetrics.ModelUsage["CheapModel"] = 0
	globalMetrics.ModelUsage["ExpensiveModel"] = 0

	// Set up the router
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", LLMProxyHandler)
	mux.HandleFunc("/metrics", MetricsHandler)

	port := ":8080"
	log.Printf("✅ LLM Gateway (Prompt-Size Router & Metrics) starting on http://localhost%s\n", port)

	// Start the server
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
