package usecases

import (
	"context"
	"log/slog"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
)

// SecurityPolicyUseCase handles network security policies.
type SecurityPolicyUseCase struct {
	securityMgr outbound.SecurityManager
	store       outbound.ContainerNetworkStore
	logger      *slog.Logger
}

// NewSecurityPolicyUseCase creates a new SecurityPolicyUseCase.
func NewSecurityPolicyUseCase(
	securityMgr outbound.SecurityManager,
	store outbound.ContainerNetworkStore,
	logger *slog.Logger,
) *SecurityPolicyUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &SecurityPolicyUseCase{
		securityMgr: securityMgr,
		store:       store,
		logger:      logger,
	}
}

// ApplySecurityPolicy applies a network policy.
func (uc *SecurityPolicyUseCase) ApplySecurityPolicy(
	ctx context.Context,
	policy domain.NetworkPolicy,
) error {
	if err := uc.securityMgr.ApplyNetworkPolicy(ctx, policy); err != nil {
		return err
	}

	uc.logger.Info("security policy applied",
		"name", policy.Name,
		"namespace", policy.Namespace,
	)

	return nil
}

// DeleteSecurityPolicy removes a network policy.
func (uc *SecurityPolicyUseCase) DeleteSecurityPolicy(
	ctx context.Context,
	name, namespace string,
) error {
	if err := uc.securityMgr.DeleteNetworkPolicy(ctx, name, namespace); err != nil {
		return err
	}

	uc.logger.Info("security policy deleted",
		"name", name,
		"namespace", namespace,
	)

	return nil
}

// CreateSecurityGroup creates a new security group.
func (uc *SecurityPolicyUseCase) CreateSecurityGroup(
	ctx context.Context,
	vpcID string,
	spec domain.SecurityGroupSpec,
) (*domain.SecurityGroup, error) {
	sg, err := uc.securityMgr.CreateSecurityGroup(ctx, vpcID, spec)
	if err != nil {
		return nil, err
	}

	uc.logger.Info("security group created",
		"id", sg.ID,
		"name", sg.Name,
		"vpc_id", vpcID,
	)

	return sg, nil
}

// DeleteSecurityGroup deletes a security group.
func (uc *SecurityPolicyUseCase) DeleteSecurityGroup(
	ctx context.Context,
	groupID string,
) error {
	if err := uc.securityMgr.DeleteSecurityGroup(ctx, groupID); err != nil {
		return err
	}

	uc.logger.Info("security group deleted", "id", groupID)

	return nil
}

// UpdateContainerSecurityGroups updates security groups for a container.
func (uc *SecurityPolicyUseCase) UpdateContainerSecurityGroups(
	ctx context.Context,
	containerID string,
	securityGroups []string,
) error {
	allocation, err := uc.store.Get(ctx, containerID)
	if err != nil {
		return err
	}

	// Update security groups
	allocation.SecurityGroups = securityGroups

	if err := uc.store.Save(ctx, allocation); err != nil {
		return err
	}

	uc.logger.Info("container security groups updated",
		"container_id", containerID,
		"security_groups", securityGroups,
	)

	return nil
}
