module github.com/fertile-org/banyan/pkg/shared/infrastructure

go 1.24.0

require (
	github.com/fertile-org/banyan/pkg/shared/domain v0.0.0
	github.com/sirupsen/logrus v1.9.3
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/kr/pretty v0.3.1 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/fertile-org/banyan/pkg/shared/domain => ../domain
