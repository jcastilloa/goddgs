package transport

import (
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"golang.org/x/net/http2"
)

const (
	frozenDuckDuckGoSessionProfileCommit = "a12929a72429a39a0841c3d7caacb20ee17acd4d"
	frozenDuckDuckGoSessionProfileSHA256 = "5111e75b502d8991f8f98e70231fb3ca169fec94342120e2d4cbd4a5a00e87df"
)

//go:embed duckduckgo_session_profiles_source.json
var frozenDuckDuckGoSessionProfileData []byte

type duckDuckGoSessionProfileCatalog struct {
	verified []browserProfile
	off      browserProfile
}

type duckDuckGoSessionProfileCapture struct {
	Source struct {
		Commit string `json:"commit"`
	} `json:"source"`
	Profiles []duckDuckGoSessionProfileCaptureEntry `json:"profiles"`
}

type duckDuckGoSessionProfileCaptureEntry struct {
	Name                string                  `json:"name"`
	VerificationEnabled bool                    `json:"verification_enabled"`
	ClientHelloBase64   string                  `json:"client_hello_base64"`
	HTTP2               browserProfileCaptureH2 `json:"http2"`
}

var (
	duckDuckGoSessionProfileCatalogOnce sync.Once
	duckDuckGoSessionProfileCatalogData *duckDuckGoSessionProfileCatalog
	duckDuckGoSessionProfileCatalogErr  error
)

func loadDuckDuckGoSessionProfileCatalog() (*duckDuckGoSessionProfileCatalog, error) {
	duckDuckGoSessionProfileCatalogOnce.Do(func() {
		duckDuckGoSessionProfileCatalogData, duckDuckGoSessionProfileCatalogErr = decodeDuckDuckGoSessionProfileCatalog(frozenDuckDuckGoSessionProfileData)
	})
	if duckDuckGoSessionProfileCatalogErr != nil {
		return nil, duckDuckGoSessionProfileCatalogErr
	}
	return duckDuckGoSessionProfileCatalogData, nil
}

func decodeDuckDuckGoSessionProfileCatalog(data []byte) (*duckDuckGoSessionProfileCatalog, error) {
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != frozenDuckDuckGoSessionProfileSHA256 {
		return nil, errors.New("DuckDuckGo session profile asset checksum mismatch")
	}
	var capture duckDuckGoSessionProfileCapture
	if err := json.Unmarshal(data, &capture); err != nil {
		return nil, fmt.Errorf("decode DuckDuckGo session profile asset: %w", err)
	}
	if capture.Source.Commit != frozenDuckDuckGoSessionProfileCommit {
		return nil, fmt.Errorf("DuckDuckGo session profile source commit = %q, want %q", capture.Source.Commit, frozenDuckDuckGoSessionProfileCommit)
	}
	if len(capture.Profiles) != 5 {
		return nil, fmt.Errorf("DuckDuckGo session profile count = %d, want 5", len(capture.Profiles))
	}

	catalog := &duckDuckGoSessionProfileCatalog{verified: make([]browserProfile, 0, 4)}
	for index, entry := range capture.Profiles {
		profile, err := decodeDuckDuckGoSessionProfile(entry)
		if err != nil {
			return nil, fmt.Errorf("DuckDuckGo session profile %d: %w", index, err)
		}
		switch entry.Name {
		case "verify_default", "verify_max_tls12", "verify_min_tls13", "verify_no_ticket":
			if !entry.VerificationEnabled {
				return nil, fmt.Errorf("DuckDuckGo session profile %q must enable verification", entry.Name)
			}
			catalog.verified = append(catalog.verified, profile)
		case "verify_off":
			if entry.VerificationEnabled {
				return nil, errors.New("DuckDuckGo verification-off profile enables verification")
			}
			catalog.off = profile
		default:
			return nil, fmt.Errorf("unexpected DuckDuckGo session profile %q", entry.Name)
		}
	}
	if len(catalog.verified) != 4 || len(catalog.off.clientHelloRaw) == 0 {
		return nil, errors.New("DuckDuckGo session profile asset is incomplete")
	}
	return catalog, nil
}

func decodeDuckDuckGoSessionProfile(entry duckDuckGoSessionProfileCaptureEntry) (browserProfile, error) {
	raw, err := base64.StdEncoding.DecodeString(entry.ClientHelloBase64)
	if err != nil || len(raw) == 0 {
		return browserProfile{}, errors.New("has invalid ClientHello")
	}
	profile, err := decodeBrowserProfileHTTP2(entry.HTTP2)
	if err != nil {
		return browserProfile{}, err
	}
	profile.sourceVariant = entry.Name
	profile.sourceOperatingSystem = "duckduckgo"
	profile.clientHelloRaw = raw
	return profile, nil
}

