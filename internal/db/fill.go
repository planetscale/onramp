// Package db
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brianvoe/gofakeit/v6"
)

const (
	batchSize  = 1500 // rows per INSERT
	numWorkers = 16   // parallel insert goroutines
	poolSize   = 512  // pre-generated fake-value entries
)

// fkOrder is the FK-safe insertion order for all tables.
var fkOrder = []string{"users", "tags", "posts", "comments", "post_tags"}

// valuePlaceholders builds (or returns a cached) VALUES (...), ... fragment.
var (
	phMu    sync.Mutex
	phCache = make(map[[3]int]string)
)

// String pools: generated once at init, cycled at insert time.
var (
	namePool    [poolSize]string
	bodyPool    [poolSize]string
	commentPool [poolSize]string
)

// uniqueSeq provides monotonically increasing suffixes for unique fields (email, tag name).
var uniqueSeq atomic.Uint64

func init() {
	for i := range poolSize {
		namePool[i] = gofakeit.Name()
		bodyPool[i] = gofakeit.Paragraph(1, 5, 5, " ")
		commentPool[i] = gofakeit.Sentence(50)
	}
}

func valuePlaceholders(provider string, numRows, argsPerRow int) string {
	pIdx := 0
	if provider == "postgres" {
		pIdx = 1
	}
	key := [3]int{pIdx, numRows, argsPerRow}

	phMu.Lock()
	if s, ok := phCache[key]; ok {
		phMu.Unlock()
		return s
	}
	phMu.Unlock()

	rows := make([]string, numRows)
	for i := range rows {
		params := make([]string, argsPerRow)
		for j := range params {
			if provider == "postgres" {
				params[j] = fmt.Sprintf("$%d", i*argsPerRow+j+1)
			} else {
				params[j] = "?"
			}
		}
		rows[i] = "(" + strings.Join(params, ", ") + ")"
	}
	s := strings.Join(rows, ", ")

	phMu.Lock()
	phCache[key] = s
	phMu.Unlock()
	return s
}

// FillInput configures a Fill operation.
type FillInput struct {
	Rows     int // 0 = use TargetGB mode
	TargetGB int // 0 = use Rows mode
	// OnBatch is called after each batch with the number of rows inserted.
	OnBatch func(rows int)
	// OnBytes is called after each DB size check (size mode only) with current byte count.
	OnBytes  func(bytes int64)
	SetTitle func(string)
}

// Fill populates the database with dummy data either to a row count or a size target.
func Fill(ctx context.Context, sqlDB *sql.DB, provider string, in FillInput) error {
	ins, err := initBatch(ctx, sqlDB, provider, batchSize)
	if err != nil {
		return err
	}
	in.OnBatch(ins)
	in.SetTitle("Filling...")

	if in.Rows != 0 {
		return fillByRows(ctx, sqlDB, in.Rows, in.OnBatch)
	}
	return fillBySize(ctx, sqlDB, provider, int64(in.TargetGB)*1024*1024*1024, in.OnBatch, in.OnBytes)
}

// parallelFillTable fans out n row insertions across numWorkers goroutines.
func fillByRows(ctx context.Context, sqlDB *sql.DB, n int, onBatch func(int)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	perWorker := (n + numWorkers - 1) / numWorkers
	errCh := make(chan error, numWorkers)
	var wg sync.WaitGroup

	for w := range numWorkers {
		start := w * perWorker
		if start >= n {
			break
		}
		rows := min(perWorker, n-start)
		wg.Go(func() {
			var err error
			total := 0
			for total < rows {
				batch := min(batchSize, n-total)
				inserted, e := insertBatch(ctx, sqlDB, batch)
				err = e
				if err != nil {
					break
				}
				total += inserted
				if onBatch != nil {
					onBatch(inserted)
				}
			}
			if err != nil {
				errCh <- err
				cancel()
			}
		})
	}

	wg.Wait()
	close(errCh)
	return <-errCh
}

