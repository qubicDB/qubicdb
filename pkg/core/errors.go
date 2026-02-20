package core

import "errors"

var (
	ErrNeuronNotFound     = errors.New("neuron not found")
	ErrSynapseNotFound    = errors.New("synapse not found")
	ErrMatrixNotFound     = errors.New("matrix not found for user")
	ErrMatrixFull         = errors.New("matrix has reached maximum capacity")
	ErrDimensionLimit     = errors.New("dimension limit reached")
	ErrInvalidContent     = errors.New("invalid neuron content")
	ErrContentTooLarge    = errors.New("neuron content exceeds maximum allowed size")
	ErrDuplicateNeuron    = errors.New("neuron with same content hash exists")
	ErrSelfLink           = errors.New("cannot create synapse to self")
	ErrBrainNotActive     = errors.New("brain is not active")
	ErrBrainSleeping      = errors.New("brain is in sleep/consolidation state")
	ErrPersistenceFailed  = errors.New("failed to persist matrix")
	ErrLoadFailed         = errors.New("failed to load matrix from persistence")
	ErrInvalidQuery       = errors.New("invalid query")
	ErrUserNotFound       = errors.New("user not found")
)
