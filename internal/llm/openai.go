package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// openaiProvider implements Provider using the OpenAI-compatible chat
// completions API (works with OpenAI, OpenRouter, and other compatible
// endpoints).
type openaiProvider struct {
	apiKey   string
	baseURL  string
	client   *http.Client
	model    string
}

// newOpenAIProvider returns a provider that talks to the given base URL
// with the provided API key. The model defaults to "gpt-4o" if empty.
func newOpenAIProvider(apiKey, baseURL, model string) *openaiProvider {
	if model == "" {
		model = "gpt-4o"
	}
	return &openaiProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
		model:   model,
	}
}

// --- Chat completions request/response types (minimal) ---

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// violationJSON is the JSON shape the LLM is instructed to return.
type violationJSON struct {
	Rule        string `json:"rule"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Suggestion  string `json:"suggestion,omitempty"`
	RuleDoc     string `json:"ruleDoc,omitempty"`
}

// Audit reads all .go files under target, sends them along with the
// standards to the LLM, and parses the returned violations.
func (p *openaiProvider) Audit(ctx context.Context, standardsBody []byte, target string, changedFiles []string) ([]Violation, error) {
	code, err := collectGoFiles(target, changedFiles)
	if err != nil {
		return nil, fmt.Errorf("collecting source files: %w", err)
	}

	if strings.TrimSpace(code) == "" {
		return nil, nil // no Go files to audit
	}

	systemPrompt := buildSystemPrompt()
	userPrompt := buildUserPrompt(string(standardsBody), code)

	resp, err := p.chat(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM request: %w", err)
	}

	violations, err := parseViolations(resp)
	if err != nil {
		return nil, fmt.Errorf("parsing LLM response: %w", err)
	}
	return violations, nil
}

// chat sends a two-message conversation (system + user) and returns
// the assistant's text reply.
func (p *openaiProvider) chat(ctx context.Context, system, user string) (string, error) {
	body := chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.0,
		MaxTokens:   16384,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("API error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// buildSystemPrompt returns the system prompt that instructs the LLM
// how to perform the audit and what JSON shape to return.
func buildSystemPrompt() string {
	return `You are an expert Go developer and code standards auditor. Your task is to analyze Go source code against a provided set of coding standards and identify violations.

ANALYSIS RULES:
1. Read every line of code carefully against each rule in the standards.
2. Report ALL violations you find — do not suppress or skip any.
3. Be precise with file paths, line numbers, and column numbers.
4. For each violation, provide a clear description and an actionable suggestion.
5. If you find zero violations, return an empty JSON array: []

RESPONSE FORMAT:
You MUST respond with ONLY a JSON array — no markdown fences, no explanation, no extra text. Each element has this shape:

[
  {
    "rule": "rule-id-from-standards",
    "description": "Human-readable explanation of the violation",
    "severity": "info|warn|error|critical",
    "file": "path/to/file.go",
    "line": 42,
    "column": 7,
    "suggestion": "Optional: concrete fix or diff hunk",
    "ruleDoc": "Optional: URL or anchor to rule documentation"
  }
]

SEVERITY GUIDE:
- "info": style or convention preference, not a bug
- "warn": likely issue or anti-pattern, should be fixed
- "error": definite problem that will cause bugs or failures
- "critical": security vulnerability or data-loss risk

RULE FIELD: Use the exact rule id from the standards document. If the standards document does not define rule ids, derive a short kebab-case id from the rule title.

COORDINATES: file must be relative to the project root. line is 1-indexed. column is 1-indexed (byte offset in line). If you cannot determine the exact column, use 1.`
}

// buildUserPrompt assembles the user message containing the standards
// and the collected source code.
func buildUserPrompt(standards, code string) string {
	var b strings.Builder
	b.WriteString("## Standards Document\n\n")
	b.WriteString(standards)
	b.WriteString("\n\n## Source Code\n\n")
	b.WriteString(code)
	b.WriteString("\n\nAnalyze the source code above against the standards. Return the JSON array of violations.")
	return b.String()
}

// parseViolations extracts a []Violation from the raw LLM text output.
// It strips markdown code fences if present and unmarshals the JSON.
func parseViolations(raw string) ([]Violation, error) {
	// Strip markdown code fences if present.
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		// Find opening fence end
		if idx := strings.IndexByte(text, '\n'); idx != -1 {
			text = text[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
	}

	if text == "" || text == "[]" {
		return nil, nil
	}

	var rawViolations []violationJSON
	if err := json.Unmarshal([]byte(text), &rawViolations); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w (raw: %s)", err, truncate(text, 300))
	}

	violations := make([]Violation, 0, len(rawViolations))
	for _, rv := range rawViolations {
		violations = append(violations, Violation{
			Rule:        rv.Rule,
			Description: rv.Description,
			Severity:    parseSeverity(rv.Severity),
			File:        rv.File,
			Line:        rv.Line,
			Column:      rv.Column,
			Suggestion:  rv.Suggestion,
			RuleDoc:     rv.RuleDoc,
		})
	}
	return violations, nil
}

// parseSeverity maps a string severity from the LLM to the Severity enum.
func parseSeverity(s string) Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "warn", "warning":
		return SeverityWarn
	case "error":
		return SeverityError
	case "critical":
		return SeverityCritical
	default:
		return SeverityInfo
	}
}

// collectGoFiles walks target recursively and returns the concatenated
// content of all .go files, prefixed with their relative paths. If
// changedFiles is non-empty, only those files are included.
func collectGoFiles(target string, changedFiles []string) (string, error) {
	skip := map[string]bool{
		"vendor":      true,
		"node_modules": true,
		".git":        true,
	}

	// If specific files are requested, read only those.
	if len(changedFiles) > 0 {
		return collectSpecificFiles(target, changedFiles)
	}

	var b strings.Builder
	err := filepath.Walk(target, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip hidden dirs and known non-project dirs.
		if info.IsDir() {
			base := filepath.Base(path)
			if base != target && skip[base] {
				return filepath.SkipDir
			}
			if strings.HasPrefix(base, ".") && base != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			rel, err := filepath.Rel(target, path)
			if err != nil {
				rel = path
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			fmt.Fprintf(&b, "--- %s ---\n%s\n\n", rel, content)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

// collectSpecificFiles reads only the named files (relative to target).
func collectSpecificFiles(target string, files []string) (string, error) {
	var b strings.Builder
	for _, f := range files {
		full := filepath.Join(target, f)
		content, err := os.ReadFile(full)
		if err != nil {
			// Skip files that don't exist (e.g., deleted).
			continue
		}
		fmt.Fprintf(&b, "--- %s ---\n%s\n\n", f, content)
	}
	return b.String(), nil
}

// truncate shortens s to max runes, appending "..." if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
