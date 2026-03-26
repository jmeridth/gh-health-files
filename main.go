package main

import (
	"bufio"
	"context"
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
	FileName string
	Found    bool
	Path     string
	HasError bool
}

type RepoFileCheck struct {
	Owner string
	Repo  string
	Files []FileCheckResult
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

func (rfc *RepoFileCheck) ToCSV() string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "%s/%s,", rfc.Owner, rfc.Repo)

	for _, file := range rfc.Files {
		if file.HasError {
			builder.WriteString("error")
		} else if file.Found {
			builder.WriteString(file.Path)
		}
		builder.WriteString(",")
	}

	result := strings.TrimSuffix(builder.String(), ",")
	return result + "\n"
}

func getRow(ctx context.Context, client *github.Client, owner, repo string) (string, error) {
	result := &RepoFileCheck{
		Owner: owner,
		Repo:  repo,
	}

	tree, resp, err := client.Git.GetTree(ctx, owner, repo, "HEAD", true)
	if err := rateLimitWait(ctx, resp); err != nil {
		return "", err
	}
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("repository %s/%s not found", owner, repo)
		}
		return "", fmt.Errorf("getting repo tree for %s/%s: %w", owner, repo, err)
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
					return "", err
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

	return result.ToCSV(), nil
}

func getCSVHeader() string {
	var builder strings.Builder
	builder.WriteString("Repository,")
	for _, chf := range communityHealthFiles {
		fmt.Fprintf(&builder, "%s,", chf)
	}
	return strings.TrimSuffix(builder.String(), ",") + "\n"
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: community-health-file-checker <input_file>")
		os.Exit(1)
	}

	inputFile := os.Args[1]
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

	scanner := bufio.NewScanner(file)
	fmt.Print(getCSVHeader())
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "/")
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid format: %s. Expected owner/repo.\n", line)
			continue
		}
		owner, repo := parts[0], parts[1]

		row, err := getRow(ctx, client, owner, repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s/%s: %v\n", owner, repo, err)
			continue
		}
		fmt.Print(row)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}
}
