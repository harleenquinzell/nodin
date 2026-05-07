package notion_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// minimalPage is a compact but valid Notion page API response.
const minimalPage = `{
	"object": "page",
	"id": "3589c940-0284-81d3-b435-fcf079d89792",
	"created_time": "2026-05-06T00:00:00.000Z",
	"last_edited_time": "2026-05-06T20:00:00.000Z",
	"archived": false,
	"parent": {"type": "workspace", "workspace": true},
	"properties": {
		"title": {
			"type": "title",
			"title": [{
				"type": "text",
				"plain_text": "Test Page",
				"annotations": {"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"}
			}]
		}
	}
}`

func newTestClient(t *testing.T, handler http.Handler) *notion.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	// High RPS so rate limiting doesn't slow tests.
	return notion.NewClient("secret_test", 100).WithBaseURL(srv.URL)
}

func TestGetPage_Success(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Notion-Version") == "" {
			t.Error("missing Notion-Version header")
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, minimalPage)
	}))

	page, err := client.GetPage(context.Background(), "3589c940-0284-81d3-b435-fcf079d89792")
	if err != nil {
		t.Fatal(err)
	}
	if page.ID != "3589c940-0284-81d3-b435-fcf079d89792" {
		t.Errorf("ID = %q", page.ID)
	}
	if page.Title() != "Test Page" {
		t.Errorf("Title() = %q, want %q", page.Title(), "Test Page")
	}
}

func TestGetPage_NotFound(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"object":"error","status":404,"code":"object_not_found","message":"Could not find page"}`)
	}))

	_, err := client.GetPage(context.Background(), "3589c940-0284-81d3-b435-fcf079d89792")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestGetPage_Unauthorized(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"object":"error","status":401,"code":"unauthorized","message":"API token is invalid"}`)
	}))

	_, err := client.GetPage(context.Background(), "3589c940-0284-81d3-b435-fcf079d89792")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestRetry_429(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			// Retry-After: 0 so the test doesn't sleep.
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, minimalPage)
	}))

	_, err := client.GetPage(context.Background(), "3589c940-0284-81d3-b435-fcf079d89792")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("expected 3 calls, got %d", n)
	}
}

func TestRetry_5xx(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, minimalPage)
	}))

	_, err := client.GetPage(context.Background(), "3589c940-0284-81d3-b435-fcf079d89792")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("expected 3 calls, got %d", n)
	}
}

func TestNoRetry_4xx(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"object":"error","status":400,"code":"invalid_json","message":"bad request"}`)
	}))

	_, err := client.GetPage(context.Background(), "3589c940-0284-81d3-b435-fcf079d89792")
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("expected exactly 1 call (no retry on 4xx), got %d", n)
	}
}

func TestNotionVersionHeader(t *testing.T) {
	var gotVersion string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("Notion-Version")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","results":[],"next_cursor":"","has_more":false}`)
	}))

	_, _ = client.Search(context.Background(), notion.SearchOpts{Limit: 1})
	if gotVersion != "2022-06-28" {
		t.Errorf("Notion-Version = %q, want %q", gotVersion, "2022-06-28")
	}
}

func TestSearch_Pagination(t *testing.T) {
	callCount := 0
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		w.Header().Set("Content-Type", "application/json")
		if callCount == 0 {
			callCount++
			fmt.Fprintf(w, `{"object":"list","results":[%s],"next_cursor":"cursor1","has_more":true}`, minimalPage)
		} else {
			fmt.Fprintf(w, `{"object":"list","results":[%s],"next_cursor":"","has_more":false}`, minimalPage)
		}
	}))

	// Zero time = full pull; walk all pages.
	pages, err := client.IncrementalPages(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Errorf("got %d pages, want 2", len(pages))
	}
}

func TestIncrementalPages_EarlyExit(t *testing.T) {
	// The test page was last edited 2026-05-06; we ask for pages after 2030-01-01.
	// The client should stop after the first batch and return zero results.
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"object":"list","results":[%s],"next_cursor":"","has_more":false}`, minimalPage)
	}))

	future := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	pages, err := client.IncrementalPages(context.Background(), future)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages (all older than since), got %d", len(pages))
	}
}

func TestRetry_GivesUp(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := client.GetPage(context.Background(), "3589c940-0284-81d3-b435-fcf079d89792")
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	// maxRetries = 5, so 6 total attempts (attempt 0 through 5).
	if n := calls.Load(); n != 6 {
		t.Errorf("expected 6 attempts (initial + 5 retries), got %d", n)
	}
}

func TestTokenRedacted(t *testing.T) {
	const secretToken = "secret_ABCDEFGHIJKLMNOPQRSTUVWXYZ1234"
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo the Authorization header back in the error body so we can check redaction.
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"object":"error","status":400,"code":"test","message":"auth: %s"}`,
			r.Header.Get("Authorization"))
	})).WithToken(secretToken)

	_, err := client.GetPage(context.Background(), "3589c940-0284-81d3-b435-fcf079d89792")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, secretToken) {
		t.Errorf("error message contains raw token: %s", errMsg)
	}
}
