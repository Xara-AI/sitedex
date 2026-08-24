package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/Xara-AI/sitedex/internal/crawler"
)

type jobState string

const (
	jobQueued    jobState = "queued"
	jobRunning   jobState = "running"
	jobSucceeded jobState = "succeeded"
	jobFailed    jobState = "failed"
)

// job tracks one crawl job's lifecycle, triggered via POST /v1/crawl and
// polled via GET /v1/crawl/{job}.
type job struct {
	id        string
	site      string
	state     jobState
	startedAt time.Time
	endedAt   time.Time
	result    *crawler.Result
	err       string
}

// jobTracker holds every job for this server's lifetime (in memory only —
// jobs don't survive a restart, which is fine: they're a progress/status
// view onto a crawl, not the crawl's durable state, which lives in
// crawl-state.json and the index itself).
type jobTracker struct {
	mu   sync.Mutex
	next int64
	jobs map[string]*job
}

func newJobTracker() *jobTracker {
	return &jobTracker{jobs: make(map[string]*job)}
}

func (t *jobTracker) create(site string) *job {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next++
	j := &job{id: fmt.Sprintf("job-%d", t.next), site: site, state: jobQueued, startedAt: time.Now().UTC()}
	t.jobs[j.id] = j
	return j
}

func (t *jobTracker) get(id string) (*job, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	j, ok := t.jobs[id]
	return j, ok
}

func (t *jobTracker) setRunning(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.state = jobRunning
	}
}

func (t *jobTracker) setSucceeded(id string, res *crawler.Result) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.state = jobSucceeded
		j.result = res
		j.endedAt = time.Now().UTC()
	}
}

func (t *jobTracker) setFailed(id string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.state = jobFailed
		j.err = err.Error()
		j.endedAt = time.Now().UTC()
	}
}

type jobStatusDTO struct {
	JobID     string          `json:"job_id"`
	Site      string          `json:"site"`
	Status    jobState        `json:"status"`
	StartedAt string          `json:"started_at"`
	EndedAt   string          `json:"ended_at,omitempty"`
	Error     string          `json:"error,omitempty"`
	Result    *crawlResultDTO `json:"result,omitempty"`
}

type crawlResultDTO struct {
	Site         string `json:"site"`
	PagesVisited int    `json:"pages_visited"`
	PagesFetched int    `json:"pages_fetched"`
	PagesSkipped int    `json:"pages_skipped"`
	PagesFailed  int    `json:"pages_failed"`
	DurationMs   int64  `json:"duration_ms"`
}

func (j *job) toDTO() jobStatusDTO {
	dto := jobStatusDTO{
		JobID: j.id, Site: j.site, Status: j.state,
		StartedAt: j.startedAt.Format(time.RFC3339), Error: j.err,
	}
	if !j.endedAt.IsZero() {
		dto.EndedAt = j.endedAt.Format(time.RFC3339)
	}
	if j.result != nil {
		dto.Result = &crawlResultDTO{
			Site: j.result.Site, PagesVisited: j.result.PagesVisited, PagesFetched: j.result.PagesFetched,
			PagesSkipped: j.result.PagesSkipped, PagesFailed: j.result.PagesFailed,
			DurationMs: j.result.Duration.Milliseconds(),
		}
	}
	return dto
}
