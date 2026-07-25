package transport

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/net/http2"
)

const (
	frozenBrowserProfileCommit = "a12929a72429a39a0841c3d7caacb20ee17acd4d"
	frozenBrowserProfileSHA256 = "931866c179b009efb1d2813591e6f9729241d435018fe92434199e25987f1a7e"
)

//go:embed browser_profiles_source.json
var frozenBrowserProfileData []byte

// browserProfileCatalog is the private, immutable source compatibility
// catalog. It is deliberately not exported: Python DDGS exposes no browser
// impersonation option to its callers.
type browserProfileCatalog struct {
	variants         []string
	operatingSystems []string
	profiles         map[string]browserProfile
}

type browserProfileCapture struct {
	Source struct {
		Commit string `json:"commit"`
	} `json:"source"`
	Profiles []browserProfileCaptureEntry `json:"profiles"`
}

type browserProfileCaptureEntry struct {
	Profile           string                  `json:"profile"`
	OperatingSystem   string                  `json:"operating_system"`
	ClientHelloBase64 string                  `json:"client_hello_base64"`
	ClientHelloSHA256 string                  `json:"client_hello_sha256"`
	HTTP2             browserProfileCaptureH2 `json:"http2"`
}

type browserProfileCaptureH2 struct {
	Settings                  [][]uint32                     `json:"settings"`
	ConnectionWindowIncrement uint32                         `json:"connection_window_increment"`
	StreamWindowIncrement     *uint32                        `json:"stream_window_increment"`
	InitialStreamID           uint32                         `json:"initial_stream_id"`
	Priority                  *browserProfileCapturePriority `json:"priority"`
	PseudoOrder               []string                       `json:"pseudo_order"`
	Headers                   [][]string                     `json:"headers"`
}

type browserProfileCapturePriority struct {
	StreamDependency uint32 `json:"stream_dependency"`
	Exclusive        bool   `json:"exclusive"`
	Weight           uint16 `json:"weight"`
}

var (
	browserProfileCatalogOnce sync.Once
	browserProfileCatalogData *browserProfileCatalog
	browserProfileCatalogErr  error
)

func loadBrowserProfileCatalog() (*browserProfileCatalog, error) {
	browserProfileCatalogOnce.Do(func() {
		browserProfileCatalogData, browserProfileCatalogErr = decodeBrowserProfileCatalog(frozenBrowserProfileData)
	})
	if browserProfileCatalogErr != nil {
		return nil, browserProfileCatalogErr
	}
	return browserProfileCatalogData, nil
}

func (catalog *browserProfileCatalog) variantCount() int {
	return len(catalog.variants)
}

func (catalog *browserProfileCatalog) operatingSystemCount() int {
	return len(catalog.operatingSystems)
}

func (catalog *browserProfileCatalog) variantNames() []string {
	return append([]string(nil), catalog.variants...)
}

func (catalog *browserProfileCatalog) operatingSystemNames() []string {
	return append([]string(nil), catalog.operatingSystems...)
}

func (catalog *browserProfileCatalog) profile(variantIndex, operatingSystemIndex int) (browserProfile, error) {
	if variantIndex < 0 || variantIndex >= len(catalog.variants) {
		return browserProfile{}, fmt.Errorf("browser variant index %d is outside catalog", variantIndex)
	}
	if operatingSystemIndex < 0 || operatingSystemIndex >= len(catalog.operatingSystems) {
		return browserProfile{}, fmt.Errorf("browser operating-system index %d is outside catalog", operatingSystemIndex)
	}
	key := browserProfileKey(catalog.variants[variantIndex], catalog.operatingSystems[operatingSystemIndex])
	profile, ok := catalog.profiles[key]
	if !ok {
		return browserProfile{}, fmt.Errorf("browser profile %q is missing from catalog", key)
	}
	return cloneBrowserProfile(profile), nil
}

func selectSourceBrowserProfile(catalog *browserProfileCatalog, choose func(int) (int, error)) (browserProfile, error) {
	if catalog == nil || choose == nil {
		return browserProfile{}, errors.New("browser profile selector is unavailable")
	}
	// primp-python resolves impersonate_os="random" before it applies the
	// random browser impersonation to its builder. Keep both the chronology and
	// the binding's declaration order observable by deterministic tests.
	operatingSystemIndex, err := choose(catalog.operatingSystemCount())
	if err != nil {
		return browserProfile{}, fmt.Errorf("select browser operating system: %w", err)
	}
	variantIndex, err := choose(catalog.variantCount())
	if err != nil {
		return browserProfile{}, fmt.Errorf("select browser variant: %w", err)
	}
	return catalog.profile(variantIndex, operatingSystemIndex)
}

