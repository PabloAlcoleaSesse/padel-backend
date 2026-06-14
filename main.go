package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type app struct {
	db *pgxpool.Pool
}

type player struct {
	ID          string    `json:"id"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	Gender      string    `json:"gender"`
	IsAvailable bool      `json:"is_available"`
	CreatedAt   time.Time `json:"created_at"`
}

type pair struct {
	ID        string    `json:"id"`
	Player1ID string    `json:"player1_id"`
	Player2ID string    `json:"player2_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type match struct {
	ID           string     `json:"id"`
	GroupID      *string    `json:"group_id,omitempty"`
	Pair1ID      string     `json:"pair1_id"`
	Pair2ID      string     `json:"pair2_id"`
	Pair1Name    string     `json:"pair1_name,omitempty"`
	Pair2Name    string     `json:"pair2_name,omitempty"`
	Round        string     `json:"round"`
	Status       string     `json:"status"`
	ScheduledAt  *time.Time `json:"scheduled_at,omitempty"`
	WinnerPairID *string    `json:"winner_pair_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	Sets         []matchSet `json:"sets,omitempty"`
}

type matchSet struct {
	ID         string `json:"id,omitempty"`
	MatchID    string `json:"match_id,omitempty"`
	SetNumber  int    `json:"set_number,omitempty"`
	Pair1Games int    `json:"pair1_games"`
	Pair2Games int    `json:"pair2_games"`
}

type groupResponse struct {
	Name  string              `json:"name"`
	Pairs []groupPairResponse `json:"pairs"`
}

type groupPairResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	PJ    int    `json:"pj"`
	PG    int    `json:"pg"`
	PP    int    `json:"pp"`
	Sets  string `json:"sets"`
	Games string `json:"games"`
	PTS   int    `json:"pts"`
}

type standingsRow struct {
	GroupID   string
	GroupName string
	PairID    string
	PairName  string
	Played    int
	Wins      int
	Losses    int
	SetsWon   int
	SetsLost  int
	GamesWon  int
	GamesLost int
	Points    int
}

type apiError struct {
	Error string `json:"error"`
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}

	a := &app{db: pool}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /players", a.listPlayers)
	mux.HandleFunc("POST /players", a.createPlayer)
	mux.HandleFunc("POST /tournament/randomize", a.randomizeTournament)
	mux.HandleFunc("GET /groups", a.listGroups)
	mux.HandleFunc("GET /matches", a.listMatches)
	mux.HandleFunc("POST /matches/{id}/result", a.submitMatchResult)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, cors(jsonOnly(mux))); err != nil {
		log.Fatal(err)
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) listPlayers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		select id, first_name, last_name, gender, is_available, created_at
		from players
		order by created_at, last_name, first_name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list players")
		return
	}
	defer rows.Close()

	players, err := pgx.CollectRows(rows, pgx.RowToStructByName[player])
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read players")
		return
	}
	writeJSON(w, http.StatusOK, players)
}

func (a *app) createPlayer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		Gender      string `json:"gender"`
		IsAvailable *bool  `json:"is_available"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Gender = strings.ToLower(strings.TrimSpace(req.Gender))
	if req.FirstName == "" || req.LastName == "" {
		writeError(w, http.StatusBadRequest, "first_name and last_name are required")
		return
	}
	if req.Gender != "male" && req.Gender != "female" {
		writeError(w, http.StatusBadRequest, "gender must be male or female")
		return
	}
	isAvailable := true
	if req.IsAvailable != nil {
		isAvailable = *req.IsAvailable
	}

	var p player
	err := a.db.QueryRow(r.Context(), `
		insert into players (first_name, last_name, gender, is_available)
		values ($1, $2, $3, $4)
		returning id, first_name, last_name, gender, is_available, created_at`,
		req.FirstName, req.LastName, req.Gender, isAvailable,
	).Scan(&p.ID, &p.FirstName, &p.LastName, &p.Gender, &p.IsAvailable, &p.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create player")
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

func (a *app) randomizeTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, err := a.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer rollback(ctx, tx)

	rows, err := tx.Query(ctx, `
		select id, first_name, last_name, gender, is_available, created_at
		from players
		where is_available = true
		order by created_at
		for update`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load available players")
		return
	}
	available, err := pgx.CollectRows(rows, pgx.RowToStructByName[player])
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read available players")
		return
	}

	var males, females []player
	for _, p := range available {
		switch p.Gender {
		case "male":
			males = append(males, p)
		case "female":
			females = append(females, p)
		}
	}
	if len(males) != 6 || len(females) != 6 {
		writeError(w, http.StatusBadRequest, "tournament requires exactly 6 available male players and 6 available female players")
		return
	}

	if err := shufflePlayers(males); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to shuffle males")
		return
	}
	if err := shufflePlayers(females); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to shuffle females")
		return
	}

	if _, err := tx.Exec(ctx, `delete from match_sets`); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset match sets")
		return
	}
	if _, err := tx.Exec(ctx, `delete from matches`); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset matches")
		return
	}
	if _, err := tx.Exec(ctx, `delete from group_standings`); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset standings")
		return
	}
	if _, err := tx.Exec(ctx, `delete from group_pairs`); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset group pairs")
		return
	}
	if _, err := tx.Exec(ctx, `delete from pairs`); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset pairs")
		return
	}

	groupAID, err := upsertGroup(ctx, tx, "Group A")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Group A")
		return
	}
	groupBID, err := upsertGroup(ctx, tx, "Group B")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Group B")
		return
	}

	pairs := make([]pair, 0, 6)
	for i := range males {
		name := pairName(males[i], females[i])
		var pr pair
		err := tx.QueryRow(ctx, `
			insert into pairs (player1_id, player2_id, name)
			values ($1, $2, $3)
			returning id, player1_id, player2_id, name, created_at`,
			males[i].ID, females[i].ID, name,
		).Scan(&pr.ID, &pr.Player1ID, &pr.Player2ID, &pr.Name, &pr.CreatedAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create pair")
			return
		}
		pairs = append(pairs, pr)
	}
	if err := shufflePairs(pairs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to shuffle pairs")
		return
	}

	groupIDs := []string{groupAID, groupAID, groupAID, groupBID, groupBID, groupBID}
	for i, pr := range pairs {
		if _, err := tx.Exec(ctx, `insert into group_pairs (group_id, pair_id) values ($1, $2)`, groupIDs[i], pr.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to assign pair to group")
			return
		}
		if _, err := tx.Exec(ctx, `insert into group_standings (group_id, pair_id) values ($1, $2)`, groupIDs[i], pr.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create standings")
			return
		}
	}

	createdMatches := make([]match, 0, 6)
	if err := createGroupMatches(ctx, tx, groupAID, pairs[:3], &createdMatches); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Group A matches")
		return
	}
	if err := createGroupMatches(ctx, tx, groupBID, pairs[3:], &createdMatches); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Group B matches")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit tournament")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"groups":  []map[string]string{{"id": groupAID, "name": "Group A"}, {"id": groupBID, "name": "Group B"}},
		"pairs":   pairs,
		"matches": createdMatches,
	})
}

