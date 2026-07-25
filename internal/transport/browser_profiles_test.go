package transport

import (
	"bytes"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	utls "github.com/sardanioss/utls"
)

func TestBrowserProfileCatalog_CoversEveryFrozenSourceOutcome(t *testing.T) {
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	if got, want := catalog.variantCount(), 23; got != want {
		t.Fatalf("browser variant count = %d, want %d", got, want)
	}
	if got, want := catalog.operatingSystemCount(), 5; got != want {
		t.Fatalf("operating-system count = %d, want %d", got, want)
	}

	expectedVariants := []string{
		"chrome_144", "chrome_145", "chrome_146", "chrome_147", "chrome_148",
		"edge_144", "edge_145", "edge_146", "edge_147", "edge_148",
		"opera_126", "opera_127", "opera_128", "opera_129", "opera_130", "opera_131",
		"safari_26", "safari_26.3", "safari_18.5",
		"firefox_140", "firefox_146", "firefox_147", "firefox_148",
	}
	// primp-python selects from IMPERSONATEOS_LIST before it applies the
	// random browser impersonation: Android, iOS, Linux, macOS, Windows.
	expectedOperatingSystems := []string{"android", "ios", "linux", "macos", "windows"}
	if got := catalog.variantNames(); !reflect.DeepEqual(got, expectedVariants) {
		t.Fatalf("browser variants = %#v, want %#v", got, expectedVariants)
	}
	if got := catalog.operatingSystemNames(); !reflect.DeepEqual(got, expectedOperatingSystems) {
		t.Fatalf("operating systems = %#v, want %#v", got, expectedOperatingSystems)
	}

	seen := make(map[string]struct{}, len(expectedVariants)*len(expectedOperatingSystems))
	for variantIndex, variant := range expectedVariants {
		for operatingSystemIndex, operatingSystem := range expectedOperatingSystems {
			profile, err := catalog.profile(variantIndex, operatingSystemIndex)
			if err != nil {
				t.Fatalf("profile(%d, %d): %v", variantIndex, operatingSystemIndex, err)
			}
			if profile.sourceVariant != variant || profile.sourceOperatingSystem != operatingSystem {
				t.Fatalf("profile(%d, %d) = %s/%s, want %s/%s", variantIndex, operatingSystemIndex, profile.sourceVariant, profile.sourceOperatingSystem, variant, operatingSystem)
			}
			if len(profile.clientHelloRaw) == 0 {
				t.Fatalf("%s/%s has no captured ClientHello", variant, operatingSystem)
			}
			if len(profile.defaultHeaders) == 0 || len(profile.settings) == 0 || len(profile.pseudoOrder) != 4 {
				t.Fatalf("%s/%s has incomplete browser bundle", variant, operatingSystem)
			}
			seen[variant+"/"+operatingSystem] = struct{}{}
		}
	}
	if got, want := len(seen), 115; got != want {
		t.Fatalf("unique profile outcomes = %d, want %d", got, want)
	}
}

func TestSelectSourceBrowserProfile_UsesIndependentUniformOutcomeIndexes(t *testing.T) {
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	// The Python binding resolves impersonate_os="random" before it applies
	// impersonate="random" to the primp builder.
	choices := []int{4, 22}
	choiceIndex := 0
	profile, err := selectSourceBrowserProfile(catalog, func(limit int) (int, error) {
		if choiceIndex >= len(choices) {
			return 0, fmt.Errorf("unexpected draw for limit %d", limit)
		}
		got := choices[choiceIndex]
		choiceIndex++
		if choiceIndex == 1 && limit != 5 {
			return 0, fmt.Errorf("operating-system limit = %d, want 5", limit)
		}
		if choiceIndex == 2 && limit != 23 {
			return 0, fmt.Errorf("variant limit = %d, want 23", limit)
		}
		return got, nil
	})
	if err != nil {
		t.Fatalf("select profile: %v", err)
	}
	if choiceIndex != 2 {
		t.Fatalf("draw count = %d, want two independent draws", choiceIndex)
	}
	if got, want := profile.sourceVariant, "firefox_148"; got != want {
		t.Fatalf("variant = %q, want %q", got, want)
	}
	if got, want := profile.sourceOperatingSystem, "windows"; got != want {
		t.Fatalf("operating system = %q, want %q", got, want)
	}
}

