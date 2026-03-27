# gh-health-files

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
- An input file with a list of GitHub repositories in the format `owner/repo` (one per line), or use `--org` to discover repositories automatically
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
just run repos.txt            # build and run with an input file
just run --org myorg           # build and run, discover repos in an org
just csv          # build and run, save output to results.csv
just json         # build and run, save output to results.json
just markdown     # build and run, save output to results.md
just test         # build, vet, and run tests
just vet          # run go vet
just lint         # run golangci-lint
just build        # build the binary
just help         # list all available targets
```

### Options

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--format` | — | `csv` | Output format: `csv`, `json`, or `markdown` |
| `--org` | — | — | Discover and check all repositories in the specified GitHub organization |
| `--api-url` | `GITHUB_API_URL` | `https://api.github.com` | GitHub API base URL (for GitHub Enterprise Server) |

The `--org` flag and input file are mutually exclusive — use one or the other.

The `--api-url` flag takes precedence over the `GITHUB_API_URL` environment variable.

### Organization Discovery

Use `--org` to automatically discover and check all repositories in a GitHub organization:

```bash
gh-health-files --org myorg
gh-health-files --org myorg --format json
```

Archived repositories and forks are excluded from discovery. This keeps results focused on actively maintained, original repositories.

### GitHub Enterprise Server

To use this tool with a GitHub Enterprise Server instance, provide the API base URL:

```bash
export GITHUB_API_URL=https://github.example.com/api/v3
gh-health-files repos.txt
```

Or via the flag:

```bash
gh-health-files --api-url https://github.example.com/api/v3 repos.txt
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
