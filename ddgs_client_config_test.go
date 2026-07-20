package ddgs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type clientConstructorFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Arguments        map[string]json.RawMessage `json:"arguments"`
		EnvironmentProxy *string                    `json:"environment_proxy"`
	} `json:"input"`
	Result struct {
		Output struct {
			Proxy   *string         `json:"proxy"`
			Timeout *int64          `json:"timeout"`
			Verify  json.RawMessage `json:"verify"`
		} `json:"output"`
	} `json:"result"`
}

func TestNew_MatchesFrozenClientConstructorFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/contracts/pure/pure.client-*.json")
	if err != nil {
		t.Fatalf("find client fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no client constructor fixtures")
	}

	for _, path := range paths {
		fixture := loadClientConstructorFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			setOptionalEnvironment(t, "DDGS_PROXY", fixture.Input.EnvironmentProxy)

			client := New(clientFixtureOptions(t, fixture.Input.Arguments)...)
			assertClientFixtureConfig(t, client, fixture)
		})
	}
}

func loadClientConstructorFixture(t *testing.T, path string) clientConstructorFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var fixture clientConstructorFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func setOptionalEnvironment(t *testing.T, key string, value *string) {
	t.Helper()

	previous, wasSet := os.LookupEnv(key)
	if value == nil {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	} else if err := os.Setenv(key, *value); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func clientFixtureOptions(t *testing.T, arguments map[string]json.RawMessage) []Option {
	t.Helper()

	options := make([]Option, 0, len(arguments))
	if rawProxy, ok := arguments["proxy"]; ok {
		var proxy string
		if err := json.Unmarshal(rawProxy, &proxy); err != nil {
			t.Fatalf("decode proxy option: %v", err)
		}
		options = append(options, WithProxy(proxy))
	}
	if rawTimeout, ok := arguments["timeout"]; ok {
		if string(rawTimeout) == "null" {
			options = append(options, WithoutTimeout())
		} else {
			var seconds int64
			if err := json.Unmarshal(rawTimeout, &seconds); err != nil {
				t.Fatalf("decode timeout option: %v", err)
			}
			options = append(options, WithTimeout(time.Duration(seconds)*time.Second))
		}
	}
	if rawVerify, ok := arguments["verify"]; ok {
		var enabled bool
		if err := json.Unmarshal(rawVerify, &enabled); err == nil {
			options = append(options, WithTLSVerification(enabled))
		} else {
			var pemPath string
			if err := json.Unmarshal(rawVerify, &pemPath); err != nil {
				t.Fatalf("decode verify option: %v", err)
			}
			options = append(options, WithTLSRootCAFile(pemPath))
		}
	}
	return options
}

func assertClientFixtureConfig(t *testing.T, client *DDGS, fixture clientConstructorFixture) {
	t.Helper()

	if got, want := client.config.proxy.set, fixture.Result.Output.Proxy != nil; got != want {
		t.Fatalf("proxy presence = %t, want %t", got, want)
	}
	if fixture.Result.Output.Proxy != nil && client.config.proxy.value != *fixture.Result.Output.Proxy {
		t.Fatalf("proxy = %q, want %q", client.config.proxy.value, *fixture.Result.Output.Proxy)
	}

	if fixture.Result.Output.Timeout == nil {
		if client.config.timeout != nil {
			t.Fatalf("timeout = %s, want nil", *client.config.timeout)
		}
	} else {
		want := time.Duration(*fixture.Result.Output.Timeout) * time.Second
		if client.config.timeout == nil || *client.config.timeout != want {
			if client.config.timeout == nil {
				t.Fatalf("timeout = nil, want %s", want)
			}
			t.Fatalf("timeout = %s, want %s", *client.config.timeout, want)
		}
	}

	var wantVerify any
	if err := json.Unmarshal(fixture.Result.Output.Verify, &wantVerify); err != nil {
		t.Fatalf("decode expected verify: %v", err)
	}
	if got := client.config.verification.sourceValue(); !reflect.DeepEqual(got, wantVerify) {
		t.Fatalf("verify = %#v, want %#v", got, wantVerify)
	}
}
