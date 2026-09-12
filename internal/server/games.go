package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"math/big"
	"net/http"
)

type gamePlayer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Alive bool   `json:"alive"`
	Score int    `json:"score"`
	Word  string `json:"word,omitempty"`
	Spy   bool   `json:"spy,omitempty"`
	Vote  string `json:"vote,omitempty"`
	Guess int    `json:"guess,omitempty"`
}
type gameState struct {
	ID         string       `json:"id"`
	Kind       string       `json:"kind"`
	Phase      string       `json:"phase"`
	Language   string       `json:"language"`
	Version    int          `json:"version"`
	Round      int          `json:"round"`
	Turn       int          `json:"turn"`
	Players    []gamePlayer `json:"players"`
	Candidates []string     `json:"candidates"`
	Tie        bool         `json:"tie"`
	Result     string       `json:"result"`
	Die        int          `json:"die"`
}
type gameCommand struct {
	GameID   string `json:"game_id"`
	Round    int    `json:"round"`
	Phase    string `json:"phase"`
	Tie      bool   `json:"tie"`
	Action   string `json:"action"`
	Version  int    `json:"version"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Target   string `json:"target"`
	Guess    int    `json:"guess"`
}

func randomNumber(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}
func (s *Server) loadGame(room string) (*gameState, error) {
	var raw string
	err := s.db.QueryRow("SELECT state FROM games WHERE room_id=?", room).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var g gameState
	err = json.Unmarshal([]byte(raw), &g)
	if err == nil && g.Phase != "finished" {
		changed := false
		kept := []gamePlayer{}
		for _, player := range g.Players {
			var revoked int
			if e := s.db.QueryRow("SELECT revoked FROM participants WHERE id=?", player.ID).Scan(&revoked); e == nil && revoked != 0 {
				changed = true
				continue
			}
			kept = append(kept, player)
		}
		if changed {
			if g.Phase == "lobby" {
				g.Players = kept
			} else {
				g.Phase = "finished"
				g.Result = "cancelled"
				for i := range g.Players {
					g.Players[i].Word = ""
					g.Players[i].Spy = false
				}
			}
			g.Version++
			encoded, e := json.Marshal(g)
			if e != nil {
				return nil, e
			}
			if _, e = s.db.Exec("UPDATE games SET state=? WHERE room_id=?", string(encoded), room); e != nil {
				return nil, e
			}
			s.hub.notify(room)
		}
	}
	return &g, err
}
func gameView(g *gameState, p *principal) interface{} {
	if g == nil {
		return nil
	}
	// Construct a public DTO explicitly: do not marshal private state then hide it in the UI.
	players := []map[string]interface{}{}
	var mine interface{}
	for _, v := range g.Players {
		row := map[string]interface{}{"id": v.ID, "name": v.Name, "alive": v.Alive, "score": v.Score, "submitted": v.Vote != "" || v.Guess > 0}
		if g.Phase == "finished" && g.Kind == "undercover" {
			row["word"] = v.Word
			row["spy"] = v.Spy
		}
		if g.Phase == "result" && g.Kind == "dice" {
			row["guess"] = v.Guess
		}
		if v.ID == p.ID && p.Role != "display" {
			mine = map[string]interface{}{"word": v.Word, "guess": v.Guess, "vote": v.Vote}
		}
		players = append(players, row)
	}
	return map[string]interface{}{"id": g.ID, "kind": g.Kind, "phase": g.Phase, "language": g.Language, "version": g.Version, "round": g.Round, "turn": g.Turn, "players": players, "candidates": g.Candidates, "tie": g.Tie, "result": g.Result, "die": g.Die, "mine": mine}
}
func (s *Server) gameSnapshot(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.loadGame(room.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, gameView(g, p))
}
func (s *Server) gameAction(w http.ResponseWriter, r *http.Request) {
	p, _ := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role == "display" {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	var in gameCommand
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.getRoom(r.PathValue("room"))
	if err != nil {
		s.internal(w, err)
		return
	}
	// Revalidate membership after acquiring the mutation lock.
	p = s.identity(r, p.Role)
	if p == nil {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	if !room.Active() {
		apiError(w, 409, "ROOM_CLOSED")
		return
	}
	if p.Role == "guest" {
		var muted bool
		if s.db.QueryRow("SELECT muted FROM participants WHERE id=?", p.ID).Scan(&muted) != nil || muted {
			apiError(w, 403, "FORBIDDEN")
			return
		}
	}
	g, err := s.loadGame(room.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	version := 0
	if g != nil {
		version = g.Version
	}
	concurrent := g != nil && in.GameID == g.ID && in.Phase == g.Phase && in.Round == g.Round && in.Tie == g.Tie && (in.Action == "join" || in.Action == "leave" || in.Action == "guess" || in.Action == "vote")
	if in.Version != version && !concurrent {
		apiError(w, 409, "VERSION_CONFLICT")
		return
	}
	if in.Action == "create" {
		if !s.permitted(p, room.ID, "games") {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if g != nil && g.Phase != "finished" {
			apiError(w, 409, "GAME_ACTIVE")
			return
		}
		if (in.Kind != "undercover" && in.Kind != "dice") || (in.Language != "zh" && in.Language != "en") {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
		g = &gameState{ID: randomID(16), Kind: in.Kind, Language: in.Language, Phase: "lobby", Players: []gamePlayer{}, Candidates: []string{}}
	} else {
		if g == nil {
			apiError(w, 409, "GAME_MISSING")
			return
		}
		actor := *p
		if s.permitted(p, room.ID, "games") {
			actor.Role = "host"
		}
		if code := g.apply(in, &actor); code != "" {
			apiError(w, 409, code)
			return
		}
	}
	g.Version = version + 1
	raw, err := json.Marshal(g)
	if err != nil {
		s.internal(w, err)
		return
	}
	_, err = s.db.Exec("INSERT INTO games(room_id,state) VALUES(?,?) ON CONFLICT(room_id) DO UPDATE SET state=excluded.state", room.ID, string(raw))
	if err != nil {
		s.internal(w, err)
		return
	}
	s.hub.notify(room.ID)
	writeJSON(w, 200, gameView(g, p))
}
func (g *gameState) apply(in gameCommand, p *principal) string {
	index := -1
	for i, v := range g.Players {
		if v.ID == p.ID {
			index = i
		}
	}
	switch in.Action {
	case "join":
		if g.Phase != "lobby" {
			return "GAME_PHASE"
		}
		if index >= 0 {
			return "GAME_PHASE"
		}
		if len(g.Players) >= 12 {
			return "GAME_FULL"
		}
		g.Players = append(g.Players, gamePlayer{ID: p.ID, Name: p.Name, Alive: true})
	case "leave":
		if g.Phase != "lobby" || index < 0 {
			return "GAME_PHASE"
		}
		g.Players = append(g.Players[:index], g.Players[index+1:]...)
	case "cancel":
		if p.Role != "host" {
			return "FORBIDDEN"
		}
		g.Phase = "finished"
		g.Result = "cancelled"
		for i := range g.Players {
			g.Players[i].Word = ""
			g.Players[i].Spy = false
		}
	case "start":
		if p.Role != "host" {
			return "FORBIDDEN"
		}
		if g.Phase != "lobby" {
			return "GAME_PHASE"
		}
		minimum := 2
		if g.Kind == "undercover" {
			minimum = 4
		}
		if len(g.Players) < minimum {
			return "GAME_PLAYERS"
		}
		g.Round = 1
		g.Turn = 0
		if g.Kind == "dice" {
			g.Phase = "guess"
		} else {
			zh := [][2]string{{"咖啡", "奶茶"}, {"地铁", "高铁"}, {"月亮", "太阳"}, {"饺子", "馄饨"}, {"蛋糕", "面包"}, {"海豚", "鲸鱼"}, {"钢琴", "吉他"}, {"雨衣", "雨伞"}, {"西瓜", "哈密瓜"}, {"电影", "电视剧"}, {"冰箱", "空调"}, {"滑雪", "滑冰"}}
			en := [][2]string{{"Coffee", "Tea"}, {"Train", "Subway"}, {"Moon", "Sun"}, {"Cake", "Bread"}, {"Dolphin", "Whale"}, {"Piano", "Guitar"}, {"Raincoat", "Umbrella"}, {"Movie", "TV series"}, {"Skiing", "Ice skating"}, {"Beach", "Desert"}, {"Library", "Bookshop"}, {"Butterfly", "Dragonfly"}}
			pairs := zh
			if g.Language == "en" {
				pairs = en
			}
			pair := pairs[randomNumber(len(pairs))]
			spy := randomNumber(len(g.Players))
			for i := range g.Players {
				g.Players[i].Word = pair[0]
				g.Players[i].Spy = i == spy
				if i == spy {
					g.Players[i].Word = pair[1]
				}
			}
			g.Phase = "describe"
		}
	case "next":
		if g.Phase != "describe" || (p.Role != "host" && (index != g.Turn || index < 0)) {
			return "GAME_PHASE"
		}
		next := g.Turn + 1
		for next < len(g.Players) && !g.Players[next].Alive {
			next++
		}
		if next == len(g.Players) {
			g.Phase = "vote"
		} else {
			g.Turn = next
		}
	case "vote":
		if g.Phase != "vote" || index < 0 || !g.Players[index].Alive || g.Players[index].Vote != "" {
			return "GAME_PHASE"
		}
		valid := false
		for _, v := range g.Players {
			if v.ID == in.Target && v.Alive && v.ID != p.ID {
				valid = true
			}
		}
		if len(g.Candidates) > 0 {
			eligible := false
			for _, id := range g.Candidates {
				if id == in.Target {
					eligible = true
				}
			}
			valid = valid && eligible
		}
		if !valid {
			return "INVALID_INPUT"
		}
		g.Players[index].Vote = in.Target
		complete := true
		for _, v := range g.Players {
			if v.Alive && v.Vote == "" {
				complete = false
			}
		}
		if complete {
			g.resolveVote()
		}
	case "guess":
		if g.Kind != "dice" || g.Phase != "guess" || index < 0 || g.Players[index].Guess != 0 {
			return "GAME_PHASE"
		}
		if in.Guess < 1 || in.Guess > 6 {
			return "INVALID_INPUT"
		}
		g.Players[index].Guess = in.Guess
		complete := true
		for _, v := range g.Players {
			if v.Guess == 0 {
				complete = false
			}
		}
		if complete {
			g.Die = randomNumber(6) + 1
			g.Phase = "result"
			for i := range g.Players {
				if g.Players[i].Guess == g.Die {
					g.Players[i].Score++
				}
			}
		}
	case "round":
		if p.Role != "host" {
			return "FORBIDDEN"
		}
		if g.Kind != "dice" || g.Phase != "result" {
			return "GAME_PHASE"
		}
		g.Round++
		g.Die = 0
		g.Phase = "guess"
		for i := range g.Players {
			g.Players[i].Guess = 0
		}
	default:
		return "INVALID_INPUT"
	}
	return ""
}
func (g *gameState) resolveVote() {
	counts := map[string]int{}
	for _, v := range g.Players {
		if v.Alive {
			counts[v.Vote]++
		}
	}
	maximum := 0
	ties := []string{}
	for _, v := range g.Players {
		n := counts[v.ID]
		if n > maximum {
			maximum = n
			ties = []string{v.ID}
		} else if n == maximum && n > 0 {
			ties = append(ties, v.ID)
		}
	}
	for i := range g.Players {
		g.Players[i].Vote = ""
	}
	if len(ties) > 1 && !g.Tie {
		g.Tie = true
		g.Candidates = ties
		return
	}
	g.Candidates = []string{}
	g.Tie = false
	if len(ties) == 1 {
		for i := range g.Players {
			if g.Players[i].ID == ties[0] {
				g.Players[i].Alive = false
			}
		}
	}
	spies, civilians := 0, 0
	for _, v := range g.Players {
		if v.Alive {
			if v.Spy {
				spies++
			} else {
				civilians++
			}
		}
	}
	if spies == 0 {
		g.Phase = "finished"
		g.Result = "civilians"
	} else if spies >= civilians {
		g.Phase = "finished"
		g.Result = "spy"
	} else {
		g.Round++
		g.Phase = "describe"
		g.Turn = 0
		for !g.Players[g.Turn].Alive {
			g.Turn++
		}
	}
}