func decodeBrowserProfileHTTP2(capture browserProfileCaptureH2) (browserProfile, error) {
	if len(capture.Settings) != 7 {
		return browserProfile{}, fmt.Errorf("HTTP/2 setting count = %d, want 7", len(capture.Settings))
	}
	settings := make([]http2.Setting, len(capture.Settings))
	for index, setting := range capture.Settings {
		if len(setting) != 2 {
			return browserProfile{}, fmt.Errorf("HTTP/2 setting %d has %d values, want 2", index, len(setting))
		}
		settings[index] = http2.Setting{ID: http2.SettingID(setting[0]), Val: setting[1]}
	}
	if capture.ConnectionWindowIncrement != 1<<24 {
		return browserProfile{}, fmt.Errorf("connection window increment = %d, want %d", capture.ConnectionWindowIncrement, 1<<24)
	}
	if capture.InitialStreamID != 1 || capture.Priority != nil || capture.StreamWindowIncrement == nil {
		return browserProfile{}, errors.New("HTTP/2 stream profile is not source-compatible")
	}
	if *capture.StreamWindowIncrement != 1<<24 {
		return browserProfile{}, fmt.Errorf("stream window increment = %d, want %d", *capture.StreamWindowIncrement, 1<<24)
	}
	if got, want := capture.PseudoOrder, []string{"method", "authority", "scheme", "path"}; !sameStrings(got, want) {
		return browserProfile{}, fmt.Errorf("pseudo-header order = %v, want %v", got, want)
	}
	if len(capture.Headers) != 3 {
		return browserProfile{}, fmt.Errorf("header count = %d, want 3", len(capture.Headers))
	}
	defaults := make([]Field, len(capture.Headers))
	for index, header := range capture.Headers {
		if len(header) != 2 {
			return browserProfile{}, fmt.Errorf("header %d has %d values, want 2", index, len(header))
		}
		defaults[index] = Field{Name: header[0], Value: header[1]}
	}
	return browserProfile{
		defaultHeaders:   defaults,
		headerOrder:      []string{"accept", "accept-encoding", "user-agent"},
		settings:         settings,
		connectionWindow: capture.ConnectionWindowIncrement,
		streamWindow:     capture.StreamWindowIncrement,
		initialStreamID:  capture.InitialStreamID,
		pseudoOrder:      capture.PseudoOrder,
	}, nil
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

type duckDuckGoSessionProfileChooser func(int) (int, error)

func chooseDuckDuckGoSessionProfileIndex(limit int) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("DuckDuckGo TLS policy selection limit %d is invalid", limit)
	}
	choice, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, fmt.Errorf("read DuckDuckGo TLS policy randomness: %w", err)
	}
	return int(choice.Int64()), nil
}

func selectDuckDuckGoSessionProfile(settings clientSettings, chooser duckDuckGoSessionProfileChooser) (browserProfile, error) {
	return selectDuckDuckGoSessionProfileWithCipherShuffler(settings, chooser, shuffleDuckDuckGoCipherSuites)
}

type duckDuckGoCipherShuffler func([]uint16) error

func selectDuckDuckGoSessionProfileWithCipherShuffler(settings clientSettings, chooser duckDuckGoSessionProfileChooser, shuffler duckDuckGoCipherShuffler) (browserProfile, error) {
	catalog, err := loadDuckDuckGoSessionProfileCatalog()
	if err != nil {
		return browserProfile{}, err
	}
	if !settings.verify {
		return cloneBrowserProfile(catalog.off), nil
	}
	if chooser == nil {
		chooser = chooseDuckDuckGoSessionProfileIndex
	}
	index, err := chooser(len(catalog.verified))
	if err != nil {
		return browserProfile{}, err
	}
	if index < 0 || index >= len(catalog.verified) {
		return browserProfile{}, fmt.Errorf("DuckDuckGo TLS policy index %d is outside catalog", index)
	}
	return shuffleDuckDuckGoSessionCipherSuites(catalog.verified[index], shuffler)
}

func shuffleDuckDuckGoCipherSuites(suites []uint16) error {
	for index := len(suites) - 1; index > 0; index-- {
		choice, err := rand.Int(rand.Reader, big.NewInt(int64(index+1)))
		if err != nil {
			return fmt.Errorf("read DuckDuckGo TLS cipher randomness: %w", err)
		}
		suites[index], suites[choice.Int64()] = suites[choice.Int64()], suites[index]
	}
	return nil
}

func shuffleDuckDuckGoSessionCipherSuites(profile browserProfile, shuffler duckDuckGoCipherShuffler) (browserProfile, error) {
	if shuffler == nil {
		return browserProfile{}, errors.New("DuckDuckGo TLS cipher shuffler is unavailable")
	}
	variableStart, variableEnd, err := duckDuckGoVariableCipherSuiteRange(profile)
	if err != nil {
		return browserProfile{}, err
	}
	profile = cloneBrowserProfile(profile)
	suites := make([]uint16, 0, (variableEnd-variableStart)/2)
	for offset := variableStart; offset < variableEnd; offset += 2 {
		suites = append(suites, binary.BigEndian.Uint16(profile.clientHelloRaw[offset:offset+2]))
	}
	if err := shuffler(suites); err != nil {
		return browserProfile{}, err
	}
	for index, suite := range suites {
		binary.BigEndian.PutUint16(profile.clientHelloRaw[variableStart+index*2:], suite)
	}
	return profile, nil
}

