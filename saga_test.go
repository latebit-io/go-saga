package gosaga

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"
	"time"
)

// Test NewSagaState
func TestNewSagaState(t *testing.T) {
	sagaID := "test-saga-123"
	state := NewSagaState(sagaID)

	if state.SagaID != sagaID {
		t.Errorf("Expected SagaID to be %s, got %s", sagaID, state.SagaID)
	}
	if state.Status != created {
		t.Errorf("Expected Status to be %s, got %s", created, state.Status)
	}
	if state.CompensatedSteps == nil {
		t.Error("Expected CompensatedSteps to be initialized")
	}
	if len(state.CompensatedSteps) != 0 {
		t.Errorf("Expected CompensatedSteps to be empty, got %d items", len(state.CompensatedSteps))
	}
	if state.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	if state.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be set")
	}
}

// Test DefaultLogger
func TestDefaultLogger_Log(t *testing.T) {
	logger := NewDefaultLogger(log.New(os.Stdout, "", 0))
	if logger == nil {
		t.Error("Expected non-nil logger")
	}

	// Just test that it doesn't panic
	logger.Log("info", "test message")
}

// Test NewSaga
func TestNewSaga(t *testing.T) {
	data := "test data"
	sagaID := "saga-001"
	stateStore := NewNoStateStore()

	saga := NewSaga(stateStore, sagaID, &data)

	if saga.SagaID != sagaID {
		t.Errorf("Expected SagaID to be %s, got %s", sagaID, saga.SagaID)
	}
	if saga.Data == nil || *saga.Data != data {
		t.Error("Expected Data to be set correctly")
	}
	if saga.Steps == nil {
		t.Error("Expected Steps to be initialized")
	}
	if len(saga.Steps) != 0 {
		t.Errorf("Expected Steps to be empty, got %d items", len(saga.Steps))
	}
	if saga.stateStore == nil {
		t.Error("Expected stateStore to be set")
	}
	if saga.logger == nil {
		t.Error("Expected logger to be set")
	}
	if saga.compensationStrategy == nil {
		t.Error("Expected compensationStrategy to be set")
	}
}

// Test AddStep
func TestSaga_AddStep(t *testing.T) {
	data := "test"
	saga := NewSaga(NewNoStateStore(), "saga-002", &data)

	executeFunc := func(ctx context.Context, data *string) error {
		return nil
	}
	compensateFunc := func(ctx context.Context, data *string) error {
		return nil
	}

	saga.AddStep("step1", executeFunc, compensateFunc)

	if len(saga.Steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(saga.Steps))
	}
	if saga.Steps[0].Name != "step1" {
		t.Errorf("Expected step name to be 'step1', got '%s'", saga.Steps[0].Name)
	}

	// Test fluent API
	saga.AddStep("step2", executeFunc, compensateFunc).
		AddStep("step3", executeFunc, compensateFunc)

	if len(saga.Steps) != 3 {
		t.Errorf("Expected 3 steps, got %d", len(saga.Steps))
	}
}

// Test WithCompensationStrategy
func TestSaga_WithCompensationStrategy(t *testing.T) {
	data := "test"
	saga := NewSaga(NewNoStateStore(), "saga-003", &data)

	retryStrategy := NewRetryStrategy[string](DefaultRetryConfig())
	saga.WithCompensationStrategy(retryStrategy)

	if saga.compensationStrategy == nil {
		t.Error("Expected compensationStrategy to be set")
	}
}

