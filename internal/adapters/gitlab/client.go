// Package gitlab is an adapter that reads pipeline and job state from the
// GitLab REST API. It implements application.GitLabClient with per-request
// timeouts, bounded retries with jitter, and pagination.
package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
)

// Defaults for the HTTP client and retry policy.
const (
	defaultTimeout    = 10 * time.Second
	defaultMaxRetries = 3
	defaultBaseDelay  = 200 * time.Millisecond
	defaultPerPage    = 100
	maxJobPages       = 50 // hard bound so a misbehaving server cannot loop forever
	tokenHeader       = "PRIVATE-TOKEN"
)

// Client talks to a GitLab instance.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	maxRetries int
	baseDelay  time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying HTTP client (e.g. in tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithRetry overrides the retry policy. A baseDelay near zero keeps tests fast.
func WithRetry(maxRetries int, baseDelay time.Duration) Option {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.baseDelay = baseDelay
	}
}

// New constructs a Client for the given base URL (e.g. https://gitlab.com) and
// access token.
func New(baseURL, token string, opts ...Option) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: defaultTimeout},
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultBaseDelay,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

var _ application.GitLabClient = (*Client)(nil)

// apiPipeline mirrors the GitLab pipeline JSON we consume.
type apiPipeline struct {
	ID         int64  `json:"id"`
	Ref        string `json:"ref"`
	SHA        string `json:"sha"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Duration   *int   `json:"duration"`
}

// apiJob mirrors the GitLab job JSON we consume.
type apiJob struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Stage      string   `json:"stage"`
	Status     string   `json:"status"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at"`
	Duration   *float64 `json:"duration"`
}

// GetPipeline fetches a single pipeline.
func (c *Client) GetPipeline(ctx context.Context, gitlabProjectID, pipelineID int64) (application.GitLabPipeline, error) {
	path := fmt.Sprintf("/api/v4/projects/%d/pipelines/%d", gitlabProjectID, pipelineID)

	var raw apiPipeline
	if _, err := c.getJSON(ctx, path, nil, &raw); err != nil {
		return application.GitLabPipeline{}, err
	}
	return application.GitLabPipeline{
		ID:         raw.ID,
		Ref:        raw.Ref,
		SHA:        raw.SHA,
		Status:     raw.Status,
		StartedAt:  parseTime(raw.StartedAt),
		FinishedAt: parseTime(raw.FinishedAt),
		Duration:   raw.Duration,
	}, nil
}

// ListPipelineJobs fetches all jobs for a pipeline, following pagination.
func (c *Client) ListPipelineJobs(ctx context.Context, gitlabProjectID, pipelineID int64) ([]application.GitLabJob, error) {
	path := fmt.Sprintf("/api/v4/projects/%d/pipelines/%d/jobs", gitlabProjectID, pipelineID)

	var out []application.GitLabJob
	page := 1
	for pages := 0; pages < maxJobPages; pages++ {
		q := url.Values{}
		q.Set("per_page", strconv.Itoa(defaultPerPage))
		q.Set("page", strconv.Itoa(page))

		var raw []apiJob
		headers, err := c.getJSON(ctx, path, q, &raw)
		if err != nil {
			return nil, err
		}
		for _, j := range raw {
			out = append(out, application.GitLabJob{
				ID:         j.ID,
				Name:       j.Name,
				Stage:      j.Stage,
				Status:     j.Status,
				StartedAt:  parseTime(j.StartedAt),
				FinishedAt: parseTime(j.FinishedAt),
				Duration:   secondsToInt(j.Duration),
			})
		}

		next := strings.TrimSpace(headers.Get("X-Next-Page"))
		if next == "" {
			break
		}
		n, err := strconv.Atoi(next)
		if err != nil {
			break
		}
		page = n
	}
	return out, nil
}

// getJSON performs a GET with retries and decodes a 2xx body into out. It
// returns the response headers so callers can read pagination markers.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) (http.Header, error) {
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, c.backoff(attempt)); err != nil {
				return nil, err
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set(tokenHeader, c.token)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("gitlab request: %w", err)
			continue // transient transport error → retry
		}

		body, headers, status, readErr := drain(resp)
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if status >= 200 && status < 300 {
			if err := json.Unmarshal(body, out); err != nil {
				return nil, fmt.Errorf("decode gitlab response: %w", err)
			}
			return headers, nil
		}
		if retryableStatus(status) {
			lastErr = fmt.Errorf("gitlab status %d", status)
			continue
		}
		// Non-retryable (e.g. 401, 403, 404).
		return nil, fmt.Errorf("gitlab status %d for %s", status, path)
	}
	return nil, fmt.Errorf("gitlab request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// drain reads and closes the response body.
func drain(resp *http.Response) (body []byte, headers http.Header, status int, err error) {
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MiB cap
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read gitlab body: %w", err)
	}
	return b, resp.Header, resp.StatusCode, nil
}

// backoff is exponential with full jitter.
func (c *Client) backoff(attempt int) time.Duration {
	base := c.baseDelay * time.Duration(1<<(attempt-1))
	if base <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(base) + 1))
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// sleepCtx sleeps for d unless ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func parseTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

func secondsToInt(d *float64) *int {
	if d == nil {
		return nil
	}
	n := int(*d + 0.5)
	return &n
}
