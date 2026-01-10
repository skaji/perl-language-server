build:
	go build ./cmd/perl-language-server

test:
	go test ./...

lint:
	bash maint/lint.sh

prettier-fix:
	prettier -w .
