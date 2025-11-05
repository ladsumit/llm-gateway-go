package main

import (
	"fmt"
	"log"
	"net/http"
)

// cheapHandler handles requests for the cheap model.
func cheapHandler(w http.ResponseWriter, r *http.Request) {
	// Set the response header
	w.Header().Set("Content-Type", "application/json")

	// Write the fast, cheap response body
	response := `{"model": "SLM-7B-cheap", "response": "Summary: The main idea is to use Go for high concurrency routing.", "cost_estimate": 0.0001}`

	// Log the request being served
	log.Printf("-> Served request for CHEAP model (%s) in 0ms\n", r.URL.Path)

	fmt.Fprint(w, response)
}

func main() {
	// Define the port for the cheap model
	port := "8081"

	// Register the handler function for all incoming paths ("/")
	http.HandleFunc("/", cheapHandler)

	fmt.Printf("Starting CHEAP LLM Server on http://localhost:%s\n", port)

	// ListenAndServe starts the HTTP server.
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start cheap server: %v", err)
	}
}
