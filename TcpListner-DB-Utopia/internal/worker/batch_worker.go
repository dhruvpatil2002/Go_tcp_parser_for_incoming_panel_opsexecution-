package worker

import (
	"context"
	"time"

	"github.com/shehbaaz/db-utopia/internal/logger"
)

type BatchProcessor[T any] func(ctx context.Context, batch []T) error

func StartBatchWorker[T any](
	ctx context.Context,
	workerName string,
	workerID int,
	queue <-chan T,
	batchSize int,
	flushInterval time.Duration,
	process BatchProcessor[T],
) {

	logger.Infof("[%s-%d] started (batch=%d, flush=%s)",
		workerName, workerID, batchSize, flushInterval)
	batch := make([]T, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		//  Prevent using cancelled context
		if ctx.Err() != nil {
			return
		}

		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := process(dbCtx, batch)
		cancel()
		if err != nil {
			logger.Errorf("[%s-%d] batch failed: %v (size=%d)",
				workerName, workerID, err, len(batch))
		}

		batch = make([]T, 0, batchSize)
	}

	for {
		select {

		case item, ok := <-queue:

			if !ok {
				logger.Warnf("[%s-%d] shutting down (queue closed)", workerName, workerID)
				flush()
				return
			}

			batch = append(batch, item)

			if len(batch) >= batchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-ctx.Done():
			logger.Warnf("[%s-%d] shutting down (context cancelled)", workerName, workerID)
			flush()
			return
		}
	}
}