func decodeBrowserProfileCatalog(data []byte) (*browserProfileCatalog, error) {
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != frozenBrowserProfileSHA256 {
		return nil, fmt.Errorf("browser profile asset SHA-256 = %s, want %s", got, frozenBrowserProfileSHA256)
	}
	var captured browserProfileCapture
	if err := json.Unmarshal(data, &captured); err != nil {
		return nil, fmt.Errorf("decode browser profile asset: %w", err)
	}
	if captured.Source.Commit != frozenBrowserProfileCommit {
		return nil, fmt.Errorf("browser profile asset source commit = %q, want %q", captured.Source.Commit, frozenBrowserProfileCommit)
	}
	catalog := &browserProfileCatalog{profiles: make(map[string]browserProfile, len(captured.Profiles))}
	for _, entry := range captured.Profiles {
		profile, err := decodeBrowserProfile(entry)
		if err != nil {
			return nil, err
		}
		key := browserProfileKey(profile.sourceVariant, profile.sourceOperatingSystem)
		if _, duplicate := catalog.profiles[key]; duplicate {
			return nil, fmt.Errorf("browser profile asset has duplicate %q", key)
		}
		catalog.profiles[key] = profile
	}
	// Source random selection is defined over this exact declaration order, not
	// lexicographic order. Keep it explicit in one reviewable place.
	catalog.variants = []string{
		"chrome_144", "chrome_145", "chrome_146", "chrome_147", "chrome_148",
		"edge_144", "edge_145", "edge_146", "edge_147", "edge_148",
		"opera_126", "opera_127", "opera_128", "opera_129", "opera_130", "opera_131",
		"safari_26", "safari_26.3", "safari_18.5",
		"firefox_140", "firefox_146", "firefox_147", "firefox_148",
	}
	// This is IMPERSONATEOS_LIST from primp-python, not the separate core-Rust
	// helper ordering. The binding makes the source random choice from this list.
	catalog.operatingSystems = []string{"android", "ios", "linux", "macos", "windows"}
	if len(catalog.profiles) != len(catalog.variants)*len(catalog.operatingSystems) {
		return nil, fmt.Errorf("browser profile asset has %d profiles, want %d", len(catalog.profiles), len(catalog.variants)*len(catalog.operatingSystems))
	}
	for _, variant := range catalog.variants {
		for _, operatingSystem := range catalog.operatingSystems {
			if _, ok := catalog.profiles[browserProfileKey(variant, operatingSystem)]; !ok {
				return nil, fmt.Errorf("browser profile asset is missing %s/%s", variant, operatingSystem)
			}
		}
	}
	return catalog, nil
}

func decodeBrowserProfile(entry browserProfileCaptureEntry) (browserProfile, error) {
	raw, err := base64.StdEncoding.DecodeString(entry.ClientHelloBase64)
	if err != nil || len(raw) == 0 {
		return browserProfile{}, fmt.Errorf("browser profile %s/%s has invalid ClientHello", entry.Profile, entry.OperatingSystem)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != entry.ClientHelloSHA256 {
		return browserProfile{}, fmt.Errorf("browser profile %s/%s ClientHello SHA-256 mismatch", entry.Profile, entry.OperatingSystem)
	}
	profile := browserProfile{
		sourceVariant:         entry.Profile,
		sourceOperatingSystem: entry.OperatingSystem,
		clientHelloRaw:        raw,
		connectionWindow:      entry.HTTP2.ConnectionWindowIncrement,
		streamWindow:          entry.HTTP2.StreamWindowIncrement,
		initialStreamID:       entry.HTTP2.InitialStreamID,
		pseudoOrder:           append([]string(nil), entry.HTTP2.PseudoOrder...),
	}
	if profile.sourceVariant == "" || profile.sourceOperatingSystem == "" || len(profile.pseudoOrder) != 4 || profile.initialStreamID == 0 || profile.initialStreamID%2 == 0 {
		return browserProfile{}, fmt.Errorf("browser profile %s/%s has incomplete HTTP/2 identity", entry.Profile, entry.OperatingSystem)
	}
	for _, header := range entry.HTTP2.Headers {
		if len(header) != 2 || header[0] == "" {
			return browserProfile{}, fmt.Errorf("browser profile %s/%s has invalid header", entry.Profile, entry.OperatingSystem)
		}
		profile.defaultHeaders = append(profile.defaultHeaders, Field{Name: header[0], Value: header[1]})
		profile.headerOrder = append(profile.headerOrder, header[0])
	}
	for _, setting := range entry.HTTP2.Settings {
		if len(setting) != 2 {
			return browserProfile{}, fmt.Errorf("browser profile %s/%s has invalid HTTP/2 setting", entry.Profile, entry.OperatingSystem)
		}
		profile.settings = append(profile.settings, http2.Setting{ID: http2.SettingID(setting[0]), Val: setting[1]})
	}
	if entry.HTTP2.Priority != nil {
		if entry.HTTP2.Priority.Weight == 0 || entry.HTTP2.Priority.Weight > 256 {
			return browserProfile{}, fmt.Errorf("browser profile %s/%s has invalid priority weight", entry.Profile, entry.OperatingSystem)
		}
		profile.priority = &http2.PriorityParam{
			StreamDep: entry.HTTP2.Priority.StreamDependency,
			Exclusive: entry.HTTP2.Priority.Exclusive,
			Weight:    uint8(entry.HTTP2.Priority.Weight - 1),
		}
	}
	if len(profile.defaultHeaders) == 0 || len(profile.settings) == 0 {
		return browserProfile{}, fmt.Errorf("browser profile %s/%s has incomplete bundle", entry.Profile, entry.OperatingSystem)
	}
	return profile, nil
}

func browserProfileKey(variant, operatingSystem string) string {
	return variant + "/" + operatingSystem
}

func cloneBrowserProfile(profile browserProfile) browserProfile {
	clone := profile
	clone.clientHelloRaw = append([]byte(nil), profile.clientHelloRaw...)
	clone.defaultHeaders = append([]Field(nil), profile.defaultHeaders...)
	clone.headerOrder = append([]string(nil), profile.headerOrder...)
	clone.settings = append([]http2.Setting(nil), profile.settings...)
	clone.pseudoOrder = append([]string(nil), profile.pseudoOrder...)
	if profile.streamWindow != nil {
		value := *profile.streamWindow
		clone.streamWindow = &value
	}
	if profile.priority != nil {
		value := *profile.priority
		clone.priority = &value
	}
	return clone
}
