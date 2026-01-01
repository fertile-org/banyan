package domain

import "errors"

var (
	// ErrRuleNotFound indicates that a security rule was not found.
	ErrRuleNotFound = errors.New("security rule not found")

	// ErrInvalidRule indicates that a security rule is invalid.
	ErrInvalidRule = errors.New("invalid security rule")

	// ErrRuleAlreadyExists indicates that a security rule already exists.
	ErrRuleAlreadyExists = errors.New("security rule already exists")

	// ErrInvalidNetworkID indicates that the network ID is invalid.
	ErrInvalidNetworkID = errors.New("invalid network ID")

	// ErrApplyFailed indicates that applying rules to the system failed.
	ErrApplyFailed = errors.New("failed to apply security rules")

	// ErrInvalidDirection indicates an invalid direction value.
	ErrInvalidDirection = errors.New("invalid direction: must be ingress or egress")

	// ErrInvalidAction indicates an invalid action value.
	ErrInvalidAction = errors.New("invalid action: must be allow or deny")

	// ErrInvalidProtocol indicates an invalid protocol value.
	ErrInvalidProtocol = errors.New("invalid protocol")
)
