package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateFileNameVariations(t *testing.T) {
	fileName := "CODE_OF_CONDUCT.md"
	expected := []string{
		"CODE_OF_CONDUCT.md",
		"CODE_OF_CONDUCT.MD",
		"code_of_conduct.md",
		"CODE-OF-CONDUCT.md",
		"code-of-conduct.md",
		"Code_Of_Conduct.md",
	}

	variations := generateFileNameVariations(fileName)
	assert.ElementsMatch(t, expected, variations, "File name variations do not match expected values")
}

func TestRateLimitWait(t *testing.T) {
	t.Run("waits until reset", func(t *testing.T) {
		resp := &github.Response{
			Rate: github.Rate{
				Remaining: 0,
				Reset:     github.Timestamp{Time: time.Now().Add(2 * time.Second)},
			},
		}

		start := time.Now()
		err := rateLimitWait(context.Background(), resp)
		duration := time.Since(start)

		require.NoError(t, err)
		assert.GreaterOrEqual(t, duration, 2*time.Second, "Expected rate limit wait of at least 2 seconds")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		resp := &github.Response{
			Rate: github.Rate{
				Remaining: 0,
				Reset:     github.Timestamp{Time: time.Now().Add(10 * time.Second)},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := rateLimitWait(ctx, resp)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("no-op when remaining > 0", func(t *testing.T) {
		resp := &github.Response{
			Rate: github.Rate{
				Remaining: 100,
			},
		}

		err := rateLimitWait(context.Background(), resp)
		require.NoError(t, err)
	})

	t.Run("no-op when resp is nil", func(t *testing.T) {
		err := rateLimitWait(context.Background(), nil)
		require.NoError(t, err)
	})
}

func newTestRepoFileCheck() *RepoFileCheck {
	return &RepoFileCheck{
		Owner: "octocat",
		Repo:  "hello-world",
		Files: []FileCheckResult{
			{FileName: "CODE_OF_CONDUCT.md", Found: true, Path: ".github/CODE_OF_CONDUCT.md"},
			{FileName: "CONTRIBUTING.md", Found: false},
			{FileName: "FUNDING.yml", Found: true, Path: "FUNDING.yml"},
			{FileName: "GOVERNANCE.md", Found: false},
			{FileName: "SECURITY.md", Found: true, Path: "docs/SECURITY.md"},
			{FileName: "SUPPORT.md", Found: false, HasError: true},
		},
	}
}

func TestFormatCSVRow(t *testing.T) {
	rfc := newTestRepoFileCheck()
	got := formatCSVRow(rfc)
	expected := "octocat/hello-world,.github/CODE_OF_CONDUCT.md,,FUNDING.yml,,docs/SECURITY.md,error\n"
	assert.Equal(t, expected, got)
}

func TestFormatCSVHeader(t *testing.T) {
	got := formatCSVHeader()
	expected := "Repository,CODE_OF_CONDUCT.md,CONTRIBUTING.md,FUNDING.yml,GOVERNANCE.md,SECURITY.md,SUPPORT.md\n"
	assert.Equal(t, expected, got)
}

func TestFormatJSON(t *testing.T) {
	rfc := newTestRepoFileCheck()
	got, err := formatJSON([]*RepoFileCheck{rfc})
	require.NoError(t, err)

	var parsed []map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &parsed))

	assert.Len(t, parsed, 1)
	assert.Equal(t, "octocat", parsed[0]["owner"])
	assert.Equal(t, "hello-world", parsed[0]["repo"])

	files, ok := parsed[0]["files"].([]any)
	require.True(t, ok)
	assert.Len(t, files, 6)

	first := files[0].(map[string]any)
	assert.Equal(t, "CODE_OF_CONDUCT.md", first["file_name"])
	assert.Equal(t, true, first["found"])
	assert.Equal(t, ".github/CODE_OF_CONDUCT.md", first["path"])
}

func TestFormatMarkdownRow(t *testing.T) {
	rfc := newTestRepoFileCheck()
	got := formatMarkdownRow(rfc)
	expected := "| octocat/hello-world | .github/CODE_OF_CONDUCT.md | | FUNDING.yml | | docs/SECURITY.md | error |\n"
	assert.Equal(t, expected, got)
}

func TestFormatMarkdownHeader(t *testing.T) {
	got := formatMarkdownHeader()
	assert.Contains(t, got, "| Repository |")
	assert.Contains(t, got, "| CODE_OF_CONDUCT.md |")
	assert.Contains(t, got, "|---|")
}
