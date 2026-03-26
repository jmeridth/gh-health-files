package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	batchSize    = 20
	treeFragment = `{ ... on Tree { entries { name } } }`
)

var graphQLURL = "https://api.github.com/graphql"

func setGraphQLURL(url string) {
	graphQLURL = url
}

var communityHealthFiles = []string{
	"CODE_OF_CONDUCT.md",
	"CONTRIBUTING.md",
	"FUNDING.yml",
	"GOVERNANCE.md",
	"SECURITY.md",
	"SUPPORT.md",
}

type FileCheckResult struct {
	FileName string `json:"file_name"`
	Found    bool   `json:"found"`
	Path     string `json:"path,omitempty"`
	HasError bool   `json:"has_error,omitempty"`
}

type RepoFileCheck struct {
	Owner string            `json:"owner"`
	Repo  string            `json:"repo"`
	Files []FileCheckResult `json:"files"`
}

func (rfc *RepoFileCheck) Repository() string {
	return rfc.Owner + "/" + rfc.Repo
}

func (rfc *RepoFileCheck) hasMissingFiles() bool {
	for _, f := range rfc.Files {
		if !f.Found && !f.HasError {
			return true
		}
	}
	return false
}

type repoInput struct {
	Owner string
	Repo  string
}

// GraphQL types

type graphQLRequest struct {
	Query string `json:"query"`
}

type graphQLResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []graphQLError             `json:"errors"`
}

type graphQLError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type treeEntry struct {
	Name string `json:"name"`
}

type treeResult struct {
	Entries []treeEntry `json:"entries"`
}

type repoTrees struct {
	Root      *treeResult `json:"root"`
	DotGithub *treeResult `json:"dotGithub"`
	Docs      *treeResult `json:"docs"`
}

type rateLimitResult struct {
	Remaining int    `json:"remaining"`
	ResetAt   string `json:"resetAt"`
}

// Filename variation generation

func generateFileNameVariations(fileName string) []string {
	seen := make(map[string]struct{})
	var result []string

	add := func(s string) {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}

	add(fileName)

	if strings.Contains(fileName, "_") {
		add(strings.ReplaceAll(fileName, "_", "-"))
		add(strings.ReplaceAll(strings.ToLower(fileName), "_", "-"))
	}

	if strings.Contains(fileName, "-") {
		add(strings.ReplaceAll(fileName, "-", "_"))
		add(strings.ReplaceAll(strings.ToLower(fileName), "-", "_"))
	}

	add(strings.ToLower(fileName))
	add(strings.ToUpper(fileName))

	if strings.Contains(fileName, "_") || strings.Contains(fileName, "-") {
		titleCaser := cases.Title(language.English)
		titleCase := strings.ReplaceAll(titleCaser.String(strings.ReplaceAll(fileName, "_", " ")), " ", "_")
		add(titleCase)
	}

	return result
}

func checkFileInEntries(entries []treeEntry, fileName string) (bool, string) {
	variations := generateFileNameVariations(fileName)
	for _, v := range variations {
		for _, e := range entries {
			if e.Name == v {
				return true, v
			}
		}
	}
	return false, ""
}

// GraphQL query construction

func buildRepoQuery(repos []repoInput) string {
	var b strings.Builder
	b.WriteString("query {\n")
	for i, r := range repos {
		fmt.Fprintf(&b, "  repo%d: repository(owner: %q, name: %q) {\n", i, r.Owner, r.Repo)
		fmt.Fprintf(&b, "    root: object(expression: \"HEAD:\") %s\n", treeFragment)
		fmt.Fprintf(&b, "    dotGithub: object(expression: \"HEAD:.github\") %s\n", treeFragment)
		fmt.Fprintf(&b, "    docs: object(expression: \"HEAD:docs\") %s\n", treeFragment)
		b.WriteString("  }\n")
	}
	b.WriteString("  rateLimit { remaining resetAt }\n")
	b.WriteString("}\n")
	return b.String()
}

func buildOrgQuery(owners []string) string {
	var b strings.Builder
	b.WriteString("query {\n")
	for i, owner := range owners {
		fmt.Fprintf(&b, "  org%d: repository(owner: %q, name: \".github\") {\n", i, owner)
		fmt.Fprintf(&b, "    root: object(expression: \"HEAD:\") %s\n", treeFragment)
		fmt.Fprintf(&b, "    dotGithub: object(expression: \"HEAD:.github\") %s\n", treeFragment)
		fmt.Fprintf(&b, "    docs: object(expression: \"HEAD:docs\") %s\n", treeFragment)
		b.WriteString("  }\n")
	}
	b.WriteString("  rateLimit { remaining resetAt }\n")
	b.WriteString("}\n")
	return b.String()
}

