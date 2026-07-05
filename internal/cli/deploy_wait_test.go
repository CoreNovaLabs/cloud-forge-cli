package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeURLs(t *testing.T) {
	urls := probeURLs(map[string]string{
		"ServiceURL": "https://203.0.113.10/",
	})
	if len(urls) != 2 || urls[0] != "https://203.0.113.10/health" {
		t.Fatalf("unexpected urls: %#v", urls)
	}

	urls = probeURLs(map[string]string{"PublicIP": "203.0.113.11"})
	if urls[0] != "https://203.0.113.11/health" {
		t.Fatalf("unexpected public ip urls: %#v", urls)
	}

	if got := probeURLs(map[string]string{}); got != nil {
		t.Fatalf("expected nil urls for empty outputs, got: %#v", got)
	}
}

func TestProbeURLsIPServiceURL(t *testing.T) {
	urls := probeURLs(map[string]string{
		"ServiceURL": "https://203.0.113.10",
		"PublicIP":   "203.0.113.10",
	})
	if len(urls) != 2 {
		t.Fatalf("unexpected url count: %#v", urls)
	}
	if urls[0] != "https://203.0.113.10/health" || urls[1] != "https://203.0.113.10/" {
		t.Fatalf("unexpected urls: %#v", urls)
	}
}

func TestServiceReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	if !serviceReady(context.Background(), server.URL+"/health") {
		t.Fatal("expected /health to be ready")
	}
	if serviceReady(context.Background(), server.URL+"/missing") {
		t.Fatal("expected missing path to be not ready")
	}
}

func TestServiceHTTPClientForURL(t *testing.T) {
	if serviceHTTPClientForURL("https://203.0.113.10/health") != insecureIPServiceHTTPClient {
		t.Fatal("expected IP HTTPS endpoint to use insecure IP client")
	}
	if serviceHTTPClientForURL("https://git.example.com/health") != serviceHTTPClient {
		t.Fatal("expected DNS endpoint to use normal TLS client")
	}
}

func TestWaitServiceReady(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			http.Error(w, "not yet", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	oldInterval := bootstrapPollInterval
	bootstrapPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { bootstrapPollInterval = oldInterval })

	var stdout bytes.Buffer
	err := waitServiceReady(
		context.Background(),
		&stdout,
		map[string]string{
			"ServiceURL": server.URL,
			"PublicIP":   "203.0.113.10",
		},
		time.Now().Add(5*time.Second),
		true,
	)
	if err != nil {
		t.Fatalf("waitServiceReady: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Service is ready.")) {
		t.Fatalf("expected ready message, got: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Public IP:      203.0.113.10")) {
		t.Fatalf("expected public IP hint, got: %s", stdout.String())
	}
}

func TestWaitServiceReadyTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not yet", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	oldInterval := bootstrapPollInterval
	bootstrapPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { bootstrapPollInterval = oldInterval })

	err := waitServiceReady(
		context.Background(),
		ioDiscard{},
		map[string]string{"ServiceURL": server.URL},
		time.Now().Add(50*time.Millisecond),
		false,
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitServiceReadyMissingOutputs(t *testing.T) {
	err := waitServiceReady(
		context.Background(),
		ioDiscard{},
		map[string]string{},
		time.Now().Add(time.Second),
		false,
	)
	if err == nil {
		t.Fatal("expected error when outputs are missing")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
