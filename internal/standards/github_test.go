package standards

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPFetcher_OK(t *testing.T) {
	want := []byte("hello world\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/contents/.archon/standards.md" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":"` + base64.StdEncoding.EncodeToString(want) + `","sha":"abc123"}`))
	}))
	defer srv.Close()

	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	body, sha, err := fetcher.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(body) != string(want) {
		t.Errorf("body = %q, want %q", body, want)
	}
	if sha != "abc123" {
		t.Errorf("sha = %q, want abc123", sha)
	}
}

func TestHTTPFetcher_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	_, _, err := fetcher.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var nf *ErrNotFound
	if !errors.As(err, &nf) {
		t.Errorf("expected ErrNotFound, got %T: %v", err, err)
	}
}

func TestHTTPFetcher_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	_, _, err := fetcher.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var fe *ErrFetch
	if !errors.As(err, &fe) {
		t.Errorf("expected ErrFetch, got %T: %v", err, err)
	}
	if fe == nil || !strings.Contains(fe.URL, "/repos/o/r/contents/.archon/standards.md") {
		t.Errorf("URL = %q, expected to contain the path", fe.URL)
	}
}

func TestHTTPFetcher_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":"!!!not-base64!!!","sha":"abc"}`))
	}))
	defer srv.Close()
	fetcher := &httpFetcher{client: srv.Client(), base: srv.URL}
	_, _, err := fetcher.Fetch(context.Background(), "o", "r", ".archon/standards.md")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewHTTPFetcherWithClient_NilClient(t *testing.T) {
	// nil client should fall back to http.DefaultClient.
	f := NewHTTPFetcherWithClient(nil)
	if f == nil {
		t.Fatal("expected non-nil fetcher")
	}
}

func TestNewHTTPFetcher_Default(t *testing.T) {
	f := NewHTTPFetcher()
	if f == nil {
		t.Fatal("expected non-nil fetcher")
	}
}

func TestErrNotFound_Error(t *testing.T) {
	e := &ErrNotFound{Owner: "o", Repo: "r", Path: "p"}
	if !strings.Contains(e.Error(), "o/r/p") {
		t.Errorf("Error = %q, expected to contain o/r/p", e.Error())
	}
}

func TestErrFetch_ErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	e := &ErrFetch{URL: "http://x", Err: inner}
	if !strings.Contains(e.Error(), "inner") {
		t.Errorf("Error = %q, expected to mention inner", e.Error())
	}
	if !errors.Is(e, inner) {
		t.Errorf("expected errors.Is to find inner via Unwrap")
	}
}
