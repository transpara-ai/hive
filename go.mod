module github.com/lovyou-ai/hive

go 1.24.2

require (
	github.com/anthropics/anthropic-sdk-go v1.26.0
	github.com/jackc/pgx/v5 v5.8.0
	github.com/lovyou-ai/agent v0.0.0
	github.com/lovyou-ai/eventgraph/go v0.0.0-20260309152918-5602caa542f2
	github.com/lovyou-ai/work v0.0.0
)

replace (
	github.com/lovyou-ai/agent => ../agent
	github.com/lovyou-ai/eventgraph/go => ../eventgraph/go
	github.com/lovyou-ai/work => ../work
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
