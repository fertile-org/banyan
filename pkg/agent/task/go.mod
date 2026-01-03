module github.com/fertile-org/banyan/pkg/agent/task

go 1.24.0

replace (
	github.com/fertile-org/banyan/pkg/agent/container => ../container
	github.com/fertile-org/banyan/pkg/shared/domain => ../../shared/domain
)

require github.com/google/uuid v1.6.0
