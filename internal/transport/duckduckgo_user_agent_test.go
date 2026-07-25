package transport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type duckDuckGoUserAgentFixture struct {
	Input struct {
		Distribution string `json:"distribution"`
		Selection    string `json:"selection"`
		Version      string `json:"version"`
	} `json:"input"`
	Result struct {
		Output struct {
			OrderedRowsSHA256 string `json:"ordered_rows_sha256"`
			Positions         []struct {
				Index     int    `json:"index"`
				UserAgent string `json:"user_agent"`
			} `json:"positions"`
			RowCount    int `json:"row_count"`
			UniqueCount int `json:"unique_count"`
		} `json:"output"`
	} `json:"result"`
}

func TestDuckDuckGoTextUserAgentPool_MatchesFrozenSourceFixture(t *testing.T) {
	fixture := loadDuckDuckGoUserAgentFixture(t)
	pool, err := duckDuckGoTextUserAgentPool()
	if err != nil {
		t.Fatalf("load DuckDuckGo user-agent pool: %v", err)
	}

	if got, want := len(pool), fixture.Result.Output.RowCount; got != want {
		t.Fatalf("pool rows = %d, want %d", got, want)
	}
	if got, want := uniqueStringCount(pool), fixture.Result.Output.UniqueCount; got != want {
		t.Fatalf("pool unique rows = %d, want %d", got, want)
	}
	encoded := []byte(strings.Join(pool, "\n") + "\n")
	sum := sha256.Sum256(encoded)
	if got := hex.EncodeToString(sum[:]); got != fixture.Result.Output.OrderedRowsSHA256 {
		t.Fatalf("pool SHA-256 = %s, want %s", got, fixture.Result.Output.OrderedRowsSHA256)
	}
	for _, position := range fixture.Result.Output.Positions {
		if got := pool[position.Index]; got != position.UserAgent {
			t.Fatalf("pool[%d] = %q, want %q", position.Index, got, position.UserAgent)
		}
	}
}

func TestDuckDuckGoTextUserAgentPicker_UsesFrozenOrderedPool(t *testing.T) {
	fixture := loadDuckDuckGoUserAgentFixture(t)
	pool, err := duckDuckGoTextUserAgentPool()
	if err != nil {
		t.Fatalf("load DuckDuckGo user-agent pool: %v", err)
	}

	for _, position := range fixture.Result.Output.Positions {
		position := position
		t.Run("index-"+strconv.Itoa(position.Index), func(t *testing.T) {
			picker := newDuckDuckGoTextUserAgentPicker(pool, func(limit int) (int, error) {
				if limit != fixture.Result.Output.RowCount {
					t.Fatalf("picker limit = %d, want %d", limit, fixture.Result.Output.RowCount)
				}
				return position.Index, nil
			})
			got, err := picker()
			if err != nil {
				t.Fatalf("pick user agent: %v", err)
			}
			if got != position.UserAgent {
				t.Fatalf("picked user agent = %q, want %q", got, position.UserAgent)
			}
		})
	}
}

func TestDuckDuckGoTextUserAgent_IsProcessLifetimeAndRaceSafe(t *testing.T) {
	const callers = 32
	pool, err := duckDuckGoTextUserAgentPool()
	if err != nil {
		t.Fatalf("load DuckDuckGo user-agent pool: %v", err)
	}
	values := make(chan string, callers)
	errors := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := DuckDuckGoTextUserAgent()
			if err != nil {
				errors <- err
				return
			}
			values <- value
		}()
	}
	group.Wait()
	close(values)
	close(errors)
	for err := range errors {
		t.Fatalf("select DuckDuckGo user agent: %v", err)
	}

	var selected string
	for value := range values {
		if selected == "" {
			selected = value
		}
		if value != selected {
			t.Fatalf("process-lifetime user agent = %q, want %q", value, selected)
		}
	}
	if !stringSliceContains(pool, selected) {
		t.Fatalf("selected user agent %q is not in frozen source pool", selected)
	}
}

func loadDuckDuckGoUserAgentFixture(t testing.TB) duckDuckGoUserAgentFixture {
	t.Helper()
	contents, err := os.ReadFile("../../testdata/contracts/pure/pure.duckduckgo-text-user-agent-pool.json")
	if err != nil {
		t.Fatalf("read user-agent fixture: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var fixture duckDuckGoUserAgentFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode user-agent fixture: %v", err)
	}
	return fixture
}

func uniqueStringCount(values []string) int {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[value] = struct{}{}
	}
	return len(unique)
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
