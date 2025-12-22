module github.com/fertile-org/banyan/cmd/cli

go 1.24.0

require github.com/fertile-org/banyan/internal/common v0.0.0

require (
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/sys v0.38.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/fertile-org/banyan/internal/common => ../../internal/common
