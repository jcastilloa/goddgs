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

// TestBrowserProfileFamilies_LiveFingerprintObservation observes one explicit
// frozen pair from each primp browser family. It has no default endpoint and
// never contacts a search engine.
func TestBrowserProfileFamilies_LiveFingerprintObservation(t *testing.T) {
	if os.Getenv(fingerprintEndpointEnvironment) == "" {
		t.Skipf("set %s to run the controlled live fingerprint observation", fingerprintEndpointEnvironment)
	}
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load browser profile catalog: %v", err)
	}

	profiles := []struct {
		variant         string
		operatingSystem string
	}{
		{variant: "chrome_146", operatingSystem: "windows"},
		{variant: "edge_148", operatingSystem: "linux"},
		{variant: "opera_131", operatingSystem: "android"},
		{variant: "safari_26.3", operatingSystem: "macos"},
		{variant: "firefox_148", operatingSystem: "ios"},
	}
	for _, profile := range profiles {
		t.Run(profile.variant+"/"+profile.operatingSystem, func(t *testing.T) {
			variantIndex := indexOfBrowserProfilePart(t, catalog.variants, profile.variant)
			operatingSystemIndex := indexOfBrowserProfilePart(t, catalog.operatingSystems, profile.operatingSystem)
			client, err := newClientWithBehaviorAndBrowserProfileChooser(
				Config{Timeout: WithTimeout(15 * time.Second)},
				nil,
				clientBehavior{followRedirects: true},
				fixedBrowserProfileChooser(operatingSystemIndex, variantIndex),
			)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			if client.browserProfile == nil {
				t.Fatal("browser profile = nil")
			}
			if got, want := client.browserProfile.sourceVariant, profile.variant; got != want {
				t.Fatalf("browser variant = %q, want %q", got, want)
			}
			if got, want := client.browserProfile.sourceOperatingSystem, profile.operatingSystem; got != want {
				t.Fatalf("operating system = %q, want %q", got, want)
			}
			defer client.CloseIdleConnections()
			observeLiveFingerprint(t, client)
		})
	}
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
