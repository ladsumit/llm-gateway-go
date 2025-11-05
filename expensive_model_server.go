package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// expensiveHandler handles requests for the expensive model.
func expensiveHandler(w http.ResponseWriter, r *http.Request) {
	// Set the response header
	w.Header().Set("Content-Type", "application/json")

	// Simulate a delay of 500 milliseconds (0.5 seconds)
	delay := 500 * time.Millisecond
	time.Sleep(delay)

	// Write the slower, detailed response body
	response := `{"model": "LLM-150B-expensive", "response": "Detailed Architectural Analysis: Go is superior for network proxies due to Goroutine efficiency...", "cost_estimate": 0.0125}`

	// Log the request being served
	log.Printf("-> Served request for EXPENSIVE model (%s) in %v\n", r.URL.Path, delay)

	fmt.Fprint(w, response)
}

func main() {
	// Define the port for the expensive model
	port := "8082"

	// Register the handler function for all incoming paths ("/")
	http.HandleFunc("/", expensiveHandler)

	fmt.Printf("Starting EXPENSIVE LLM Server on http://localhost:%s\n", port)

	// ListenAndServe starts the HTTP server.
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start expensive server: %v", err)
	}
}
