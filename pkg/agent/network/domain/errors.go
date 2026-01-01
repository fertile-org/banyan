package domain

import "errors"

var (
	// ErrNetworkNotFound is returned when a network cannot be found.
	ErrNetworkNotFound = errors.New("network not found")

	// ErrContainerNotFound is returned when a container cannot be found.
	ErrContainerNotFound = errors.New("container not found")

	// ErrEndpointNotFound is returned when an endpoint cannot be found.
	ErrEndpointNotFound = errors.New("endpoint not found")

	// ErrAlreadyConnected is returned when a container is already connected to a network.
	ErrAlreadyConnected = errors.New("container already connected to network")

	// ErrNotConnected is returned when a container is not connected to a network.
	ErrNotConnected = errors.New("container not connected to network")

	// ErrIPAllocationFailed is returned when IP allocation fails.
	ErrIPAllocationFailed = errors.New("failed to allocate IP address")

	// ErrInvalidNetworkID is returned when a network ID is invalid.
	ErrInvalidNetworkID = errors.New("invalid network ID")

	// ErrInvalidContainerID is returned when a container ID is invalid.
	ErrInvalidContainerID = errors.New("invalid container ID")

	// ErrInvalidIPAddress is returned when an IP address is invalid.
	ErrInvalidIPAddress = errors.New("invalid IP address")

	// ErrNetworkOperationFailed is returned when a network operation fails.
	ErrNetworkOperationFailed = errors.New("network operation failed")
)
