// grpc_handlers_secrets.go contains gRPC handlers for secret management RPCs:
// CreateSecret, ListSecrets, GetSecret, DeleteSecret.
package engine

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

func (s *engineGRPCServer) CreateSecret(ctx context.Context, req *banyanpb.CreateSecretRequest) (*banyanpb.CreateSecretResponse, error) {
	if s.secrets == nil {
		return nil, status.Error(codes.FailedPrecondition, "secrets not enabled (missing secrets.key on engine)")
	}
	if err := ValidateSecretName(req.Name); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	// Try create first; if exists, update
	if err := s.secrets.Create(ctx, req.Name, req.Value); err != nil {
		// If already exists, update
		if updateErr := s.secrets.Update(ctx, req.Name, req.Value); updateErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to create/update secret: %v", updateErr)
		}
		s.emitEvent("secret.updated", fmt.Sprintf("Secret %q updated", req.Name), "info")
	} else {
		s.emitEvent("secret.created", fmt.Sprintf("Secret %q created", req.Name), "info")
	}
	return &banyanpb.CreateSecretResponse{}, nil
}

func (s *engineGRPCServer) ListSecrets(ctx context.Context, _ *banyanpb.ListSecretsRequest) (*banyanpb.ListSecretsResponse, error) {
	if s.secrets == nil {
		return &banyanpb.ListSecretsResponse{}, nil
	}
	records, err := s.secrets.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list secrets: %v", err)
	}
	var infos []*banyanpb.SecretInfo
	for _, r := range records {
		infos = append(infos, &banyanpb.SecretInfo{
			Name:      r.Name,
			CreatedAt: r.CreatedAt.Format(time.RFC3339),
			UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
		})
	}
	return &banyanpb.ListSecretsResponse{Secrets: infos}, nil
}

func (s *engineGRPCServer) GetSecret(ctx context.Context, req *banyanpb.GetSecretRequest) (*banyanpb.GetSecretResponse, error) {
	if s.secrets == nil {
		return nil, status.Error(codes.FailedPrecondition, "secrets not enabled (missing secrets.key on engine)")
	}
	meta, err := s.secrets.GetMetadata(ctx, req.Name)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	resp := &banyanpb.GetSecretResponse{
		Name:      meta.Name,
		CreatedAt: meta.CreatedAt.Format(time.RFC3339),
		UpdatedAt: meta.UpdatedAt.Format(time.RFC3339),
	}
	if req.Reveal {
		value, decErr := s.secrets.Get(ctx, req.Name)
		if decErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to decrypt secret: %v", decErr)
		}
		resp.Value = value
	}
	return resp, nil
}

func (s *engineGRPCServer) DeleteSecret(ctx context.Context, req *banyanpb.DeleteSecretRequest) (*banyanpb.DeleteSecretResponse, error) {
	if s.secrets == nil {
		return nil, status.Error(codes.FailedPrecondition, "secrets not enabled (missing secrets.key on engine)")
	}

	// Block deletion if any running deployment references this secret
	depKeys, _ := s.store.List(ctx, types.KeyDeployments)
	for _, key := range depKeys {
		var dep types.DeploymentRecord
		if getErr := s.store.Get(ctx, key, &dep); getErr != nil {
			continue
		}
		if dep.Status != types.StatusRunning {
			continue
		}
		for svcName, svc := range dep.Services {
			for _, ref := range svc.Secrets {
				if ref == req.Name {
					return nil, status.Errorf(codes.FailedPrecondition,
						"cannot delete secret %q: referenced by deployment %q (service: %s)",
						req.Name, dep.Name, svcName)
				}
			}
		}
	}

	if err := s.secrets.Delete(ctx, req.Name); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	s.emitEvent("secret.deleted", fmt.Sprintf("Secret %q deleted", req.Name), "info")
	return &banyanpb.DeleteSecretResponse{}, nil
}
