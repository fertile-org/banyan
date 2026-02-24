module github.com/fertile-org/banyan/pkg/engine

go 1.24.3

require (
	github.com/fertile-org/banyan/pkg/rpc v0.0.0
	github.com/fertile-org/banyan/pkg/storage v0.0.0
	github.com/fertile-org/banyan/pkg/types v0.0.0
	github.com/fertile-org/banyan/pkg/vpc v0.0.0
	github.com/fertile-org/banyan/pkg/vpc/overlay v0.0.0
	github.com/google/go-containerregistry v0.20.7
	golang.org/x/crypto v0.48.0
	google.golang.org/grpc v1.72.1
)

require (
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	go.etcd.io/etcd/api/v3 v3.5.21 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.21 // indirect
	go.etcd.io/etcd/client/v3 v3.5.21 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/protobuf v1.36.7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/fertile-org/banyan/pkg/rpc => ../rpc
	github.com/fertile-org/banyan/pkg/storage => ../storage
	github.com/fertile-org/banyan/pkg/types => ../types
	github.com/fertile-org/banyan/pkg/vpc => ../vpc
	github.com/fertile-org/banyan/pkg/vpc/overlay => ../vpc/overlay
)
