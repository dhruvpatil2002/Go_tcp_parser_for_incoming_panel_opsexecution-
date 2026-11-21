package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shehbaaz/db-utopia/internal/db"
	"github.com/shehbaaz/db-utopia/internal/models"
)

func StartHeartbeatWorker(
	ctx context.Context,
	workerID int,
	dbPool *pgxpool.Pool,
	queue <-chan models.PanelHeartBeat,
) {
	StartBatchWorker(
		ctx,
		"HEARTBEAT-WORKER",
		workerID,
		queue,
		1000,
		60*time.Second,
		func(ctx context.Context, batch []models.PanelHeartBeat) error {
			return db.BatchUpsertPanelHeartbeat(ctx, dbPool, batch)
		},
	)
}
