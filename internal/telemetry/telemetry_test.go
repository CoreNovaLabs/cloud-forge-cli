package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestTrackPostsAnonymousEvent(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "1")

	events := make(chan Event, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var event Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatal(err)
		}
		events <- event
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewClient(Config{
		CacheDir:   t.TempDir(),
		Endpoint:   server.URL + "/v1/events",
		CLIVersion: "0.1.0",
	})

	client.Track(context.Background(), Event{
		Event:      "template_fetch",
		AppID:      "hello-nginx",
		AppVersion: "1.0.0",
		Cloud:      "aws",
		Status:     "success",
	})

	select {
	case event := <-events:
		if event.AnonymousID == "" {
			t.Fatal("expected anonymous id")
		}
		if event.CLIVersion != "0.1.0" {
			t.Fatalf("unexpected cli version %q", event.CLIVersion)
		}
		if event.OS == "" || event.Arch == "" {
			t.Fatal("expected os and arch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for telemetry event")
	}
}

func TestTrackCanBeDisabled(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "0")

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := NewClient(Config{
		CacheDir:   t.TempDir(),
		Endpoint:   server.URL,
		CLIVersion: "0.1.0",
	})
	client.Track(context.Background(), Event{Event: "search"})
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Fatal("telemetry endpoint should not be called when disabled")
	}
}

func TestAnonymousIDIsStable(t *testing.T) {
	t.Setenv("CLOUD_FORGE_TELEMETRY", "1")

	dir := t.TempDir()
	first, err := loadOrCreateAnonymousID(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateAnonymousID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("expected stable id, got %q and %q", first, second)
	}

	if _, err := os.Stat(dir + "/telemetry_id"); err != nil {
		t.Fatal(err)
	}
}
