package search

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

const (
	sourceValueErrorType     = "ValueError"
	sourceDDGSErrorType      = "DDGSException"
	sourceTimeoutErrorType   = "TimeoutException"
	sourceEmptyQueryMessage  = "query is mandatory."
	sourceNoResultsMessage   = "No results found."
	sourceWorkerErrorMessage = "max_workers must be greater than 0"
)

// EngineRequest carries the source search inputs consumed by one engine call.
// It stays internal so transport and parser types cannot cross the library API.
type EngineRequest struct {
	Query      string
	Region     string
	SafeSearch string
	TimeLimit  *string
	Page       int
}

// EngineSearch is the scheduler's consumer-side execution boundary.
//
// Implementations must treat EngineRequest as read-only, honor ctx, and return
// errors instead of panicking. The scheduler joins its worker pool before
// returning, but Go cannot safely force-stop a non-cooperative callback.
type EngineSearch func(context.Context, EngineRequest) ([]Result, error)

// ScheduledEngine is immutable metadata plus one per-call execution function.
type ScheduledEngine struct {
	Name     string
	Provider string
	Search   EngineSearch
}

// ScheduleRequest holds source scheduling inputs without normalizing their
// omitted, zero, or negative forms.
type ScheduleRequest struct {
	Query       string
	Region      string
	SafeSearch  string
	TimeLimit   *string
	MaxResults  *int
	Page        int
	Threads     *int
	WaitTimeout *time.Duration
}

// Scheduler coordinates source-compatible engine execution.
type Scheduler struct {
	ranker      schedulerRanker
	cacheFields []string
}

type schedulerRanker interface {
	Rank([]map[string]any, string) ([]map[string]any, error)
}

// NewScheduler constructs the internal scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		ranker:      NewSimpleFilterRanker(),
		cacheFields: []string{"href", "image", "url", "embed_url"},
	}
}

func sourceWorkerCount(engines []ScheduledEngine, maxResults, threads *int) (int, error) {
	providers := make(map[string]struct{}, len(engines))
	for _, engine := range engines {
		providers[engine.Provider] = struct{}{}
	}

	workers := len(providers)
	if sourceMaxResultsIsTruthy(maxResults) {
		workers = min(workers, sourceMaximumWorkers(*maxResults))
	}
	if threads != nil && *threads != 0 {
		workers = min(workers, *threads)
	}
	if workers <= 0 {
		return 0, newSourceSchedulerError(sourceValueErrorType, sourceWorkerErrorMessage, nil)
	}
	return workers, nil
}