func (a *app) listGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		select g.id, g.name, p.id, p.name, gs.played, gs.wins, gs.losses,
		       gs.sets_won, gs.sets_lost, gs.games_won, gs.games_lost, gs.points
		from groups g
		join group_standings gs on gs.group_id = g.id
		join pairs p on p.id = gs.pair_id
		order by g.name,
		         gs.points desc,
		         (gs.sets_won - gs.sets_lost) desc,
		         (gs.games_won - gs.games_lost) desc,
		         gs.sets_won desc,
		         gs.games_won desc,
		         p.name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	defer rows.Close()

	groupMap := map[string]*groupResponse{}
	order := []string{}
	for rows.Next() {
		var row standingsRow
		if err := rows.Scan(&row.GroupID, &row.GroupName, &row.PairID, &row.PairName, &row.Played, &row.Wins, &row.Losses, &row.SetsWon, &row.SetsLost, &row.GamesWon, &row.GamesLost, &row.Points); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read groups")
			return
		}
		gr, ok := groupMap[row.GroupID]
		if !ok {
			gr = &groupResponse{Name: row.GroupName}
			groupMap[row.GroupID] = gr
			order = append(order, row.GroupID)
		}
		gr.Pairs = append(gr.Pairs, groupPairResponse{
			ID:    row.PairID,
			Name:  row.PairName,
			PJ:    row.Played,
			PG:    row.Wins,
			PP:    row.Losses,
			Sets:  fmt.Sprintf("%d-%d", row.SetsWon, row.SetsLost),
			Games: fmt.Sprintf("%d-%d", row.GamesWon, row.GamesLost),
			PTS:   row.Points,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read groups")
		return
	}

	resp := make([]groupResponse, 0, len(order))
	for _, groupID := range order {
		resp = append(resp, *groupMap[groupID])
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *app) listMatches(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		select m.id, m.group_id, m.pair1_id, m.pair2_id, p1.name, p2.name,
		       m.round, m.status, m.scheduled_at, m.winner_pair_id, m.created_at
		from matches m
		join pairs p1 on p1.id = m.pair1_id
		join pairs p2 on p2.id = m.pair2_id
		order by case m.round when 'group' then 1 when 'semifinal' then 2 else 3 end, m.created_at`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list matches")
		return
	}
	defer rows.Close()

	resp := map[string][]match{"group": {}, "semifinal": {}, "final": {}}
	for rows.Next() {
		var m match
		if err := rows.Scan(&m.ID, &m.GroupID, &m.Pair1ID, &m.Pair2ID, &m.Pair1Name, &m.Pair2Name, &m.Round, &m.Status, &m.ScheduledAt, &m.WinnerPairID, &m.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read matches")
			return
		}
		resp[m.Round] = append(resp[m.Round], m)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read matches")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *app) submitMatchResult(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	var req struct {
		Sets []matchSet `json:"sets"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Sets) == 0 || len(req.Sets) > 3 {
		writeError(w, http.StatusBadRequest, "sets must contain between 1 and 3 sets")
		return
	}
	for _, set := range req.Sets {
		if set.Pair1Games < 0 || set.Pair2Games < 0 || set.Pair1Games == set.Pair2Games {
			writeError(w, http.StatusBadRequest, "each set must have non-negative games and a winner")
			return
		}
	}

	ctx := r.Context()
	tx, err := a.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer rollback(ctx, tx)

	var m match
	err = tx.QueryRow(ctx, `
		select id, group_id, pair1_id, pair2_id, round, status, scheduled_at, winner_pair_id, created_at
		from matches
		where id = $1
		for update`, matchID,
	).Scan(&m.ID, &m.GroupID, &m.Pair1ID, &m.Pair2ID, &m.Round, &m.Status, &m.ScheduledAt, &m.WinnerPairID, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load match")
		return
	}
	if m.Status == "completed" {
		writeError(w, http.StatusConflict, "match is already completed")
		return
	}

	stats := calculateMatchStats(req.Sets)
	if stats.Pair1SetsWon == stats.Pair2SetsWon {
		writeError(w, http.StatusBadRequest, "result must have a match winner")
		return
	}
	winnerID := m.Pair1ID
	if stats.Pair2SetsWon > stats.Pair1SetsWon {
		winnerID = m.Pair2ID
	}

	for i, set := range req.Sets {
		_, err := tx.Exec(ctx, `
			insert into match_sets (match_id, set_number, pair1_games, pair2_games)
			values ($1, $2, $3, $4)`,
			m.ID, i+1, set.Pair1Games, set.Pair2Games,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save match sets")
			return
		}
	}

	if _, err := tx.Exec(ctx, `
		update matches
		set status = 'completed', winner_pair_id = $1
		where id = $2`, winnerID, m.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete match")
		return
	}

	if m.Round == "group" {
		if m.GroupID == nil {
			writeError(w, http.StatusInternalServerError, "group match is missing group_id")
			return
		}
		if err := updateStandings(ctx, tx, *m.GroupID, m.Pair1ID, m.Pair2ID, winnerID, stats); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update standings")
			return
		}
		if err := createSemifinalsIfReady(ctx, tx); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if m.Round == "semifinal" {
		if err := createFinalIfReady(ctx, tx); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit result")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"match_id":       m.ID,
		"status":         "completed",
		"winner_pair_id": winnerID,
	})
}

