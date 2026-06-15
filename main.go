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

const (
	tournamentPairs       = 5
	tournamentGroupName   = "Group A"
	semifinalistsRequired = 4
)

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

type pairSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type displayMatch struct {
	ID           string      `json:"id"`
	Round        string      `json:"round"`
	GroupName    *string     `json:"group_name,omitempty"`
	Pair1        pairSummary `json:"pair1"`
	Pair2        pairSummary `json:"pair2"`
	Status       string      `json:"status"`
	WinnerPairID *string     `json:"winner_pair_id,omitempty"`
	Sets         []scoreSet  `json:"sets"`
}

type scoreSet struct {
	SetNumber  int `json:"set_number"`
	Pair1Games int `json:"pair1_games"`
	Pair2Games int `json:"pair2_games"`
}

type groupMatchesResponse struct {
	Name    string         `json:"name"`
	Matches []displayMatch `json:"matches"`
}

type bracketResponse struct {
	Semifinals []displayMatch `json:"semifinals"`
	ThirdPlace *displayMatch  `json:"third_place"`
	Final      *displayMatch  `json:"final"`
}

type championsResponse struct {
	Champion        *pairSummary  `json:"champion"`
	RunnerUp        *pairSummary  `json:"runner_up"`
	ThirdPlace      *pairSummary  `json:"third_place"`
	Final           *displayMatch `json:"final"`
	ThirdPlaceMatch *displayMatch `json:"third_place_match"`
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
	if err := ensureDatabaseUpgrades(ctx, pool); err != nil {
		log.Fatalf("upgrade database: %v", err)
	}
	if err := ensureDerivedMatches(ctx, pool); err != nil {
		log.Fatalf("reconcile tournament matches: %v", err)
	}

	a := &app{db: pool}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /players", a.listPlayers)
	mux.HandleFunc("POST /players", a.createPlayer)
	mux.HandleFunc("PUT /players/{id}", a.updatePlayer)
	mux.HandleFunc("POST /tournament/reset", a.resetTournament)
	mux.HandleFunc("POST /tournament/randomize", a.randomizeTournament)
	mux.HandleFunc("GET /tournament", a.tournamentOverview)
	mux.HandleFunc("GET /groups", a.listGroups)
	mux.HandleFunc("GET /results", a.listResults)
	mux.HandleFunc("GET /bracket", a.listBracket)
	mux.HandleFunc("GET /champions", a.listChampions)
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
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

func ensureDatabaseUpgrades(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `alter type match_round add value if not exists 'third_place' after 'semifinal'`)
	return err
}