// Search executes the supplied engine calls.
func (s *Scheduler) Search(ctx context.Context, request ScheduleRequest, engines []ScheduledEngine) ([]map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request = copyScheduleRequest(request)
	engines = append([]ScheduledEngine(nil), engines...)
	if request.Query == "" {
		return nil, newSourceSchedulerError(sourceDDGSErrorType, sourceEmptyQueryMessage, nil)
	}
	workers, err := sourceWorkerCount(engines, request.MaxResults, request.Threads)
	if err != nil {
		return nil, err
	}

	operationContext, cancel := context.WithCancel(ctx)
	defer cancel()

	aggregator, err := NewResultsAggregator(s.cacheFields)
	if err != nil {
		return nil, err
	}
	completions := make(chan schedulerCompletion, len(engines))
	jobs := make(chan *scheduledFuture, len(engines))
	var workersGroup sync.WaitGroup
	engineRequest := copyEngineRequest(EngineRequest{
		Query:      request.Query,
		Region:     request.Region,
		SafeSearch: request.SafeSearch,
		TimeLimit:  request.TimeLimit,
		Page:       request.Page,
	})
	startSchedulerWorkers(&workersGroup, workers, jobs, completions, operationContext, engineRequest)
	jobsClosed := false
	closeJobs := func() {
		if !jobsClosed {
			close(jobs)
			jobsClosed = true
		}
	}
	joinWorkers := func() {
		closeJobs()
		workersGroup.Wait()
	}

	seenProviders := make(map[string]struct{}, workers)
	pending := make([]*scheduledFuture, 0, workers)
	var lastError error

	for index, engine := range engines {
		position := index + 1 // Python's enumerate starts at one.
		if err := ctx.Err(); err != nil {
			cancel()
			joinWorkers()
			return nil, err
		}
		if _, seen := seenProviders[engine.Provider]; seen {
			continue
		}

		future := &scheduledFuture{engine: engine}
		pending = append(pending, future)
		jobs <- future

		if len(pending) >= workers || position >= workers {
			done, remaining, waitError := waitForSourceBatch(operationContext, completions, pending, request.WaitTimeout)
			if waitError != nil {
				cancel()
				joinWorkers()
				if ctxError := ctx.Err(); ctxError != nil {
					return nil, ctxError
				}
				return nil, waitError
			}
			lastError = processSourceCompletions(done, pending, aggregator, seenProviders, lastError)
			pending = remaining
		}

		if sourceMaxResultsIsTruthy(request.MaxResults) && aggregator.Len() >= *request.MaxResults {
			break
		}
	}

	joinWorkers()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rawResults := resultMaps(aggregator.Extract())
	ranked, err := s.ranker.Rank(rawResults, request.Query)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ranked) == 0 {
		return nil, sourceNoResultsError(lastError)
	}
	return sourceFinalSlice(ranked, request.MaxResults), nil
}

func copyScheduleRequest(request ScheduleRequest) ScheduleRequest {
	requestCopy := request
	if request.TimeLimit != nil {
		timeLimit := *request.TimeLimit
		requestCopy.TimeLimit = &timeLimit
	}
	if request.MaxResults != nil {
		maxResults := *request.MaxResults
		requestCopy.MaxResults = &maxResults
	}
	if request.Threads != nil {
		threads := *request.Threads
		requestCopy.Threads = &threads
	}
	if request.WaitTimeout != nil {
		waitTimeout := *request.WaitTimeout
		requestCopy.WaitTimeout = &waitTimeout
	}
	return requestCopy
}

type scheduledFuture struct {
	engine ScheduledEngine
}

type schedulerCompletion struct {
	future  *scheduledFuture
	results []Result
	err     error
}

type sourceSchedulerError struct {
	sourceType string
	message    string
	cause      error
}

func (e *sourceSchedulerError) Error() string {
	return e.message
}

func (e *sourceSchedulerError) Unwrap() error {
	return e.cause
}

func newSourceSchedulerError(sourceType, message string, cause error) *sourceSchedulerError {
	return &sourceSchedulerError{sourceType: sourceType, message: message, cause: cause}
}

func startSchedulerWorkers(
	group *sync.WaitGroup,
	workers int,
	jobs <-chan *scheduledFuture,
	completions chan<- schedulerCompletion,
	ctx context.Context,
	request EngineRequest,
) {
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case future, open := <-jobs:
					if !open || ctx.Err() != nil {
						return
					}
					results, err := future.engine.Search(ctx, copyEngineRequest(request))
					completions <- schedulerCompletion{future: future, results: results, err: err}
				}
			}
		}()
	}
}

func copyEngineRequest(request EngineRequest) EngineRequest {
	requestCopy := request
	if request.TimeLimit != nil {
		timeLimit := *request.TimeLimit
		requestCopy.TimeLimit = &timeLimit
	}
	return requestCopy
}

