module github.com/fertile-org/banyan/cmd/cli

go 1.23.0

require github.com/fertile-org/banyan/internal/common v0.0.0

require (
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
)

replace github.com/fertile-org/banyan/internal/common => ../../internal/common
