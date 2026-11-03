package hammer

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/brianvoe/gofakeit/v6"

	"onramp/internal/db"
	"onramp/internal/hammer/htypes"
)

// queryData is the template data available to flow query templates.
type queryData struct {
	UserID int64
}

// queryFuncMap returns a template.FuncMap with gofakeit functions.
// String values have single quotes escaped for SQL safety.
func queryFuncMap() template.FuncMap {
	esc := func(s string) string {
		return strings.ReplaceAll(s, "'", "''")
	}
	return template.FuncMap{
		"Name":     func() string { return esc(gofakeit.Name()) },
		"Email":    func() string { return esc(gofakeit.Email()) },
		"Word":     func() string { return esc(gofakeit.Word()) },
		"Sentence": func(n int) string { return esc(gofakeit.Sentence(n)) },
		"Paragraph": func(paragraphCount, sentenceCount, wordCount int) string {
			return esc(gofakeit.Paragraph(paragraphCount, sentenceCount, wordCount, " "))
		},
		"UUID": gofakeit.UUID,
	}
}

// renderQuery processes a flow query template with the given user ID.
func renderQuery(tmplStr string, userID int64) (string, error) {
	tmpl, err := template.New("q").Funcs(queryFuncMap()).Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, queryData{UserID: userID}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// placeholder returns the SQL positional placeholder for the given provider and index.
func placeholder(provider string, n int) string {
	if provider == "postgres" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// activeUser tracks a running user goroutine.
type activeUser struct {
	id     int
	userID int64
	cancel context.CancelFunc
}

// Engine drives the user simulation loop.
type Engine struct {
	state    *SimulationState
	db       *sql.DB
	provider string

	rootCtx    context.Context
	rootCancel context.CancelFunc

	startCh    chan struct{}
	stopCh     chan struct{}
	finishedCh chan struct{}

	mu        sync.Mutex
	users     map[int]*activeUser
	userOrder []int // insertion order for oldest-first deletion
	nextID    int
}

// NewEngine constructs an idle Engine.
func NewEngine(state *SimulationState, db *sql.DB, provider string) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		state:      state,
		db:         db,
		provider:   provider,
		rootCtx:    ctx,
		rootCancel: cancel,
		startCh:    make(chan struct{}, 1),
		stopCh:     make(chan struct{}, 1),
		users:      make(map[int]*activeUser),
		finishedCh: make(chan struct{}, 1),
	}
}

// Start signals the engine to begin the simulation.
func (e *Engine) Start() {
	select {
	case e.startCh <- struct{}{}:
	default:
	}
}

// Stop signals the engine to halt the simulation.
func (e *Engine) Stop() {
	select {
	case e.stopCh <- struct{}{}:
	default:
	}
}

// Shutdown permanently terminates the engine (called on server shutdown).
func (e *Engine) Shutdown() {
	e.rootCancel()
	<-e.finishedCh
}

// Run is the engine's main goroutine. Call via go engine.Run().
func (e *Engine) Run() {
	for {
		select {
		case <-e.rootCtx.Done():
			e.killAllUsers()
			close(e.finishedCh)
			return
		case <-e.startCh:
			e.runSimulation()
		}
	}
}

func (e *Engine) runSimulation() {
	uc := e.state.GetUserConfig()
	spawnInterval := spawnDuration(uc.NewUsersPerSecond)
	deleteInterval := spawnDuration(uc.DeletedUsersPerSecond)

	spawnTicker := time.NewTicker(spawnInterval)
	deleteTicker := time.NewTicker(deleteInterval)
	defer spawnTicker.Stop()
	defer deleteTicker.Stop()

	for {
		select {
		case <-e.rootCtx.Done():
			return
		case <-e.stopCh:
			return
		case <-spawnTicker.C:
			uc = e.state.GetUserConfig()
			e.mu.Lock()
			if len(e.users) < uc.MaxUsers {
				e.spawnUser(e.rootCtx, uc.UserFlowDelay)
			}
			e.mu.Unlock()
			// Dynamically adjust ticker if rate changed.
			if ni := spawnDuration(uc.NewUsersPerSecond); ni != spawnInterval {
				spawnInterval = ni
				spawnTicker.Reset(spawnInterval)
			}
		case <-deleteTicker.C:
			uc = e.state.GetUserConfig()
			e.mu.Lock()
			if len(e.users) > uc.MinUsers {
				e.killOldestUser()
			}
			e.mu.Unlock()
			if ni := spawnDuration(uc.DeletedUsersPerSecond); ni != deleteInterval {
				deleteInterval = ni
				deleteTicker.Reset(deleteInterval)
			}
		}
	}
}

func spawnDuration(perSecond float64) time.Duration {
	if perSecond <= 0 {
		return time.Hour // effectively stopped
	}
	return time.Duration(float64(time.Second) / perSecond)
}

