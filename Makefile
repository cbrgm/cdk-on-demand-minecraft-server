WATCHDOG_BIN := cmd/watchdog/watchdog
LAUNCHER_LAMBDA_BIN := cmd/lambda/launcher/bootstrap
LOGFORWARDER_LAMBDA_BIN := cmd/lambda/logforwarder/bootstrap

SOURCES := $(shell find . -path ./vendor -prune -o -path ./cdk.out -prune -o -name '*.go' -type f -print)
PACKAGES := $(shell go list ./... | grep -v '/vendor/' | grep -v '/cdk.out/')
LDFLAGS := "-s -w"

.PHONY: default sync clean fmt vet lint generate test install build cdk-diff cdk-deploy

# Default Recipe
default: build

# Sync dependencies
sync:
	go mod download

# Clean build artifacts
clean:
	go clean -i ./...
	rm -rf $(LOGFORWARDER_LAMBDA_BIN) $(LAUNCHER_LAMBDA_BIN) $(WATCHDOG_BIN)

# Format Go source files
fmt:
	gofmt -s -w $(SOURCES)

# Run go vet for static analysis
vet:
	go vet $(PACKAGES)

# Run golangci-lint for linting
lint:
	golangci-lint run --out-format=colored-line-number --timeout 5m

# Generate Go source files
generate:
	go generate $(SOURCES)

# Run tests with coverage
test:
	go test -coverprofile coverage.out $(PACKAGES)

# Install binaries
install:
	go install -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' ./cmd/$(NAME)

# Build binaries for watchdog and lambda
build: $(WATCHDOG_BIN) $(LAUNCHER_LAMBDA_BIN) $(LOGFORWARDER_LAMBDA_BIN)

$(WATCHDOG_BIN): cmd/watchdog/main.go
	GOOS=linux GOARCH=arm64 go build -o $(WATCHDOG_BIN) -ldflags $(LDFLAGS) ./cmd/watchdog

$(LAUNCHER_LAMBDA_BIN): cmd/lambda/launcher/main.go
	GOOS=linux GOARCH=arm64 go build -o $(LAUNCHER_LAMBDA_BIN) -ldflags $(LDFLAGS) ./cmd/lambda/launcher

$(LOGFORWARDER_LAMBDA_BIN): cmd/lambda/logforwarder/main.go
	GOOS=linux GOARCH=arm64 go build -o $(LOGFORWARDER_LAMBDA_BIN) -ldflags $(LDFLAGS) ./cmd/lambda/logforwarder

# CDK diff (requires build)
cdk-diff: build
	cdk diff --all

# CDK deploy (requires build)
cdk-deploy: build
	cdk deploy --all