func ensureDerivedMatches(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	if err := createFinalsIfReady(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
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

	normalized, err := normalizePlayerInput(req.FirstName, req.LastName, req.Gender, req.IsAvailable)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var p player
	err = a.db.QueryRow(r.Context(), `
		insert into players (first_name, last_name, gender, is_available)
		values ($1, $2, $3, $4)
		returning id, first_name, last_name, gender, is_available, created_at`,
		normalized.FirstName, normalized.LastName, normalized.Gender, normalized.IsAvailable,
	).Scan(&p.ID, &p.FirstName, &p.LastName, &p.Gender, &p.IsAvailable, &p.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create player")
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

func (a *app) updatePlayer(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("id")
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

	normalized, err := normalizePlayerInput(req.FirstName, req.LastName, req.Gender, req.IsAvailable)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var p player
	err = a.db.QueryRow(r.Context(), `
		update players
		set first_name = $1, last_name = $2, gender = $3, is_available = $4
		where id = $5
		returning id, first_name, last_name, gender, is_available, created_at`,
		normalized.FirstName, normalized.LastName, normalized.Gender, normalized.IsAvailable, playerID,
	).Scan(&p.ID, &p.FirstName, &p.LastName, &p.Gender, &p.IsAvailable, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update player")
		return
	}

	writeJSON(w, http.StatusOK, p)
}

type normalizedPlayerInput struct {
	FirstName   string
	LastName    string
	Gender      string
	IsAvailable bool
}

func normalizePlayerInput(firstName, lastName, gender string, isAvailable *bool) (normalizedPlayerInput, error) {
	input := normalizedPlayerInput{
		FirstName:   strings.TrimSpace(firstName),
		LastName:    strings.TrimSpace(lastName),
		Gender:      strings.ToLower(strings.TrimSpace(gender)),
		IsAvailable: true,
	}
	if isAvailable != nil {
		input.IsAvailable = *isAvailable
	}
	if input.FirstName == "" || input.LastName == "" {
		return input, errors.New("first_name and last_name are required")
	}
	if input.Gender != "male" && input.Gender != "female" {
		return input, errors.New("gender must be male or female")
	}
	return input, nil
}

func (a *app) resetTournament(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, err := a.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer rollback(ctx, tx)

	if err := clearTournament(ctx, tx); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit tournament reset")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func clearTournament(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `delete from match_sets`); err != nil {
		return fmt.Errorf("failed to reset match sets")
	}
	if _, err := tx.Exec(ctx, `delete from matches`); err != nil {
		return fmt.Errorf("failed to reset matches")
	}
	if _, err := tx.Exec(ctx, `delete from group_standings`); err != nil {
		return fmt.Errorf("failed to reset standings")
	}
	if _, err := tx.Exec(ctx, `delete from group_pairs`); err != nil {
		return fmt.Errorf("failed to reset group pairs")
	}
	if _, err := tx.Exec(ctx, `delete from pairs`); err != nil {
		return fmt.Errorf("failed to reset pairs")
	}
	return nil
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
	if len(males) != tournamentPairs || len(females) != tournamentPairs {
		writeError(w, http.StatusBadRequest, "tournament requires exactly 5 available male players and 5 available female players")
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

	if err := clearTournament(ctx, tx); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	groupAID, err := upsertGroup(ctx, tx, tournamentGroupName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create group")
		return
	}

	pairs := make([]pair, 0, tournamentPairs)
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

	for _, pr := range pairs {
		if _, err := tx.Exec(ctx, `insert into group_pairs (group_id, pair_id) values ($1, $2)`, groupAID, pr.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to assign pair to group")
			return
		}
		if _, err := tx.Exec(ctx, `insert into group_standings (group_id, pair_id) values ($1, $2)`, groupAID, pr.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create standings")
			return
		}
	}

	createdMatches := make([]match, 0, tournamentPairs*(tournamentPairs-1)/2)
	if err := createGroupMatches(ctx, tx, groupAID, pairs, &createdMatches); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create group matches")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit tournament")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"groups":  []map[string]string{{"id": groupAID, "name": tournamentGroupName}},
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

func (a *app) tournamentOverview(w http.ResponseWriter, r *http.Request) {
	groups, err := a.loadGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load groups")
		return
	}
	matches, err := a.loadDisplayMatches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load matches")
		return
	}

	results := groupStageMatches(matches)
	bracket := buildBracket(matches)
	champions := buildChampions(matches)

	writeJSON(w, http.StatusOK, map[string]any{
		"groups":    groups,
		"results":   results,
		"bracket":   bracket,
		"champions": champions,
	})
}

func (a *app) listResults(w http.ResponseWriter, r *http.Request) {
	matches, err := a.loadDisplayMatches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load results")
		return
	}
	writeJSON(w, http.StatusOK, groupStageMatches(matches))
}

func (a *app) listBracket(w http.ResponseWriter, r *http.Request) {
	matches, err := a.loadDisplayMatches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load bracket")
		return
	}
	writeJSON(w, http.StatusOK, buildBracket(matches))
}

func (a *app) listChampions(w http.ResponseWriter, r *http.Request) {
	matches, err := a.loadDisplayMatches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load champions")
		return
	}
	writeJSON(w, http.StatusOK, buildChampions(matches))
}