// spawnUser creates a new user in the DB and starts its goroutine. Must hold e.mu.
func (e *Engine) spawnUser(ctx context.Context, flowDelay time.Duration) {
	userCtx, userCancel := context.WithCancel(ctx)
	id := e.nextID
	e.nextID++
	e.state.CurrentUsers.Add(1)

	u := &activeUser{id: id, cancel: userCancel}
	e.users[id] = u
	e.userOrder = append(e.userOrder, id)

	go func() {
		// Create user record in DB.
		userID, err := e.createUserInDB()
		if err != nil {
			log.Printf("[engine] user %d: create failed: %v", id, err)
			userCancel()
			return
		}

		// Update the activeUser record with the DB userID.
		e.mu.Lock()
		if au, ok := e.users[id]; ok {
			au.userID = userID
		}
		e.mu.Unlock()

		// Run flow loop until context is cancelled.
		e.userLoop(userCtx, id, userID, flowDelay)

		// Cleanup: delete user data from DB.
		e.deleteUserFromDB(userID)
	}()
}

// killOldestUser cancels the oldest user goroutine. Must hold e.mu.
func (e *Engine) killOldestUser() {
	for _, id := range e.userOrder {
		if u, ok := e.users[id]; ok {
			u.cancel()
			delete(e.users, id)
			e.removeFromOrder(id)
			return
		}
	}
}

// killAllUsers cancels every active user goroutine.
func (e *Engine) killAllUsers() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, u := range e.users {
		u.cancel()
	}
}

func (e *Engine) removeFromOrder(id int) {
	for i, v := range e.userOrder {
		if v == id {
			e.userOrder = append(e.userOrder[:i], e.userOrder[i+1:]...)
			return
		}
	}
}

// userLoop runs flows for a single simulated user.
func (e *Engine) userLoop(ctx context.Context, id int, userID int64, flowDelay time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !e.state.Running.Load() {
			time.Sleep(2 * time.Second)
			continue
		}

		flows := e.state.GetFlows()
		if len(flows) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(flowDelay):
			}
			continue
		}

		flow, ok := pickFlow(flows)
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-time.After(flowDelay):
			}
			continue
		}
		for _, queryTmpl := range flow.Queries {
			query, err := renderQuery(queryTmpl, userID)
			if err != nil {
				log.Printf("[engine] user %d: render query error: %v", id, err)
				continue
			}
			query = db.WithAction(query, "crud")
			queryType := queryVerb(query)
			start := time.Now()
			_, err = e.db.ExecContext(ctx, query)
			elapsed := time.Since(start)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[engine] user %d  %-20s  %-7s  %s  ERROR: %v", id, flow.Name, queryType, elapsed, err)
			} else {
				log.Printf("[engine] user %d  %-20s  %-7s  %s", id, flow.Name, queryType, elapsed)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(flowDelay):
		}
	}
}

// queryVerb extracts the first word of a SQL query (e.g. INSERT, DELETE, SELECT).
func queryVerb(query string) string {
	q := strings.TrimSpace(query)
	if i := strings.IndexByte(q, ' '); i > 0 {
		return strings.ToUpper(q[:i])
	}
	return strings.ToUpper(q)
}

// pickFlow selects a flow using weighted random selection.
// Flows with a weight of zero are excluded. If no flows have a positive weight,
// ok is false.
func pickFlow(flows []htypes.Flow) (htypes.Flow, bool) {
	total := 0.0
	for _, f := range flows {
		if f.Weight > 0 {
			total += f.Weight
		}
	}
	if total == 0 {
		return htypes.Flow{}, false
	}
	r := rand.Float64() * total
	cumulative := 0.0
	for _, f := range flows {
		if f.Weight <= 0 {
			continue
		}
		cumulative += f.Weight
		if r < cumulative {
			return f, true
		}
	}
	// Fallback for floating-point rounding: return the last positive-weight flow.
	for i := len(flows) - 1; i >= 0; i-- {
		if flows[i].Weight > 0 {
			return flows[i], true
		}
	}
	return htypes.Flow{}, false
}

// createUserInDB inserts a new user and returns the database-assigned ID.
func (e *Engine) createUserInDB() (int64, error) {
	name := gofakeit.Name()
	email := fmt.Sprintf("%s_%s@%s", gofakeit.Username(), gofakeit.UUID()[:8], gofakeit.DomainName())

	if e.provider == "postgres" {
		var id int64
		err := e.db.QueryRow(
			`INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id /*action='create_user'*/`,
			name, email,
		).Scan(&id)
		return id, err
	}

	result, err := e.db.Exec(
		`INSERT INTO users (name, email) VALUES (?, ?) /*action='create_user'*/`,
		name, email,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// deleteUserFromDB removes a user's posts, comments, and user record.
func (e *Engine) deleteUserFromDB(userID int64) {
	e.state.CurrentUsers.Add(-1)
	ph := placeholder(e.provider, 1)
	queries := []string{
		fmt.Sprintf(`DELETE FROM post_tags WHERE post_id IN (SELECT id FROM posts WHERE user_id = %s) /*action='delete_user'*/`, ph),
		fmt.Sprintf(`DELETE FROM comments WHERE user_id = %s /*action='delete_user'*/`, ph),
		fmt.Sprintf(`DELETE FROM posts WHERE user_id = %s /*action='delete_user'*/`, ph),
		fmt.Sprintf(`DELETE FROM users WHERE id = %s /*action='delete_user'*/`, ph),
	}
	for _, q := range queries {
		e.db.Exec(q, userID)
	}
}
