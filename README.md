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

- Go 1.25 or later
- an input file (default: repos.txt) with a list of GitHub repositories in the format `owner/repo` (one per line)
- a GitHub personal access token with `repo` and `read:org` scopes (optional, but recommended for private repositories and decrease rate limits)

## Usage

1. Clone the repository.
2. `just csv`

## Output

The script will output a CSV file with the following columns:

- `repo`: in the format `owner/repo`
- `found path`: the path of the file (repeated for each file checked)
