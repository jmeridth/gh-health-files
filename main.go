package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
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
	Name string `json:"name"`
}

var communityHealthFiles = []CommunityHealthFile{
	{"CODE_OF_CONDUCT.md"},
	{"CONTRIBUTING.md"},
	{"FUNDING.yml"},
	{"GOVERNANCE.md"},
	{"SECURITY.md"},
	{"SUPPORT.md"},
}

func generateFileNameVariations(fileName string) []string {
	variations := mapset.NewSet[string]()

	// Add the original file name
	variations.Add(fileName)

	// Replace underscores with dashes
	if strings.Contains(fileName, "_") {
		variations.Add(strings.ReplaceAll(fileName, "_", "-"))
		variations.Add(strings.ReplaceAll(strings.ToLower(fileName), "_", "-"))
	}

	// Replace dashes with underscores
	if strings.Contains(fileName, "-") {
		variations.Add(strings.ReplaceAll(fileName, "-", "_"))
		variations.Add(strings.ReplaceAll(strings.ToLower(fileName), "_", "-"))
	}

	// Convert to lowercase
	variations.Add(strings.ToLower(fileName))

	// Convert to uppercase
	variations.Add(strings.ToUpper(fileName))

	// Title case (capitalize each word)
	if strings.Contains(fileName, "_") || strings.Contains(fileName, "-") {
		titleCaser := cases.Title(language.English)
		titleCase := strings.ReplaceAll(titleCaser.String(strings.ReplaceAll(fileName, "_", " ")), " ", "_")
		variations.Add(titleCase)
	}

	return variations.ToSlice()
}

func checkFile(tree *github.Tree, filePath string) (bool, string) {
	variations := generateFileNameVariations(filePath)

	// Iterate through the variations
	for _, variation := range variations {
		// Check if the variation exists in the tree
		for _, entry := range tree.Entries {
			if entry.GetPath() == variation {
				return true, variation // File found
			}
		}
	}

	return false, "" // File not found
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
	fmt.Fprintf(&builder, "%s/%s,", rfc.Owner, rfc.Repo)

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

func getRow(client *github.Client, owner string, repo string) string {
	result := &RepoFileCheck{
		Owner: owner,
		Repo:  repo,
		Files: []FileCheckResult{},
	}

	// Get the repository tree
	tree, resp, err := client.Git.GetTree(context.Background(), owner, repo, "HEAD", true)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return fmt.Sprintf("Repository %s/%s not found.\n", owner, repo)
		}
		return fmt.Sprintf("Error getting repo file contents: %s/%s: %v\n", owner, repo, err)
	}

	for _, chf := range communityHealthFiles {
		fileResult := FileCheckResult{
			FileName: chf.Name,
			Found:    false,
		}

		for _, basePath := range communityHealthFilePaths {
			path := fmt.Sprintf("%s%s", basePath, chf.Name)
			found, foundPath := checkFile(tree, path)
			if found {
				fileResult.Found = true
				fileResult.Path = foundPath
				break
			}
		}

		if !fileResult.Found {
			orgTree, _, _ := client.Git.GetTree(context.Background(), owner, ".github", "HEAD", true)
			if orgTree != nil {
				path := fmt.Sprintf("%s%s", ".github/", chf.Name)
				found, foundPath := checkFile(orgTree, path)
				if found {
					fileResult.Found = true
					fileResult.Path = fmt.Sprintf("%s/%s", owner, foundPath)
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
		fmt.Fprintf(&builder, "%s,", chf.Name)
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
	defer func() {
		err := file.Close()
		if err != nil {
			fmt.Printf("error closing output file %s: %v\n", inputFile, err)
			return
		}
	}()

	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}

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
