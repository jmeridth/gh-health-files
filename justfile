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

# Run tests
test: build
    @echo "  >  Validating code ..."
    @go vet ./...
    @go test ./...

# Run linter
lint:
    @echo "  >  Running linter ..."
    @golangci-lint run ./...

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
    @go run main.go {{input_file}} > results.csv