// GraphQL execution

func executeGraphQL(ctx context.Context, httpClient *http.Client, query string) (*graphQLResponse, error) {
	reqBody, err := json.Marshal(graphQLRequest{Query: query})
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphQLURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var result graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

func handleRateLimit(ctx context.Context, resp *graphQLResponse) error {
	raw, ok := resp.Data["rateLimit"]
	if !ok {
		return nil
	}

	var rl rateLimitResult
	if err := json.Unmarshal(raw, &rl); err != nil {
		return nil
	}

	if rl.Remaining > 0 {
		return nil
	}

	resetAt, err := time.Parse(time.RFC3339, rl.ResetAt)
	if err != nil {
		return nil
	}

	wait := time.Until(resetAt)
	if wait <= 0 {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Rate limit exceeded. Waiting for %v...\n", wait)

	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Result processing

type dirEntries struct {
	prefix  string
	entries []treeEntry
}

func dirsFromTrees(trees *repoTrees) []dirEntries {
	var dirs []dirEntries
	if trees.Root != nil {
		dirs = append(dirs, dirEntries{"", trees.Root.Entries})
	}
	if trees.DotGithub != nil {
		dirs = append(dirs, dirEntries{".github/", trees.DotGithub.Entries})
	}
	if trees.Docs != nil {
		dirs = append(dirs, dirEntries{"docs/", trees.Docs.Entries})
	}
	return dirs
}

func processRepoResult(owner, repo string, trees *repoTrees) *RepoFileCheck {
	result := &RepoFileCheck{Owner: owner, Repo: repo}
	dirs := dirsFromTrees(trees)

	for _, chf := range communityHealthFiles {
		fileResult := FileCheckResult{FileName: chf}

		for _, dir := range dirs {
			if found, name := checkFileInEntries(dir.entries, chf); found {
				fileResult.Found = true
				fileResult.Path = dir.prefix + name
				break
			}
		}

		result.Files = append(result.Files, fileResult)
	}

	return result
}

func applyOrgFallback(result *RepoFileCheck, orgTrees *repoTrees) {
	if orgTrees == nil {
		return
	}
	dirs := dirsFromTrees(orgTrees)

	for i, file := range result.Files {
		if file.Found || file.HasError {
			continue
		}
		for _, dir := range dirs {
			if found, name := checkFileInEntries(dir.entries, file.FileName); found {
				result.Files[i].Found = true
				result.Files[i].Path = result.Owner + "/.github/" + dir.prefix + name
				break
			}
		}
	}
}

func errorResult(owner, repo string) *RepoFileCheck {
	result := &RepoFileCheck{Owner: owner, Repo: repo}
	for _, chf := range communityHealthFiles {
		result.Files = append(result.Files, FileCheckResult{FileName: chf, HasError: true})
	}
	return result
}

// Batch processing

func processRepoBatch(ctx context.Context, httpClient *http.Client, repos []repoInput) ([]*RepoFileCheck, error) {
	query := buildRepoQuery(repos)
	resp, err := executeGraphQL(ctx, httpClient, query)
	if err != nil {
		return nil, err
	}

	if err := handleRateLimit(ctx, resp); err != nil {
		return nil, err
	}

	var results []*RepoFileCheck
	for i, repo := range repos {
		key := fmt.Sprintf("repo%d", i)
		raw, ok := resp.Data[key]
		if !ok || string(raw) == "null" {
			fmt.Fprintf(os.Stderr, "Warning: repository %s/%s not found or not accessible\n", repo.Owner, repo.Repo)
			results = append(results, errorResult(repo.Owner, repo.Repo))
			continue
		}

		var trees repoTrees
		if err := json.Unmarshal(raw, &trees); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse result for %s/%s: %v\n", repo.Owner, repo.Repo, err)
			results = append(results, errorResult(repo.Owner, repo.Repo))
			continue
		}

		results = append(results, processRepoResult(repo.Owner, repo.Repo, &trees))
	}

	return results, nil
}

func processOrgFallback(ctx context.Context, httpClient *http.Client, results []*RepoFileCheck) error {
	ownerSet := make(map[string]struct{})
	for _, r := range results {
		if r.hasMissingFiles() {
			ownerSet[r.Owner] = struct{}{}
		}
	}

	if len(ownerSet) == 0 {
		return nil
	}

	var owners []string
	for owner := range ownerSet {
		owners = append(owners, owner)
	}

	// Batch org queries
	orgTrees := make(map[string]*repoTrees)

	for start := 0; start < len(owners); start += batchSize {
		end := min(start+batchSize, len(owners))
		batch := owners[start:end]

		query := buildOrgQuery(batch)
		resp, err := executeGraphQL(ctx, httpClient, query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to query org .github repos: %v\n", err)
			return nil
		}

		if err := handleRateLimit(ctx, resp); err != nil {
			return err
		}

		for i, owner := range batch {
			key := fmt.Sprintf("org%d", i)
			raw, ok := resp.Data[key]
			if !ok || string(raw) == "null" {
				continue
			}

			var trees repoTrees
			if err := json.Unmarshal(raw, &trees); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse org .github for %s: %v\n", owner, err)
				continue
			}
			orgTrees[owner] = &trees
		}
	}

	for _, r := range results {
		if trees, ok := orgTrees[r.Owner]; ok {
			applyOrgFallback(r, trees)
		}
	}

	return nil
}

// Output formatting

func formatCSVHeader() string {
	var builder strings.Builder
	builder.WriteString("Repository,")
	for _, chf := range communityHealthFiles {
		builder.WriteString(chf)
		builder.WriteByte(',')
	}
	return strings.TrimSuffix(builder.String(), ",") + "\n"
}

func formatCSVRow(rfc *RepoFileCheck) string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "%s,", rfc.Repository())

	for _, file := range rfc.Files {
		if file.HasError {
			builder.WriteString("error")
		} else if file.Found {
			builder.WriteString(file.Path)
		}
		builder.WriteByte(',')
	}

	return strings.TrimSuffix(builder.String(), ",") + "\n"
}

