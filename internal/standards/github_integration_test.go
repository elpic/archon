//go:build integration

package standards

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// TestHTTPFetcher_RealGitHub hits the live GitHub Contents API for archon's
// own standards file. Skip on 404 (e.g. file moved) or when offline.
func TestHTTPFetcher_RealGitHub(t *testing.T) {
	client := &http.Client{Timeout: 15 * time.Second}
	fetcher := NewHTTPFetcherWithClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	body, sha, err := fetcher.Fetch(ctx, "elpic", "archon", ".archon/standards.md")
	if err != nil {
		var nf *ErrNotFound
		if errors.As(err, &nf) {
			t.Skipf("file not found at expected path: %v", err)
		}
		t.Fatalf("Fetch: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
	if sha == "" {
		t.Error("expected non-empty SHA")
	}
}
