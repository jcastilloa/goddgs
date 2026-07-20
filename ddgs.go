package ddgs

import (
	"os"
	"time"
)

const torBrowserProxy = "socks5h://127.0.0.1:9150"

const defaultTimeout = 5 * time.Second

// DDGS is a client for the source-compatible DDGS search library.
//
// Search methods are added only when their frozen Python contracts and Go
// transport evidence are available.
type DDGS struct {
	config   clientConfig
	executor operationExecutor
}

type clientConfig struct {
	proxy        optionalString
	timeout      *time.Duration
	verification verification
}

type optionalString struct {
	set   bool
	value string
}

type verificationKind uint8

const (
	verificationBool verificationKind = iota
	verificationPEMFile
)

type verification struct {
	kind verificationKind
	bool bool
	pem  string
}

func (v verification) sourceValue() any {
	if v.kind == verificationPEMFile {
		return v.pem
	}
	return v.bool
}

// Option configures a DDGS client.
type Option func(*clientConfig)

// New creates a DDGS client with source-compatible client configuration.
func New(options ...Option) *DDGS {
	config := defaultClientConfig()
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}

	environmentProxy, environmentSet := os.LookupEnv("DDGS_PROXY")
	config.proxy = resolveProxy(config.proxy, environmentProxy, environmentSet)
	return &DDGS{config: config}
}

func defaultClientConfig() clientConfig {
	timeout := defaultTimeout
	return clientConfig{
		timeout:      &timeout,
		verification: verification{bool: true},
	}
}

// WithProxy configures the source-compatible proxy value for a DDGS client.
func WithProxy(proxy string) Option {
	return func(config *clientConfig) {
		config.proxy = optionalString{set: true, value: proxy}
	}
}

// WithTimeout configures the client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(config *clientConfig) {
		config.timeout = &timeout
	}
}

// WithoutTimeout configures the client without a source timeout.
func WithoutTimeout() Option {
	return func(config *clientConfig) {
		config.timeout = nil
	}
}

// WithTLSVerification configures certificate verification.
func WithTLSVerification(verify bool) Option {
	return func(config *clientConfig) {
		config.verification = verification{bool: verify}
	}
}

// WithTLSRootCAFile configures a PEM root certificate file.
func WithTLSRootCAFile(path string) Option {
	return func(config *clientConfig) {
		config.verification = verification{kind: verificationPEMFile, pem: path}
	}
}

func resolveProxy(proxy optionalString, environmentProxy string, environmentSet bool) optionalString {
	if proxy.set && proxy.value == "tb" {
		return optionalString{set: true, value: torBrowserProxy}
	}
	if proxy.set && proxy.value != "" {
		return proxy
	}
	if environmentSet {
		return optionalString{set: true, value: environmentProxy}
	}
	return optionalString{}
}
