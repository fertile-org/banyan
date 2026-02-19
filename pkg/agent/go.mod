module github.com/fertile-org/banyan/pkg/agent

go 1.24.3

require (
	github.com/fertile-org/banyan/pkg/rpc v0.0.0
	github.com/fertile-org/banyan/pkg/types v0.0.0
	google.golang.org/grpc v1.72.1
)

require (
	github.com/kr/text v0.2.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/fertile-org/banyan/pkg/rpc => ../rpc
	github.com/fertile-org/banyan/pkg/types => ../types
)
