package db

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
)

func initBatch(ctx context.Context, sqlDB *sql.DB, provider string, n int) (int, error) {
	type result struct {
		n   int
		err error
	}

	fns := []func(context.Context, *sql.DB, string, int) (int, error){
		initUsers, initTags, initPostTags, initPosts, initComments,
	}

	results := make([]result, len(fns))
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Go(func() {
			s, err := fn(ctx, sqlDB, provider, n)
			results[i] = result{s, err}
		})
	}
	wg.Wait()

	sum := 0
	for _, r := range results {
		sum += r.n
		if r.err != nil {
			return sum, r.err
		}
	}
	return sum, nil
}

func initUsers(ctx context.Context, sqlDB *sql.DB, provider string, n int) (int, error) {
	args := make([]any, 0, n*2)
	base := int(uniqueSeq.Add(uint64(n))) - n
	for i := range n {
		seq := base + i
		args = append(args,
			namePool[seq%poolSize],
			fmt.Sprintf("u%d@x.co", seq), // sequential → no B-tree fragmentation on email index
		)
	}
	q := WithAction("INSERT INTO users (name, email) VALUES "+valuePlaceholders(provider, n, 2), "init_users")
	res, err := sqlDB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("insert users: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func initTags(ctx context.Context, sqlDB *sql.DB, provider string, n int) (int, error) {
	args := make([]any, 0, n)
	base := int(uniqueSeq.Add(uint64(n))) - n
	for i := range n {
		args = append(args, fmt.Sprintf("t%d", base+i)) // sequential → no B-tree fragmentation on name index
	}
	q := WithAction("INSERT INTO tags (name) VALUES "+valuePlaceholders(provider, n, 1), "init_tags")
	res, err := sqlDB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("insert tags: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func initPosts(ctx context.Context, sqlDB *sql.DB, provider string, n int) (int, error) {
	args := make([]any, 0, n*2)
	offset := rand.Intn(poolSize)
	for i := range n {
		idx := (offset + i) % poolSize
		args = append(args, rand.Intn(100000), bodyPool[idx])
	}
	q := WithAction("INSERT INTO posts (user_id, body) VALUES "+valuePlaceholders(provider, n, 2), "init_posts")
	res, err := sqlDB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("insert posts: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func initComments(ctx context.Context, sqlDB *sql.DB, provider string, n int) (int, error) {
	args := make([]any, 0, n*3)
	offset := rand.Intn(poolSize)
	for i := range n {
		args = append(args, rand.Intn(100000), rand.Intn(100000), commentPool[(offset+i)%poolSize])
	}
	q := WithAction("INSERT INTO comments (post_id, user_id, body) VALUES "+valuePlaceholders(provider, n, 3), "init_comments")
	res, err := sqlDB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("insert comments: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}

func initPostTags(ctx context.Context, sqlDB *sql.DB, provider string, n int) (int, error) {
	args := make([]any, 0, n*2)
	for range n {
		args = append(args, rand.Intn(100000), rand.Intn(100000))
	}
	vals := valuePlaceholders(provider, n, 2)
	var q string
	if provider == "postgres" {
		q = "INSERT INTO post_tags (post_id, tag_id) VALUES " + vals + " ON CONFLICT DO NOTHING"
	} else {
		q = "INSERT IGNORE INTO post_tags (post_id, tag_id) VALUES " + vals
	}
	q = WithAction(q, "init_post_tags")
	res, err := sqlDB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("insert post_tags: %w", err)
	}
	affected, _ := res.RowsAffected()
	return int(affected), nil
}
