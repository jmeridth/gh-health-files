package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestCheckFileInEntries(t *testing.T) {
	entries := []treeEntry{
		{Name: "README.md"},
		{Name: "code-of-conduct.md"},
		{Name: "CONTRIBUTING.md"},
	}

	t.Run("finds exact match", func(t *testing.T) {
		found, name := checkFileInEntries(entries, "CONTRIBUTING.md")
		assert.True(t, found)
		assert.Equal(t, "CONTRIBUTING.md", name)
	})

	t.Run("finds variation match", func(t *testing.T) {
		found, name := checkFileInEntries(entries, "CODE_OF_CONDUCT.md")
		assert.True(t, found)
		assert.Equal(t, "code-of-conduct.md", name)
	})

	t.Run("returns false when not found", func(t *testing.T) {
		found, name := checkFileInEntries(entries, "SECURITY.md")
		assert.False(t, found)
		assert.Empty(t, name)
	})

	t.Run("handles empty entries", func(t *testing.T) {
		found, name := checkFileInEntries(nil, "SECURITY.md")
		assert.False(t, found)
		assert.Empty(t, name)
	})
}

func TestProcessRepoResult(t *testing.T) {
	trees := &repoTrees{
		Root: &treeResult{
			Entries: []treeEntry{
				{Name: "README.md"},
				{Name: "FUNDING.yml"},
			},
		},
		DotGithub: &treeResult{
			Entries: []treeEntry{
				{Name: "CODE_OF_CONDUCT.md"},
				{Name: "SECURITY.md"},
			},
		},
		Docs: &treeResult{
			Entries: []treeEntry{
				{Name: "CONTRIBUTING.md"},
			},
		},
	}

	result := processRepoResult("octocat", "hello-world", trees)

	assert.Equal(t, "octocat", result.Owner)
	assert.Equal(t, "hello-world", result.Repo)
	assert.Len(t, result.Files, 6)

	expected := map[string]string{
		"CODE_OF_CONDUCT.md": ".github/CODE_OF_CONDUCT.md",
		"CONTRIBUTING.md":    "docs/CONTRIBUTING.md",
		"FUNDING.yml":        "FUNDING.yml",
		"SECURITY.md":        ".github/SECURITY.md",
	}

	for _, f := range result.Files {
		if path, ok := expected[f.FileName]; ok {
			assert.True(t, f.Found, "expected %s to be found", f.FileName)
			assert.Equal(t, path, f.Path)
		} else {
			assert.False(t, f.Found, "expected %s to not be found", f.FileName)
		}
	}
}

func TestProcessRepoResultNilTrees(t *testing.T) {
	trees := &repoTrees{}

	result := processRepoResult("octocat", "empty-repo", trees)
	assert.Len(t, result.Files, 6)
	for _, f := range result.Files {
		assert.False(t, f.Found)
	}
}

func TestApplyOrgFallback(t *testing.T) {
	result := &RepoFileCheck{
		Owner: "octocat",
		Repo:  "hello-world",
		Files: []FileCheckResult{
			{FileName: "CODE_OF_CONDUCT.md", Found: true, Path: "CODE_OF_CONDUCT.md"},
			{FileName: "CONTRIBUTING.md", Found: false},
			{FileName: "FUNDING.yml", Found: false},
			{FileName: "GOVERNANCE.md", Found: false},
			{FileName: "SECURITY.md", Found: false},
			{FileName: "SUPPORT.md", Found: false, HasError: true},
		},
	}

	orgTrees := &repoTrees{
		Root: &treeResult{
			Entries: []treeEntry{
				{Name: "CONTRIBUTING.md"},
				{Name: "SECURITY.md"},
			},
		},
	}

	applyOrgFallback(result, orgTrees)

	assert.True(t, result.Files[0].Found, "already found should stay found")
	assert.Equal(t, "CODE_OF_CONDUCT.md", result.Files[0].Path, "already found path should not change")

	assert.True(t, result.Files[1].Found, "CONTRIBUTING.md should be found via org fallback")
	assert.Equal(t, "octocat/.github/CONTRIBUTING.md", result.Files[1].Path)

	assert.False(t, result.Files[2].Found, "FUNDING.yml not in org, should stay not found")

	assert.True(t, result.Files[4].Found, "SECURITY.md should be found via org fallback")
	assert.Equal(t, "octocat/.github/SECURITY.md", result.Files[4].Path)

	assert.True(t, result.Files[5].HasError, "HasError should not be overwritten by fallback")
	assert.False(t, result.Files[5].Found)
}

func TestApplyOrgFallbackNil(t *testing.T) {
	result := &RepoFileCheck{
		Owner: "octocat",
		Repo:  "test",
		Files: []FileCheckResult{
			{FileName: "SECURITY.md", Found: false},
		},
	}
	applyOrgFallback(result, nil)
	assert.False(t, result.Files[0].Found, "nil org trees should be no-op")
}

