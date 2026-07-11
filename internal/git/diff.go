package git

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// DiffOptions configures git diff behavior.
type DiffOptions struct {
	// Target is the repository path (default: current directory)
	Target string
	// Since is the Git ref to diff against (e.g., "HEAD~1", "main", "abc123")
	Since string
	// ChangedOnly if true, uses "HEAD~1" as default since
	ChangedOnly bool
}

// ChangedFiles returns the list of files changed between the current state
// and the given ref. Returns relative paths from the repository root.
func ChangedFiles(ctx context.Context, opts DiffOptions) ([]string, error) {
	if opts.Target == "" {
		opts.Target = "."
	}

	repo, err := git.PlainOpen(opts.Target)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	var fromCommit *object.Commit

	if opts.ChangedOnly {
		fromCommit, err = getCommitFromRef(repo, "HEAD~1")
		if err != nil {
			return nil, fmt.Errorf("get HEAD~1: %w", err)
		}
	} else if opts.Since != "" {
		fromCommit, err = getCommitFromRef(repo, opts.Since)
		if err != nil {
			return nil, fmt.Errorf("get since ref %q: %w", opts.Since, err)
		}
	} else {
		return nil, errors.New("git: must specify either --changed or --since <ref>")
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("get HEAD: %w", err)
	}

	toCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("get HEAD commit: %w", err)
	}

	// Get the tree for both commits
	fromTree, err := fromCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("from tree: %w", err)
	}

	toTree, err := toCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("to tree: %w", err)
	}

	// Compute diff
	changes, err := fromTree.Diff(toTree)
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}

	var files []string
	for _, change := range changes {
		action, _ := change.Action()
		if action == merkletrie.Delete {
			files = append(files, change.From.Name)
		} else {
			files = append(files, change.To.Name)
		}
	}

	return files, nil
}

func getCommitFromRef(repo *git.Repository, ref string) (*object.Commit, error) {
	// Handle HEAD~n notation
	if strings.HasPrefix(ref, "HEAD~") {
		n := 1
		if len(ref) > len("HEAD~") {
			if _, err := fmt.Sscanf(ref[len("HEAD~"):], "%d", &n); err != nil {
				return nil, fmt.Errorf("invalid HEAD~n format: %q", ref)
			}
		}
		head, err := repo.Head()
		if err != nil {
			return nil, err
		}
		commit, err := repo.CommitObject(head.Hash())
		if err != nil {
			return nil, err
		}
		for i := 0; i < n; i++ {
			if commit.NumParents() == 0 {
				return nil, fmt.Errorf("no parent commit for HEAD~%d", i+1)
			}
			commit, err = commit.Parent(0)
			if err != nil {
				return nil, err
			}
		}
		return commit, nil
	}

	// Try to resolve as a reference first
	refName := plumbing.ReferenceName(ref)
	if ref, err := repo.Reference(refName, true); err == nil {
		if ref.Type() == plumbing.HashReference {
			return repo.CommitObject(ref.Hash())
		}
		// If it's a symbolic reference, try to resolve
		return repo.CommitObject(ref.Hash())
	}

	// Try as a commit hash
	hash := plumbing.NewHash(ref)
	commit, err := repo.CommitObject(hash)
	if err == nil {
		return commit, nil
	}

	// Try as a tag
	tagRef, err := repo.Tag(ref)
	if err == nil {
		commit, err := repo.CommitObject(tagRef.Hash())
		if err == nil {
			return commit, nil
		}
	}

	// Try as branch name
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(ref), true)
	if err == nil {
		return repo.CommitObject(branchRef.Hash())
	}

	// Try as remote branch
	remoteBranchRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", ref), true)
	if err == nil {
		return repo.CommitObject(remoteBranchRef.Hash())
	}

	return nil, fmt.Errorf("cannot resolve ref %q: not a valid commit, tag, branch, or hash", ref)
}

// IsGitRepo checks if the target directory is a git repository.
func IsGitRepo(target string) bool {
	_, err := git.PlainOpen(target)
	return err == nil
}

// GetHEADCommit returns the current HEAD commit SHA.
func GetHEADCommit(target string) (string, error) {
	repo, err := git.PlainOpen(target)
	if err != nil {
		return "", err
	}
	head, err := repo.Head()
	if err != nil {
		return "", err
	}
	return head.Hash().String(), nil
}

// GetBaseSHA returns the base SHA for PR contexts (GitHub Actions).
// Falls back to HEAD~1 if not in a PR context.
func GetBaseSHA() string {
	// In GitHub Actions, the base SHA is available via the event payload
	// For now, we fall back to HEAD~1
	// In a real implementation, this would read from GITHUB_EVENT_PATH
	return "HEAD~1"
}