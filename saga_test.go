package gosaga

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
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

	saga, _ := LoadOrCreateNewSaga(context.Background(), stateStore, sagaID, &data)

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
	saga, _ := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "saga-002", &data)

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
	saga, _ := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "saga-003", &data)

	retryStrategy := NewRetryStrategy[string](DefaultRetryConfig())
	saga.WithCompensationStrategy(retryStrategy)

	if saga.compensationStrategy == nil {
		t.Error("Expected compensationStrategy to be set")
	}
}

// Test Execute success
func TestSaga_Execute_Success(t *testing.T) {
	data := "initial"
	saga, _ := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "saga-004", &data)

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
	saga, _ := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "saga-005", &data)

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
	saga, _ := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "saga-006", &data)

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
	saga, _ := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "saga-007", &data)

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
	saga, _ := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "saga-008", &data)

	err := saga.SaveState(context.Background())

	if err == nil {
		t.Error("Expected marshal error")
	}
}

// Test Compensate
func TestSaga_Compensate(t *testing.T) {
	data := "test"
	saga, _ := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "saga-009", &data)

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

// Test LoadState with Postgres store
func TestSaga_LoadState_Postgres(t *testing.T) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	// Skip if no database connection is available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping postgres test")
	}

	ctx := context.Background()

	// Connect to the database
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close(ctx)

	// Clean up any existing test data
	sagaID := "saga-postgres-001"
	_, err = conn.Exec(ctx, "DELETE FROM saga_states WHERE saga_id = $1", sagaID)
	if err != nil {
		t.Fatalf("Failed to clean up test data: %v", err)
	}

	// Create a saga with postgres store
	type TestData struct {
		Value string
		Count int
	}
	data := TestData{Value: "test", Count: 42}

	postgresStore := NewPostgresSagaStore(conn)
	postgresStore.CreateSchema(ctx)
	saga, _ := LoadOrCreateNewSaga(ctx, postgresStore, sagaID, &data)

	// Add some steps
	saga.AddStep("step1",
		func(ctx context.Context, data *TestData) error {
			data.Count++
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			return nil
		},
	).AddStep("step2",
		func(ctx context.Context, data *TestData) error {
			data.Value = data.Value + "-updated"
			return fmt.Errorf("%v", "Step failed")
		},
		func(ctx context.Context, data *TestData) error {
			return nil
		},
	)

	// Execute the saga to create some state
	err = saga.Execute(ctx)
	if err == nil {
		t.Fatalf("should have failed")
	}

	// Now create a new saga instance and load the state
	newData := TestData{}
	newSaga, err := LoadOrCreateNewSaga(ctx, postgresStore, sagaID, &newData)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	// Verify the loaded state
	if newSaga.State.SagaID != sagaID {
		t.Errorf("Expected SagaID to be %s, got %s", sagaID, newSaga.State.SagaID)
	}
	if newSaga.State.Status != failed {
		t.Errorf("Expected status to be %s, got %s", complete, newSaga.State.Status)
	}
	if newSaga.State.TotalSteps != 2 {
		t.Errorf("Expected TotalSteps to be 2, got %d", newSaga.State.TotalSteps)
	}
	if newSaga.State.CurrentStep != 2 {
		t.Errorf("Expected CurrentStep to be 2, got %d", newSaga.State.CurrentStep)
	}

	// Verify the data was restored from the loaded state
	if newSaga.Data.Count != 43 {
		t.Errorf("Expected loaded Count to be 43, got %d", newSaga.Data.Count)
	}
	if newSaga.Data.Value != "test-updated" {
		t.Errorf("Expected loaded Value to be 'test-updated', got '%s'", newSaga.Data.Value)
	}

	// Clean up test data
	_, err = conn.Exec(ctx, "DELETE FROM saga_states WHERE saga_id = $1", sagaID)
	if err != nil {
		t.Logf("Warning: Failed to clean up test data: %v", err)
	}
}

