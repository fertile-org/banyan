package usecases

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fertile-org/banyan/pkg/engine/parser/adapters"
	"github.com/fertile-org/banyan/pkg/engine/parser/ports/inbound"
)

func TestParseComposeUseCase_Parse(t *testing.T) {
	tests := []struct {
		name           string
		composeContent string
		banyanContent  string
		opts           inbound.ParseOptions
		wantServices   int
		wantErr        bool
	}{
		{
			name: "simple service",
			composeContent: `
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
`,
			wantServices: 1,
			wantErr:      false,
		},
		{
			name: "multiple services with dependencies",
			composeContent: `
services:
  web:
    image: nginx:latest
    depends_on:
      - api
  api:
    image: myapi:latest
    depends_on:
      - db
  db:
    image: postgres:15
`,
			wantServices: 3,
			wantErr:      false,
		},
		{
			name: "with networks and volumes",
			composeContent: `
services:
  web:
    image: nginx:latest
    networks:
      - frontend

networks:
  frontend:
    driver: bridge

volumes:
  data:
    driver: local
`,
			wantServices: 1,
			wantErr:      false,
		},
		{
			name: "with banyan extensions",
			composeContent: `
services:
  web:
    image: nginx:latest
`,
			banyanContent: `
version: "1"
vpc:
  cidr: "10.0.0.0/16"
services:
  web:
    placement:
      constraints:
        - node.role == worker
    scaling:
      min: 2
      max: 10
`,
			wantServices: 1,
			wantErr:      false,
		},
		{
			name: "circular dependency should fail",
			composeContent: `
services:
  a:
    image: a:latest
    depends_on:
      - b
  b:
    image: b:latest
    depends_on:
      - c
  c:
    image: c:latest
    depends_on:
      - a
`,
			wantServices: 0,
			wantErr:      true, // compose-go detects this
		},
		{
			name: "missing image and build should fail",
			composeContent: `
services:
  web:
    ports:
      - "80:80"
`,
			wantServices: 0,
			wantErr:      true, // compose-go validates this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewParseComposeUseCase(
				adapters.NewComposeGoAdapter(),
				adapters.NewYAMLParserAdapter(),
				nil, // schema validator
				adapters.NewEnvInterpolatorAdapter(),
				nil, // logger
			)

			result, err := uc.Parse(context.Background(), tt.composeContent, tt.banyanContent, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, result.Compose.Services, tt.wantServices)
		})
	}
}

func TestParseComposeUseCase_Interpolation(t *testing.T) {
	composeContent := `
services:
  web:
    image: ${IMAGE_NAME}:${IMAGE_TAG:-latest}
    environment:
      DB_HOST: ${DB_HOST:-localhost}
`

	uc := NewParseComposeUseCase(
		adapters.NewComposeGoAdapter(),
		adapters.NewYAMLParserAdapter(),
		nil,
		adapters.NewEnvInterpolatorAdapter(),
		nil,
	)

	opts := inbound.ParseOptions{
		Environment: map[string]string{
			"IMAGE_NAME": "myapp",
			"IMAGE_TAG":  "v1.0",
		},
		SkipValidation: true, // Skip validation to focus on interpolation
	}

	result, err := uc.Parse(context.Background(), composeContent, "", opts)
	require.NoError(t, err)
	require.Len(t, result.Compose.Services, 1)

	web := result.Compose.Services["web"]
	assert.Equal(t, "myapp:v1.0", web.Image)
	assert.Equal(t, "localhost", web.Environment["DB_HOST"])
}