func (a *app) listMatches(w http.ResponseWriter, r *http.Request) {
	matches, err := a.loadDisplayMatches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list matches")
		return
	}

	resp := map[string][]displayMatch{"group": {}, "semifinal": {}, "third_place": {}, "final": {}}
	for _, m := range matches {
		resp[m.Round] = append(resp[m.Round], m)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *app) loadGroups(ctx context.Context) ([]groupResponse, error) {
	rows, err := a.db.Query(ctx, `
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
		return nil, err
	}
	defer rows.Close()

	groupMap := map[string]*groupResponse{}
	order := []string{}
	for rows.Next() {
		var row standingsRow
		if err := rows.Scan(&row.GroupID, &row.GroupName, &row.PairID, &row.PairName, &row.Played, &row.Wins, &row.Losses, &row.SetsWon, &row.SetsLost, &row.GamesWon, &row.GamesLost, &row.Points); err != nil {
			return nil, err
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
		return nil, err
	}

	resp := make([]groupResponse, 0, len(order))
	for _, groupID := range order {
		resp = append(resp, *groupMap[groupID])
	}
	return resp, nil
}

func (a *app) loadDisplayMatches(ctx context.Context) ([]displayMatch, error) {
	rows, err := a.db.Query(ctx, `
		select m.id, m.round, g.name, p1.id, p1.name, p2.id, p2.name,
		       m.status, m.winner_pair_id
		from matches m
		left join groups g on g.id = m.group_id
		join pairs p1 on p1.id = m.pair1_id
		join pairs p2 on p2.id = m.pair2_id
		order by case m.round when 'group' then 1 when 'semifinal' then 2 when 'third_place' then 3 else 4 end,
		         g.name nulls last,
		         m.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := []displayMatch{}
	matchIndexByID := map[string]int{}
	for rows.Next() {
		var m displayMatch
		if err := rows.Scan(&m.ID, &m.Round, &m.GroupName, &m.Pair1.ID, &m.Pair1.Name, &m.Pair2.ID, &m.Pair2.Name, &m.Status, &m.WinnerPairID); err != nil {
			return nil, err
		}
		m.Sets = []scoreSet{}
		matches = append(matches, m)
		matchIndexByID[m.ID] = len(matches) - 1
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	setRows, err := a.db.Query(ctx, `
		select match_id, set_number, pair1_games, pair2_games
		from match_sets
		order by set_number`)
	if err != nil {
		return nil, err
	}
	defer setRows.Close()

	for setRows.Next() {
		var matchID string
		var set scoreSet
		if err := setRows.Scan(&matchID, &set.SetNumber, &set.Pair1Games, &set.Pair2Games); err != nil {
			return nil, err
		}
		if index, ok := matchIndexByID[matchID]; ok {
			matches[index].Sets = append(matches[index].Sets, set)
		}
	}
	if err := setRows.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

func groupStageMatches(matches []displayMatch) []groupMatchesResponse {
	groupMap := map[string]*groupMatchesResponse{}
	order := []string{}
	for _, m := range matches {
		if m.Round != "group" || m.GroupName == nil {
			continue
		}
		groupName := *m.GroupName
		group, ok := groupMap[groupName]
		if !ok {
			group = &groupMatchesResponse{Name: groupName}
			groupMap[groupName] = group
			order = append(order, groupName)
		}
		group.Matches = append(group.Matches, m)
	}

	resp := make([]groupMatchesResponse, 0, len(order))
	for _, groupName := range order {
		resp = append(resp, *groupMap[groupName])
	}
	return resp
}

func buildBracket(matches []displayMatch) bracketResponse {
	bracket := bracketResponse{Semifinals: []displayMatch{}}
	for _, m := range matches {
		switch m.Round {
		case "semifinal":
			bracket.Semifinals = append(bracket.Semifinals, m)
		case "third_place":
			thirdPlace := m
			bracket.ThirdPlace = &thirdPlace
		case "final":
			final := m
			bracket.Final = &final
		}
	}
	return bracket
}

func buildChampions(matches []displayMatch) championsResponse {
	var final *displayMatch
	var thirdPlaceMatch *displayMatch
	for _, m := range matches {
		switch m.Round {
		case "final":
			copyMatch := m
			final = &copyMatch
		case "third_place":
			copyMatch := m
			thirdPlaceMatch = &copyMatch
		}
	}
	if final == nil || final.Status != "completed" || final.WinnerPairID == nil {
		return championsResponse{Final: final, ThirdPlaceMatch: thirdPlaceMatch}
	}

	winnerID := *final.WinnerPairID
	champion := final.Pair1
	runnerUp := final.Pair2
	if winnerID == final.Pair2.ID {
		champion = final.Pair2
		runnerUp = final.Pair1
	}

	resp := championsResponse{
		Champion:        &champion,
		RunnerUp:        &runnerUp,
		Final:           final,
		ThirdPlaceMatch: thirdPlaceMatch,
	}
	if thirdPlaceMatch != nil && thirdPlaceMatch.Status == "completed" && thirdPlaceMatch.WinnerPairID != nil {
		thirdPlace := thirdPlaceMatch.Pair1
		if *thirdPlaceMatch.WinnerPairID == thirdPlaceMatch.Pair2.ID {
			thirdPlace = thirdPlaceMatch.Pair2
		}
		resp.ThirdPlace = &thirdPlace
	}
	return resp
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
		if err := createFinalsIfReady(ctx, tx); err != nil {
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
	group := leaders[tournamentGroupName]
	if len(group) < semifinalistsRequired {
		return fmt.Errorf("failed to determine semifinalists")
	}

	if _, err := tx.Exec(ctx, `insert into matches (pair1_id, pair2_id, round) values ($1, $2, 'semifinal')`, group[0].PairID, group[3].PairID); err != nil {
		return fmt.Errorf("failed to create semifinal 1")
	}
	if _, err := tx.Exec(ctx, `insert into matches (pair1_id, pair2_id, round) values ($1, $2, 'semifinal')`, group[1].PairID, group[2].PairID); err != nil {
		return fmt.Errorf("failed to create semifinal 2")
	}
	return nil
}

func createFinalsIfReady(ctx context.Context, tx pgx.Tx) error {
	var existingFinals int
	if err := tx.QueryRow(ctx, `select count(*) from matches where round = 'final'`).Scan(&existingFinals); err != nil {
		return fmt.Errorf("failed to check final")
	}
	var existingThirdPlace int
	if err := tx.QueryRow(ctx, `select count(*) from matches where round = 'third_place'`).Scan(&existingThirdPlace); err != nil {
		return fmt.Errorf("failed to check third-place match")
	}

	rows, err := tx.Query(ctx, `
		select pair1_id, pair2_id, winner_pair_id
		from matches
		where round = 'semifinal'
		order by created_at`)
	if err != nil {
		return fmt.Errorf("failed to read semifinals")
	}
	defer rows.Close()

	var winners []string
	var losers []string
	for rows.Next() {
		var pair1ID string
		var pair2ID string
		var winnerID *string
		if err := rows.Scan(&pair1ID, &pair2ID, &winnerID); err != nil {
			return fmt.Errorf("failed to read semifinal winner")
		}
		if winnerID != nil {
			winners = append(winners, *winnerID)
			if *winnerID == pair1ID {
				losers = append(losers, pair2ID)
			} else {
				losers = append(losers, pair1ID)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read semifinal winners")
	}
	if len(winners) < 2 {
		return nil
	}

	if existingThirdPlace == 0 {
		if _, err := tx.Exec(ctx, `insert into matches (pair1_id, pair2_id, round) values ($1, $2, 'third_place')`, losers[0], losers[1]); err != nil {
			return fmt.Errorf("failed to create third-place match")
		}
	}
	if existingFinals > 0 {
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
	for i := 0; i < len(groupPairs); i++ {
		for j := i + 1; j < len(groupPairs); j++ {
			pair1 := groupPairs[i]
			pair2 := groupPairs[j]
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
