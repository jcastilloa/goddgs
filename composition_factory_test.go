package ddgs

import (
	"reflect"
	"testing"

	"github.com/jcastillo/goddgs/internal/engine"
	"github.com/jcastillo/goddgs/internal/transport"
)

type factoryEngineKey struct {
	category string
	name     string
}

func TestSourceEngineFactory_CreatesFrozenActiveAdaptersWithIsolatedClients(t *testing.T) {
	factory := newSourceEngineFactory()

	baseClientCalls := 0
	duckDuckGoTextClientCalls := 0
	var duckDuckGoHeaders []transport.Field
	factory.newClient = func(config transport.Config) (*transport.Client, error) {
		baseClientCalls++
		return transport.NewClient(config)
	}
	factory.newDuckDuckGoTextClient = func(config transport.Config, headers []transport.Field) (*transport.DuckDuckGoTextClient, error) {
		duckDuckGoTextClientCalls++
		duckDuckGoHeaders = append([]transport.Field(nil), headers...)
		return transport.NewDuckDuckGoTextClient(config, headers)
	}
	factory.duckDuckGoTextUserAgent = func() (string, error) {
		return "fixture DuckDuckGo User-Agent", nil
	}

	wantTypes := map[factoryEngineKey]reflect.Type{
		{category: "books", name: "annasarchive"}: reflect.TypeOf((*engine.AnnasArchive)(nil)),
		{category: "images", name: "bing"}:        reflect.TypeOf((*engine.BingImages)(nil)),
		{category: "images", name: "duckduckgo"}:  reflect.TypeOf((*engine.DuckDuckGoImages)(nil)),
		{category: "news", name: "bing"}:          reflect.TypeOf((*engine.BingNews)(nil)),
		{category: "news", name: "duckduckgo"}:    reflect.TypeOf((*engine.DuckDuckGoNews)(nil)),
		{category: "news", name: "yahoo"}:         reflect.TypeOf((*engine.YahooNews)(nil)),
		{category: "text", name: "brave"}:         reflect.TypeOf((*engine.Brave)(nil)),
		{category: "text", name: "duckduckgo"}:    reflect.TypeOf((*engine.DuckDuckGoText)(nil)),
		{category: "text", name: "google"}:        reflect.TypeOf((*engine.Google)(nil)),
		{category: "text", name: "grokipedia"}:    reflect.TypeOf((*engine.Grokipedia)(nil)),
		{category: "text", name: "mojeek"}:        reflect.TypeOf((*engine.Mojeek)(nil)),
		{category: "text", name: "startpage"}:     reflect.TypeOf((*engine.Startpage)(nil)),
		{category: "text", name: "wikipedia"}:     reflect.TypeOf((*engine.Wikipedia)(nil)),
		{category: "text", name: "yahoo"}:         reflect.TypeOf((*engine.Yahoo)(nil)),
		{category: "text", name: "yandex"}:        reflect.TypeOf((*engine.Yandex)(nil)),
		{category: "videos", name: "duckduckgo"}:  reflect.TypeOf((*engine.DuckDuckGoVideos)(nil)),
	}

	created := make(map[factoryEngineKey]engine.Searcher, len(wantTypes))
	for _, category := range engine.FrozenRegistry().Categories() {
		for _, metadata := range category.Engines {
			key := factoryEngineKey{category: metadata.Category, name: metadata.Name}
			adapter, err := factory.Create(metadata, transport.Config{})
			if err != nil {
				t.Fatalf("Create(%s/%s): %v", metadata.Category, metadata.Name, err)
			}
			if got, want := reflect.TypeOf(adapter), wantTypes[key]; got != want {
				t.Fatalf("Create(%s/%s) type = %v, want %v", metadata.Category, metadata.Name, got, want)
			}
			created[key] = adapter
		}
	}
	if got, want := len(created), len(wantTypes); got != want {
		t.Fatalf("created active adapters = %d, want %d", got, want)
	}
	if got, want := baseClientCalls, len(wantTypes)-1; got != want {
		t.Fatalf("base client constructions = %d, want %d", got, want)
	}
	if got, want := duckDuckGoTextClientCalls, 1; got != want {
		t.Fatalf("DuckDuckGo text client constructions = %d, want %d", got, want)
	}
	if got, want := duckDuckGoHeaders, []transport.Field{{Name: "User-Agent", Value: "fixture DuckDuckGo User-Agent"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DuckDuckGo text headers = %#v, want %#v", got, want)
	}

	brave := engine.Metadata{Name: "brave", Category: "text", Provider: "brave", Priority: 1}
	secondBrave, err := factory.Create(brave, transport.Config{})
	if err != nil {
		t.Fatalf("Create(second brave): %v", err)
	}
	if secondBrave == created[factoryEngineKey{category: "text", name: "brave"}] {
		t.Fatal("factory reused a Brave adapter instead of creating an isolated adapter")
	}
	if got, want := baseClientCalls, len(wantTypes); got != want {
		t.Fatalf("base client constructions after second Brave = %d, want %d", got, want)
	}
}

func TestNew_WiresFrozenSourceComposition(t *testing.T) {
	client := New()
	if _, ok := client.executor.(*composedExecutor); !ok {
		t.Fatalf("New executor = %T, want *composedExecutor", client.executor)
	}
}
