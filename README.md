# go-saga
A Go implementation of the saga pattern with support for distributed transactions, compensation strategies, and state persistence.

## Overview

The saga pattern is a design pattern for managing distributed transactions across microservices. Instead of a single ACID transaction, a saga coordinates a sequence of local transactions, each with a compensating transaction to undo its effects if needed.

This library provides:
- Type-safe saga orchestration using Go generics
- Multiple compensation strategies (Retry, FailFast, ContinueAll)
- Pluggable state persistence
- State loading and saga resumption
- Context-aware execution
- Fluent API for building sagas

## Installation

```bash
go get github.com/latebit-io/go-saga
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
)

type OrderData struct {
    OrderID   string
    Amount    float64
    Reserved  bool
    Charged   bool
}

func main() {
    data := &OrderData{
        OrderID: "order-123",
        Amount:  99.99,
    }

    // Create a new saga with no state persistence
    // LoadOrCreateNewSaga will load existing state if found, or create a new saga
    saga, err := LoadOrCreateNewSaga(context.Background(), NewNoStateStore(), "order-saga-123", data)
    if err != nil {
        fmt.Printf("Failed to load or create saga: %v\n", err)
        return
    }

    // Add steps with execute and compensate functions
    saga.AddStep("reserve-inventory",
        func(ctx context.Context, data *OrderData) error {
            // Execute: Reserve inventory
            data.Reserved = true
            fmt.Println("Inventory reserved")
            return nil
        },
        func(ctx context.Context, data *OrderData) error {
            // Compensate: Release inventory
            data.Reserved = false
            fmt.Println("Inventory released")
            return nil
        },
    ).AddStep("charge-payment",
        func(ctx context.Context, data *OrderData) error {
            // Execute: Charge payment
            data.Charged = true
            fmt.Println("Payment charged")
            return nil
        },
        func(ctx context.Context, data *OrderData) error {
            // Compensate: Refund payment
            data.Charged = false
            fmt.Println("Payment refunded")
            return nil
        },
    )

    // Execute the saga
    if err := saga.Execute(context.Background()); err != nil {
        fmt.Printf("Saga failed: %v\n", err)
        
        // Compensate on failure
        if compErr := saga.Compensate(context.Background()); compErr != nil {
            fmt.Printf("Compensation failed: %v\n", compErr)
        }
        return
    }

    fmt.Println("Saga completed successfully")
}
```

## Core Concepts

### Saga Creation

Create or load a saga with a state store, unique ID, and data:

```go
data := &MyData{}
saga, err := LoadOrCreateNewSaga(ctx, stateStore, "saga-id", data)
if err != nil {
    // Handle error - could be a database error or saga already completed
    return err
}
```

The `LoadOrCreateNewSaga` function will:
- Load existing saga state if found in the state store
- Create a new saga if no state exists
- Return an error if the saga is already completed
- Populate the data object with the saved state

### Adding Steps

Each step has two functions: execute and compensate.

```go
saga.AddStep("step-name",
    // Execute function - runs during forward execution
    func(ctx context.Context, data *MyData) error {
        // Perform action
        return nil
    },
    // Compensate function - runs during rollback
    func(ctx context.Context, data *MyData) error {
        // Undo action
        return nil
    },
)
```

### Execution Flow

1. **Execute**: Steps run sequentially. If any step fails, execution stops.
2. **Compensate**: If execution fails, compensation runs in reverse order.

```go
// Execute the saga
err := saga.Execute(ctx)
if err != nil {
    // Compensate on failure
    saga.Compensate(ctx)
}
```

## Compensation Strategies

### 1. FailFast (Default)

Stops compensation immediately on first failure.

```go
saga, err := LoadOrCreateNewSaga(ctx, stateStore, "saga-id", data)
// FailFast is the default strategy
```

Or explicitly:

```go
saga.WithCompensationStrategy(NewFailFastStrategy[MyData]())
```

### 2. Retry with Exponential Backoff

