package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v72/github"
	"golang.org/x/oauth2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var communityHealthFilePaths = []string{
	"",
	".github/",
	"docs/",
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

func checkFile(tree *github.Tree, filePath string) (bool, string) {
	variations := generateFileNameVariations(filePath)

	for _, variation := range variations {
		for _, entry := range tree.Entries {
			if entry.GetPath() == variation {
				return true, variation
			}
		}
	}

	return false, ""
}

func rateLimitWait(ctx context.Context, resp *github.Response) error {
	if resp == nil || resp.Rate.Remaining > 0 {
		return nil
	}

	resetTime := time.Until(resp.Rate.Reset.Time)
	if resetTime <= 0 {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Rate limit exceeded. Waiting for %v...\n", resetTime)

	select {
	case <-time.After(resetTime):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func getRepoFileCheck(ctx context.Context, client *github.Client, owner, repo string) (*RepoFileCheck, error) {
	result := &RepoFileCheck{
		Owner: owner,
		Repo:  repo,
	}

	tree, resp, err := client.Git.GetTree(ctx, owner, repo, "HEAD", true)
	if err := rateLimitWait(ctx, resp); err != nil {
		return nil, err
	}
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("repository %s/%s not found", owner, repo)
		}
		return nil, fmt.Errorf("getting repo tree for %s/%s: %w", owner, repo, err)
	}

	var orgTree *github.Tree
	orgTreeFetched := false

	for _, chf := range communityHealthFiles {
		fileResult := FileCheckResult{
			FileName: chf,
		}

		for _, basePath := range communityHealthFilePaths {
			found, foundPath := checkFile(tree, basePath+chf)
			if found {
				fileResult.Found = true
				fileResult.Path = foundPath
				break
			}
		}

		if !fileResult.Found {
			if !orgTreeFetched {
				orgTreeFetched = true
				orgTree, resp, err = client.Git.GetTree(ctx, owner, ".github", "HEAD", true)
				if err := rateLimitWait(ctx, resp); err != nil {
					return nil, err
				}
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not fetch org .github repo for %s: %v\n", owner, err)
				}
			}

			if orgTree != nil {
				found, foundPath := checkFile(orgTree, ".github/"+chf)
				if found {
					fileResult.Found = true
					fileResult.Path = owner + "/" + foundPath
				}
			}
		}

		result.Files = append(result.Files, fileResult)
	}

	return result, nil
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
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

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

	var results []*RepoFileCheck

	scanner := bufio.NewScanner(file)

	switch *format {
	case "csv":
		fmt.Print(formatCSVHeader())
	case "markdown":
		fmt.Print(formatMarkdownHeader())
	}

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "/")
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid format: %s. Expected owner/repo.\n", line)
			continue
		}
		owner, repo := parts[0], parts[1]

		rfc, err := getRepoFileCheck(ctx, client, owner, repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s/%s: %v\n", owner, repo, err)
			continue
		}

		switch *format {
		case "csv":
			fmt.Print(formatCSVRow(rfc))
		case "markdown":
			fmt.Print(formatMarkdownRow(rfc))
		case "json":
			results = append(results, rfc)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	if *format == "json" {
		out, err := formatJSON(results)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(out)
	}
}
