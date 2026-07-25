//go:build integration

package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

const fingerprintEndpointEnvironment = "GODDGS_FINGERPRINT_ENDPOINT"

type fingerprintObservation struct {
	HTTPVersion string `json:"http_version"`
	TLS         struct {
		JA3Hash string `json:"ja3_hash"`
		JA4     string `json:"ja4"`
	} `json:"tls"`
	HTTP2 *fingerprintHTTP2Observation `json:"http2"`
}

type fingerprintHTTP2Observation struct {
	AkamaiFingerprintHash string `json:"akamai_fingerprint_hash"`
}

func TestClient_LiveFingerprintObservation(t *testing.T) {
	client, err := NewClient(Config{Timeout: WithTimeout(15 * time.Second)})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	observeLiveFingerprint(t, client)
}

func TestDuckDuckGoTextClient_LiveFingerprintObservation(t *testing.T) {
	client, err := NewDuckDuckGoTextClient(Config{Timeout: WithTimeout(15 * time.Second)}, nil)
	if err != nil {
		t.Fatalf("NewDuckDuckGoTextClient: %v", err)
	}
	observeLiveFingerprint(t, client)
}

type fingerprintRequester interface {
	Do(context.Context, Request) (Response, error)
}

func observeLiveFingerprint(t *testing.T, client fingerprintRequester) {
	t.Helper()
	endpoint := os.Getenv(fingerprintEndpointEnvironment)
	if endpoint == "" {
		t.Skipf("set %s to run the controlled live fingerprint observation", fingerprintEndpointEnvironment)
	}

	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: endpoint})
	if err != nil {
		t.Fatalf("Do %q: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("fingerprint status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	var observation fingerprintObservation
	if err := json.Unmarshal(response.Content, &observation); err != nil {
		t.Fatalf("decode fingerprint response: %v", err)
	}
	if observation.HTTPVersion == "" || observation.TLS.JA3Hash == "" || observation.TLS.JA4 == "" {
		t.Fatalf("incomplete sanitized fingerprint observation: %#v", observation)
	}
	if observation.HTTPVersion == "h2" && observation.HTTP2 == nil {
		t.Fatalf("HTTP/2 observation omits HTTP/2 fingerprint: %#v", observation)
	}

	t.Logf("fingerprint observation: http=%s tls_ja3=%s tls_ja4=%s h2=%t h2_akamai=%s", observation.HTTPVersion, observation.TLS.JA3Hash, observation.TLS.JA4, observation.HTTP2 != nil, observation.http2Fingerprint())
}

func (observation fingerprintObservation) http2Fingerprint() string {
	if observation.HTTP2 == nil {
		return ""
	}
	return observation.HTTP2.AkamaiFingerprintHash
}