// Test LoadState with compensation failure and retry
func TestSaga_LoadState_Postgres_CompensationFailureAndRetry(t *testing.T) {
	// Skip if no database connection is available
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	// Skip if no database connection is available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping postgres test")
	}

	ctx := context.Background()

	// Connect to the database
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close(ctx)

	// Clean up any existing test data
	sagaID := "saga-postgres-comp-failure-001"
	_, err = conn.Exec(ctx, "DELETE FROM saga_states WHERE saga_id = $1", sagaID)
	if err != nil {
		t.Fatalf("Failed to clean up test data: %v", err)
	}

	// Create test data
	type TestData struct {
		Value                 string
		Count                 int
		Step1Executed         bool
		Step2Executed         bool
		Step3Executed         bool
		Step1Compensated      bool
		Step2Compensated      bool
		FailOnStep3           bool
		FailOnStep1Compensate bool
	}
	data := TestData{
		Value:                 "initial",
		Count:                 0,
		FailOnStep3:           true, // This will cause step3 to fail
		FailOnStep1Compensate: true, // This will cause step1 compensation to fail
	}

	postgresStore := NewPostgresSagaStore(conn)
	postgresStore.CreateSchema(ctx)
	saga, err := LoadOrCreateNewSaga(ctx, postgresStore, sagaID, &data)
	if err != nil {
		t.Fatalf("error creating sage %v", err)
	}

	// Add steps - step 3 will fail, and step1 compensation will also fail
	saga.AddStep("step1",
		func(ctx context.Context, data *TestData) error {
			data.Step1Executed = true
			data.Count++
			data.Value += "-step1"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			if data.FailOnStep1Compensate {
				return errors.New("step1 compensation intentionally failed")
			}
			data.Step1Compensated = true
			data.Count--
			return nil
		},
	).AddStep("step2",
		func(ctx context.Context, data *TestData) error {
			data.Step2Executed = true
			data.Count++
			data.Value += "-step2"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			data.Step2Compensated = true
			data.Count--
			return nil
		},
	).AddStep("step3",
		func(ctx context.Context, data *TestData) error {
			data.Step3Executed = true
			if data.FailOnStep3 {
				return errors.New("step3 intentionally failed")
			}
			data.Value += "-step3"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			return nil
		},
	)

	// Execute the saga - it should fail on step3, then fail during compensation
	err = saga.Execute(ctx)
	if err != nil {
		err = saga.Compensate(ctx)
		if err == nil {
			t.Fatal("Expected saga to fail, but it succeeded")
		}
	}

	// Verify the saga failed
	if saga.State.Status != failed {
		t.Errorf("Expected status to be %s, got %s", failed, saga.State.Status)
	}
	if !data.Step1Executed {
		t.Error("Expected step1 to have executed")
	}
	if !data.Step2Executed {
		t.Error("Expected step2 to have executed")
	}
	if !data.Step3Executed {
		t.Error("Expected step3 to have executed (and failed)")
	}

	// Verify step2 was compensated but step1 failed to compensate
	if !data.Step2Compensated {
		t.Error("Expected step2 to have been compensated")
	}
	if data.Step1Compensated {
		t.Error("Expected step1 compensation to have failed")
	}

	// Now load the state into a new saga instance
	newData := TestData{
		FailOnStep3:           false, // This time step3 won't fail
		FailOnStep1Compensate: false, // This time compensation will succeed
	}
	newSaga, err := LoadOrCreateNewSaga(ctx, postgresStore, sagaID, &newData)
	if err != nil {
		t.Error(err)
	}

	// Verify the loaded state shows compensation was incomplete
	if newSaga.State.Status != failed {
		t.Errorf("Expected loaded status to be %s, got %s", failed, newSaga.State.Status)
	}

	// Verify the data was restored
	if newSaga.Data.Step2Compensated != true {
		t.Error("Expected loaded data to show step2 was compensated")
	}
	if newSaga.Data.Step1Compensated != false {
		t.Error("Expected loaded data to show step1 was NOT compensated")
	}

	// Re-add the steps to the new saga (since steps aren't persisted)
	newSaga.AddStep("step1",
		func(ctx context.Context, data *TestData) error {
			data.Step1Executed = true
			data.Count++
			data.Value += "-step1"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			data.Step1Compensated = true
			data.Count--
			return nil
		},
	).AddStep("step2",
		func(ctx context.Context, data *TestData) error {
			data.Step2Executed = true
			data.Count++
			data.Value += "-step2"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			data.Step2Compensated = true
			data.Count--
			return nil
		},
	).AddStep("step3",
		func(ctx context.Context, data *TestData) error {
			data.Step3Executed = true
			if data.FailOnStep3 {
				return errors.New("step3 intentionally failed")
			}
			data.Value += "-step3"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			return nil
		},
	)

	// Retry the compensation explicitly
	err = newSaga.Compensate(ctx)
	if err != nil {
		t.Fatalf("Expected compensation to succeed on retry, got error: %v", err)
	}

	// Verify compensation completed successfully this time
	if !newSaga.Data.Step1Compensated {
		t.Error("Expected step1 to have been compensated on retry")
	}

	// Clean up test data
	_, err = conn.Exec(ctx, "DELETE FROM saga_states WHERE saga_id = $1", sagaID)
	if err != nil {
		t.Logf("Warning: Failed to clean up test data: %v", err)
	}
}