func waitForSourceBatch(
	ctx context.Context,
	completions <-chan schedulerCompletion,
	pending []*scheduledFuture,
	timeout *time.Duration,
) (map[*scheduledFuture]schedulerCompletion, []*scheduledFuture, error) {
	done := make(map[*scheduledFuture]schedulerCompletion, len(pending))
	var timer *time.Timer
	var timeoutChannel <-chan time.Time
	if timeout != nil {
		timer = time.NewTimer(*timeout)
		timeoutChannel = timer.C
		defer stopSourceTimer(timer)
	}

	for {
		if err := ctx.Err(); err != nil {
			return done, sourcePending(pending, done), err
		}
		sourceBatchSnapshot(completions, done)
		if sourceBatchReady(done, pending) {
			return done, sourcePending(pending, done), nil
		}

		select {
		case <-ctx.Done():
			return done, sourcePending(pending, done), ctx.Err()
		case completion := <-completions:
			done[completion.future] = completion
			if completion.err != nil {
				drainSourceCompletions(completions, done)
				return done, sourcePending(pending, done), nil
			}
		case <-timeoutChannel:
			drainSourceCompletions(completions, done)
			return done, sourcePending(pending, done), nil
		}
	}
}

func sourceBatchSnapshot(
	completions <-chan schedulerCompletion,
	done map[*scheduledFuture]schedulerCompletion,
) bool {
	observed := false
	for {
		select {
		case completion := <-completions:
			done[completion.future] = completion
			observed = true
		default:
			return observed
		}
	}
}

func sourceBatchReady(done map[*scheduledFuture]schedulerCompletion, pending []*scheduledFuture) bool {
	if len(done) == len(pending) {
		return true
	}
	for _, completion := range done {
		if completion.err != nil {
			return true
		}
	}
	return false
}

func stopSourceTimer(timer *time.Timer) {
	if timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

func drainSourceCompletions(completions <-chan schedulerCompletion, done map[*scheduledFuture]schedulerCompletion) {
	sourceBatchSnapshot(completions, done)
}

func sourcePending(pending []*scheduledFuture, done map[*scheduledFuture]schedulerCompletion) []*scheduledFuture {
	remaining := make([]*scheduledFuture, 0, len(pending)-len(done))
	for _, future := range pending {
		if _, complete := done[future]; !complete {
			remaining = append(remaining, future)
		}
	}
	return remaining
}

func processSourceCompletions(
	done map[*scheduledFuture]schedulerCompletion,
	pending []*scheduledFuture,
	aggregator *ResultsAggregator,
	seenProviders map[string]struct{},
	lastError error,
) error {
	for _, future := range pending {
		completion, complete := done[future]
		if !complete {
			continue
		}
		if completion.err != nil {
			lastError = completion.err
			continue
		}
		if len(completion.results) == 0 {
			continue
		}

		if err := appendSourceResults(aggregator, completion.results); err != nil {
			lastError = err
			continue
		}
		seenProviders[future.engine.Provider] = struct{}{}
	}
	return lastError
}

func appendSourceResults(aggregator *ResultsAggregator, results []Result) error {
	for _, result := range results {
		if err := aggregator.Append(result); err != nil {
			return err
		}
	}
	return nil
}

func resultMaps(results []Result) []map[string]any {
	maps := make([]map[string]any, len(results))
	for index, result := range results {
		maps[index] = result.Map()
	}
	return maps
}

func sourceMaxResultsIsTruthy(maxResults *int) bool {
	return maxResults != nil && *maxResults != 0
}

func sourceMaximumWorkers(maxResults int) int {
	return int(math.Ceil(float64(maxResults)/10)) + 1
}

func sourceFinalSlice(results []map[string]any, maxResults *int) []map[string]any {
	if !sourceMaxResultsIsTruthy(maxResults) {
		return results
	}
	if *maxResults > 0 {
		return results[:min(len(results), *maxResults)]
	}
	return results[:max(0, len(results)+*maxResults)]
}

func sourceNoResultsError(lastError error) error {
	if lastError == nil {
		return newSourceSchedulerError(sourceDDGSErrorType, sourceNoResultsMessage, nil)
	}
	message := fmt.Sprint(lastError)
	if strings.Contains(message, "timed out") {
		return newSourceSchedulerError(sourceTimeoutErrorType, message, lastError)
	}
	return newSourceSchedulerError(sourceDDGSErrorType, message, lastError)
}