func TestParseComposeUseCase_ParseBanyan(t *testing.T) {
	banyanContent := `
version: "1"
vpc:
  name: my-vpc
  cidr: "10.0.0.0/16"
  subnets:
    - name: public
      cidr: "10.0.1.0/24"
      public: true
    - name: private
      cidr: "10.0.2.0/24"
      public: false
services:
  web:
    placement:
      constraints:
        - node.role == worker
    scaling:
      min: 2
      max: 10
      target_cpu: 80
plugins:
  - name: logging
    version: "1.0"
    config:
      driver: json-file
`

	uc := NewParseComposeUseCase(
		adapters.NewComposeGoAdapter(),
		adapters.NewYAMLParserAdapter(),
		nil,
		nil,
		nil,
	)

	result, err := uc.ParseBanyan(context.Background(), banyanContent)
	require.NoError(t, err)

	assert.Equal(t, "1", result.Version)
	assert.Equal(t, "my-vpc", result.VPC.Name)
	assert.Equal(t, "10.0.0.0/16", result.VPC.CIDR)
	assert.Len(t, result.VPC.Subnets, 2)
	assert.Len(t, result.Services, 1)
	assert.Len(t, result.Plugins, 1)

	webExt := result.Services["web"]
	assert.Equal(t, 2, webExt.Scaling.Min)
	assert.Equal(t, 10, webExt.Scaling.Max)
	assert.Equal(t, 80, webExt.Scaling.TargetCPU)
}

func TestParseComposeUseCase_Validate(t *testing.T) {
	tests := []struct {
		name           string
		composeContent string
		wantValid      bool
		wantErr        bool // Error from compose-go loader
	}{
		{
			name: "valid compose",
			composeContent: `
services:
  web:
    image: nginx:latest
`,
			wantValid: true,
			wantErr:   false,
		},
		{
			name: "missing image fails at compose-go level",
			composeContent: `
services:
  web:
    ports:
      - "80:80"
`,
			wantValid: false,
			wantErr:   true, // compose-go rejects this before our validation
		},
		{
			name: "invalid dependency reference fails at compose-go level",
			composeContent: `
services:
  web:
    image: nginx:latest
    depends_on:
      - nonexistent
`,
			wantValid: false,
			wantErr:   true, // compose-go rejects this before our validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewParseComposeUseCase(
				adapters.NewComposeGoAdapter(),
				adapters.NewYAMLParserAdapter(),
				nil,
				nil,
				nil,
			)

			result, err := uc.Validate(context.Background(), tt.composeContent)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantValid, result.Valid)
		})
	}
}

func TestParseComposeUseCase_GetSupportedVersions(t *testing.T) {
	uc := NewParseComposeUseCase(nil, nil, nil, nil, nil)

	versions := uc.GetSupportedVersions()

	assert.Contains(t, versions, "3.8")
	assert.Contains(t, versions, "3")
	assert.Contains(t, versions, "2.4")
}

func TestParseComposeUseCase_WithNetworks(t *testing.T) {
	composeContent := `
services:
  web:
    image: nginx:latest
    networks:
      - frontend
      - backend

networks:
  frontend:
    driver: bridge
    ipam:
      config:
        - subnet: 172.16.0.0/24
  backend:
    internal: true
`

	uc := NewParseComposeUseCase(
		adapters.NewComposeGoAdapter(),
		adapters.NewYAMLParserAdapter(),
		nil,
		nil,
		nil,
	)

	result, err := uc.Parse(context.Background(), composeContent, "", inbound.ParseOptions{})
	require.NoError(t, err)

	// Check networks are parsed
	assert.Contains(t, result.Compose.Networks, "frontend")
	assert.Contains(t, result.Compose.Networks, "backend")

	frontend := result.Compose.Networks["frontend"]
	assert.Equal(t, "bridge", frontend.Driver)
	assert.NotNil(t, frontend.IPAM)
	assert.Len(t, frontend.IPAM.Config, 1)
	assert.Equal(t, "172.16.0.0/24", frontend.IPAM.Config[0].Subnet)

	backend := result.Compose.Networks["backend"]
	assert.True(t, backend.Internal)
}

func TestParseComposeUseCase_WithVolumes(t *testing.T) {
	composeContent := `
services:
  db:
    image: postgres:15
    volumes:
      - dbdata:/var/lib/postgresql/data

volumes:
  dbdata:
    driver: local
`

	uc := NewParseComposeUseCase(
		adapters.NewComposeGoAdapter(),
		adapters.NewYAMLParserAdapter(),
		nil,
		nil,
		nil,
	)

	result, err := uc.Parse(context.Background(), composeContent, "", inbound.ParseOptions{})
	require.NoError(t, err)

	assert.Contains(t, result.Compose.Volumes, "dbdata")

	dbdata := result.Compose.Volumes["dbdata"]
	assert.Equal(t, "local", dbdata.Driver)
}
