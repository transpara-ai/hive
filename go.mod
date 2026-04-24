module github.com/transpara-ai/hive

go 1.25.0

require (
	github.com/anthropics/anthropic-sdk-go v1.26.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.8.0
	github.com/stretchr/testify v1.11.1
	github.com/transpara-ai/agent v0.0.0
	github.com/transpara-ai/eventgraph/go v0.0.0-20260309152918-5602caa542f2
	github.com/transpara-ai/work v0.0.0
)

replace (
	github.com/transpara-ai/agent => ../agent
	github.com/transpara-ai/eventgraph/go => ../eventgraph/go
	github.com/transpara-ai/work => ../work
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