Retries failed compensations with exponential backoff.

```go
config := RetryConfig{
    MaxRetries:      3,
    InitialBackoff:  1 * time.Second,
    MaxBackoff:      30 * time.Second,
    BackoffMultiple: 2.0,
}

saga.WithCompensationStrategy(NewRetryStrategy[MyData](config))
```

Or use defaults:

```go
saga.WithCompensationStrategy(
    NewRetryStrategy[MyData](DefaultRetryConfig()),
)
```

### 3. ContinueAll

Attempts to compensate all steps, collecting errors.

```go
saga.WithCompensationStrategy(
    NewContinueAllStrategy[MyData](DefaultRetryConfig()),
)
```

Check for partial failures:

```go
err := saga.Compensate(ctx)
if compErr, ok := IsCompensationError(err); ok {
    for _, failure := range compErr.Failures {
        fmt.Printf("Step %s failed: %v\n", failure.StepName, failure.Error)
    }
}
```

## State Persistence

### No State Store (In-Memory)

For simple cases or testing:

```go
saga, err := LoadOrCreateNewSaga(ctx, NewNoStateStore(), "saga-id", data)
if err != nil {
    return err
}
```

### PostgreSQL State Store

For production use with state persistence:

```go
db, _ := sql.Open("postgres", connectionString)
stateStore := NewPostgresStateStore(db)

saga, err := LoadOrCreateNewSaga(ctx, stateStore, "saga-id", data)
if err != nil {
    return err
}
```

The PostgreSQL store requires a table:

```sql
CREATE TABLE saga_state (
    saga_id VARCHAR(255) PRIMARY KEY,
    total_steps INT,
    current_step INT,
    status VARCHAR(50),
    data JSONB,
    failed_step INT,
    compensated_steps JSONB,
    compensated_status VARCHAR(50),
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

### Custom State Store

Implement the `SagaStateStore` interface:

```go
type SagaStateStore interface {
    SaveState(ctx context.Context, state *SagaState) error
    LoadState(ctx context.Context, sagaID string) (*SagaState, error)
}
```

## Advanced Usage

### Resuming Sagas

When using a persistent state store, sagas can be resumed after failures:

```go
// First execution - saga fails at step 2
data := &MyData{}
saga, err := LoadOrCreateNewSaga(ctx, stateStore, "saga-123", data)
saga.AddStep("step1", execute1, compensate1)
saga.AddStep("step2", execute2, compensate2)  // Fails here
saga.AddStep("step3", execute3, compensate3)

err := saga.Execute(ctx)  // Fails at step 2

// Later - resume the same saga
data := &MyData{}
saga, err := LoadOrCreateNewSaga(ctx, stateStore, "saga-123", data)
// The data will be populated with the saved state
// Add the same steps again
saga.AddStep("step1", execute1, compensate1)
saga.AddStep("step2", execute2Fixed, compensate2)  // Fixed version
saga.AddStep("step3", execute3, compensate3)

err := saga.Execute(ctx)  // Continues from step 1
```

**Note**: Completed sagas (either successfully executed or fully compensated) cannot be resumed and will return an error.

### Custom Logger

```go
type MyLogger struct{}

func (l *MyLogger) Log(level string, msg string) {
    // Custom logging implementation
}

saga.logger = &MyLogger{}
```

### Context Cancellation

All operations respect context cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

err := saga.Execute(ctx)
```

### Complex Data Types

Use any type with your saga:

```go
type PaymentFlow struct {
    UserID      string
    Amount      float64
    Items       []Item
    Transaction *Transaction
}

data := &PaymentFlow{}
saga, err := LoadOrCreateNewSaga[PaymentFlow](ctx, stateStore, "payment-123", data)
if err != nil {
    return err
}
```

## Error Handling

### Execution Errors

```go
err := saga.Execute(ctx)
if err != nil {
    fmt.Printf("Saga failed at step %d: %v\n", saga.State.FailedStep, err)
}
```

### Compensation Errors

