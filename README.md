# llm-gateway-go

## LLM Routing Gateway with Cost Optimization

This project is a simple, high-concurrency LLM Proxy Gateway built in Go.  
It saves money by routing chat completion requests to different models based on the prompt's complexity.

Two mock LLM services simulate a "Cheap" and an "Expensive" model API for demonstration purposes.

---

## Key Features

### Cost-Based Routing
The gateway checks the length of the user's prompt and routes accordingly:
- **Short Prompts (≤ 150 characters):** Routed to the Cheap Model (running on port 8081)
- **Long Prompts (> 150 characters):** Routed to the Expensive Model (running on port 8082)

### Real-Time Metrics
A dedicated `/metrics` endpoint provides a live, thread-safe dashboard showing:
- Total number of requests handled
- Total estimated cost incurred in USD
- Usage count for both the Cheap and Expensive models

### Concurrency
Uses Go's Goroutines and channels to handle many incoming proxy requests simultaneously.

---

## Setup and Running the Gateway

**Prerequisites**
- Go 1.18+ installed
- Three terminal windows (or background processes)

You must run three separate components for the system to work:  
two mock servers (model APIs) and the main gateway.

---

### 1. Start Mock LLM Servers (Provider APIs)

Run these two commands in separate terminal windows:

```bash
# Terminal 1: Cheap Model Server
go run cheap_model_server.go
# Output: Mock server for CheapModel running on http://localhost:8081
```

```bash
# Terminal 2: Expensive Model Server
go run expensive_model_server.go
# Output: Mock server for ExpensiveModel running on http://localhost:8082
```

---

### 2. Start the Main Gateway

Run the gateway itself, which will listen on port 8080:

```bash
# Terminal 3: LLM Routing Gateway
go run gateway.go
# Output: LLM Gateway (Prompt-Size Router & Metrics) starting on http://localhost:8080
```

---

## Testing Routing and Metrics

Once all three services are running, you can test the system using `curl`.

### A. Test Cost-Based Routing

**Short Prompt (Should use Cheap Model):**
```bash
curl -X POST http://localhost:8080/v1/chat/completions      -H "Content-Type: application/json"      -d '{"messages": [{"role": "user", "content": "What is the capital of France?"}]}'
```

**Long Prompt (Should use Expensive Model):**
```bash
curl -X POST http://localhost:8080/v1/chat/completions      -H "Content-Type: application/json"      -d '{
           "messages": [
             {"role": "user", "content": "Explain the concept of quantum entanglement and its implications for modern computing. Cover Bell's theorem and experimental verification methods in detail."}
           ]
         }'
```

---

### B. View Gateway Metrics

Check the metrics endpoint to confirm that the gateway tracked both requests:

```bash
curl http://localhost:8080/metrics
```

**Expected Result Example:**
```json
{
  "totalRequests": 2,
  "totalCostUsd": 0.000185,
  "modelUsageCount": {
    "CheapModel": 1,
    "ExpensiveModel": 1
  }
}
```

---

## Project Structure (Suggested)

```
llm-gateway-go/
├── cheap_model_server.go
├── expensive_model_server.go
├── gateway.go
├── go.mod
└── README.md
```

---

## License

This project is open-source and licensed under the [MIT License](LICENSE).
