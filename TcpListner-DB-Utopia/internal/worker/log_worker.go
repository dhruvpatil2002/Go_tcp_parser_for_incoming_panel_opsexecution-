package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shehbaaz/db-utopia/internal/db"
	"github.com/shehbaaz/db-utopia/internal/models"
)

func StartLogWorker(
	ctx context.Context,
	workerID int,
	dbPool *pgxpool.Pool,
	queue <-chan models.PanelLog,
) {
	StartBatchWorker(
		ctx,
		"LOG-WORKER",
		workerID,
		queue,
		1000,
		15*time.Second,
		func(ctx context.Context, batch []models.PanelLog) error {
			return db.BatchInsertPanelLogs(ctx, dbPool, batch)
		},
	)
}