type matchStats struct {
	Pair1SetsWon  int
	Pair2SetsWon  int
	Pair1GamesWon int
	Pair2GamesWon int
}

func calculateMatchStats(sets []matchSet) matchStats {
	var stats matchStats
	for _, set := range sets {
		stats.Pair1GamesWon += set.Pair1Games
		stats.Pair2GamesWon += set.Pair2Games
		if set.Pair1Games > set.Pair2Games {
			stats.Pair1SetsWon++
		} else {
			stats.Pair2SetsWon++
		}
	}
	return stats
}

func updateStandings(ctx context.Context, tx pgx.Tx, groupID, pair1ID, pair2ID, winnerID string, stats matchStats) error {
	pair1Won := winnerID == pair1ID
	pair2Won := winnerID == pair2ID

	_, err := tx.Exec(ctx, `
		update group_standings
		set played = played + 1,
		    wins = wins + $1,
		    losses = losses + $2,
		    sets_won = sets_won + $3,
		    sets_lost = sets_lost + $4,
		    games_won = games_won + $5,
		    games_lost = games_lost + $6,
		    points = points + $1
		where group_id = $7 and pair_id = $8`,
		boolInt(pair1Won), boolInt(!pair1Won), stats.Pair1SetsWon, stats.Pair2SetsWon, stats.Pair1GamesWon, stats.Pair2GamesWon, groupID, pair1ID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		update group_standings
		set played = played + 1,
		    wins = wins + $1,
		    losses = losses + $2,
		    sets_won = sets_won + $3,
		    sets_lost = sets_lost + $4,
		    games_won = games_won + $5,
		    games_lost = games_lost + $6,
		    points = points + $1
		where group_id = $7 and pair_id = $8`,
		boolInt(pair2Won), boolInt(!pair2Won), stats.Pair2SetsWon, stats.Pair1SetsWon, stats.Pair2GamesWon, stats.Pair1GamesWon, groupID, pair2ID,
	)
	return err
}

func createSemifinalsIfReady(ctx context.Context, tx pgx.Tx) error {
	var pendingGroupMatches int
	if err := tx.QueryRow(ctx, `select count(*) from matches where round = 'group' and status <> 'completed'`).Scan(&pendingGroupMatches); err != nil {
		return fmt.Errorf("failed to check group matches")
	}
	if pendingGroupMatches > 0 {
		return nil
	}

	var existingSemifinals int
	if err := tx.QueryRow(ctx, `select count(*) from matches where round = 'semifinal'`).Scan(&existingSemifinals); err != nil {
		return fmt.Errorf("failed to check semifinals")
	}
	if existingSemifinals > 0 {
		return nil
	}

	leaders, err := groupLeaders(ctx, tx)
	if err != nil {
		return err
	}
	a := leaders["Group A"]
	b := leaders["Group B"]
	if len(a) < 2 || len(b) < 2 {
		return fmt.Errorf("failed to determine semifinalists")
	}

	if _, err := tx.Exec(ctx, `insert into matches (pair1_id, pair2_id, round) values ($1, $2, 'semifinal')`, a[0].PairID, b[1].PairID); err != nil {
		return fmt.Errorf("failed to create semifinal 1")
	}
	if _, err := tx.Exec(ctx, `insert into matches (pair1_id, pair2_id, round) values ($1, $2, 'semifinal')`, b[0].PairID, a[1].PairID); err != nil {
		return fmt.Errorf("failed to create semifinal 2")
	}
	return nil
}

func createFinalIfReady(ctx context.Context, tx pgx.Tx) error {
	var existingFinals int
	if err := tx.QueryRow(ctx, `select count(*) from matches where round = 'final'`).Scan(&existingFinals); err != nil {
		return fmt.Errorf("failed to check final")
	}
	if existingFinals > 0 {
		return nil
	}

	rows, err := tx.Query(ctx, `
		select winner_pair_id
		from matches
		where round = 'semifinal'
		order by created_at`)
	if err != nil {
		return fmt.Errorf("failed to read semifinals")
	}
	defer rows.Close()

	var winners []string
	for rows.Next() {
		var winnerID *string
		if err := rows.Scan(&winnerID); err != nil {
			return fmt.Errorf("failed to read semifinal winner")
		}
		if winnerID != nil {
			winners = append(winners, *winnerID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read semifinal winners")
	}
	if len(winners) < 2 {
		return nil
	}

	if _, err := tx.Exec(ctx, `insert into matches (pair1_id, pair2_id, round) values ($1, $2, 'final')`, winners[0], winners[1]); err != nil {
		return fmt.Errorf("failed to create final")
	}
	return nil
}

func groupLeaders(ctx context.Context, tx pgx.Tx) (map[string][]standingsRow, error) {
	rows, err := tx.Query(ctx, `
		select g.id, g.name, p.id, p.name, gs.played, gs.wins, gs.losses,
		       gs.sets_won, gs.sets_lost, gs.games_won, gs.games_lost, gs.points
		from group_standings gs
		join groups g on g.id = gs.group_id
		join pairs p on p.id = gs.pair_id`)
	if err != nil {
		return nil, fmt.Errorf("failed to read standings")
	}
	defer rows.Close()

	leaders := map[string][]standingsRow{}
	for rows.Next() {
		var row standingsRow
		if err := rows.Scan(&row.GroupID, &row.GroupName, &row.PairID, &row.PairName, &row.Played, &row.Wins, &row.Losses, &row.SetsWon, &row.SetsLost, &row.GamesWon, &row.GamesLost, &row.Points); err != nil {
			return nil, fmt.Errorf("failed to scan standings")
		}
		leaders[row.GroupName] = append(leaders[row.GroupName], row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read standings")
	}

	for groupName := range leaders {
		sort.Slice(leaders[groupName], func(i, j int) bool {
			left := leaders[groupName][i]
			right := leaders[groupName][j]
			return compareStandings(left, right)
		})
	}
	return leaders, nil
}

func compareStandings(a, b standingsRow) bool {
	if a.Points != b.Points {
		return a.Points > b.Points
	}
	if setDiffA, setDiffB := a.SetsWon-a.SetsLost, b.SetsWon-b.SetsLost; setDiffA != setDiffB {
		return setDiffA > setDiffB
	}
	if gameDiffA, gameDiffB := a.GamesWon-a.GamesLost, b.GamesWon-b.GamesLost; gameDiffA != gameDiffB {
		return gameDiffA > gameDiffB
	}
	if a.SetsWon != b.SetsWon {
		return a.SetsWon > b.SetsWon
	}
	if a.GamesWon != b.GamesWon {
		return a.GamesWon > b.GamesWon
	}
	return a.PairName < b.PairName
}

func createGroupMatches(ctx context.Context, tx pgx.Tx, groupID string, groupPairs []pair, matches *[]match) error {
	fixtures := [][2]int{{0, 1}, {0, 2}, {1, 2}}
	for _, fixture := range fixtures {
		pair1 := groupPairs[fixture[0]]
		pair2 := groupPairs[fixture[1]]
		var m match
		err := tx.QueryRow(ctx, `
			insert into matches (group_id, pair1_id, pair2_id, round)
			values ($1, $2, $3, 'group')
			returning id, group_id, pair1_id, pair2_id, round, status, scheduled_at, winner_pair_id, created_at`,
			groupID, pair1.ID, pair2.ID,
		).Scan(&m.ID, &m.GroupID, &m.Pair1ID, &m.Pair2ID, &m.Round, &m.Status, &m.ScheduledAt, &m.WinnerPairID, &m.CreatedAt)
		if err != nil {
			return err
		}
		m.Pair1Name = pair1.Name
		m.Pair2Name = pair2.Name
		*matches = append(*matches, m)
	}
	return nil
}

func upsertGroup(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		insert into groups (name)
		values ($1)
		on conflict (name) do update set name = excluded.name
		returning id`, name,
	).Scan(&id)
	return id, err
}

func pairName(male player, female player) string {
	return fmt.Sprintf("%s. %s · %s. %s",
		initial(male.FirstName), male.LastName,
		initial(female.FirstName), female.LastName,
	)
}

func initial(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToUpper(string([]rune(name)[0]))
}

func shufflePlayers(items []player) error {
	return shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
}

func shufflePairs(items []pair) error {
	return shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
}

func shuffle(n int, swap func(i, j int)) error {
	for i := n - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		swap(i, int(jBig.Int64()))
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiError{Error: message})
}

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		log.Printf("rollback: %v", err)
	}
}
