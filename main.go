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
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
	SingleLocation string `json:"single_location"`
}

var communityHealthFiles = []CommunityHealthFile{
	{"CODE_OF_CONDUCT.md", ""},
	{"CONTRIBUTING.md", ""},
	{"FUNDING.yml", ".github/"},
	{"GOVERNANCE.md", ""},
	{"SECURITY.md", ""},
	{"SUPPORT.md", ""},
}

func generateFileNameVariations(fileName string) []string {
	var variations []string

	// Add the original file name
	variations = append(variations, fileName)

	// Replace underscores with dashes
	if strings.Contains(fileName, "_") {
		variations = append(variations, strings.ReplaceAll(fileName, "_", "-"))
	}

	// Replace dashes with underscores
	if strings.Contains(fileName, "-") {
		variations = append(variations, strings.ReplaceAll(fileName, "-", "_"))
	}

	// Convert to lowercase
	variations = append(variations, strings.ToLower(fileName))

	// Convert to uppercase
	variations = append(variations, strings.ToUpper(fileName))

	// Title case (capitalize each word)
	if strings.Contains(fileName, "_") || strings.Contains(fileName, "-") {
		titleCaser := cases.Title(language.English)
		titleCase := strings.ReplaceAll(titleCaser.String(strings.ReplaceAll(fileName, "_", " ")), " ", "_")
		variations = append(variations, titleCase)
	}

	return variations
}

func checkFile(client *github.Client, owner, repo, filePath string) (bool, error) {
	variations := generateFileNameVariations(filePath)

	for _, variation := range variations {
		_, _, resp, err := client.Repositories.GetContents(context.Background(), owner, repo, variation, nil)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue // Try the next variation
			}
			return false, err // Return error if it's not a 404
		}
		return true, nil // File found
	}

	return false, nil // File not found
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

	// owner/repo
	builder.WriteString(fmt.Sprintf("%s/%s,", rfc.Owner, rfc.Repo))

	// Add each file check result
	for _, file := range rfc.Files {
		if file.HasError {
			builder.WriteString("error")
		} else if file.Found {
			builder.WriteString(file.Path)
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

		if chf.SingleLocation != "" {
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
		}

		result.Files = append(result.Files, fileResult)
	}

	return result.ToCSV()
}

func getCSVHeader() string {
	var builder strings.Builder
	builder.WriteString("Repository,")
	for _, chf := range communityHealthFiles {
		builder.WriteString(fmt.Sprintf("%s,", chf.Name))
	}
	return strings.TrimSuffix(builder.String(), ",") + "\n"
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <input_file>")
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
	fmt.Print(getCSVHeader())
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
