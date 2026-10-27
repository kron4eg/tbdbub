# Common tasks for tbdbub. Run `just` to list them.

binary := "tbdbub"
build_dir := env_var_or_default("BUILD_DIR", "/tmp")

# List available recipes.
default:
    @just --list

# Build the binary outside the repo (BUILD_DIR defaults to /tmp).
build:
    go build -o "{{ build_dir }}/{{ binary }}" .

# Run the TUI from source.
run *args:
    go run . {{ args }}

# Run tests (all packages by default); accepts packages or go test flags.
test *args:
    go test {{ if args == "" { "./..." } else { args } }}

# Run tests with the race detector.
test-race:
    go test -race ./...

# Run tests with coverage and print per-function totals.
cover:
    go test -coverprofile="{{ build_dir }}/{{ binary }}.cover.out" ./...
    go tool cover -func="{{ build_dir }}/{{ binary }}.cover.out"

# Check for vet issues.
vet:
    go vet ./...

# Format Go sources in place.
fmt:
    gofmt -w .

# Fail if any Go source needs formatting.
fmt-check:
    @unformatted="$(gofmt -l .)"; \
    if [ -n "$unformatted" ]; then \
        echo "gofmt required for:"; \
        echo "$unformatted"; \
        exit 1; \
    fi

# Sync go.mod and go.sum.
tidy:
    go mod tidy

# Run the pre-commit checks: formatting, vet, and tests.
check: fmt-check vet test

# Remove build and coverage artifacts.
clean:
    rm -f "{{ build_dir }}/{{ binary }}" "{{ build_dir }}/{{ binary }}.cover.out"
