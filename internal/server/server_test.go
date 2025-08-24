package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerInitialization(t *testing.T) {
	// Initialize the server
	srv := New()
	if srv == nil {
		t.Fatal("Failed to initialize server")
	}
	t.Log("Server initialized successfully")
}

func TestAPIHealthEndpoint(t *testing.T) {
	// Initialize the server
	srv := New()

	// Create a test HTTP server
	testServer := httptest.NewServer(srv.Router())
	defer testServer.Close()

	// Test the /api/health endpoint
	resp, err := http.Get(testServer.URL + "/api/health")
	if err != nil {
		t.Fatalf("Failed to send GET request to /api/health: %v", err)
	}
	defer resp.Body.Close()

	// Check the status code
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Validate the response body
	body := make([]byte, resp.ContentLength)
	resp.Body.Read(body)
	expectedBody := `{"Status":"ok"}`
	if string(body) != expectedBody {
		t.Fatalf("Expected response body %s, got %s", expectedBody, string(body))
	}

	t.Log("API health endpoint responded correctly")
}
