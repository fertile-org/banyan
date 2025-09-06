module github.com/fertile-org/banyan/test

go 1.21

require (
	github.com/fertile-org/banyan/internal/common v0.0.0
	github.com/fertile-org/banyan/pkg/interfaces v0.0.0
	github.com/fertile-org/banyan/pkg/plugin-sdk v0.0.0
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/fertile-org/banyan/internal/common => ../internal/common

replace github.com/fertile-org/banyan/pkg/interfaces => ../pkg/interfaces

replace github.com/fertile-org/banyan/pkg/plugin-sdk => ../pkg/plugin-sdk
