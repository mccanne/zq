vet:
	@go vet -copylocks ./...

test-unit:
	@go test -short ./...

install:
	@go install .
