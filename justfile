build_flags := "-X 'main.BuiltAt=`date +%FT%T%z`'"
input_file := env("INPUT_FILE", "repos.txt")

# Display help
help:
    @just --list

# Get dependencies
tidy:
    @echo "  >  Tidying go.mod ..."
    @go mod tidy

# Build the binary
build: tidy
    @echo "  >  Building binary ..."
    go build -o community-health-file-checker -ldflags="{{build_flags}}"

# Run go vet
vet:
    @echo "  >  Vetting code ..."
    @go vet ./...

# Run tests
test: build vet
    @echo "  >  Running tests ..."
    @go test ./...

# Run linter
lint:
    @echo "  >  Running linter ..."
    @golangci-lint run ./...

# Run the checker and output to stdout
run: build
    @echo "  >  Running checker ..."
    @./community-health-file-checker {{input_file}}

# Build Windows binary
build-win: tidy
    @echo "  >  Building Windows binary ..."
    @env GOOS=windows GOARCH=amd64 go build -o community-health-file-checker.exe

# Build Linux binary
build-linux: tidy
    @echo "  >  Building Linux binary ..."
    @env GOOS=linux GOARCH=amd64 go build -o community-health-file-checker-linux

# Build Darwin binary
build-darwin: tidy
    @echo "  >  Building Darwin binary ..."
    @env GOOS=darwin GOARCH=amd64 go build -o community-health-file-checker-darwin

# Generate CSV
csv: build
    @echo "  >  Generating results.csv ..."
    @./community-health-file-checker {{input_file}} > results.csv

# Generate JSON
json: build
    @echo "  >  Generating results.json ..."
    @./community-health-file-checker --format json {{input_file}} > results.json

# Generate Markdown
markdown: build
    @echo "  >  Generating results.md ..."
    @./community-health-file-checker --format markdown {{input_file}} > results.md
