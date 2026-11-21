package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shehbaaz/db-utopia/internal/db"
	"github.com/shehbaaz/db-utopia/internal/models"
)

func StartUpdateIPWorker(
	ctx context.Context,
	workerID int,
	dbPool *pgxpool.Pool,
	queue <-chan models.PanelIPUpdate,
) {
	StartBatchWorker(
		ctx,
		"UPDATE-IP-WORKER",
		workerID,
		queue,
		1000,
		15*time.Second,
		func(ctx context.Context, batch []models.PanelIPUpdate) error {
			return db.BatchUpdatePanelIP(ctx, dbPool, batch)
		},
	)
}