func TestSelectSourceBrowserProfile_RejectsUnavailableOrInvalidSourceDraws(t *testing.T) {
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	tests := []struct {
		name    string
		catalog *browserProfileCatalog
		choose  func(int) (int, error)
		want    string
	}{
		{
			name:    "missing catalog",
			catalog: nil,
			choose:  fixedBrowserProfileChooser(0, 0),
			want:    "browser profile selector is unavailable",
		},
		{
			name:    "missing chooser",
			catalog: catalog,
			want:    "browser profile selector is unavailable",
		},
		{
			name:    "operating system draw error",
			catalog: catalog,
			choose:  func(int) (int, error) { return 0, errors.New("entropy unavailable") },
			want:    "select browser operating system: entropy unavailable",
		},
		{
			name:    "browser variant draw error",
			catalog: catalog,
			choose:  fixedBrowserProfileChooser(0),
			want:    "select browser variant: unexpected profile selection with limit 23",
		},
		{
			name:    "operating system index outside source list",
			catalog: catalog,
			choose:  fixedBrowserProfileChooser(5, 0),
			want:    "browser operating-system index 5 is outside catalog",
		},
		{
			name:    "browser variant index outside source list",
			catalog: catalog,
			choose:  fixedBrowserProfileChooser(0, 23),
			want:    "browser variant index 23 is outside catalog",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := selectSourceBrowserProfile(test.catalog, test.choose)
			if err == nil || err.Error() != test.want {
				t.Fatalf("select source profile error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBrowserProfileCatalog_ReturnsIndependentImmutableBundles(t *testing.T) {
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	first, err := catalog.profile(2, 0)
	if err != nil {
		t.Fatalf("first profile: %v", err)
	}
	first.defaultHeaders[0].Value = "mutated"
	first.settings[0].Val = 1
	first.clientHelloRaw[0] ^= 0xFF

	second, err := catalog.profile(2, 0)
	if err != nil {
		t.Fatalf("second profile: %v", err)
	}
	if second.defaultHeaders[0].Value == "mutated" || second.settings[0].Val == 1 || first.clientHelloRaw[0] == second.clientHelloRaw[0] {
		t.Fatal("profile returned mutable shared compatibility data")
	}
}

func TestNewClient_SelectsOneFrozenBundleAtConstruction(t *testing.T) {
	choices := []int{4, 22}
	choiceIndex := 0
	client, err := newClientWithBehaviorAndBrowserProfileChooser(Config{}, nil, clientBehavior{followRedirects: true}, func(limit int) (int, error) {
		if choiceIndex >= len(choices) {
			return 0, fmt.Errorf("unexpected selection draw for limit %d", limit)
		}
		choice := choices[choiceIndex]
		choiceIndex++
		return choice, nil
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if got, want := choiceIndex, 2; got != want {
		t.Fatalf("selection draws = %d, want %d", got, want)
	}
	if client.browserProfile == nil {
		t.Fatal("client browser profile = nil")
	}
	if got, want := client.browserProfile.sourceVariant, "firefox_148"; got != want {
		t.Fatalf("selected browser variant = %q, want %q", got, want)
	}
	if got, want := client.browserProfile.sourceOperatingSystem, "windows"; got != want {
		t.Fatalf("selected operating system = %q, want %q", got, want)
	}
	roundTripper, err := client.newRoundTripper()
	if err != nil {
		t.Fatalf("new round tripper: %v", err)
	}
	browser, ok := roundTripper.(*browserRoundTripper)
	if !ok {
		t.Fatalf("round tripper = %T, want browserRoundTripper", roundTripper)
	}
	if got, want := browser.config.profile.sourceVariant, client.browserProfile.sourceVariant; got != want {
		t.Fatalf("round tripper browser variant = %q, want client %q", got, want)
	}
	if got, want := browser.config.profile.sourceOperatingSystem, client.browserProfile.sourceOperatingSystem; got != want {
		t.Fatalf("round tripper operating system = %q, want client %q", got, want)
	}
	if got, want := choiceIndex, 2; got != want {
		t.Fatalf("selection draws after round-tripper construction = %d, want %d", got, want)
	}
}

func TestNewClient_KeepsBrowserProfilesIsolatedBetweenClients(t *testing.T) {
	first, err := newClientWithBehaviorAndBrowserProfileChooser(
		Config{}, nil, clientBehavior{followRedirects: true}, fixedBrowserProfileChooser(1, 22),
	)
	if err != nil {
		t.Fatalf("new first client: %v", err)
	}
	second, err := newClientWithBehaviorAndBrowserProfileChooser(
		Config{}, nil, clientBehavior{followRedirects: true}, fixedBrowserProfileChooser(4, 2),
	)
	if err != nil {
		t.Fatalf("new second client: %v", err)
	}
	if first.browserProfile == nil || second.browserProfile == nil {
		t.Fatal("browser profile = nil")
	}
	if first.browserProfile == second.browserProfile {
		t.Fatal("clients share mutable browser profile ownership")
	}
	if got, want := first.browserProfile.sourceVariant+"/"+first.browserProfile.sourceOperatingSystem, "firefox_148/ios"; got != want {
		t.Fatalf("first profile = %q, want %q", got, want)
	}
	if got, want := second.browserProfile.sourceVariant+"/"+second.browserProfile.sourceOperatingSystem, "chrome_146/windows"; got != want {
		t.Fatalf("second profile = %q, want %q", got, want)
	}

	firstRoundTripper, err := first.newRoundTripper()
	if err != nil {
		t.Fatalf("new first round tripper: %v", err)
	}
	secondRoundTripper, err := second.newRoundTripper()
	if err != nil {
		t.Fatalf("new second round tripper: %v", err)
	}
	firstBrowser := firstRoundTripper.(*browserRoundTripper)
	secondBrowser := secondRoundTripper.(*browserRoundTripper)
	if got, want := firstBrowser.config.profile.sourceVariant, "firefox_148"; got != want {
		t.Fatalf("first round tripper variant = %q, want %q", got, want)
	}
	if got, want := secondBrowser.config.profile.sourceVariant, "chrome_146"; got != want {
		t.Fatalf("second round tripper variant = %q, want %q", got, want)
	}
}

func TestNewClient_DirectTLSOverridesRetainOneFrozenBundle(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("test TLS server certificate = nil")
	}
	pemPath := filepath.Join(t.TempDir(), "test-root.pem")
	if err := os.WriteFile(pemPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o600); err != nil {
		t.Fatalf("write test PEM: %v", err)
	}

	tests := []struct {
		name   string
		config Config
	}{
		{
			name:   "disabled verification",
			config: Config{Verification: SkipCertificateVerification()},
		},
		{
			name:   "custom root",
			config: Config{Verification: VerifyWithPEMFile(pemPath)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := newClientWithBehaviorAndBrowserProfileChooser(test.config, nil, clientBehavior{followRedirects: true}, fixedBrowserProfileChooser(4, 2))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			if client.browserProfile == nil {
				t.Fatal("direct TLS override did not select a browser bundle")
			}
			roundTripper, err := client.newRoundTripper()
			if err != nil {
				t.Fatalf("new round tripper: %v", err)
			}
			if _, ok := roundTripper.(*browserRoundTripper); !ok {
				t.Fatalf("round tripper = %T, want browserRoundTripper", roundTripper)
			}
		})
	}
}

func TestBrowserProfile_TLSClientHelloKeepsIdentityAndRefreshesEntropy(t *testing.T) {
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load browser catalog: %v", err)
	}

	for variantIndex, variant := range catalog.variants {
		for operatingSystemIndex, operatingSystem := range catalog.operatingSystems {
			profile, err := catalog.profile(variantIndex, operatingSystemIndex)
			if err != nil {
				t.Fatalf("catalog profile %s/%s: %v", variant, operatingSystem, err)
			}
			t.Run(variant+"/"+operatingSystem, func(t *testing.T) {
				first := buildBrowserClientHello(t, profile)
				second := buildBrowserClientHello(t, profile)

				if got, want := first.ServerName, "profile.goddgs.test"; got != want {
					t.Fatalf("first SNI = %q, want request host %q", got, want)
				}
				if got, want := second.ServerName, "profile.goddgs.test"; got != want {
					t.Fatalf("second SNI = %q, want request host %q", got, want)
				}
				if !reflect.DeepEqual(first.AlpnProtocols, second.AlpnProtocols) {
					t.Fatalf("ALPN differs between connections: %#v != %#v", first.AlpnProtocols, second.AlpnProtocols)
				}
				if got, want := normalizeGREASEValues(first.CipherSuites), normalizeGREASEValues(second.CipherSuites); !reflect.DeepEqual(got, want) {
					t.Fatalf("normalized cipher suites differ: %#v != %#v", got, want)
				}
				if got, want := normalizeGREASEValues(first.SupportedVersions), normalizeGREASEValues(second.SupportedVersions); !reflect.DeepEqual(got, want) {
					t.Fatalf("normalized supported versions differ: %#v != %#v", got, want)
				}
				if bytes.Equal(first.Random, second.Random) {
					t.Fatal("TLS random was reused between independent connections")
				}
				if bytes.Equal(first.Raw, second.Raw) {
					t.Fatal("ClientHello bytes were reused between independent connections")
				}
			})
		}
	}
}

func TestBrowserProfile_TLSJA3SemanticsMatchFrozenSource(t *testing.T) {
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load browser catalog: %v", err)
	}

	for variantIndex, variant := range catalog.variants {
		for operatingSystemIndex, operatingSystem := range catalog.operatingSystems {
			profile, err := catalog.profile(variantIndex, operatingSystemIndex)
			if err != nil {
				t.Fatalf("catalog profile %s/%s: %v", variant, operatingSystem, err)
			}
			t.Run(variant+"/"+operatingSystem, func(t *testing.T) {
				generated := buildBrowserClientHello(t, profile)
				if got, want := ja3Signature(t, generated.Raw), ja3Signature(t, profile.clientHelloRaw); got != want {
					t.Fatalf("generated JA3 semantics = %q, want frozen source %q", got, want)
				}
			})
		}
	}
}

func buildBrowserClientHello(t testing.TB, profile browserProfile) *utls.PubClientHelloMsg {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = peer.Close()
	})
	connection, err := newBrowserTLSConnection(client, &utls.Config{
		ServerName: "profile.goddgs.test",
		NextProtos: []string{"h2", "http/1.1"},
	}, profile)
	if err != nil {
		t.Fatalf("new browser TLS connection: %v", err)
	}
	if err := connection.BuildHandshakeStateWithoutSession(); err != nil {
		t.Fatalf("build browser ClientHello: %v", err)
	}
	return connection.HandshakeState.Hello
}

func normalizeGREASEValues(values []uint16) []uint16 {
	normalized := make([]uint16, len(values))
	for index, value := range values {
		if value&0x0F0F == 0x0A0A && value>>8 == value&0xFF {
			normalized[index] = 0x0A0A
			continue
		}
		normalized[index] = value
	}
	return normalized
}

func ja3Signature(t testing.TB, raw []byte) string {
	t.Helper()
	handshake := clientHelloHandshake(t, raw)
	if len(handshake) < 39 || handshake[0] != 1 {
		t.Fatalf("invalid ClientHello handshake")
	}
	offset := 4
	version := binary.BigEndian.Uint16(handshake[offset : offset+2])
	offset += 2 + 32
	offset += 1 + int(handshake[offset])
	cipherLength := int(binary.BigEndian.Uint16(handshake[offset : offset+2]))
	offset += 2
	ciphers := ja3Uint16List(t, handshake[offset:offset+cipherLength])
	offset += cipherLength
	offset += 1 + int(handshake[offset])
	extensionLength := int(binary.BigEndian.Uint16(handshake[offset : offset+2]))
	offset += 2
	if offset+extensionLength != len(handshake) {
		t.Fatalf("ClientHello extension length = %d, remaining = %d", extensionLength, len(handshake)-offset)
	}

	var extensions, curves []string
	var pointFormats []string
	for end := offset + extensionLength; offset < end; {
		if offset+4 > end {
			t.Fatal("truncated ClientHello extension")
		}
		extensionType := binary.BigEndian.Uint16(handshake[offset : offset+2])
		contentLength := int(binary.BigEndian.Uint16(handshake[offset+2 : offset+4]))
		offset += 4
		if offset+contentLength > end {
			t.Fatal("truncated ClientHello extension content")
		}
		content := handshake[offset : offset+contentLength]
		offset += contentLength
		if !isGREASEValue(extensionType) {
			extensions = append(extensions, strconv.Itoa(int(extensionType)))
		}
		switch extensionType {
		case 10:
			if len(content) < 2 || int(binary.BigEndian.Uint16(content[:2]))+2 != len(content) {
				t.Fatal("invalid supported-groups extension")
			}
			curves = ja3Uint16List(t, content[2:])
		case 11:
			if len(content) < 1 || int(content[0])+1 != len(content) {
				t.Fatal("invalid EC point-format extension")
			}
			for _, value := range content[1:] {
				pointFormats = append(pointFormats, strconv.Itoa(int(value)))
			}
		}
	}
	return strings.Join([]string{
		strconv.Itoa(int(version)),
		strings.Join(ciphers, "-"),
		strings.Join(extensions, "-"),
		strings.Join(curves, "-"),
		strings.Join(pointFormats, "-"),
	}, ",")
}

func clientHelloHandshake(t testing.TB, raw []byte) []byte {
	t.Helper()
	if len(raw) >= 5 && raw[0] == 22 {
		recordLength := int(binary.BigEndian.Uint16(raw[3:5]))
		if recordLength+5 != len(raw) {
			t.Fatalf("TLS record length = %d, raw length = %d", recordLength, len(raw))
		}
		raw = raw[5:]
	}
	if len(raw) < 4 || raw[0] != 1 {
		t.Fatal("ClientHello handshake is missing")
	}
	length := int(raw[1])<<16 | int(raw[2])<<8 | int(raw[3])
	if length+4 != len(raw) {
		t.Fatalf("ClientHello length = %d, raw length = %d", length, len(raw))
	}
	return raw
}

func ja3Uint16List(t testing.TB, values []byte) []string {
	t.Helper()
	if len(values)%2 != 0 {
		t.Fatal("invalid uint16 list")
	}
	result := make([]string, 0, len(values)/2)
	for offset := 0; offset < len(values); offset += 2 {
		value := binary.BigEndian.Uint16(values[offset : offset+2])
		if !isGREASEValue(value) {
			result = append(result, strconv.Itoa(int(value)))
		}
	}
	return result
}

func isGREASEValue(value uint16) bool {
	return value&0x0F0F == 0x0A0A && value>>8 == value&0xFF
}

// fixedBrowserProfileChooser accepts source draw order: operating-system index,
// then browser-variant index.
func fixedBrowserProfileChooser(choices ...int) browserProfileChooser {
	index := 0
	return func(limit int) (int, error) {
		if index >= len(choices) {
			return 0, fmt.Errorf("unexpected profile selection with limit %d", limit)
		}
		choice := choices[index]
		index++
		return choice, nil
	}
}
