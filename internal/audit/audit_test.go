package audit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/elpic/archon/internal/llm"
	"github.com/elpic/archon/internal/standards"
)

// stubFetcher returns canned bytes for any (owner, repo, path) it knows
// about, and ErrNotFound otherwise.
type stubFetcher struct {
	responses map[string]stubResponse
}

type stubResponse struct {
	body []byte
	sha  string
}

func (s *stubFetcher) Fetch(_ context.Context, owner, repo, path string) ([]byte, string, error) {
	r, ok := s.responses[owner+"/"+repo+"/"+path]
	if !ok {
		return nil, "", &standards.ErrNotFound{Owner: owner, Repo: repo, Path: path}
	}
	return r.body, r.sha, nil
}

type fakeProvider struct {
	violations []llm.Violation
	err        error
	calledWith struct {
		body   []byte
		target string
	}
}

func (f *fakeProvider) Audit(_ context.Context, body []byte, target string) ([]llm.Violation, error) {
	f.calledWith.body = body
	f.calledWith.target = target
	return f.violations, f.err
}

func writeProjectFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".archon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".archon", "standards.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunner_ReportStandardsSource_FromOrg: when the project file is a
// `from:` redirect, the Runner reports the org source on the Report so
// the inheritance is observable, not invisible.
func TestRunner_ReportStandardsSource_FromOrg(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "<!-- from: owner/repo -->\n")

	fetcher := &stubFetcher{
		responses: map[string]stubResponse{
			"owner/repo/.archon/standards.md": {body: []byte("ORG BODY"), sha: "deadbeef"},
		},
	}
	resolver, err := standards.NewResolver(".", standards.WithFetcher(fetcher), standards.WithFallback("other/repo"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	runner := NewRunner(resolver, provider)

	report, err := runner.Run(context.Background(), dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.StandardsSource != "github.com/owner/repo@deadbeef" {
		t.Errorf("StandardsSource = %q, want github.com/owner/repo@deadbeef", report.StandardsSource)
	}
	if string(provider.calledWith.body) != "ORG BODY" {
		t.Errorf("provider received body %q, want ORG BODY", provider.calledWith.body)
	}
	if provider.calledWith.target != dir {
		t.Errorf("provider received target %q, want %q", provider.calledWith.target, dir)
	}
}

// TestRunner_ReportStandardsSource_Local: when the project file has
// substantive body, the Runner reports the local path as the source.
func TestRunner_ReportStandardsSource_Local(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "# Project Standards\n\nReal content.\n")

	resolver, err := standards.NewResolver(".")
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	runner := NewRunner(resolver, provider)

	report, err := runner.Run(context.Background(), dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filepath.Join(dir, ".archon", "standards.md")
	if report.StandardsSource != want {
		t.Errorf("StandardsSource = %q, want %q", report.StandardsSource, want)
	}
	if string(provider.calledWith.body) != "# Project Standards\n\nReal content.\n" {
		t.Errorf("provider body = %q", provider.calledWith.body)
	}
}

// TestRunner_PropagatesResolverError: when the resolver fails, the
// Runner surfaces the error.
func TestRunner_PropagatesResolverError(t *testing.T) {
	dir := t.TempDir() // no project file, no fallback
	resolver, err := standards.NewResolver(".", standards.WithFetcher(&stubFetcher{}))
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	runner := NewRunner(resolver, provider)
	if _, err := runner.Run(context.Background(), dir); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestRunner_PropagatesProviderError: when the LLM provider fails, the
// Runner surfaces the error.
func TestRunner_PropagatesProviderError(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "# X\n")
	resolver, err := standards.NewResolver(".")
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("llm blew up")
	provider := &fakeProvider{err: wantErr}
	runner := NewRunner(resolver, provider)
	_, err = runner.Run(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped provider error, got %v", err)
	}
}

// TestRunner_ReportIncludesViolations: violations flow from the provider
// into the Report unchanged.
func TestRunner_ReportIncludesViolations(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "# X\n")
	resolver, err := standards.NewResolver(".")
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{
		violations: []llm.Violation{
			{Rule: "r1", Description: "d1", Severity: llm.SeverityError},
			{Rule: "r2", Description: "d2", Severity: llm.SeverityWarn},
		},
	}
	runner := NewRunner(resolver, provider)
	report, err := runner.Run(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Violations) != 2 {
		t.Fatalf("got %d violations, want 2", len(report.Violations))
	}
	if report.Violations[0].Rule != "r1" || report.Violations[1].Rule != "r2" {
		t.Errorf("violations out of order: %+v", report.Violations)
	}
}
