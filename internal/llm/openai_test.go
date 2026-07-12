package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wrapChatResponse wraps a message string in the OpenAI chat completions
// response envelope so unmarshalInto chatResponse succeeds.
func wrapChatResponse(content string) string {
	resp := chatResponse{
		Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			{Message: struct {
				Content string `json:"content"`
			}{Content: content}},
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

// wrapChatError wraps an error message in the OpenAI error response envelope.
func wrapChatError(msg string) string {
	resp := struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}{
		Error: &struct {
			Message string `json:"message"`
		}{Message: msg},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

// mockOpenAIServer builds a test HTTP server that responds with the
// given response body for every /chat/completions request. The handler
// also captures the request so tests can inspect it.
func mockOpenAIServer(t *testing.T, responseBody string, statusCode int) (*httptest.Server, *chatRequest) {
	t.Helper()
	var captured chatRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing Bearer token")
		}
		// Capture the request body.
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			captured = req
		}
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody)) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	return srv, &captured
}

// createTestDir creates a temp dir with the given .go files.
func createTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestOpenAIProvider_Audit_Success(t *testing.T) {
	dir := createTestDir(t, map[string]string{
		"main.go":       "package main\n\nfunc main() {}\n",
		"cmd/server.go": "package cmd\n\nfunc Run() {}\n",
	})

	violationsJSON := `[
		{
			"rule": "no-comments",
			"description": "Comments are forbidden in this file",
			"severity": "warn",
			"file": "main.go",
			"line": 5,
			"column": 1,
			"suggestion": "// removed",
			"ruleDoc": "https://example.com/no-comments"
		},
		{
			"rule": "error-handling",
			"description": "Ignored error return value",
			"severity": "error",
			"file": "cmd/server.go",
			"line": 12,
			"column": 3
		}
	]`

	srv, captured := mockOpenAIServer(t, wrapChatResponse(violationsJSON), http.StatusOK)
	defer srv.Close()

	provider := newOpenAIProvider("test-key", srv.URL, "gpt-4o")

	violations, err := provider.Audit(context.Background(), []byte("no-comments: do not add comments"), dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(violations))
	}

	// Verify the first violation.
	v := violations[0]
	if v.Rule != "no-comments" {
		t.Errorf("Rule = %q, want %q", v.Rule, "no-comments")
	}
	if v.Description != "Comments are forbidden in this file" {
		t.Errorf("Description = %q, want %q", v.Description, "Comments are forbidden in this file")
	}
	if v.Severity != SeverityWarn {
		t.Errorf("Severity = %v, want %v", v.Severity, SeverityWarn)
	}
	if v.File != "main.go" {
		t.Errorf("File = %q, want %q", v.File, "main.go")
	}
	if v.Line != 5 {
		t.Errorf("Line = %d, want %d", v.Line, 5)
	}
	if v.Column != 1 {
		t.Errorf("Column = %d, want %d", v.Column, 1)
	}
	if v.Suggestion != "// removed" {
		t.Errorf("Suggestion = %q, want %q", v.Suggestion, "// removed")
	}
	if v.RuleDoc != "https://example.com/no-comments" {
		t.Errorf("RuleDoc = %q, want %q", v.RuleDoc, "https://example.com/no-comments")
	}

	// Verify the second violation.
	v2 := violations[1]
	if v2.Rule != "error-handling" {
		t.Errorf("Rule = %q, want %q", v2.Rule, "error-handling")
	}
	if v2.Severity != SeverityError {
		t.Errorf("Severity = %v, want %v", v2.Severity, SeverityError)
	}
	if v2.Suggestion != "" {
		t.Errorf("Suggestion should be empty, got %q", v2.Suggestion)
	}

	// Verify the request was well-formed.
	if captured.Model != "gpt-4o" {
		t.Errorf("model = %q, want %q", captured.Model, "gpt-4o")
	}
	if len(captured.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want %q", captured.Messages[0].Role, "system")
	}
	if captured.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want %q", captured.Messages[1].Role, "user")
	}
	// The user message should contain the standards text.
	if !strings.Contains(captured.Messages[1].Content, "no-comments: do not add comments") {
		t.Error("user message should contain the standards body")
	}
	// The user message should contain at least one of the source files.
	if !strings.Contains(captured.Messages[1].Content, "package main") {
		t.Error("user message should contain source code")
	}
}

