package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type FallbackResponse struct {
	Message string `json:"message"`
	Source  string `json:"source"`
	Status  string `json:"status"`
}

func sendFallback(w http.ResponseWriter) {
	fallback := FallbackResponse{
		Message: "Catalog Service is currently degraded. Showing cached highlights.",
		Source:  "api-gateway-cache",
		Status:  "degraded",
	}
	w.WriteHeader(http.StatusOK) // We still return 200 OK because we handled it gracefully!
	json.NewEncoder(w).Encode(fallback)
}

func main() {
	// Configure an HTTP client with a strict 500ms timeout
	// This is our Circuit Breaker / Timeout pattern
	client := &http.Client{
		Timeout: 500 * time.Millisecond,
	}

	http.HandleFunc("/api/catalog", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		// Attempt to fetch from the actual Catalog Service (.NET)
		resp, err := client.Get("http://catalog-service:80/catalog")
		
		if err != nil {
			log.Printf("Catalog Service unavailable (Error: %v), triggering fallback.\n", err)
			sendFallback(w)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			log.Printf("Catalog Service returned status %d, triggering fallback.\n", resp.StatusCode)
			sendFallback(w)
			return
		}

		// Success: Proxy the response
		body, _ := ioutil.ReadAll(resp.Body)
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})

	fmt.Println("API Gateway running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
