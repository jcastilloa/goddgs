package ddgs

import (
	"errors"
	"fmt"

	"github.com/jcastillo/goddgs/internal/engine"
	"github.com/jcastillo/goddgs/internal/transport"
)

type sourceEngineFactory struct {
	newClient               func(transport.Config) (*transport.Client, error)
	newDuckDuckGoTextClient func(transport.Config, []transport.Field) (*transport.DuckDuckGoTextClient, error)
	duckDuckGoTextUserAgent func() (string, error)
}

func newSourceEngineFactory() *sourceEngineFactory {
	return &sourceEngineFactory{
		newClient:               transport.NewClient,
		newDuckDuckGoTextClient: transport.NewDuckDuckGoTextClient,
		duckDuckGoTextUserAgent: transport.DuckDuckGoTextUserAgent,
	}
}

func (factory *sourceEngineFactory) Create(metadata engine.Metadata, config transport.Config) (engine.Searcher, error) {
	if factory == nil {
		return nil, errors.New("source engine factory is unavailable")
	}
	if metadata.Category == "text" && metadata.Name == "duckduckgo" {
		return factory.newDuckDuckGoText(config)
	}

	client, err := factory.newClient(config)
	if err != nil {
		return nil, err
	}
	return sourceAdapter(metadata, client)
}

func (factory *sourceEngineFactory) newDuckDuckGoText(config transport.Config) (engine.Searcher, error) {
	userAgent, err := factory.duckDuckGoTextUserAgent()
	if err != nil {
		return nil, err
	}
	client, err := factory.newDuckDuckGoTextClient(config, []transport.Field{{Name: "User-Agent", Value: userAgent}})
	if err != nil {
		return nil, err
	}
	return engine.NewDuckDuckGoText(client), nil
}

func sourceAdapter(metadata engine.Metadata, client *transport.Client) (engine.Searcher, error) {
	switch metadata.Category {
	case "books":
		if metadata.Name == "annasarchive" {
			return engine.NewAnnasArchive(client)
		}
	case "images":
		switch metadata.Name {
		case "bing":
			return engine.NewBingImages(client), nil
		case "duckduckgo":
			return engine.NewDuckDuckGoImages(client), nil
		}
	case "news":
		switch metadata.Name {
		case "bing":
			return engine.NewBingNews(client), nil
		case "duckduckgo":
			return engine.NewDuckDuckGoNews(client), nil
		case "yahoo":
			return engine.NewYahooNews(client), nil
		}
	case "text":
		switch metadata.Name {
		case "brave":
			return engine.NewBrave(client), nil
		case "google":
			return engine.NewGoogle(client)
		case "grokipedia":
			return engine.NewGrokipedia(client), nil
		case "mojeek":
			return engine.NewMojeek(client), nil
		case "startpage":
			return engine.NewStartpage(client), nil
		case "wikipedia":
			return engine.NewWikipedia(client), nil
		case "yahoo":
			return engine.NewYahoo(client), nil
		case "yandex":
			return engine.NewYandex(client), nil
		}
	case "videos":
		if metadata.Name == "duckduckgo" {
			return engine.NewDuckDuckGoVideos(client), nil
		}
	}
	return nil, fmt.Errorf("unsupported frozen engine %q/%q", metadata.Category, metadata.Name)
}
