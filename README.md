# Community Health File Checker

This is a simple script to check the health of community health files in a GitHub repository. It checks for the presence of the following files:

- `CODE_OF_CONDUCT.md`
- `CONTRIBUTING.md`
- `FUNDING.yaml`
- `GOVERNANCE.md`
- `SECURITY.md`
- `SUPPORT.md`

Coming soon:

- `Discussion category forms`
- `Issue and Pull Request Templates and config.yml`

[Documentation about Community Health Files](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/creating-a-default-community-health-file#supported-file-types)

## Requirements

- Go 1.26 or later
- [just](https://github.com/casey/just) command runner
- An input file (default: repos.txt) with a list of GitHub repositories in the format `owner/repo` (one per line)
- A GitHub authentication token (see [Authentication](#authentication) below)

### Installing just

```bash
brew install just
```

## Authentication

### Fine-Grained Personal Access Token (recommended)

Create a [fine-grained personal access token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#creating-a-fine-grained-personal-access-token) with the following permissions:

- **Repository access**: Select the repositories you want to check (or "All repositories")
- **Repository permissions**:
  - `Contents`: Read-only
- **Organization permissions** (if checking org-level `.github` repo fallback):
  - `Members`: Read-only

Set the token as the `GITHUB_TOKEN` environment variable:

```bash
export GITHUB_TOKEN=github_pat_...
```

### GitHub App Installation Token

For organizational use, a [GitHub App](https://docs.github.com/en/apps/creating-github-apps/about-creating-github-apps/about-creating-github-apps) provides better security and auditability than a personal access token. Install a GitHub App with the following permissions:

- **Repository permissions**:
  - `Contents`: Read-only
- **Organization permissions**:
  - `Members`: Read-only

Generate an [installation access token](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app) and set it as `GITHUB_TOKEN`. GitHub App tokens are scoped to specific repositories and expire automatically, reducing the blast radius of a leaked credential.

## Usage

1. Clone the repository.
2. Run a target with `just <target>`:

```bash
just run          # build and run the checker, output to stdout
just csv          # build and run, save output to results.csv
just json         # build and run, save output to results.json
just markdown     # build and run, save output to results.md
just test         # build, vet, and run tests
just vet          # run go vet
just lint         # run golangci-lint
just build        # build the binary
just help         # list all available targets
```

The `run` and `csv` targets use `repos.txt` by default. Override with:

```bash
INPUT_FILE=my-repos.txt just run
```

### Options

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--format` | — | `csv` | Output format: `csv`, `json`, or `markdown` |
| `--api-url` | `GITHUB_API_URL` | `https://api.github.com` | GitHub API base URL (for GitHub Enterprise Server) |

The `--api-url` flag takes precedence over the `GITHUB_API_URL` environment variable.

### GitHub Enterprise Server

To use this tool with a GitHub Enterprise Server instance, provide the API base URL:

```bash
export GITHUB_API_URL=https://github.example.com/api/v3
community-health-file-checker repos.txt
```

Or via the flag:

```bash
community-health-file-checker --api-url https://github.example.com/api/v3 repos.txt
```

The tool validates custom API URLs before sending any credentials:
- HTTPS is required (HTTP is only permitted for localhost)
- A preflight request verifies the endpoint responds with GitHub-specific headers

### Inaccessible Repositories

If any repositories in the input file are not accessible (due to insufficient token permissions, the repository not existing, or the repository being private without proper access), the tool prints a summary to stderr at the end of execution:

```
The following 2 repository(ies) were not accessible (check token permissions or repository existence):
  - org/private-repo-1
  - org/private-repo-2
```

These repositories will show `error` in all file columns in the output.

## Output

The script will output a CSV file with the following columns:

- `repo`: in the format `owner/repo`
- `found path`: the path of the file (repeated for each file checked)