// fillBySize runs parallel workers inserting continuously until the DB reaches targetBytes.
func fillBySize(ctx context.Context, sqlDB *sql.DB, provider string, targetBytes int64, onBatch func(int), onBytes func(int64)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, numWorkers+1)
	var wg sync.WaitGroup

	for w := range numWorkers {
		wg.Go(func() {
			tableIdx := w % len(fkOrder)

			for {
				if ctx.Err() != nil {
					return
				}
				tableIdx++
				inserted, err := insertBatch(ctx, sqlDB, batchSize)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					errCh <- err
					cancel()
					return
				}
				if onBatch != nil {
					onBatch(inserted)
				}
			}
		})
	}

	// Size-checker: polls every 3 s and cancels workers when target is reached.
	wg.Go(func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				curBytes, err := currentDBBytes(ctx, sqlDB, provider)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					errCh <- err
					cancel()
					return
				}
				if onBytes != nil {
					onBytes(curBytes)
				}
				if curBytes >= targetBytes {
					cancel()
					return
				}
			}
		}
	})

	wg.Wait()
	close(errCh)
	return <-errCh
}

// insertBatch inserts rows from inside the db via duplication
// doesnt insert rows to post tags to keep one table small
func insertBatch(ctx context.Context, sqlDB *sql.DB, n int) (int, error) {
	total := 0
	// Comments
	q := WithAction(fmt.Sprintf("INSERT INTO comments (post_id, user_id, body) SELECT post_id, user_id, body FROM comments LIMIT %d", n), "fill_comments")
	res, err := sqlDB.ExecContext(ctx, q)
	if err != nil {
		return total, fmt.Errorf("insert comments: %w", err)
	}
	affected, _ := res.RowsAffected()
	total += int(affected)
	// Users
	q = WithAction(fmt.Sprintf("INSERT INTO users (name, email) SELECT name, email FROM users LIMIT %d", n), "fill_users")
	res, err = sqlDB.ExecContext(ctx, q)
	if err != nil {
		return total, fmt.Errorf("insert users: %w", err)
	}
	affected, _ = res.RowsAffected()
	total += int(affected)
	// Tags
	q = WithAction(fmt.Sprintf("INSERT INTO tags (name) SELECT name FROM tags LIMIT %d", n), "fill_tags")
	res, err = sqlDB.ExecContext(ctx, q)
	if err != nil {
		return total, fmt.Errorf("insert tags: %w", err)
	}
	affected, _ = res.RowsAffected()
	total += int(affected)
	// Posts
	q = WithAction(fmt.Sprintf(`INSERT INTO posts (user_id, body) SELECT user_id, CONCAT(body, body) FROM posts LIMIT %d`, n), "fill_posts")
	res, err = sqlDB.ExecContext(ctx, q)
	if err != nil {
		return total, fmt.Errorf("insert posts: %w", err)
	}
	affected, _ = res.RowsAffected()
	total += int(affected)
	return total, nil
}

// currentDBBytes returns the total database size in bytes.
func currentDBBytes(ctx context.Context, sqlDB *sql.DB, provider string) (int64, error) {
	var size int64
	var err error
	if provider == "postgres" {
		err = sqlDB.QueryRowContext(ctx, WithAction("SELECT pg_database_size(current_database())", "db_size")).Scan(&size)
	} else {
		var a any
		_ = sqlDB.QueryRowContext(ctx, WithAction("ANALYZE TABLE posts", "db_size")).Scan(&a)
		_ = sqlDB.QueryRowContext(ctx, WithAction("ANALYZE TABLE users", "db_size")).Scan(&a)
		_ = sqlDB.QueryRowContext(ctx, WithAction("ANALYZE TABLE tags", "db_size")).Scan(&a)
		_ = sqlDB.QueryRowContext(ctx, WithAction("ANALYZE TABLE comments", "db_size")).Scan(&a)
		err = sqlDB.QueryRowContext(ctx, WithAction(`
			SELECT COALESCE(SUM(data_length + index_length), 0)
			FROM information_schema.tables
			WHERE table_schema = DATABASE()`, "db_size")).Scan(&size)
	}
	if err != nil {
		return 0, fmt.Errorf("query db size: %w", err)
	}
	return size, nil
}