func TestOpenAIProvider_Audit_EmptyViolations(t *testing.T) {
	dir := createTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	srv, _ := mockOpenAIServer(t, wrapChatResponse("[]"), http.StatusOK)
	defer srv.Close()

	provider := newOpenAIProvider("test-key", srv.URL, "gpt-4o")

	violations, err := provider.Audit(context.Background(), []byte("some standards"), dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestOpenAIProvider_Audit_MalformedJSON(t *testing.T) {
	dir := createTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	srv, _ := mockOpenAIServer(t, wrapChatResponse("this is not json at all"), http.StatusOK)
	defer srv.Close()

	provider := newOpenAIProvider("test-key", srv.URL, "gpt-4o")

	_, err := provider.Audit(context.Background(), []byte("standards"), dir, nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing LLM response") {
		t.Errorf("error should mention parsing, got: %v", err)
	}
}

func TestOpenAIProvider_Audit_MarkdownFencedJSON(t *testing.T) {
	dir := createTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	// Some LLMs wrap JSON in markdown fences — parseViolations should handle it.
	llmResponse := "```json\n[{\"rule\":\"r\",\"description\":\"d\",\"severity\":\"info\",\"file\":\"f.go\",\"line\":1,\"column\":1}]\n```"

	srv, _ := mockOpenAIServer(t, wrapChatResponse(llmResponse), http.StatusOK)
	defer srv.Close()

	provider := newOpenAIProvider("test-key", srv.URL, "gpt-4o")

	violations, err := provider.Audit(context.Background(), []byte("standards"), dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Rule != "r" {
		t.Errorf("Rule = %q, want %q", violations[0].Rule, "r")
	}
}

func TestOpenAIProvider_Audit_APIError(t *testing.T) {
	dir := createTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	srv, _ := mockOpenAIServer(t, wrapChatError("rate limited"), http.StatusTooManyRequests)
	defer srv.Close()

	provider := newOpenAIProvider("test-key", srv.URL, "gpt-4o")

	_, err := provider.Audit(context.Background(), []byte("standards"), dir, nil)
	if err == nil {
		t.Fatal("expected error for API error, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should contain status code 429, got: %v", err)
	}
}

func TestOpenAIProvider_Audit_NoGoFiles(t *testing.T) {
	// An empty temp dir has no .go files — should return nil violations, nil error.
	provider := newOpenAIProvider("test-key", "http://unused", "gpt-4o")

	violations, err := provider.Audit(context.Background(), []byte("standards"), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if violations != nil {
		t.Errorf("expected nil violations, got %v", violations)
	}
}

func TestOpenAIProvider_Audit_WithChangedFiles(t *testing.T) {
	dir := createTestDir(t, map[string]string{
		"a.go": "package main\n",
		"b.go": "package main\n",
	})

	violationsJSON := `[{"rule":"r","description":"d","severity":"info","file":"a.go","line":1,"column":1}]`
	srv, captured := mockOpenAIServer(t, wrapChatResponse(violationsJSON), http.StatusOK)
	defer srv.Close()

	provider := newOpenAIProvider("test-key", srv.URL, "gpt-4o")

	violations, err := provider.Audit(context.Background(), []byte("std"), dir, []string{"a.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}

	// The user prompt should only contain a.go, not b.go.
	userMsg := captured.Messages[1].Content
	if !strings.Contains(userMsg, "a.go") {
		t.Error("user message should contain a.go")
	}
	if strings.Contains(userMsg, "b.go") {
		t.Error("user message should NOT contain b.go")
	}
}

func TestOpenAIProvider_Audit_APIErrorResponse(t *testing.T) {
	dir := createTestDir(t, map[string]string{
		"main.go": "package main\n",
	})

	srv, _ := mockOpenAIServer(t, wrapChatError("invalid key"), http.StatusUnauthorized)
	defer srv.Close()

	provider := newOpenAIProvider("test-key", srv.URL, "gpt-4o")

	_, err := provider.Audit(context.Background(), []byte("standards"), dir, nil)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should contain status 401, got: %v", err)
	}
}

func TestParseSeverity(t *testing.T) {
	cases := []struct {
		input string
		want  Severity
	}{
		{"info", SeverityInfo},
		{"warn", SeverityWarn},
		{"warning", SeverityWarn},
		{"error", SeverityError},
		{"critical", SeverityCritical},
		{"CRITICAL", SeverityCritical},
		{"", SeverityInfo},
		{"bogus", SeverityInfo},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := parseSeverity(tc.input); got != tc.want {
				t.Errorf("parseSeverity(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseViolations_Empty(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty array", "[]"},
		{"empty string", ""},
		{"whitespace", "  \n  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := parseViolations(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != nil {
				t.Errorf("expected nil, got %v", v)
			}
		})
	}
}

// TestParseViolations_InvalidJSON ensures invalid JSON returns an error
// with the raw input truncated in the message for debuggability.
func TestParseViolations_InvalidJSON(t *testing.T) {
	_, err := parseViolations("{not valid json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error should mention invalid JSON, got: %v", err)
	}
}

// TestBuildSystemPrompt_ContainsResponseFormat checks that the system
// prompt instructs the LLM about the expected JSON response shape.
func TestBuildSystemPrompt_ContainsResponseFormat(t *testing.T) {
	prompt := buildSystemPrompt()
	mustContain := []string{
		"JSON",
		"rule",
		"description",
		"severity",
		"file",
		"line",
		"column",
		"suggestion",
	}
	for _, s := range mustContain {
		if !strings.Contains(prompt, s) {
			t.Errorf("system prompt should contain %q", s)
		}
	}
}

// TestBuildUserPrompt_ContainsStandardsAndCode verifies the user prompt
// embeds both the standards and code sections.
func TestBuildUserPrompt_ContainsStandardsAndCode(t *testing.T) {
	prompt := buildUserPrompt("my-standards", "my-code")
	if !strings.Contains(prompt, "my-standards") {
		t.Error("user prompt should contain the standards")
	}
	if !strings.Contains(prompt, "my-code") {
		t.Error("user prompt should contain the code")
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short", "hi", 10, "hi"},
		{"exact", "hello", 5, "hello"},
		{"long", "hello world", 5, "hello..."},
		{"empty", "", 5, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncate(tc.s, tc.max); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
			}
		})
	}
}

func TestCollectGoFiles_SkipsVendor(t *testing.T) {
	dir := createTestDir(t, map[string]string{
		"main.go":         "package main\n",
		"vendor/foo.go":   "package foo\n",
		"node_modules/x.go": "package x\n",
		".git/config.txt": "nothing",
	})

	result, err := collectGoFiles(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "vendor") {
		t.Error("should not contain vendor files")
	}
	if strings.Contains(result, "node_modules") {
		t.Error("should not contain node_modules files")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("should contain main.go")
	}
}

// TestNew_EnvDispatch verifies that New selects the right provider
// based on environment variables.
func TestNew_EnvDispatch(t *testing.T) {
	cases := []struct {
		name       string
		envKey     string
		envVal     string
		wantErr    bool
		errContain string
	}{
		{"OPENAI_API_KEY", "OPENAI_API_KEY", "sk-test", false, ""},
		{"OPENROUTER_API_KEY", "OPENROUTER_API_KEY", "or-test", false, ""},
		{"ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY", "ant-test", true, "anthropic"},
		{"no key", "", "", true, "no LLM API key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear all env vars.
			t.Setenv("OPENAI_API_KEY", "")
			t.Setenv("OPENROUTER_API_KEY", "")
			t.Setenv("ANTHROPIC_API_KEY", "")
			if tc.envKey != "" {
				t.Setenv(tc.envKey, tc.envVal)
			}

			_, err := New(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errContain != "" && !strings.Contains(err.Error(), tc.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContain)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}


