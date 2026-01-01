package errors

import "errors"

var (
	// ErrVPCNotFound is returned when a VPC is not found.
	ErrVPCNotFound = errors.New("VPC not found")

	// ErrSubnetNotFound is returned when a subnet is not found.
	ErrSubnetNotFound = errors.New("subnet not found")

	// ErrNoAvailableSubnet is returned when no subnet has available capacity.
	ErrNoAvailableSubnet = errors.New("no available subnet")

	// ErrIPExhausted is returned when IP addresses are exhausted.
	ErrIPExhausted = errors.New("IP addresses exhausted")

	// ErrAllocationNotFound is returned when a container network allocation is not found.
	ErrAllocationNotFound = errors.New("allocation not found")

	// ErrSecurityGroupNotFound is returned when a security group is not found.
	ErrSecurityGroupNotFound = errors.New("security group not found")

	// ErrNetworkAlreadyExists is returned when trying to create a network that already exists.
	ErrNetworkAlreadyExists = errors.New("network already exists")

	// ErrContainerAlreadyAttached is returned when a container is already attached to a network.
	ErrContainerAlreadyAttached = errors.New("container already attached to network")

	// ErrInvalidCIDR is returned when a CIDR is invalid.
	ErrInvalidCIDR = errors.New("invalid CIDR")
)
