package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestValidateAPIURL(t *testing.T) {
	t.Run("accepts default github.com URL", func(t *testing.T) {
		err := validateAPIURL("https://api.github.com")
		assert.NoError(t, err)
	})

	t.Run("accepts GHES URL", func(t *testing.T) {
		err := validateAPIURL("https://github.example.com/api/v3")
		assert.NoError(t, err)
	})

	t.Run("accepts localhost HTTP for testing", func(t *testing.T) {
		err := validateAPIURL("http://localhost:8080")
		assert.NoError(t, err)
	})

	t.Run("accepts 127.0.0.1 HTTP for testing", func(t *testing.T) {
		err := validateAPIURL("http://127.0.0.1:8080")
		assert.NoError(t, err)
	})

	t.Run("rejects HTTP for non-localhost", func(t *testing.T) {
		err := validateAPIURL("http://github.example.com/api/v3")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must use HTTPS")
	})

	t.Run("rejects URL without scheme", func(t *testing.T) {
		err := validateAPIURL("github.example.com/api/v3")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must include scheme and host")
	})

	t.Run("rejects empty string", func(t *testing.T) {
		err := validateAPIURL("")
		require.Error(t, err)
	})
}

func TestPreflightCheck(t *testing.T) {
	t.Run("succeeds with GitHub-like response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-GitHub-Request-Id", "test-id-123")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := preflightCheck(context.Background(), server.URL)
		assert.NoError(t, err)
	})

	t.Run("fails without GitHub header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := preflightCheck(context.Background(), server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not appear to be a GitHub API endpoint")
	})

	t.Run("fails when server is unreachable", func(t *testing.T) {
		err := preflightCheck(context.Background(), "http://localhost:1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "preflight request")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := preflightCheck(ctx, server.URL)
		require.Error(t, err)
	})
}

func TestResolveAPIURL(t *testing.T) {
	t.Run("uses flag value when provided", func(t *testing.T) {
		result, err := resolveAPIURL("https://ghes.example.com/api/v3")
		require.NoError(t, err)
		assert.Equal(t, "https://ghes.example.com/api/v3", result)
	})

	t.Run("falls back to env var", func(t *testing.T) {
		t.Setenv("GITHUB_API_URL", "https://ghes.example.com/api/v3")
		result, err := resolveAPIURL("")
		require.NoError(t, err)
		assert.Equal(t, "https://ghes.example.com/api/v3", result)
	})

	t.Run("defaults to api.github.com", func(t *testing.T) {
		t.Setenv("GITHUB_API_URL", "")
		result, err := resolveAPIURL("")
		require.NoError(t, err)
		assert.Equal(t, "https://api.github.com", result)
	})

	t.Run("strips trailing slash", func(t *testing.T) {
		result, err := resolveAPIURL("https://ghes.example.com/api/v3/")
		require.NoError(t, err)
		assert.Equal(t, "https://ghes.example.com/api/v3", result)
	})

	t.Run("rejects invalid URL", func(t *testing.T) {
		_, err := resolveAPIURL("http://evil.example.com")
		require.Error(t, err)
	})

	t.Run("flag takes precedence over env", func(t *testing.T) {
		t.Setenv("GITHUB_API_URL", "https://env.example.com/api/v3")
		result, err := resolveAPIURL("https://flag.example.com/api/v3")
		require.NoError(t, err)
		assert.Equal(t, "https://flag.example.com/api/v3", result)
	})
}