func duckDuckGoVariableCipherSuiteRange(profile browserProfile) (int, int, error) {
	var fixedCount int
	switch profile.sourceVariant {
	case "verify_default", "verify_no_ticket":
		fixedCount = 9
	case "verify_max_tls12":
		fixedCount = 6
	case "verify_min_tls13":
		fixedCount = 3
	default:
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has no cipher policy", profile.sourceVariant)
	}
	if len(profile.clientHelloRaw) < 5 || profile.clientHelloRaw[0] != 22 {
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has invalid ClientHello", profile.sourceVariant)
	}
	recordLength := int(binary.BigEndian.Uint16(profile.clientHelloRaw[3:5]))
	if recordLength+5 != len(profile.clientHelloRaw) {
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has invalid ClientHello record length", profile.sourceVariant)
	}
	handshake := profile.clientHelloRaw[5:]
	if len(handshake) < 39 || handshake[0] != 1 {
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has invalid ClientHello handshake", profile.sourceVariant)
	}
	handshakeLength := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if handshakeLength+4 != len(handshake) {
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has invalid ClientHello handshake", profile.sourceVariant)
	}
	offset := 4 + 2 + 32
	if offset >= len(handshake) {
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has no session ID", profile.sourceVariant)
	}
	offset += 1 + int(handshake[offset])
	if offset+2 > len(handshake) {
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has no cipher suites", profile.sourceVariant)
	}
	cipherLength := int(binary.BigEndian.Uint16(handshake[offset : offset+2]))
	offset += 2
	if cipherLength%2 != 0 || offset+cipherLength > len(handshake) {
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has invalid cipher suites", profile.sourceVariant)
	}
	cipherCount := cipherLength / 2
	if cipherCount < fixedCount+1 {
		return 0, 0, fmt.Errorf("DuckDuckGo TLS profile %q has %d cipher suites, need at least %d", profile.sourceVariant, cipherCount, fixedCount+1)
	}
	// OpenSSL appends TLS_EMPTY_RENEGOTIATION_INFO_SCSV after applying the
	// configured cipher string. The source shuffle changes only the preceding
	// legacy suffix, never the fixed policy prefix or this final SCSV marker.
	return 5 + offset + fixedCount*2, 5 + offset + (cipherCount-1)*2, nil
}

type duckDuckGoHTTP2Random func(uint32, uint32) (uint32, error)

func randomDuckDuckGoHTTP2Value(minimum, maximum uint32) (uint32, error) {
	if maximum < minimum {
		return 0, fmt.Errorf("DuckDuckGo HTTP/2 range [%d, %d] is invalid", minimum, maximum)
	}
	span := new(big.Int).SetUint64(uint64(maximum-minimum) + 1)
	value, err := rand.Int(rand.Reader, span)
	if err != nil {
		return 0, fmt.Errorf("read DuckDuckGo HTTP/2 randomness: %w", err)
	}
	return minimum + uint32(value.Uint64()), nil
}

func newDuckDuckGoHTTP2Profile(session browserProfile, randomValue duckDuckGoHTTP2Random) (browserProfile, error) {
	if randomValue == nil {
		randomValue = randomDuckDuckGoHTTP2Value
	}
	initialWindow, err := randomValue(100, 200)
	if err != nil {
		return browserProfile{}, err
	}
	headerTable, err := randomValue(4000, 5000)
	if err != nil {
		return browserProfile{}, err
	}
	maxFrame, err := randomValue(16384, 65535)
	if err != nil {
		return browserProfile{}, err
	}
	maxConcurrent, err := randomValue(100, 200)
	if err != nil {
		return browserProfile{}, err
	}
	maxHeaderList, err := randomValue(65500, 66500)
	if err != nil {
		return browserProfile{}, err
	}
	enableConnect, err := randomValue(0, 1)
	if err != nil {
		return browserProfile{}, err
	}
	enablePush, err := randomValue(0, 1)
	if err != nil {
		return browserProfile{}, err
	}

	profile := cloneBrowserProfile(session)
	profile.settings = []http2.Setting{
		{ID: http2.SettingHeaderTableSize, Val: headerTable},
		{ID: http2.SettingEnablePush, Val: enablePush},
		{ID: http2.SettingInitialWindowSize, Val: initialWindow},
		{ID: http2.SettingMaxFrameSize, Val: maxFrame},
		{ID: http2.SettingEnableConnectProtocol, Val: enableConnect},
		{ID: http2.SettingMaxConcurrentStreams, Val: maxConcurrent},
		{ID: http2.SettingMaxHeaderListSize, Val: maxHeaderList},
	}
	profile.connectionWindow = 1 << 24
	profile.initialStreamID = 1
	profile.priority = nil
	profile.pseudoOrder = []string{"method", "authority", "scheme", "path"}
	return profile, nil
}
