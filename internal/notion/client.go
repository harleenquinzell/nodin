package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.notion.com/v1"
	notionVersion  = "2022-06-28"
	maxRetries     = 5
	requestTimeout = 30 * time.Second
)

var tokenRe = regexp.MustCompile(`(secret_|ntn_)[A-Za-z0-9]+`)

// Client is a rate-limited, retrying Notion API client.
// Create with NewClient; configure rate limit with the rps argument.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
	limiter *rateLimiter
}

// NewClient creates a Client for the given token.
// rps sets the request rate limit (Notion caps at ~3 req/s; default if ≤0).
func NewClient(token string, rps int) *Client {
	if rps <= 0 {
		rps = 3
	}
	return &Client{
		token:   token,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: requestTimeout},
		limiter: newRateLimiter(rps),
	}
}

// WithBaseURL returns a shallow copy of the client using a different API base URL.
// Intended for testing.
func (c *Client) WithBaseURL(u string) *Client {
	cc := *c
	cc.baseURL = strings.TrimRight(u, "/")
	return &cc
}

// WithToken returns a shallow copy of the client using a different token.
// Intended for testing.
func (c *Client) WithToken(token string) *Client {
	cc := *c
	cc.token = token
	return &cc
}

// do sends an authenticated request to path, retrying on 429 and 5xx.
// body is JSON-marshalled if non-nil.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}

		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}

		data, retry, err := c.attempt(ctx, method, path, bodyBytes)
		if err == nil {
			return data, nil
		}
		if !retry {
			return nil, err
		}
		lastErr = err

		// 429: honour Retry-After before the next loop iteration's backoff.
		var ratErr *retryAfterError
		if errors.As(err, &ratErr) && ratErr.seconds > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(ratErr.seconds) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("notion: max retries exceeded: %w", lastErr)
}

// attempt performs one HTTP call. Returns (data, retry=false) on success or
// permanent error, (nil, retry=true) on transient error.
func (c *Client) attempt(ctx context.Context, method, path string, body []byte) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", notionVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("read response body: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
		return respBody, false, nil

	case resp.StatusCode == http.StatusTooManyRequests:
		secs := 1
		if v := resp.Header.Get("Retry-After"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				secs = n
			}
		}
		return nil, true, &retryAfterError{seconds: secs}

	case resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("server error %d", resp.StatusCode)

	case resp.StatusCode == http.StatusUnauthorized:
		return nil, false, ErrUnauthorized

	case resp.StatusCode == http.StatusNotFound:
		return nil, false, ErrNotFound

	default:
		var ne NotionError
		if json.Unmarshal(respBody, &ne) == nil && ne.Code != "" {
			ne.Status = resp.StatusCode
			return nil, false, &ne
		}
		return nil, false, fmt.Errorf("notion: HTTP %d", resp.StatusCode)
	}
}

// redactTokens replaces any token-like strings in s with masked versions.
// Use this before including API error messages in user-facing output.
func redactTokens(s string) string {
	return tokenRe.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "secret_") {
			return "secret_***"
		}
		if strings.HasPrefix(m, "ntn_") {
			return "ntn_***"
		}
		return "***"
	})
}

type retryAfterError struct {
	seconds int
}

func (e *retryAfterError) Error() string {
	return fmt.Sprintf("rate limited; retry after %ds", e.seconds)
}

func backoff(attempt int) time.Duration {
	base := 250 * time.Millisecond
	exp := base * (1 << (attempt - 1))
	if exp > 4*time.Second {
		exp = 4 * time.Second
	}
	jitter := time.Duration(rand.Int63n(int64(base / 2)))
	return exp + jitter
}

var (
	ErrUnauthorized = errors.New("notion: unauthorized")
	ErrNotFound     = errors.New("notion: not found")
	ErrRateLimited  = errors.New("notion: rate limited")
)