func TestBuildRepoQuery(t *testing.T) {
	repos := []repoInput{
		{Owner: "octocat", Repo: "hello-world"},
		{Owner: "github", Repo: "docs"},
	}

	query := buildRepoQuery(repos)

	assert.Contains(t, query, `repo0: repository(owner: "octocat", name: "hello-world")`)
	assert.Contains(t, query, `repo1: repository(owner: "github", name: "docs")`)
	assert.Contains(t, query, `root: object(expression: "HEAD:")`)
	assert.Contains(t, query, `dotGithub: object(expression: "HEAD:.github")`)
	assert.Contains(t, query, `docs: object(expression: "HEAD:docs")`)
	assert.Contains(t, query, "rateLimit")
}

func TestBuildOrgQuery(t *testing.T) {
	owners := []string{"octocat", "github"}
	query := buildOrgQuery(owners)

	assert.Contains(t, query, `org0: repository(owner: "octocat", name: ".github")`)
	assert.Contains(t, query, `org1: repository(owner: "github", name: ".github")`)
	assert.Contains(t, query, "rateLimit")
}

func TestExecuteGraphQL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, http.MethodPost, r.Method)

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"data": {
				"repo0": {
					"root": {"entries": [{"name": "README.md"}, {"name": "CODE_OF_CONDUCT.md"}]},
					"dotGithub": {"entries": [{"name": "SECURITY.md"}]},
					"docs": null
				},
				"rateLimit": {"remaining": 4999, "resetAt": "2026-01-01T00:00:00Z"}
			}
		}`)
	}))
	defer server.Close()

	// Override the URL for testing
	origURL := graphQLURL
	defer func() { setGraphQLURL(origURL) }()
	setGraphQLURL(server.URL)

	resp, err := executeGraphQL(context.Background(), server.Client(), buildRepoQuery([]repoInput{{Owner: "test", Repo: "repo"}}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	raw := resp.Data["repo0"]
	var trees repoTrees
	require.NoError(t, json.Unmarshal(raw, &trees))

	assert.NotNil(t, trees.Root)
	assert.Len(t, trees.Root.Entries, 2)
	assert.NotNil(t, trees.DotGithub)
	assert.Len(t, trees.DotGithub.Entries, 1)
	assert.Nil(t, trees.Docs)
}

func TestHandleRateLimit(t *testing.T) {
	t.Run("no-op when remaining > 0", func(t *testing.T) {
		resp := &graphQLResponse{
			Data: map[string]json.RawMessage{
				"rateLimit": json.RawMessage(`{"remaining": 100, "resetAt": "2026-01-01T00:00:00Z"}`),
			},
		}
		err := handleRateLimit(context.Background(), resp)
		require.NoError(t, err)
	})

	t.Run("no-op when no rateLimit in response", func(t *testing.T) {
		resp := &graphQLResponse{
			Data: map[string]json.RawMessage{},
		}
		err := handleRateLimit(context.Background(), resp)
		require.NoError(t, err)
	})

	t.Run("waits when remaining is 0", func(t *testing.T) {
		// Add 3s and truncate to next second to ensure at least 2s wait
		// despite RFC3339 having only second precision
		resetAt := time.Now().Add(3 * time.Second).Truncate(time.Second).UTC().Format(time.RFC3339)
		resp := &graphQLResponse{
			Data: map[string]json.RawMessage{
				"rateLimit": json.RawMessage(fmt.Sprintf(`{"remaining": 0, "resetAt": %q}`, resetAt)),
			},
		}

		start := time.Now()
		err := handleRateLimit(context.Background(), resp)
		duration := time.Since(start)

		require.NoError(t, err)
		assert.GreaterOrEqual(t, duration, 2*time.Second, "Expected rate limit wait of at least 2 seconds")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		resetAt := time.Now().Add(10 * time.Second).UTC().Format(time.RFC3339)
		resp := &graphQLResponse{
			Data: map[string]json.RawMessage{
				"rateLimit": json.RawMessage(fmt.Sprintf(`{"remaining": 0, "resetAt": %q}`, resetAt)),
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := handleRateLimit(ctx, resp)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestHasMissingFiles(t *testing.T) {
	t.Run("all found", func(t *testing.T) {
		rfc := &RepoFileCheck{
			Files: []FileCheckResult{
				{Found: true},
				{Found: true},
			},
		}
		assert.False(t, rfc.hasMissingFiles())
	})

	t.Run("has missing", func(t *testing.T) {
		rfc := &RepoFileCheck{
			Files: []FileCheckResult{
				{Found: true},
				{Found: false},
			},
		}
		assert.True(t, rfc.hasMissingFiles())
	})

	t.Run("error is not missing", func(t *testing.T) {
		rfc := &RepoFileCheck{
			Files: []FileCheckResult{
				{Found: true},
				{Found: false, HasError: true},
			},
		}
		assert.False(t, rfc.hasMissingFiles())
	})
}

// Output formatting tests

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
