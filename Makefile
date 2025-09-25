.PHONY: help install-tools generate-mocks test clean build run

help: ## Shows this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

install-tools: ## Installs necessary tools (mockery)
	@echo "Installing Mockery..."
	go install github.com/vektra/mockery/v2@latest
	@echo "Mockery installed successfully!"

generate-mocks: ## Generates all interface mocks
	@echo "Generating mocks..."
	~/go/bin/mockery
	@echo "Mocks generated successfully!"

test: ## Runs all tests
	@echo "Running tests..."
	go test -v ./...

test-coverage: ## Runs tests with coverage
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated in coverage.html"

clean: ## Cleans temporary files
	@echo "Cleaning temporary files..."
	go clean
	rm -f coverage.out coverage.html

build: ## Compiles the application
	@echo "Compiling application..."
	go build -o bin/main ./cmd/api

run: ## Runs the application
	@echo "Running application..."
	go run ./cmd/api

docker-build: ## Builds Docker image
	@echo "Building Docker image..."
	docker build -t memberclass-backend .

docker-run: ## Runs the application in Docker
	@echo "Running application in Docker..."
	docker-compose up

dev-setup: install-tools generate-mocks ## Sets up development environment
	@echo "Development environment configured!"

ci: generate-mocks test ## Runs CI pipeline (generate mocks + tests)
	@echo "CI pipeline executed successfully!"