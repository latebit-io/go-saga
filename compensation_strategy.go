package gosaga

import (
	"context"
	"fmt"
	"time"
)

// CompensationStrategy defines how to handle compensation failures
type CompensationStrategy[T any] interface {
	Compensate(ctx context.Context, saga Saga[T]) error
}

// CompensationResult tracks the result of compensating a single step
type CompensationResult struct {
	StepName string
	Success  bool
	Error    error
	Attempts int
}

// =====================================
// Strategy 1: Retry with Exponential Backoff
// =====================================

type RetryConfig struct {
	MaxRetries      int
	InitialBackoff  time.Duration
	MaxBackoff      time.Duration
	BackoffMultiple float64
}

// DefaultRetryConfig provides sensible defaults for retry behavior
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      3,
		InitialBackoff:  1 * time.Second,
		MaxBackoff:      30 * time.Second,
		BackoffMultiple: 2.0,
	}
}

type RetryStrategy[T any] struct {
	config RetryConfig
}

func NewRetryStrategy[T any](config RetryConfig) *RetryStrategy[T] {
	return &RetryStrategy[T]{config: config}
}

func (r *RetryStrategy[T]) Compensate(ctx context.Context, saga Saga[T]) error {
	// Compensate in reverse order
	for i := saga.State.FailedStep - 1; i >= 0; i-- {
		step := saga.Steps[i]

		if err := r.compensateStepWithRetry(ctx, step, saga.Data); err != nil {
			return fmt.Errorf("compensation failed for step %s after %d attempts: %w",
				step.Name, r.config.MaxRetries+1, err)
		}

		saga.logger.Log("info", fmt.Sprintf("✓ Compensated: %s", step.Name))
	}
	return nil
}

func (r *RetryStrategy[T]) compensateStepWithRetry(ctx context.Context, step *SagaStep[T], data *T) error {
	var lastErr error
	backoff := r.config.InitialBackoff

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		lastErr = step.Compensate(ctx, data)
		if lastErr == nil {
			return nil
		}

		if attempt < r.config.MaxRetries {
			// saga.logger.Log("info", fmt.Sprintf("⚠️  Compensation failed for %s (attempt %d/%d): %v. Retrying in %v...",
			// 	step.Name, attempt+1, r.config.MaxRetries+1, lastErr, backoff))

			select {
			case <-time.After(backoff):
				// Continue to next retry
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			}

			// Exponential backoff with cap
			backoff = time.Duration(float64(backoff) * r.config.BackoffMultiple)
			if backoff > r.config.MaxBackoff {
				backoff = r.config.MaxBackoff
			}
		}
	}

	return lastErr
}

// =====================================
// Strategy 2: Continue All (Collect All Errors)
// =====================================

type ContinueAllStrategy[T any] struct {
	retryConfig RetryConfig
}

func NewContinueAllStrategy[T any](retryConfig RetryConfig) *ContinueAllStrategy[T] {
	return &ContinueAllStrategy[T]{retryConfig: retryConfig}
}

func (c *ContinueAllStrategy[T]) Compensate(ctx context.Context, saga Saga[T]) error {
	var compensationErrors []CompensationResult
	retryHelper := NewRetryStrategy[T](c.retryConfig)

	// Try to compensate all steps, even if some fail
	for i := saga.State.FailedStep - 1; i >= 0; i-- {
		step := saga.Steps[i]

		err := retryHelper.compensateStepWithRetry(ctx, step, saga.Data)

		result := CompensationResult{
			StepName: step.Name,
			Success:  err == nil,
			Error:    err,
			Attempts: c.retryConfig.MaxRetries + 1,
		}

		if err != nil {
			compensationErrors = append(compensationErrors, result)
			saga.logger.Log("info", fmt.Sprintf("❌ CRITICAL: Compensation failed for %s after all retries: %v", step.Name, err))
		} else {
			saga.logger.Log("info", fmt.Sprintf("✓ Compensated: %s", step.Name))
		}
	}

	// If any compensations failed, return a detailed error
	if len(compensationErrors) > 0 {
		return &CompensationError{
			Message:  "one or more compensation steps failed",
			Failures: compensationErrors,
		}
	}

	return nil
}

// =====================================
// Strategy 3: Fail Fast
// =====================================

type FailFastStrategy[T any] struct{}

func NewFailFastStrategy[T any]() *FailFastStrategy[T] {
	return &FailFastStrategy[T]{}
}

func (f *FailFastStrategy[T]) Compensate(ctx context.Context, saga Saga[T]) error {
	for i := saga.State.FailedStep - 1; i >= 0; i-- {
		step := saga.Steps[i]
		saga.State.CompensatedSteps = append(saga.State.CompensatedSteps, i)
		if err := step.Compensate(ctx, saga.Data); err != nil {
			saga.State.CompensatedStatus = failed
			saga.SaveState(ctx)
			return fmt.Errorf("compensation failed for step %s: %w", step.Name, err)
		}
		saga.State.CompensatedStatus = compensating
		saga.SaveState(ctx)
		saga.logger.Log("info", fmt.Sprintf("✓ Compensated: %s", step.Name))
	}
	saga.State.CompensatedStatus = complete
	saga.SaveState(ctx)
	return nil
}

// =====================================
// Custom Error Type for Detailed Reporting
// =====================================

type CompensationError struct {
	Message  string
	Failures []CompensationResult
}

func (e *CompensationError) Error() string {
	msg := fmt.Sprintf("%s:\n", e.Message)
	for _, failure := range e.Failures {
		msg += fmt.Sprintf("  - %s: %v (attempts: %d)\n", failure.StepName, failure.Error, failure.Attempts)
	}
	return msg
}

// Helper to check if an error is a compensation error
func IsCompensationError(err error) (*CompensationError, bool) {
	if compErr, ok := err.(*CompensationError); ok {
		return compErr, true
	}
	return nil, false
}
