module github.com/fertile-org/banyan/cmd/engine

go 1.21

require github.com/fertile-org/banyan/internal/common v0.0.0

require (
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
)

replace github.com/fertile-org/banyan/internal/common => ../../internal/common
