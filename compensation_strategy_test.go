package gosaga

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Test DefaultRetryConfig
func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries to be 3, got %d", config.MaxRetries)
	}
	if config.InitialBackoff != 1*time.Second {
		t.Errorf("Expected InitialBackoff to be 1s, got %v", config.InitialBackoff)
	}
	if config.MaxBackoff != 30*time.Second {
		t.Errorf("Expected MaxBackoff to be 30s, got %v", config.MaxBackoff)
	}
	if config.BackoffMultiple != 2.0 {
		t.Errorf("Expected BackoffMultiple to be 2.0, got %f", config.BackoffMultiple)
	}
}

// Test RetryStrategy success case
func TestRetryStrategy_Compensate_Success(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      2,
		InitialBackoff:  10 * time.Millisecond,
		MaxBackoff:      100 * time.Millisecond,
		BackoffMultiple: 2.0,
	}

	strategy := NewRetryStrategy[string](config)
	saga := createTestSaga(2, false)

	err := strategy.Compensate(context.Background(), &saga)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// Test RetryStrategy failure after retries
func TestRetryStrategy_Compensate_FailureAfterRetries(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      2,
		InitialBackoff:  10 * time.Millisecond,
		MaxBackoff:      100 * time.Millisecond,
		BackoffMultiple: 2.0,
	}

	strategy := NewRetryStrategy[string](config)
	saga := createTestSaga(2, true)

	err := strategy.Compensate(context.Background(), &saga)
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// Test RetryStrategy with context cancellation
func TestRetryStrategy_Compensate_ContextCancelled(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      2,
		InitialBackoff:  50 * time.Millisecond,
		MaxBackoff:      200 * time.Millisecond,
		BackoffMultiple: 2.0,
	}

	strategy := NewRetryStrategy[string](config)
	saga := createTestSaga(2, true)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := strategy.Compensate(ctx, &saga)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

// Test ContinueAllStrategy success case
func TestContinueAllStrategy_Compensate_Success(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      1,
		InitialBackoff:  10 * time.Millisecond,
		MaxBackoff:      100 * time.Millisecond,
		BackoffMultiple: 2.0,
	}

	strategy := NewContinueAllStrategy[string](config)
	saga := createTestSaga(3, false)

	err := strategy.Compensate(context.Background(), &saga)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// Test ContinueAllStrategy with some failures
func TestContinueAllStrategy_Compensate_PartialFailure(t *testing.T) {
	config := RetryConfig{
		MaxRetries:      1,
		InitialBackoff:  10 * time.Millisecond,
		MaxBackoff:      100 * time.Millisecond,
		BackoffMultiple: 2.0,
	}

	strategy := NewContinueAllStrategy[string](config)
	saga := createMixedTestSaga()

	err := strategy.Compensate(context.Background(), &saga)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Check if it's a CompensationError
	compErr, ok := IsCompensationError(err)
	if !ok {
		t.Error("Expected CompensationError")
	}

	if len(compErr.Failures) == 0 {
		t.Error("Expected failures to be recorded")
	}
}

// Test FailFastStrategy success case
func TestFailFastStrategy_Compensate_Success(t *testing.T) {
	strategy := NewFailFastStrategy[string]()
	saga := createTestSaga(2, false)

	err := strategy.Compensate(context.Background(), &saga)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// Test FailFastStrategy failure
func TestFailFastStrategy_Compensate_Failure(t *testing.T) {
	strategy := NewFailFastStrategy[string]()
	saga := createTestSaga(2, true)

	err := strategy.Compensate(context.Background(), &saga)
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// Test CompensationError
func TestCompensationError_Error(t *testing.T) {
	compErr := &CompensationError{
		Message: "test error",
		Failures: []CompensationResult{
			{
				StepName: "step1",
				Success:  false,
				Error:    errors.New("step1 failed"),
				Attempts: 3,
			},
			{
				StepName: "step2",
				Success:  false,
				Error:    errors.New("step2 failed"),
				Attempts: 2,
			},
		},
	}

	errMsg := compErr.Error()
	if errMsg == "" {
		t.Error("Expected non-empty error message")
	}
	if len(errMsg) < len("test error") {
		t.Error("Expected error message to contain details")
	}
}

// Test IsCompensationError
func TestIsCompensationError(t *testing.T) {
	compErr := &CompensationError{
		Message:  "test",
		Failures: []CompensationResult{},
	}

	result, ok := IsCompensationError(compErr)
	if !ok {
		t.Error("Expected IsCompensationError to return true")
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}

	regularErr := errors.New("regular error")
	_, ok = IsCompensationError(regularErr)
	if ok {
		t.Error("Expected IsCompensationError to return false for regular error")
	}
}

// Helper functions

func createTestSaga(failedStep int, shouldFail bool) Saga[string] {
	data := "test data"
	steps := []*SagaStep[string]{
		{
			Name: "step1",
			Execute: func(ctx context.Context, data *string) error {
				return nil
			},
			Compensate: func(ctx context.Context, data *string) error {
				if shouldFail {
					return errors.New("compensation failed")
				}
				return nil
			},
		},
		{
			Name: "step2",
			Execute: func(ctx context.Context, data *string) error {
				return nil
			},
			Compensate: func(ctx context.Context, data *string) error {
				if shouldFail {
					return errors.New("compensation failed")
				}
				return nil
			},
		},
		{
			Name: "step3",
			Execute: func(ctx context.Context, data *string) error {
				return nil
			},
			Compensate: func(ctx context.Context, data *string) error {
				if shouldFail {
					return errors.New("compensation failed")
				}
				return nil
			},
		},
	}

	return Saga[string]{
		Data:  &data,
		Steps: steps,
		State: SagaState{
			FailedStep: failedStep,
		},
		logger:     &consoleLogger{},
		stateStore: NewNoStateStore(),
	}
}

func createMixedTestSaga() Saga[string] {
	data := "test data"
	steps := []*SagaStep[string]{
		{
			Name: "step1",
			Execute: func(ctx context.Context, data *string) error {
				return nil
			},
			Compensate: func(ctx context.Context, data *string) error {
				return nil // Success
			},
		},
		{
			Name: "step2",
			Execute: func(ctx context.Context, data *string) error {
				return nil
			},
			Compensate: func(ctx context.Context, data *string) error {
				return errors.New("step2 compensation failed")
			},
		},
		{
			Name: "step3",
			Execute: func(ctx context.Context, data *string) error {
				return nil
			},
			Compensate: func(ctx context.Context, data *string) error {
				return nil // Success
			},
		},
	}

	return Saga[string]{
		Data:  &data,
		Steps: steps,
		State: SagaState{
			FailedStep: 3,
		},
		logger:     &consoleLogger{},
		stateStore: NewNoStateStore(),
	}
}

// Simple console logger for tests
type consoleLogger struct{}

func (c *consoleLogger) Log(level string, message string) {
	// Silent logger for tests
	//fmt.Sprintf("%s: %s", level, message)
}
