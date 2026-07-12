.PHONY: build test vet verify verify-canonical-paths verify-hive-lifecycle-skill

CANONICAL_REPOS_ROOT := /Transpara/transpara-ai/repos
LEGACY_REPOS_ROOT := /Transpara/transpara-ai/data/repos

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

verify-canonical-paths:
	test -f loop/mcp-knowledge.json
	test -f docs/OPERATOR-UI-CONTRACT.md
	grep -Fq "$(CANONICAL_REPOS_ROOT)/hive/cmd/mcp-knowledge" loop/mcp-knowledge.json
	grep -Fq '"HIVE_DIR": "$(CANONICAL_REPOS_ROOT)/hive"' loop/mcp-knowledge.json
	grep -Fq '"SITE_DIR": "$(CANONICAL_REPOS_ROOT)/site"' loop/mcp-knowledge.json
	grep -Fq '"WORKSPACE": "$(CANONICAL_REPOS_ROOT)"' loop/mcp-knowledge.json
	grep -Fq '"repo_path": "$(CANONICAL_REPOS_ROOT)/hive"' docs/OPERATOR-UI-CONTRACT.md
	@# Keep doc scanning narrow so historical superpower plans remain preserved.
	@output=$$(grep -RnF "$(LEGACY_REPOS_ROOT)" loop pkg docs/OPERATOR-UI-CONTRACT.md); status=$$?; \
	if [ $$status -eq 0 ]; then \
		printf '%s\n' "$$output"; \
		echo "canonical-path violation: use $(CANONICAL_REPOS_ROOT)"; \
		exit 1; \
	fi; \
	if [ $$status -ne 1 ]; then \
		echo "canonical-path scan failed"; \
		exit $$status; \
	fi

verify-hive-lifecycle-skill:
	bash scripts/verify-hive-lifecycle-skill.sh

verify: verify-canonical-paths verify-hive-lifecycle-skill build test vet
