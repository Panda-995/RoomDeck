package server

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type boardPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type boardStroke struct {
	ID     string       `json:"id"`
	Owner  string       `json:"owner"`
	Color  string       `json:"color"`
	Width  int          `json:"width"`
	Points []boardPoint `json:"points"`
}
type boardPlayer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Score   int    `json:"score"`
	Guessed bool   `json:"guessed"`
}
type boardGuess struct {
	Name    string `json:"name"`
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
}
type boardState struct {
	Version   int           `json:"version"`
	Epoch     int           `json:"epoch"`
	Phase     string        `json:"phase"`
	Language  string        `json:"language"`
	Strokes   []boardStroke `json:"strokes"`
	Players   []boardPlayer `json:"players"`
	Guesses   []boardGuess  `json:"guesses"`
	Turn      int           `json:"turn"`
	Deadline  int64         `json:"deadline"`
	Word      string        `json:"word"`
	Cancelled bool          `json:"cancelled"`
}
type boardCommand struct {
	Action   string      `json:"action"`
	Version  int         `json:"version"`
	Epoch    int         `json:"epoch"`
	Language string      `json:"language"`
	Text     string      `json:"text"`
	Stroke   boardStroke `json:"stroke"`
	ID       string      `json:"id"`
}

func newBoard() *boardState {
	return &boardState{Phase: "free", Language: "zh", Epoch: 1, Strokes: []boardStroke{}, Players: []boardPlayer{}, Guesses: []boardGuess{}}
}
func (s *Server) loadBoard(room string) (*boardState, error) {
	var raw string
	err := s.db.QueryRow("SELECT state FROM boards WHERE room_id=?", room).Scan(&raw)
	if err == sql.ErrNoRows {
		return newBoard(), nil
	}
	if err != nil {
		return nil, err
	}
	var b boardState
	if err = json.Unmarshal([]byte(raw), &b); err != nil {
		return nil, err
	}
	changed := false
	if b.Phase == "drawing" && time.Now().Unix() >= b.Deadline {
		b.Phase = "result"
		changed = true
	}
	if b.Phase == "drawing" || b.Phase == "lobby" || b.Phase == "result" {
		for _, player := range b.Players {
			var revoked bool
			if s.db.QueryRow("SELECT revoked FROM participants WHERE id=?", player.ID).Scan(&revoked) == nil && revoked {
				b.Phase = "finished"
				b.Cancelled = true
				b.Word = ""
				changed = true
				break
			}
		}
	}
	if changed {
		if err = s.saveBoard(room, &b); err != nil {
			return nil, err
		}
	}
	return &b, nil
}
func (s *Server) saveBoard(room string, b *boardState) error {
	b.Version++
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO boards(room_id,state) VALUES(?,?) ON CONFLICT(room_id) DO UPDATE SET state=excluded.state", room, string(raw))
	return err
}
func boardView(b *boardState, p *principal) interface{} {
	// Secret words never leave the server except for the current artist or reveal phase.
	word := ""
	artist := ""
	if b.Turn < len(b.Players) {
		artist = b.Players[b.Turn].ID
	}
	if (b.Phase == "drawing" && p.Role != "display" && p.ID == artist) || b.Phase == "result" {
		word = b.Word
	}
	return map[string]interface{}{"version": b.Version, "epoch": b.Epoch, "phase": b.Phase, "language": b.Language, "strokes": b.Strokes, "players": b.Players, "guesses": b.Guesses, "turn": b.Turn, "deadline": b.Deadline, "word": word, "artist": artist, "cancelled": b.Cancelled, "server_time": time.Now().Unix()}
}
func (s *Server) boardSnapshot(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role == "display" && (!room.Active() || !room.Enabled("display") || room.Settings.DisplayMode != "board") {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	b, err := s.loadBoard(room.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if r.URL.Query().Get("since") == strconv.Itoa(b.Version) {
		writeJSON(w, 200, map[string]interface{}{"unchanged": true, "server_time": time.Now().Unix()})
		return
	}
	writeJSON(w, 200, boardView(b, p))
}
func normalizeGuess(s string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s))
}
func (b *boardState) nextDrawing() {
	zh := []string{"雨伞", "自行车", "熊猫", "生日蛋糕", "雪人", "吉他", "飞机", "西瓜", "长颈鹿", "机器人", "火山", "眼镜", "章鱼", "冰淇淋", "足球", "向日葵", "帆船", "城堡", "企鹅", "闹钟"}
	en := []string{"Umbrella", "Bicycle", "Panda", "Birthday cake", "Snowman", "Guitar", "Airplane", "Watermelon", "Giraffe", "Robot", "Volcano", "Glasses", "Octopus", "Ice cream", "Football", "Sunflower", "Sailboat", "Castle", "Penguin", "Alarm clock"}
	words := zh
	if b.Language == "en" {
		words = en
	}
	previous := b.Word
	for {
		b.Word = words[randomNumber(len(words))]
		if b.Word != previous {
			break
		}
	}
	b.Phase = "drawing"
	b.Deadline = time.Now().Unix() + 90
	b.Epoch++
	b.Strokes = []boardStroke{}
	b.Guesses = []boardGuess{}
	for i := range b.Players {
		b.Players[i].Guessed = false
	}
}
func (s *Server) boardAction(w http.ResponseWriter, r *http.Request) {
	var in boardCommand
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil || !s.canInteract(w, p, room) {
		return
	}
	b, err := s.loadBoard(room.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	manager := s.permitted(p, room.ID, "games")
	if in.Epoch != b.Epoch {
		apiError(w, 409, "BOARD_CHANGED")
		return
	}
	control := in.Action == "free" || in.Action == "create" || in.Action == "start" || in.Action == "next" || in.Action == "finish" || in.Action == "clear"
	if control && !manager {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	if control && in.Version != b.Version {
		apiError(w, 409, "VERSION_CONFLICT")
		return
	}
	index := -1
	for i, v := range b.Players {
		if v.ID == p.ID {
			index = i
		}
	}
	switch in.Action {
	case "stroke":
		if b.Phase != "free" && (b.Phase != "drawing" || index != b.Turn || index < 0) {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if !validText(in.Stroke.ID, 1, 80) || len(in.Stroke.Points) < 1 || len(in.Stroke.Points) > 128 || (in.Stroke.Width != 3 && in.Stroke.Width != 7 && in.Stroke.Width != 14) {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
		colors := map[string]bool{"#263b33": true, "#30664d": true, "#cc554d": true, "#d09a35": true, "#4c7db5": true, "#9369aa": true, "#ffffff": true}
		if !colors[in.Stroke.Color] {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
		for _, point := range in.Stroke.Points {
			if math.IsNaN(point.X) || math.IsNaN(point.Y) || point.X < 0 || point.X > 1000 || point.Y < 0 || point.Y > 625 {
				apiError(w, 400, "INVALID_INPUT")
				return
			}
		}
		for _, stroke := range b.Strokes {
			if stroke.ID == in.Stroke.ID {
				if stroke.Owner != p.ID {
					apiError(w, 409, "VERSION_CONFLICT")
					return
				}
				writeJSON(w, 200, map[string]bool{"ok": true})
				return
			}
		}
		if len(b.Strokes) >= 600 {
			apiError(w, 409, "BOARD_FULL")
			return
		}
		if s.limited(r, "board-stroke:"+room.ID+":"+p.ID, 600) {
			apiError(w, 429, "RATE_LIMITED")
			return
		}
		in.Stroke.Owner = p.ID
		b.Strokes = append(b.Strokes, in.Stroke)
	case "undo":
		if b.Phase != "free" && (b.Phase != "drawing" || index != b.Turn || index < 0) {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		for i := len(b.Strokes) - 1; i >= 0; i-- {
			if b.Strokes[i].Owner == p.ID && b.Strokes[i].ID == in.ID {
				b.Strokes = append(b.Strokes[:i], b.Strokes[i+1:]...)
				break
			}
		}
	case "clear":
		b.Strokes = []boardStroke{}
		b.Epoch++
	case "free", "create":
		if b.Phase == "drawing" || b.Phase == "lobby" || b.Phase == "result" {
			apiError(w, 409, "GAME_ACTIVE")
			return
		}
		if in.Language != "zh" && in.Language != "en" {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
		epoch, version := b.Epoch, b.Version
		b = newBoard()
		b.Epoch = epoch + 1
		b.Version = version
		b.Language = in.Language
		if in.Action == "create" {
			b.Phase = "lobby"
		}
	case "join":
		if b.Phase != "lobby" || len(b.Players) >= 12 {
			apiError(w, 409, "GAME_PHASE")
			return
		}
		if index < 0 {
			b.Players = append(b.Players, boardPlayer{ID: p.ID, Name: p.Name})
		}
	case "leave":
		if b.Phase != "lobby" {
			apiError(w, 409, "GAME_PHASE")
			return
		}
		if index >= 0 {
			b.Players = append(b.Players[:index], b.Players[index+1:]...)
		}
	case "start":
		if b.Phase != "lobby" || len(b.Players) < 2 {
			apiError(w, 409, "GAME_PLAYERS")
			return
		}
		b.nextDrawing()
	case "guess":
		if b.Phase != "drawing" || index < 0 || index == b.Turn || b.Players[index].Guessed {
			apiError(w, 409, "GAME_PHASE")
			return
		}
		if !validText(in.Text, 1, 60) {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
		if s.limited(r, "board-guess:"+room.ID+":"+p.ID, 60) {
			apiError(w, 429, "RATE_LIMITED")
			return
		}
		correct := normalizeGuess(in.Text) == normalizeGuess(b.Word)
		text := strings.TrimSpace(in.Text)
		if correct {
			b.Players[index].Guessed = true
			b.Players[index].Score += 100
			b.Players[b.Turn].Score += 50
			text = ""
		}
		b.Guesses = append(b.Guesses, boardGuess{Name: p.Name, Text: text, Correct: correct})
		if len(b.Guesses) > 20 {
			b.Guesses = b.Guesses[len(b.Guesses)-20:]
		}
		complete := true
		for i, v := range b.Players {
			if i != b.Turn && !v.Guessed {
				complete = false
			}
		}
		if complete {
			b.Phase = "result"
		}
	case "next":
		if b.Phase != "result" {
			apiError(w, 409, "GAME_PHASE")
			return
		}
		b.Turn++
		if b.Turn >= len(b.Players) {
			b.Phase = "finished"
			b.Word = ""
		} else {
			b.nextDrawing()
		}
	case "finish":
		b.Phase = "finished"
		b.Word = ""
		b.Cancelled = true
		b.Epoch++
	default:
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	if err = s.saveBoard(room.ID, b); err != nil {
		s.internal(w, err)
		return
	}
	if control {
		s.audit(room.ID, "board."+in.Action, p.ID)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
