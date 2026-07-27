package ddgs

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/jcastilloa/goddgs/internal/engine"
	"github.com/jcastilloa/goddgs/internal/extract"
	"github.com/jcastilloa/goddgs/internal/search"
	"github.com/jcastilloa/goddgs/internal/transport"
)

type composedEngineFactory interface {
	Create(engine.Metadata, transport.Config) (engine.Searcher, error)
}

type composedEngineSelector interface {
	Select(category, backend string) ([]engine.Metadata, error)
}

type composedExecutor struct {
	config    clientConfig
	factory   composedEngineFactory
	selector  composedEngineSelector
	scheduler *search.Scheduler

	enginesMu sync.Mutex
	engines   map[composedEngineKey]engine.Searcher
}

type composedEngineKey struct {
	category string
	name     string
}

func newComposedExecutor(
	config clientConfig,
	factory composedEngineFactory,
	selector composedEngineSelector,
) *composedExecutor {
	return &composedExecutor{
		config:    config,
		factory:   factory,
		selector:  selector,
		scheduler: search.NewScheduler(),
		engines:   make(map[composedEngineKey]engine.Searcher),
	}
}

func (executor *composedExecutor) search(ctx context.Context, request searchRequest) ([]RawResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if executor == nil || executor.factory == nil || executor.selector == nil || executor.scheduler == nil {
		return nil, errFacadeUnavailable
	}

	metadata, err := executor.selector.Select(string(request.category), request.config.backend)
	if err != nil {
		return nil, publicOperationError(err)
	}
	adapters := make([]engine.Searcher, len(metadata))
	for index, entry := range metadata {
		adapters[index], err = executor.engineFor(entry)
		if err != nil {
			return nil, publicOperationError(err)
		}
	}
	engines := make([]search.ScheduledEngine, len(metadata))
	for index, entry := range metadata {
		engines[index] = search.ScheduledEngine{
			Name:     entry.Name,
			Provider: entry.Provider,
			Search:   searchEngine(adapters[index]),
		}
	}

	scheduleRequest := search.ScheduleRequest{
		Query:      request.query,
		Region:     request.config.region,
		SafeSearch: request.config.safeSearch,
		TimeLimit:  copyStringPointer(request.config.timeLimit),
		MaxResults: copyIntPointer(request.config.maxResults),
		Page:       request.config.page,
		Parameters: sourceParameters(request.config.sourceKeywords),
	}
	if request.config.diagnostics != nil {
		scheduleRequest.OnComplete = func(completion search.Completion) {
			request.config.diagnostics(SearchDiagnostic{
				Backend:     completion.Name,
				Provider:    completion.Provider,
				ResultCount: completion.ResultCount,
				Err:         completion.Err,
			})
		}
	}
	results, err := executor.scheduler.Search(ctx, scheduleRequest, engines)
	if err != nil {
		return nil, publicOperationError(err)
	}
	return rawResults(results), nil
}

func publicOperationError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	kind := ddgsErrorGeneric
	if strings.Contains(err.Error(), "timed out") {
		kind = ddgsErrorTimeout
	}
	return newDDGSError(kind, err.Error(), err)
}

func (executor *composedExecutor) extract(ctx context.Context, request extractRequest) (ExtractResult, error) {
	if err := ctx.Err(); err != nil {
		return ExtractResult{}, err
	}
	if executor == nil {
		return ExtractResult{}, errFacadeUnavailable
	}

	config := transportConfig(executor.config)
	fetcher, err := extract.NewTransportFetcher(config)
	if err != nil {
		return ExtractResult{}, publicOperationError(err)
	}
	result, err := extract.New(fetcher, extract.NewPracticalRenderer()).Extract(ctx, extract.Request{
		Method: http.MethodGet,
		URL:    request.url,
		Format: request.config.format,
		Config: config,
	})
	if err != nil {
		return ExtractResult{}, publicOperationError(err)
	}
	return ExtractResult{URL: result.URL, Content: result.Content}, nil
}

func searchEngine(adapter engine.Searcher) search.EngineSearch {
	return func(ctx context.Context, request search.EngineRequest) ([]search.Result, error) {
		return adapter.Search(ctx, engine.SearchRequest{
			Query:      request.Query,
			Region:     request.Region,
			SafeSearch: request.SafeSearch,
			TimeLimit:  copyStringPointer(request.TimeLimit),
			Page:       request.Page,
			Parameters: engineParameters(request.Parameters),
		})
	}
}

func (executor *composedExecutor) engineFor(metadata engine.Metadata) (engine.Searcher, error) {
	key := composedEngineKey{category: metadata.Category, name: metadata.Name}
	executor.enginesMu.Lock()
	defer executor.enginesMu.Unlock()
	if adapter, exists := executor.engines[key]; exists {
		return adapter, nil
	}
	adapter, err := executor.factory.Create(metadata, transportConfig(executor.config))
	if err != nil {
		return nil, err
	}
	executor.engines[key] = adapter
	return adapter, nil
}

func transportConfig(config clientConfig) transport.Config {
	result := transport.Config{Verification: transport.VerifyCertificates()}
	if config.proxy.set {
		proxy := config.proxy.value
		result.Proxy = &proxy
	}
	if config.timeout == nil {
		result.Timeout = transport.WithoutTimeout()
	} else {
		result.Timeout = transport.WithTimeout(*config.timeout)
	}
	if config.verification.kind == verificationPEMFile {
		result.Verification = transport.VerifyWithPEMFile(config.verification.pem)
	} else if !config.verification.bool {
		result.Verification = transport.SkipCertificateVerification()
	}
	return result
}

func sourceParameters(keywords []sourceKeyword) []search.SourceParameter {
	parameters := make([]search.SourceParameter, len(keywords))
	for index, keyword := range keywords {
		parameters[index] = search.SourceParameter{Name: keyword.name, Value: keyword.value}
	}
	return parameters
}

func engineParameters(parameters []search.SourceParameter) []engine.SearchParameter {
	copyOfParameters := make([]engine.SearchParameter, len(parameters))
	for index, parameter := range parameters {
		copyOfParameters[index] = engine.SearchParameter{Name: parameter.Name, Value: parameter.Value}
	}
	return copyOfParameters
}

func rawResults(results []map[string]any) []RawResult {
	converted := make([]RawResult, len(results))
	for index, result := range results {
		converted[index] = RawResult(result)
	}
	return converted
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func copyIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}