func formatJSON(results []*RepoFileCheck) (string, error) {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling JSON: %w", err)
	}
	return string(data) + "\n", nil
}

func formatMarkdownHeader() string {
	var header, separator strings.Builder

	header.WriteString("| Repository |")
	separator.WriteString("|---|")

	for _, chf := range communityHealthFiles {
		fmt.Fprintf(&header, " %s |", chf)
		separator.WriteString("---|")
	}

	return header.String() + "\n" + separator.String() + "\n"
}

func formatMarkdownRow(rfc *RepoFileCheck) string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "| %s |", rfc.Repository())

	for _, file := range rfc.Files {
		switch {
		case file.HasError:
			builder.WriteString(" error |")
		case file.Found:
			fmt.Fprintf(&builder, " %s |", file.Path)
		default:
			builder.WriteString(" |")
		}
	}

	return builder.String() + "\n"
}

func main() {
	format := flag.String("format", "csv", "output format: csv, json, or markdown")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: community-health-file-checker [--format csv|json|markdown] <input_file>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	switch *format {
	case "csv", "json", "markdown":
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %s. Must be csv, json, or markdown.\n", *format)
		os.Exit(1)
	}

	inputFile := flag.Arg(0)
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "Please set the GITHUB_TOKEN environment variable.")
		os.Exit(1)
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)

	file, err := os.Open(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing file: %v\n", err)
		}
	}()

	var repos []repoInput
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "/")
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid format: %s. Expected owner/repo.\n", line)
			continue
		}
		repos = append(repos, repoInput{Owner: parts[0], Repo: parts[1]})
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var allResults []*RepoFileCheck

	for start := 0; start < len(repos); start += batchSize {
		end := min(start+batchSize, len(repos))

		results, err := processRepoBatch(ctx, httpClient, repos[start:end])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing batch: %v\n", err)
			os.Exit(1)
		}
		allResults = append(allResults, results...)
	}

	if err := processOrgFallback(ctx, httpClient, allResults); err != nil {
		fmt.Fprintf(os.Stderr, "Error processing org fallback: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "csv":
		fmt.Print(formatCSVHeader())
		for _, r := range allResults {
			fmt.Print(formatCSVRow(r))
		}
	case "markdown":
		fmt.Print(formatMarkdownHeader())
		for _, r := range allResults {
			fmt.Print(formatMarkdownRow(r))
		}
	case "json":
		out, err := formatJSON(allResults)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(out)
	}
}
