//go:build integration

package standards

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestHTTPFetcher_RealGitHub hits the live GitHub Contents API for archon's
// own standards file. Skip on 404 (e.g. file moved) or when offline.
//
// Also skip on 403 with a rate-limit body. The unauthenticated GitHub API
// limit is 60 requests per hour per IP, and shared CI egress IPs burn
// through this fast. Failing the build on a 403 is unhelpful; skipping
// is the right behavior.
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
		// Rate limit: GitHub returns 403 with a body that includes
		// the marker "rate limit". The ErrFetch wraps the status and
		// the trimmed body, so the substring lives inside err.Error().
		if strings.Contains(err.Error(), rateLimitMarker) {
			t.Skipf("GitHub rate limit: %v", err)
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
