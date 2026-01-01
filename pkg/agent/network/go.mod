module github.com/fertile-org/banyan/pkg/agent/network

go 1.24.0

require (
	github.com/fertile-org/banyan/pkg/agent/container v0.0.0
	github.com/fertile-org/banyan/pkg/vpc v0.0.0
)

replace (
	github.com/fertile-org/banyan/pkg/agent/container => ../container
	github.com/fertile-org/banyan/pkg/shared/domain => ../../shared/domain
	github.com/fertile-org/banyan/pkg/vpc => ../../vpc
)
