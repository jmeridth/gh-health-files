# Community Health File Checker

This is a simple script to check the health of community health files in a GitHub repository. It checks for the presence of the following files:

- `FUNDING.yaml`
- `SECURITY.md`

Coming soon:

- `CODE_OF_CONDUCT.md`
- `CONTRIBUTING.md`
- `Discussion category forms`
- `GOVERNANCE.md`
- `Issue and Pull Request Templates and config.yml`
- `SUPPORT.md`

[Documentation about Community Health Files](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/creating-a-default-community-health-file#supported-file-types)

## Requirements

- Go 1.24 or later
- an input file (default: repos.txt) with a list of GitHub repositories in the format `owner/repo` (one per line)
- a GitHub personal access token with `repo` and `read:org` scopes (optional, but recommended for private repositories and decrease rate limits)

## Usage

1. Clone the repository.
2. `make csv`

## Output

The script will output a CSV file with the following columns:

- `repo`: in the format `owner/repo`
- `funding`
- `funding_found`: true, false, or error
- `security`
- `security_found`: true, false, or error
