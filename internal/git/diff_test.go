package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestChangedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	_, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Create initial files
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("package main\n"), 0644)
	commitAll(t, tmpDir, "initial commit")

	// Modify file1 and add file3
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("package main\n\nfunc Foo() {}\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file3.go"), []byte("package main\n"), 0644)
	commitAll(t, tmpDir, "modify file1, add file3")

	// Test ChangedOnly (HEAD~1)
	ctx := context.Background()
	files, err := ChangedFiles(ctx, DiffOptions{
		Target:       tmpDir,
		ChangedOnly:  true,
	})
	if err != nil {
		t.Fatalf("ChangedFiles failed: %v", err)
	}

	expected := map[string]bool{"file1.go": true, "file3.go": true}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if !expected[f] {
			t.Errorf("unexpected file: %s", f)
		}
	}

	// Test --since with specific commit
	files2, err := ChangedFiles(context.Background(), DiffOptions{
		Target: tmpDir,
		Since:  "HEAD~1",
	})
	if err != nil {
		t.Fatalf("ChangedFiles with --since failed: %v", err)
	}
	if len(files2) != 2 {
		t.Errorf("expected 2 files with --since HEAD~1, got %d: %v", len(files2), files2)
	}

	// Test --since with explicit commit hash
	head, err := GetHEADCommit(tmpDir)
	if err != nil {
		t.Fatalf("GetHEADCommit failed: %v", err)
	}
	files3, err := ChangedFiles(context.Background(), DiffOptions{
		Target: tmpDir,
		Since:  head,
	})
	if err != nil {
		t.Fatalf("ChangedFiles with commit hash failed: %v", err)
	}
	if len(files3) != 0 {
		t.Errorf("expected 0 files since HEAD, got %d: %v", len(files3), files3)
	}
}

func TestChangedFiles_NoGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	// Not a git repo
	_, err := ChangedFiles(context.Background(), DiffOptions{
		Target:       tmpDir,
		ChangedOnly:  true,
	})
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestChangedFiles_InvalidOptions(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Neither --changed nor --since
	_, err := ChangedFiles(context.Background(), DiffOptions{
		Target: tmpDir,
	})
	if err == nil {
		t.Error("expected error when neither --changed nor --since specified")
	}
}

func TestChangedFiles_WithSinceRef(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create initial files
	os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("b"), 0644)
	commitAll(t, tmpDir, "initial")

	// Modify a.go
	os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("a modified"), 0644)
	commitAll(t, tmpDir, "modify a")

	// Test --since with branch name
	files, err := ChangedFiles(context.Background(), DiffOptions{
		Target: tmpDir,
		Since:  "HEAD~1",
	})
	if err != nil {
		t.Fatalf("ChangedFiles failed: %v", err)
	}
	if len(files) != 1 || files[0] != "a.go" {
		t.Errorf("expected [a.go], got %v", files)
	}
}

func TestIsGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	if IsGitRepo(tmpDir) {
		t.Error("expected non-git dir to return false")
	}
	initGitRepo(t, tmpDir)
	if !IsGitRepo(tmpDir) {
		t.Error("expected git dir to return true")
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init failed: %v", err)
	}
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	_, err = worktree.Add(".")
	if err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	_, err = worktree.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
		},
	})
	if err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
}

func TestGetHEADCommit(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("a"), 0644)
	commitAll(t, tmpDir, "initial")

	sha, err := GetHEADCommit(tmpDir)
	if err != nil {
		t.Fatalf("GetHEADCommit failed: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA, got %q", sha)
	}
}

func TestGetBaseSHA(t *testing.T) {
	sha := GetBaseSHA()
	if sha != "HEAD~1" {
		t.Errorf("expected HEAD~1, got %s", sha)
	}
}