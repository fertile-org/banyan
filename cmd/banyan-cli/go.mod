module github.com/fertile-org/banyan/cmd/banyan-cli

go 1.24.3

require (
	github.com/fertile-org/banyan/pkg/rpc v0.0.0
	github.com/fertile-org/banyan/pkg/types v0.0.0
	github.com/spf13/cobra v1.10.1
	google.golang.org/grpc v1.72.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.opentelemetry.io/otel v1.37.0 // indirect
	go.opentelemetry.io/otel/sdk v1.37.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/protobuf v1.36.7 // indirect
)

replace (
	github.com/fertile-org/banyan/pkg/rpc => ../../pkg/rpc
	github.com/fertile-org/banyan/pkg/types => ../../pkg/types
)
