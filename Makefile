.PHONY: build test vet verify verify-canonical-paths

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

verify-canonical-paths:
	! grep -R "/Transpara/transpara-ai/data/repos" loop pkg docs/OPERATOR-UI-CONTRACT.md

verify: verify-canonical-paths build test vet
