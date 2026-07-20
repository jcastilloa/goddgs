package normalize

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Value    json.RawMessage `json:"value"`
		Bytes    string          `json:"bytes"`
		BytesHex string          `json:"bytes_hex"`
		Query    string          `json:"query"`
	} `json:"input"`
	Result struct {
		Status string          `json:"status"`
		Output json.RawMessage `json:"output"`
		Error  struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"result"`
}

func TestText_MatchesFrozenFixtures(t *testing.T) {
	for _, path := range matchingFixtures(t, "../../testdata/contracts/pure/pure.normalize-text-*.json") {
		fixture := loadFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			input := decodeStringInput(t, fixture)
			want := decodeStringOutput(t, fixture)
			if got := Text(input); got != want {
				t.Fatalf("Text(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestURL_MatchesFrozenFixtures(t *testing.T) {
	for _, path := range matchingFixtures(t, "../../testdata/contracts/pure/pure.normalize-url-*.json") {
		fixture := loadFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			input := decodeStringInput(t, fixture)
			want := decodeStringOutput(t, fixture)
			if got := URL(input); got != want {
				t.Fatalf("URL(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestDate_MatchesFrozenFixtures(t *testing.T) {
	for _, path := range matchingFixtures(t, "../../testdata/contracts/pure/pure.normalize-date-*.json") {
		fixture := loadFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			value := decodeDateInput(t, fixture)
			got, err := Date(value)
			if fixture.Result.Status == "error" {
				assertDateErrorFixture(t, got, err, fixture)
				return
			}
			if err != nil {
				t.Fatalf("Date(%#v): %v", value, err)
			}
			want := decodeDateOutput(t, fixture)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Date(%#v) = %#v, want %#v", value, got, want)
			}
		})
	}
}

func TestVQD_MatchesFrozenFixtures(t *testing.T) {
	for _, path := range matchingFixtures(t, "../../testdata/contracts/pure/pure.vqd-*.json") {
		fixture := loadFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			got, err := VQD(decodeBytesInput(t, fixture), fixture.Input.Query)
			if fixture.Result.Error.Message != "" {
				if err == nil {
					t.Fatal("VQD error = nil, want source error")
				}
				if got := err.Error(); got != fixture.Result.Error.Message {
					t.Fatalf("VQD error = %q, want %q", got, fixture.Result.Error.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("VQD(...): %v", err)
			}
			want := decodeStringOutput(t, fixture)
			if got != want {
				t.Fatalf("VQD(...) = %q, want %q", got, want)
			}
		})
	}
}

func TestProxy_MatchesFrozenFixtures(t *testing.T) {
	for _, path := range matchingFixtures(t, "../../testdata/contracts/pure/pure.proxy-*.json") {
		fixture := loadFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			input := decodeOptionalStringInput(t, fixture)
			want := decodeOptionalStringOutput(t, fixture)
			got := Proxy(input)
			if !optionalStringEqual(got, want) {
				t.Fatalf("Proxy(%#v) = %#v, want %#v", input, got, want)
			}
		})
	}
}

func TestVQD_ErrorIsClassifiable(t *testing.T) {
	_, err := VQD([]byte("no token"), "probe")
	if !errors.Is(err, ErrVQD) {
		t.Fatalf("VQD error does not classify as ErrVQD: %v", err)
	}
}

func matchingFixtures(t *testing.T, pattern string) []string {
	t.Helper()

	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("find fixtures %q: %v", pattern, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no fixtures matching %q", pattern)
	}
	return paths
}

func loadFixture(t *testing.T, path string) fixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture fixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func decodeStringOutput(t *testing.T, fixture fixture) string {
	t.Helper()

	var output string
	if err := json.Unmarshal(fixture.Result.Output, &output); err != nil {
		t.Fatalf("decode %s output: %v", fixture.FixtureID, err)
	}
	return output
}

func decodeStringInput(t *testing.T, fixture fixture) string {
	t.Helper()

	var value string
	if err := json.Unmarshal(fixture.Input.Value, &value); err != nil {
		t.Fatalf("decode %s input: %v", fixture.FixtureID, err)
	}
	return value
}

func decodeOptionalStringInput(t *testing.T, fixture fixture) *string {
	t.Helper()

	if string(fixture.Input.Value) == "null" {
		return nil
	}
	value := decodeStringInput(t, fixture)
	return &value
}

func decodeOptionalStringOutput(t *testing.T, fixture fixture) *string {
	t.Helper()

	if string(fixture.Result.Output) == "null" {
		return nil
	}
	value := decodeStringOutput(t, fixture)
	return &value
}

func decodeBytesInput(t *testing.T, fixture fixture) []byte {
	t.Helper()

	if fixture.Input.BytesHex == "" {
		return []byte(fixture.Input.Bytes)
	}
	value, err := hex.DecodeString(fixture.Input.BytesHex)
	if err != nil {
		t.Fatalf("decode %s bytes_hex: %v", fixture.FixtureID, err)
	}
	return value
}

func decodeDateInput(t *testing.T, fixture fixture) any {
	t.Helper()
	return decodeDateValue(t, fixture.FixtureID, fixture.Input.Value)
}

func decodeDateOutput(t *testing.T, fixture fixture) any {
	t.Helper()
	return decodeDateValue(t, fixture.FixtureID, fixture.Result.Output)
}

func decodeDateValue(t *testing.T, fixtureID string, encoded json.RawMessage) any {
	t.Helper()

	var value any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode %s date value: %v", fixtureID, err)
	}
	if number, ok := value.(json.Number); ok {
		if integer, err := number.Int64(); err == nil {
			return integer
		}
		floatingPoint, err := number.Float64()
		if err != nil {
			t.Fatalf("decode %s numeric date value: %v", fixtureID, err)
		}
		return floatingPoint
	}
	return value
}

func optionalStringEqual(got, want *string) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func assertDateErrorFixture(t *testing.T, got any, err error, fixture fixture) {
	t.Helper()

	if got != nil {
		t.Fatalf("Date error result = %#v, want nil", got)
	}
	if err == nil {
		t.Fatalf("Date error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
	}
	if !errors.Is(err, ErrDate) {
		t.Fatalf("Date error does not classify as ErrDate: %v", err)
	}
	var sourceError *dateError
	if !errors.As(err, &sourceError) {
		t.Fatalf("Date error type = %T, want *dateError", err)
	}
	if got, want := sourceError.sourceType, fixture.Result.Error.Type; got != want {
		t.Fatalf("Date source type = %q, want %q", got, want)
	}
	if got, want := err.Error(), fixture.Result.Error.Message; got != want {
		t.Fatalf("Date error = %q, want %q", got, want)
	}
}
