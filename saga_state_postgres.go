package gosaga

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type SagaStateRecord struct {
	SagaID            string          `db:"saga_id" primaryKey:"true"`
	TotalSteps        int             `db:"total_steps"`
	CurrentStep       int             `db:"current_step"`
	Status            string          `db:"status" index:"true"`
	DataJSON          json.RawMessage `db:"data"`
	FailedStep        string          `db:"failed_step"`
	CompensatedSteps  []int           `db:"compensated_steps"`
	CompensatedStatus SagaStatus      `db:"compensated_status"`
	CreatedAt         time.Time       `db:"created_at"`
	UpdatedAt         time.Time       `db:"updated_at"`
}

type PostgresSagaStore struct {
	pool *pgx.Conn
}

func NewPostgresSagaStore(pool *pgx.Conn) *PostgresSagaStore {
	return &PostgresSagaStore{
		pool: pool,
	}
}

func (s *PostgresSagaStore) SaveState(ctx context.Context, state *SagaState) error {
	query := `
        INSERT INTO saga_states
        (
        	saga_id,
        	current_step,
         	total_steps,
         	status,
          	data,
            failed_step,
            compensated_steps,
            compensated_status,
            created_at,
            updated_at
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        ON CONFLICT (saga_id) DO UPDATE
        SET
        	current_step = $2,
         	total_steps = $3,
        	status = $4,
         	data = $5,
          	failed_step = $6,
            compensated_steps = $7,
            compensated_status = $8,
            updated_at = $10
    `
	_, err := s.pool.Exec(ctx, query,
		state.SagaID,
		state.CurrentStep,
		state.TotalSteps,
		state.Status,
		state.Data,
		state.FailedStep,
		state.CompensatedSteps,
		state.CompensatedStatus,
		state.CreatedAt,
		time.Now(),
	)

	if err != nil {
		return err
	}

	return nil
}

func (s *PostgresSagaStore) LoadState(ctx context.Context, sagaID string) (*SagaState, error) {
	query := `
        SELECT
        	saga_id,
        	current_step,
         	total_steps,
          	status,
         	data,
           	failed_step,
            compensated_steps,
            compensated_status,
            created_at,
            updated_at
        FROM saga_states
        WHERE saga_id = $1
    `
	state := &SagaState{}

	err := s.pool.QueryRow(ctx, query, sagaID).Scan(
		&state.SagaID,
		&state.CurrentStep,
		&state.TotalSteps,
		&state.Status,
		&state.Data,
		&state.FailedStep,
		&state.CompensatedSteps,
		&state.CompensatedStatus,
		&state.CreatedAt,
		&state.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return state, nil
}

func (s *PostgresSagaStore) MarkComplete(ctx context.Context, sagaID string) error {
	return nil
}

func (s *PostgresSagaStore) CreateSchema(ctx context.Context) error {
	query :=
		`CREATE TABLE IF NOT EXISTS saga_states (
	    	saga_id VARCHAR(36) PRIMARY KEY,
	     	current_step INT NOT NULL DEFAULT 0,
	      	total_steps INT NOT NULL DEFAULT 0,
	       	status VARCHAR(50) NOT NULL,
	        data JSONB NOT NULL,
	        failed_step INT NULL,
	        compensated_steps INT[] NOT NULL DEFAULT '{}',
	        compensated_status VARCHAR(50) NULL,
	        created_at TIMESTAMP NOT NULL DEFAULT NOW(),
	        updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);
    	CREATE INDEX IF NOT EXISTS idx_saga_states_status ON saga_states(status);
     	CREATE INDEX IF NOT EXISTS idx_saga_states_updated_at ON saga_states(updated_at);
    `

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return err
	}
	return nil
}