// Test LoadState with compensated saga and re-execution
func TestSaga_LoadState_Postgres_CompensatedAndRetry(t *testing.T) {
	// Skip if no database connection is available
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	// Skip if no database connection is available
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping postgres test")
	}

	ctx := context.Background()

	// Connect to the database
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close(ctx)

	// Clean up any existing test data
	sagaID := "saga-postgres-compensated-001"
	_, err = conn.Exec(ctx, "DELETE FROM saga_states WHERE saga_id = $1", sagaID)
	if err != nil {
		t.Fatalf("Failed to clean up test data: %v", err)
	}

	// Create test data
	type TestData struct {
		Value            string
		Count            int
		Step1Executed    bool
		Step2Executed    bool
		Step3Executed    bool
		Step1Compensated bool
		FailOnStep2      bool
	}
	data := TestData{
		Value:       "initial",
		Count:       0,
		FailOnStep2: true, // This will cause step2 to fail
	}

	postgresStore := NewPostgresSagaStore(conn)
	postgresStore.CreateSchema(ctx)
	saga, err := LoadOrCreateNewSaga(ctx, postgresStore, sagaID, &data)
	if err != nil {
		t.Fatalf("exiting test %v", err)
	}

	// Add steps - step 2 will fail, triggering compensation
	saga.AddStep("step1",
		func(ctx context.Context, data *TestData) error {
			data.Step1Executed = true
			data.Count++
			data.Value += "-step1"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			data.Step1Compensated = true
			data.Count--
			return nil
		},
	).AddStep("step2",
		func(ctx context.Context, data *TestData) error {
			data.Step2Executed = true
			if data.FailOnStep2 {
				return errors.New("step2 intentionally failed")
			}
			data.Value += "-step2"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			return nil
		},
	).AddStep("step3",
		func(ctx context.Context, data *TestData) error {
			data.Step3Executed = true
			data.Value += "-step3"
			return nil
		},
		func(ctx context.Context, data *TestData) error {
			return nil
		},
	)

	// Execute the saga - it should fail and compensate
	err = saga.Execute(ctx)
	if err != nil {
		err = saga.Compensate(ctx)
		if err != nil {
			t.Fatal("Expected saga to fail, but it succeeded", err)
		}
	}

	// Verify the saga failed and was compensated
	if saga.State.Status != failed {
		t.Errorf("Expected status to be %s, got %s", failed, saga.State.Status)
	}
	if !data.Step1Executed {
		t.Error("Expected step1 to have executed")
	}
	if !data.Step2Executed {
		t.Error("Expected step2 to have executed (and failed)")
	}
	if data.Step3Executed {
		t.Error("Expected step3 NOT to have executed")
	}
	if !data.Step1Compensated {
		t.Error("Expected step1 to have been compensated")
	}

	// Now load the state into a new saga instance
	newData := TestData{
		FailOnStep2: false, // This time we won't fail
	}
	_, err = LoadOrCreateNewSaga(ctx, postgresStore, sagaID, &newData)
	if err == nil {
		t.Fatalf("%v", "should not be usable")
	}
}
