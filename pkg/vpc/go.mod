module github.com/fertile-org/banyan/pkg/vpc

go 1.21

require (
	github.com/google/uuid v1.6.0
	go.etcd.io/etcd/client/v3 v3.5.11
)

replace github.com/fertile-org/banyan/pkg/interfaces => ../interfaces
