package gosaga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"
)

var ErrSagaNotFound = errors.New("saga state not found")

type SagaStatus string

const (
	executing    SagaStatus = "EXECUTING"
	compensating SagaStatus = "COMPENSATING"
	complete     SagaStatus = "COMPLETE"
	failed       SagaStatus = "FAILED"
	created      SagaStatus = "CREATED"
)

// SagaStep represents a single step in the saga with execute and compensate functions
type SagaStep[T any] struct {
	Name       string
	Execute    func(ctx context.Context, data *T) error
	Compensate func(ctx context.Context, data *T) error
}

// Saga represents the saga orchestrator
type Saga[T any] struct {
	SagaID               string
	Steps                []*SagaStep[T]
	Data                 *T
	State                SagaState
	logger               Logger
	compensationStrategy CompensationStrategy[T]
	stateStore           SagaStateStore
	metadata             map[string]string
}

type SagaStateStore interface {
	SaveState(ctx context.Context, state *SagaState) error
	LoadState(ctx context.Context, sagaID string) (*SagaState, error)
}

type SagaState struct {
	SagaID            string
	TotalSteps        int
	CurrentStep       int
	Status            SagaStatus
	Data              json.RawMessage
	FailedStep        int
	CompensatedSteps  []int
	CompensatedStatus SagaStatus
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewSagaState(sagaID string) SagaState {
	return SagaState{
		SagaID:           sagaID,
		Status:           created,
		CompensatedSteps: make([]int, 0),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

type Logger interface {
	Log(level string, msg string)
}

type DefaultLogger struct {
	logger *log.Logger
}

func NewDefaultLogger(logger *log.Logger) *DefaultLogger {
	return &DefaultLogger{logger: logger}
}

func (l *DefaultLogger) Log(level string, msg string) {
	l.logger.Printf("%s: %s", level, msg)
}

// NewSaga creates a new saga instance with default FailFast strategy
func LoadOrCreateNewSaga[T any](ctx context.Context, stateStore SagaStateStore, sagaID string, data *T) (*Saga[T], error) {
	state := NewSagaState(sagaID)

	saga := &Saga[T]{
		SagaID:               sagaID,
		Steps:                make([]*SagaStep[T], 0),
		Data:                 data,
		State:                state,
		stateStore:           stateStore,
		logger:               NewDefaultLogger(log.Default()),
		compensationStrategy: NewFailFastStrategy[T](),
	}
	err := saga.loadState(ctx)
	if err != nil {
		if !errors.Is(err, ErrSagaNotFound) {
			return nil, err
		}
		saga.logger.Log("info", err.Error())
	}

	if saga.State.CompensatedStatus == complete || saga.State.Status == complete {
		return nil, fmt.Errorf("saga is completed and cannot be used: %s", saga.SagaID)
	}

	return saga, nil
}

// WithCompensationStrategy sets the compensation strategy for the saga (fluent API)
func (s *Saga[T]) WithCompensationStrategy(strategy CompensationStrategy[T]) *Saga[T] {
	s.compensationStrategy = strategy
	return s
}

// AddStep adds a step to the saga
func (s *Saga[T]) AddStep(name string, execute, compensate func(ctx context.Context, data *T) error) *Saga[T] {
	step := &SagaStep[T]{
		Name:       name,
		Execute:    execute,
		Compensate: compensate,
	}
	s.Steps = append(s.Steps, step)
	return s
}

// LoadState loads a saved state
func (s *Saga[T]) loadState(ctx context.Context) error {
	state, err := s.stateStore.LoadState(ctx, s.SagaID)

	if err != nil {
		return err
	}

	if state == nil {
		s.logger.Log("info", "state is nil, no state to load")
		return nil
	}

	err = json.Unmarshal(state.Data, s.Data)
	if err != nil {
		return err
	}

	s.State = *state

	return nil
}

// Execute runs the saga
func (s *Saga[T]) Execute(ctx context.Context) error {
	if s.State.CompensatedStatus == complete || s.State.Status == complete {
		return fmt.Errorf("cannot execute completed sagaID: %s", s.SagaID)
	}
	s.State.TotalSteps = len(s.Steps)
	for i, step := range s.Steps {
		s.State.CurrentStep = i + 1
		if err := step.Execute(ctx, s.Data); err != nil {
			s.State.FailedStep = i
			s.State.Status = failed
			s.State.UpdatedAt = time.Now()
			s.logger.Log("info", fmt.Sprintf("Step %s failed: %v", step.Name, err))
			s.SaveState(ctx)
			return fmt.Errorf("saga failed and needs to be rolled back: %w", err)
		}
		s.SaveState(ctx)
		s.logger.Log("info", fmt.Sprintf("Executed: %d - %s", i, step.Name))
	}

	s.State.Status = complete
	err := s.SaveState(ctx)
	if err != nil {
		s.logger.Log("info", fmt.Sprintf("Failed to write: %s", err))
	}

	return nil
}

func (s *Saga[T]) Compensate(ctx context.Context) error {
	if s.State.CompensatedStatus == complete || s.State.Status == complete {
		return fmt.Errorf("cannot compensate completed sagaID: %s", s.SagaID)
	}
	return s.compensationStrategy.Compensate(ctx, s)
}

func (s *Saga[T]) SaveState(ctx context.Context) error {
	marshaledData, err := json.Marshal(*s.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	s.State.Data = marshaledData

	err = s.stateStore.SaveState(ctx, &s.State)
	if err != nil {
		return err
	}

	return nil
}
