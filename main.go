package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
)

var communityHealthFilePaths = []string{
	"",
	".github/",
	"docs/",
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

type CommunityHealthFile struct {
	Name           string `json:"name"`
	OneLocation    bool   `json:"one_location"`
	SingleLocation string `json:"single_location"`
}

var communityHealthFiles = []CommunityHealthFile{
	{"FUNDING.yml", true, ".github/"},
	{"SECURITY.md", false, ""},
}

func checkFile(client *github.Client, owner, repo, filePath string) (bool, error) {
	_, _, resp, err := client.Repositories.GetContents(context.Background(), owner, repo, filePath, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func rateLimitCheck(resp *github.Response) {
	if resp != nil && resp.Rate.Remaining == 0 {
		resetTime := time.Until(resp.Rate.Reset.Time)
		fmt.Printf("Rate limit exceeded. Waiting for %v...\n", resetTime)
		time.Sleep(resetTime)
	}
}

// Convert the struct to CSV format
func (rfc *RepoFileCheck) ToCSV() string {
	var builder strings.Builder

	// Start with owner/repo
	builder.WriteString(fmt.Sprintf("%s/%s,", rfc.Owner, rfc.Repo))

	// Add each file check result
	for _, file := range rfc.Files {
		builder.WriteString(fmt.Sprintf("%s,", file.FileName))

		if file.HasError {
			builder.WriteString("error")
		} else if file.Found {
			builder.WriteString(fmt.Sprintf("true,%s", file.Path))
		} else {
			builder.WriteString("false")
		}

		builder.WriteString(",")
	}

	// Remove trailing comma and add newline
	result := strings.TrimSuffix(builder.String(), ",")
	return result + "\n"
}

// Refactored getRow function
func getRow(client *github.Client, owner string, repo string) string {
	result := &RepoFileCheck{
		Owner: owner,
		Repo:  repo,
		Files: []FileCheckResult{},
	}

	for _, chf := range communityHealthFiles {
		fileResult := FileCheckResult{
			FileName: chf.Name,
			Found:    false,
		}

		if chf.OneLocation {
			path := fmt.Sprintf("%s%s", chf.SingleLocation, chf.Name)
			found, err := checkFile(client, owner, repo, path)

			fileResult.Found = found
			fileResult.Path = path
			fileResult.HasError = err != nil
		} else {
			for _, basePath := range communityHealthFilePaths {
				path := fmt.Sprintf("%s%s", basePath, chf.Name)
				found, err := checkFile(client, owner, repo, path)

				if found {
					fileResult.Found = true
					fileResult.Path = path
					fileResult.HasError = err != nil
					break
				}

				// Keep track of last error
				if err != nil {
					fileResult.HasError = true
				}
			}
			if !fileResult.Found {
				// Check the org/owner .github repository
				found, err := checkFile(client, owner, ".github", chf.Name)

				fileResult.Found = found
				fileResult.Path = fmt.Sprintf("%s/.github/%s", owner, chf.Name)
				fileResult.HasError = err != nil
			}
		}

		result.Files = append(result.Files, fileResult)
	}

	return result.ToCSV()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run check_repo_files.go <input_file>")
		return
	}

	inputFile := os.Args[1]
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Println("Please set the GITHUB_TOKEN environment variable.")
		return
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	file, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "/")
		if len(parts) != 2 {
			fmt.Printf("Invalid format: %s. Expected owner/repo.\n", line)
			continue
		}
		owner, repo := parts[0], parts[1]
		row := getRow(client, owner, repo)
		fmt.Print(row)

		var resp *github.Response

		rateLimitCheck(resp)
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading file: %v\n", err)
	}
}