```go
err := saga.Compensate(ctx)
if err != nil {
    // Critical: manual intervention may be required
    if compErr, ok := IsCompensationError(err); ok {
        // Handle partial compensation failures
        for _, failure := range compErr.Failures {
            log.Printf("CRITICAL: Step %s compensation failed: %v",
                failure.StepName, failure.Error)
        }
    }
}
```

## Testing

Run tests:

```bash
go test -v
```

Run with coverage:

```bash
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Examples

### E-commerce Order Processing

```go
type Order struct {
    ID              string
    Items           []string
    InventoryLocked bool
    PaymentCharged  bool
    OrderConfirmed  bool
}

saga, err := LoadOrCreateNewSaga(ctx, stateStore, "order-"+orderID, &order)
if err != nil {
    return err
}

saga.AddStep("lock-inventory",
    func(ctx context.Context, o *Order) error {
        // Lock inventory
        o.InventoryLocked = true
        return inventoryService.Lock(ctx, o.Items)
    },
    func(ctx context.Context, o *Order) error {
        // Release inventory
        o.InventoryLocked = false
        return inventoryService.Release(ctx, o.Items)
    },
).AddStep("charge-payment",
    func(ctx context.Context, o *Order) error {
        // Charge payment
        o.PaymentCharged = true
        return paymentService.Charge(ctx, o.ID)
    },
    func(ctx context.Context, o *Order) error {
        // Refund payment
        o.PaymentCharged = false
        return paymentService.Refund(ctx, o.ID)
    },
).AddStep("confirm-order",
    func(ctx context.Context, o *Order) error {
        // Confirm order
        o.OrderConfirmed = true
        return orderService.Confirm(ctx, o.ID)
    },
    func(ctx context.Context, o *Order) error {
        // Cancel order
        o.OrderConfirmed = false
        return orderService.Cancel(ctx, o.ID)
    },
)

// Execute with retry compensation
saga.WithCompensationStrategy(NewRetryStrategy[Order](DefaultRetryConfig()))

if err := saga.Execute(ctx); err != nil {
    saga.Compensate(ctx)
}
```

### Bank Transfer

```go
type Transfer struct {
    FromAccount string
    ToAccount   string
    Amount      float64
    Debited     bool
    Credited    bool
}

saga, err := LoadOrCreateNewSaga(ctx, stateStore, "transfer-"+txID, &transfer)
if err != nil {
    return err
}

saga.AddStep("debit-account",
    func(ctx context.Context, t *Transfer) error {
        t.Debited = true
        return bankService.Debit(ctx, t.FromAccount, t.Amount)
    },
    func(ctx context.Context, t *Transfer) error {
        t.Debited = false
        return bankService.Credit(ctx, t.FromAccount, t.Amount)
    },
).AddStep("credit-account",
    func(ctx context.Context, t *Transfer) error {
        t.Credited = true
        return bankService.Credit(ctx, t.ToAccount, t.Amount)
    },
    func(ctx context.Context, t *Transfer) error {
        t.Credited = false
        return bankService.Debit(ctx, t.ToAccount, t.Amount)
    },
)
```

## Best Practices

1. **Idempotency**: Ensure both execute and compensate functions are idempotent
2. **Error Handling**: Always handle compensation errors - they may require manual intervention
3. **State Persistence**: Use a persistent state store in production to enable saga resumption
4. **Unique Saga IDs**: Use unique, meaningful saga IDs that allow you to identify and resume specific saga instances
5. **Timeouts**: Set appropriate context timeouts for long-running operations
6. **Logging**: Implement custom loggers for production monitoring
7. **Compensation Strategy**: Choose the right strategy for your use case
   - `FailFast`: When you need immediate awareness of compensation issues
   - `Retry`: When transient failures are expected
   - `ContinueAll`: When partial compensation is acceptable
8. **Saga Resumption**: Design your execute functions to be resumable - they should handle cases where previous steps may have already completed

## License

MIT

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.
