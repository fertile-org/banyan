module github.com/fertile-org/banyan/cmd/banyan-agent

go 1.24.3

require (
	github.com/fertile-org/banyan/pkg/agent v0.0.0
	github.com/fertile-org/banyan/pkg/types v0.0.0
	github.com/spf13/cobra v1.10.1
)

require (
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/fertile-org/banyan/pkg/rpc v0.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.18.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.opentelemetry.io/otel/sdk v1.37.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/grpc v1.72.1 // indirect
	google.golang.org/protobuf v1.36.7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/fertile-org/banyan/pkg/agent => ../../pkg/agent
	github.com/fertile-org/banyan/pkg/rpc => ../../pkg/rpc
	github.com/fertile-org/banyan/pkg/storage => ../../pkg/storage
	github.com/fertile-org/banyan/pkg/types => ../../pkg/types
)
