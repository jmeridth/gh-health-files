BUILD_FLAGS=-X 'main.BuiltAt=`date +%FT%T%z`'
BUILD_WIN=@env GOOS=windows GOARCH=amd64 go build -o community-health-file-checker.exe
BUILD_LINUX=@env GOOS=linux GOARCH=amd64 go build -o community-health-file-checker-linux
BUILD_DARWIN=@env GOOS=darwin GOARCH=amd64 go build -o community-health-file-checker-darwin
INPUT_FILE ?= repos.txt

.PHONY : help
help: # Display help
	@awk -F ':|##' \
		'/^[^\t].+?:.*?##/ {\
			printf "\033[36m%-30s\033[0m %s\n", $$1, $$NF \
		}' $(MAKEFILE_LIST)

.PHONY : build
build: tidy ## Build the binary
	@echo "  >  Building binary ..."
	go build -o community-health-file-checker -ldflags="$(BUILD_FLAGS)"

.PHONY : tidy
tidy: ## Get dependencies
	@echo "  >  Tidying go.mod ..."
	@go mod tidy

.PHONY : csv
csv: build ## Generate CSV
	@echo "  >  Generating results.csv ..."
	@go run main.go ${INPUT_FILE} > results.csv
