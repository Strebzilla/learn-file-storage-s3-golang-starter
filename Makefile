include .env
export

BINARY_API := bin/chirpy

build: check clean
	go build -o $(BINARY_API) .

run: build
	./$(BINARY_API)

lint:
	betteralign -apply ./...
	go mod tidy
	golangci-lint run ./...

fmt:	
	gofumpt -w .
	pg_format -i sql/**/*.sql

db-start:
	service postgresql start

clean:
	rm -f $(BINARY_API)

test:
	go test ./...

check: lint test fmt
