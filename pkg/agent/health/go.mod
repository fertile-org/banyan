module github.com/fertile-org/banyan/pkg/agent/health

go 1.24.0

replace (
	github.com/fertile-org/banyan/pkg/agent/container => ../container
	github.com/fertile-org/banyan/pkg/shared/domain => ../../shared/domain
)
