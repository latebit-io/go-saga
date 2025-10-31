package gosaga

import (
	"context"
)

type NoStateStore struct{}

func NewNoStateStore() *NoStateStore {
	return &NoStateStore{}
}

func (s *NoStateStore) SaveState(ctx context.Context, state *SagaState) error {
	return nil
}

func (s *NoStateStore) LoadState(ctx context.Context, sagaID string) (*SagaState, error) {
	return nil, ErrSagaNotFound
}