// Test Execute success
func TestSaga_Execute_Success(t *testing.T) {
	data := "initial"
	saga := NewSaga(NewNoStateStore(), "saga-004", &data)

	step1Executed := false
	step2Executed := false

	saga.AddStep("step1",
		func(ctx context.Context, data *string) error {
			step1Executed = true
			*data = *data + "-step1"
			return nil
		},
		func(ctx context.Context, data *string) error {
			return nil
		},
	).AddStep("step2",
		func(ctx context.Context, data *string) error {
			step2Executed = true
			*data = *data + "-step2"
			return nil
		},
		func(ctx context.Context, data *string) error {
			return nil
		},
	)

	err := saga.Execute(context.Background())

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !step1Executed {
		t.Error("Expected step1 to be executed")
	}
	if !step2Executed {
		t.Error("Expected step2 to be executed")
	}
	if *saga.Data != "initial-step1-step2" {
		t.Errorf("Expected data to be 'initial-step1-step2', got '%s'", *saga.Data)
	}
	if saga.State.Status != complete {
		t.Errorf("Expected status to be %s, got %s", complete, saga.State.Status)
	}
	if saga.State.TotalSteps != 2 {
		t.Errorf("Expected TotalSteps to be 2, got %d", saga.State.TotalSteps)
	}
	if saga.State.CurrentStep != 2 {
		t.Errorf("Expected CurrentStep to be 2, got %d", saga.State.CurrentStep)
	}
}

// Test Execute failure
func TestSaga_Execute_Failure(t *testing.T) {
	data := "initial"
	saga := NewSaga(NewNoStateStore(), "saga-005", &data)

	step1Executed := false
	step2Executed := false
	step3Executed := false

	saga.AddStep("step1",
		func(ctx context.Context, data *string) error {
			step1Executed = true
			return nil
		},
		func(ctx context.Context, data *string) error {
			return nil
		},
	).AddStep("step2",
		func(ctx context.Context, data *string) error {
			step2Executed = true
			return errors.New("step2 failed")
		},
		func(ctx context.Context, data *string) error {
			return nil
		},
	).AddStep("step3",
		func(ctx context.Context, data *string) error {
			step3Executed = true
			return nil
		},
		func(ctx context.Context, data *string) error {
			return nil
		},
	)

	err := saga.Execute(context.Background())

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if !step1Executed {
		t.Error("Expected step1 to be executed")
	}
	if !step2Executed {
		t.Error("Expected step2 to be executed")
	}
	if step3Executed {
		t.Error("Expected step3 NOT to be executed")
	}
	if saga.State.Status != failed {
		t.Errorf("Expected status to be %s, got %s", failed, saga.State.Status)
	}
	if saga.State.FailedStep != 1 {
		t.Errorf("Expected FailedStep to be 1, got %d", saga.State.FailedStep)
	}
}

// Test Execute with context cancellation
func TestSaga_Execute_ContextCancelled(t *testing.T) {
	data := "initial"
	saga := NewSaga(NewNoStateStore(), "saga-006", &data)

	saga.AddStep("step1",
		func(ctx context.Context, data *string) error {
			select {
			case <-time.After(100 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		func(ctx context.Context, data *string) error {
			return nil
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := saga.Execute(ctx)

	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

// Test SaveState
func TestSaga_SaveState(t *testing.T) {
	type TestData struct {
		Value string
		Count int
	}

	data := TestData{Value: "test", Count: 42}
	saga := NewSaga(NewNoStateStore(), "saga-007", &data)

	err := saga.SaveState(context.Background())

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if saga.State.Data == nil {
		t.Error("Expected State.Data to be set")
	}
}

// Test SaveState with unmarshalable data
func TestSaga_SaveState_MarshalError(t *testing.T) {
	data := make(chan int)
	saga := NewSaga(NewNoStateStore(), "saga-008", &data)

	err := saga.SaveState(context.Background())

	if err == nil {
		t.Error("Expected marshal error")
	}
}

// Test Compensate
func TestSaga_Compensate(t *testing.T) {
	data := "test"
	saga := NewSaga(NewNoStateStore(), "saga-009", &data)

	compensated := false

	saga.AddStep("step1",
		func(ctx context.Context, data *string) error {
			return nil
		},
		func(ctx context.Context, data *string) error {
			compensated = true
			return nil
		},
	)

	saga.State.FailedStep = 1

	err := saga.Compensate(context.Background())

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !compensated {
		t.Error("Expected compensation to be executed")
	}
}

// Test LoadState
func TestSaga_LoadState(t *testing.T) {
	data := "test"
	saga := NewSaga(NewNoStateStore(), "saga-011", &data)

	result := saga.LoadState("saga-011")

	if result == nil {
		t.Error("Expected non-nil result")
	}
	if saga.useState {
		t.Error("Expected useState to be false")
	}
}
