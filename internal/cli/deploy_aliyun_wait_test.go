package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAliyunProbeURLs(t *testing.T) {
	urls := aliyunProbeURLs(map[string]string{
		"ServiceURL": "https://203.0.113.10/",
	})
	if len(urls) != 2 || urls[0] != "https://203.0.113.10/health" {
		t.Fatalf("unexpected urls: %#v", urls)
	}

	urls = aliyunProbeURLs(map[string]string{"PublicIP": "203.0.113.11"})
	if urls[0] != "https://203.0.113.11/health" {
		t.Fatalf("unexpected public ip urls: %#v", urls)
	}
}

func TestAliyunServiceReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	if !aliyunServiceReady(context.Background(), server.URL+"/health") {
		t.Fatal("expected /health to be ready")
	}
	if aliyunServiceReady(context.Background(), server.URL+"/missing") {
		t.Fatal("expected missing path to be not ready")
	}
}

func TestWaitAliyunServiceReady(t *testing.T) {
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

	oldInterval := aliyunBootstrapPollInterval
	aliyunBootstrapPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { aliyunBootstrapPollInterval = oldInterval })

	var stdout bytes.Buffer
	err := waitAliyunServiceReady(
		context.Background(),
		&stdout,
		map[string]string{"ServiceURL": server.URL},
		time.Now().Add(5*time.Second),
		true,
	)
	if err != nil {
		t.Fatalf("waitAliyunServiceReady: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Service is ready.")) {
		t.Fatalf("expected ready message, got: %s", stdout.String())
	}
}

func TestWaitAliyunServiceReadyTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not yet", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	oldInterval := aliyunBootstrapPollInterval
	aliyunBootstrapPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { aliyunBootstrapPollInterval = oldInterval })

	err := waitAliyunServiceReady(
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

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
