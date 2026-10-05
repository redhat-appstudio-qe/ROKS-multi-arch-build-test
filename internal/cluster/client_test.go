package cluster

import (
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestNewClientSetFromConfigBoundsDefaultHTTPRequests(t *testing.T) {
	config := &rest.Config{Host: "https://api.example"}
	clients, err := NewClientSetFromConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if clients.Config.Timeout != 20*time.Second {
		t.Fatalf("timeout = %s, want 20s", clients.Config.Timeout)
	}
	if config.Timeout != 0 {
		t.Fatalf("input config was mutated: timeout = %s", config.Timeout)
	}
}