func TestProcessRepoBatchInaccessible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"data": {
				"repo0": null,
				"repo1": {
					"root": {"entries": [{"name": "README.md"}]},
					"dotGithub": null,
					"docs": null
				},
				"repo2": null,
				"rateLimit": {"remaining": 4999, "resetAt": "2026-01-01T00:00:00Z"}
			}
		}`)
	}))
	defer server.Close()

	origURL := graphQLURL
	defer func() { setGraphQLURL(origURL) }()
	setGraphQLURL(server.URL)

	repos := []repoInput{
		{Owner: "org", Repo: "private-repo"},
		{Owner: "org", Repo: "public-repo"},
		{Owner: "org", Repo: "deleted-repo"},
	}

	var inaccessible []string
	results, err := processRepoBatch(context.Background(), server.Client(), repos, &inaccessible)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	assert.Equal(t, []string{"org/private-repo", "org/deleted-repo"}, inaccessible)

	// Inaccessible repos should have all files marked as errors
	for _, f := range results[0].Files {
		assert.True(t, f.HasError, "inaccessible repo files should have HasError=true")
	}

	// Accessible repo should have normal results
	assert.False(t, results[1].Files[0].HasError)
}

func TestPreflightCheckDoesNotSendToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		assert.Empty(t, auth, "preflight request must not include Authorization header")
		w.Header().Set("X-GitHub-Request-Id", "test-id")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := preflightCheck(context.Background(), server.URL)
	assert.NoError(t, err)
}

func TestInaccessibleRepoSummaryFormat(t *testing.T) {
	inaccessible := []string{"org/repo1", "org/repo2", "other/repo3"}

	var b strings.Builder
	fmt.Fprintf(&b, "\nThe following %d repository(ies) were not accessible (check token permissions or repository existence):\n", len(inaccessible))
	for _, repo := range inaccessible {
		fmt.Fprintf(&b, "  - %s\n", repo)
	}
	fmt.Fprintln(&b)

	output := b.String()
	assert.Contains(t, output, "3 repository(ies)")
	assert.Contains(t, output, "  - org/repo1")
	assert.Contains(t, output, "  - org/repo2")
	assert.Contains(t, output, "  - other/repo3")
}

func TestFilterRepos(t *testing.T) {
	nodes := []repoNode{
		{Name: "active-repo", Owner: struct {
			Login string `json:"login"`
		}{Login: "myorg"}, IsArchived: false, IsFork: false},
		{Name: "archived-repo", Owner: struct {
			Login string `json:"login"`
		}{Login: "myorg"}, IsArchived: true, IsFork: false},
		{Name: "forked-repo", Owner: struct {
			Login string `json:"login"`
		}{Login: "myorg"}, IsArchived: false, IsFork: true},
		{Name: "archived-fork", Owner: struct {
			Login string `json:"login"`
		}{Login: "myorg"}, IsArchived: true, IsFork: true},
		{Name: "another-active", Owner: struct {
			Login string `json:"login"`
		}{Login: "myorg"}, IsArchived: false, IsFork: false},
	}

	result := filterRepos(nodes)
	assert.Len(t, result, 2)
	assert.Equal(t, "active-repo", result[0].Repo)
	assert.Equal(t, "myorg", result[0].Owner)
	assert.Equal(t, "another-active", result[1].Repo)
}

func TestFilterReposEmpty(t *testing.T) {
	result := filterRepos(nil)
	assert.Nil(t, result)
}

func TestBuildOrgReposQuery(t *testing.T) {
	t.Run("without cursor", func(t *testing.T) {
		query := buildOrgReposQuery("myorg", "")
		assert.Contains(t, query, `organization(login: "myorg")`)
		assert.Contains(t, query, "repositories(first: 100)")
		assert.NotContains(t, query, "after:")
		assert.Contains(t, query, "rateLimit")
	})

	t.Run("with cursor", func(t *testing.T) {
		query := buildOrgReposQuery("myorg", "abc123")
		assert.Contains(t, query, `after: "abc123"`)
	})
}

func TestListOrgRepos(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			_, _ = fmt.Fprint(w, `{
				"data": {
					"organization": {
						"repositories": {
							"pageInfo": {"hasNextPage": true, "endCursor": "cursor1"},
							"nodes": [
								{"name": "repo1", "owner": {"login": "myorg"}, "isArchived": false, "isFork": false},
								{"name": "archived", "owner": {"login": "myorg"}, "isArchived": true, "isFork": false},
								{"name": "fork", "owner": {"login": "myorg"}, "isArchived": false, "isFork": true}
							]
						}
					},
					"rateLimit": {"remaining": 4999, "resetAt": "2026-01-01T00:00:00Z"}
				}
			}`)
		} else {
			_, _ = fmt.Fprint(w, `{
				"data": {
					"organization": {
						"repositories": {
							"pageInfo": {"hasNextPage": false, "endCursor": ""},
							"nodes": [
								{"name": "repo2", "owner": {"login": "myorg"}, "isArchived": false, "isFork": false}
							]
						}
					},
					"rateLimit": {"remaining": 4998, "resetAt": "2026-01-01T00:00:00Z"}
				}
			}`)
		}
	}))
	defer server.Close()

	origURL := graphQLURL
	defer func() { setGraphQLURL(origURL) }()
	setGraphQLURL(server.URL)

	repos, err := listOrgRepos(context.Background(), server.Client(), "myorg")
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "should paginate with 2 requests")
	assert.Len(t, repos, 2)
	assert.Equal(t, "repo1", repos[0].Repo)
	assert.Equal(t, "repo2", repos[1].Repo)
}

func TestListOrgReposNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"data": {
				"organization": null,
				"rateLimit": {"remaining": 4999, "resetAt": "2026-01-01T00:00:00Z"}
			}
		}`)
	}))
	defer server.Close()

	origURL := graphQLURL
	defer func() { setGraphQLURL(origURL) }()
	setGraphQLURL(server.URL)

	_, err := listOrgRepos(context.Background(), server.Client(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or not accessible")
}

func TestReadReposFromFile(t *testing.T) {
	t.Run("reads valid file", func(t *testing.T) {
		tmpFile := t.TempDir() + "/repos.txt"
		require.NoError(t, os.WriteFile(tmpFile, []byte("owner1/repo1\nowner2/repo2\n"), 0o644))

		repos, err := readReposFromFile(tmpFile)
		require.NoError(t, err)
		assert.Len(t, repos, 2)
		assert.Equal(t, "owner1", repos[0].Owner)
		assert.Equal(t, "repo1", repos[0].Repo)
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := readReposFromFile("/nonexistent/path")
		require.Error(t, err)
	})
}
