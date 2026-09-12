package server

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationFromV1(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "roomdeck.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema + "; PRAGMA user_version=1; INSERT INTO admins VALUES('original','original',X'01');"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version, count int
	_ = s.db.QueryRow("PRAGMA user_version").Scan(&version)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM admins WHERE id='original'").Scan(&count)
	if version != schemaVersion || count != 1 {
		t.Fatal("migration lost data or version", version, count)
	}
}
func TestFileMapping(t *testing.T) {
	f := newFixture(t)
	var content Content
	data := testImage()
	req, _ := http.NewRequest("POST", f.ts.URL+"/api/v1"+f.path(fmt.Sprintf("/assets?kind=photo&name=photo.png&size=%d", len(data))), bytes.NewReader(data))
	req.Header.Set("X-RoomDeck-Request", "1")
	res, err := f.host.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatal(res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&content)
	var manifest struct {
		Files []struct {
			ID    string
			Paths map[string]string
		}
	}
	f.request(t, f.host, "GET", f.path("/storage"), nil, 200, &manifest)
	if len(manifest.Files) != 1 {
		t.Fatal("missing file")
	}
	for _, path := range manifest.Files[0].Paths {
		if _, err = os.Stat(filepath.Join(f.s.config.DataDir, filepath.FromSlash(path))); err != nil {
			t.Fatal(err)
		}
	}
	f.request(t, f.guest, "GET", f.path("/storage"), nil, 403, nil)
}
func TestDiceRoundSecrecyAndDuplicate(t *testing.T) {
	f := newFixture(t)
	var g gameState
	action := func(c *http.Client, a string, guess int) {
		f.request(t, c, "POST", f.path("/game"), gameCommand{Action: a, Version: g.Version, Kind: "dice", Language: "en", Guess: guess}, 200, &g)
	}
	action(f.host, "create", 0)
	action(f.host, "join", 0)
	action(f.guest, "join", 0)
	action(f.host, "start", 0)
	action(f.host, "guess", 3)
	raw := f.request(t, f.guest, "GET", f.path("/game"), nil, 200, nil)
	if strings.Contains(string(raw), `"guess":3`) {
		t.Fatal("other guess leaked")
	}
	old := g.Version
	action(f.guest, "guess", 4)
	if g.Phase != "result" || g.Die < 1 || g.Die > 6 {
		t.Fatal("invalid roll", g)
	}
	f.request(t, f.guest, "POST", f.path("/game"), gameCommand{Action: "guess", Version: old, Guess: 4}, 409, nil)
	before := g.Die
	f.request(t, f.host, "GET", f.path("/game"), nil, 200, &g)
	if g.Die != before {
		t.Fatal("duplicate rolled again")
	}
	action(f.host, "round", 0)
	if g.Die != 0 || g.Round != 2 {
		t.Fatal("new round not reset")
	}
	f.request(t, f.host, "POST", f.path("/close"), nil, 200, nil)
	f.request(t, f.host, "POST", f.path("/game"), gameCommand{Action: "guess", Version: g.Version, Guess: 1}, 409, nil)
}
func TestUndercoverPrivateViewsAndWin(t *testing.T) {
	f := newFixture(t)
	clients := []*http.Client{f.host, f.guest, client(), client()}
	for i := 2; i < 4; i++ {
		f.request(t, clients[i], "POST", "/join", joinInput{Code: f.room.Code, Name: fmt.Sprintf("Player %d", i)}, 201, nil)
	}
	var g gameState
	action := func(c *http.Client, a, target string) {
		f.request(t, c, "POST", f.path("/game"), gameCommand{Action: a, Target: target, Version: g.Version, Kind: "undercover", Language: "zh"}, 200, &g)
	}
	action(f.host, "create", "")
	for _, c := range clients {
		action(c, "join", "")
	}
	action(f.host, "start", "")
	stored, err := f.s.loadGame(f.room.ID)
	if err != nil {
		t.Fatal(err)
	}
	spy := -1
	for i, p := range stored.Players {
		if p.Spy {
			spy = i
		}
	}
	if spy < 0 {
		t.Fatal("no spy")
	}
	for _, c := range clients {
		raw := f.request(t, c, "GET", f.path("/game"), nil, 200, nil)
		if strings.Contains(string(raw), `"spy"`) {
			t.Fatal("role leaked")
		}
		var v struct{ Players []map[string]interface{} }
		_ = json.Unmarshal(raw, &v)
		for _, p := range v.Players {
			if p["word"] != nil {
				t.Fatal("word in public list")
			}
		}
	}
	for range clients {
		action(f.host, "next", "")
	}
	for i, c := range clients {
		target := stored.Players[spy].ID
		if i == spy {
			target = stored.Players[(spy+1)%4].ID
		}
		action(c, "vote", target)
	}
	if g.Phase != "finished" || g.Result != "civilians" {
		t.Fatal("unexpected outcome", g)
	}
}
func TestUndercoverTieAndSecondTie(t *testing.T) {
	g := gameState{Kind: "undercover", Phase: "vote", Round: 1, Players: []gamePlayer{{ID: "a", Alive: true, Spy: true, Vote: "b"}, {ID: "b", Alive: true, Vote: "a"}, {ID: "c", Alive: true, Vote: "a"}, {ID: "d", Alive: true, Vote: "b"}}}
	g.resolveVote()
	if !g.Tie || g.Phase != "vote" || len(g.Candidates) != 2 {
		t.Fatal(g)
	}
	for i, target := range []string{"b", "a", "a", "b"} {
		g.Players[i].Vote = target
	}
	g.resolveVote()
	if g.Tie || g.Round != 2 || g.Phase != "describe" {
		t.Fatal(g)
	}
	for _, p := range g.Players {
		if !p.Alive {
			t.Fatal("tie eliminated player")
		}
	}
}

func TestConcurrentGuessesAndRemovedSeat(t *testing.T) {
	f := newFixture(t)
	var g gameState
	for _, a := range []struct {
		c      *http.Client
		action string
	}{{f.host, "create"}, {f.host, "join"}, {f.guest, "join"}, {f.host, "start"}} {
		f.request(t, a.c, "POST", f.path("/game"), gameCommand{Action: a.action, Kind: "dice", Language: "en", Version: g.Version}, 200, &g)
	}
	command := gameCommand{Action: "guess", GameID: g.ID, Round: g.Round, Phase: g.Phase, Version: g.Version, Guess: 2}
	f.request(t, f.host, "POST", f.path("/game"), command, 200, nil)
	f.request(t, f.guest, "POST", f.path("/game"), command, 200, &g)
	if g.Phase != "result" {
		t.Fatal("concurrent guesses rejected")
	}
	f.request(t, f.host, "POST", f.path("/game"), gameCommand{Action: "round", Version: g.Version}, 200, &g)
	f.request(t, f.host, "POST", f.path("/game"), command, 409, nil)
	_, _ = f.s.db.Exec("UPDATE participants SET revoked=1 WHERE room_id=?", f.room.ID)
	f.request(t, f.host, "GET", f.path("/game"), nil, 200, &g)
	if g.Phase != "finished" || g.Result != "cancelled" {
		t.Fatal("removed player blocked the game")
	}
}

func TestGameSurvivesRestartAndDisplayHasNoSecrets(t *testing.T) {
	f := newFixture(t)
	var g gameState
	for _, a := range []struct {
		c      *http.Client
		action string
	}{{f.host, "create"}, {f.host, "join"}, {f.guest, "join"}, {f.host, "start"}, {f.host, "guess"}} {
		f.request(t, a.c, "POST", f.path("/game"), gameCommand{Action: a.action, Kind: "dice", Language: "en", Guess: 6, Version: g.Version}, 200, &g)
	}
	stored, err := f.s.loadGame(f.room.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(gameView(stored, &principal{Role: "display", ID: stored.Players[0].ID}))
	if strings.Contains(string(b), `"guess":6`) {
		t.Fatal("display received private guess")
	}
	// Close/reopen the application while keeping its HTTP wrapper for fixture cleanup.
	f.s.Close()
	next, err := New(f.s.config)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	reopened, err := next.loadGame(f.room.ID)
	if err != nil || reopened.ID != stored.ID || reopened.Players[0].Guess != 6 || reopened.Phase != "guess" {
		t.Fatal("game did not survive restart", err)
	}
}
func TestMediaPermissionsAndRevocation(t *testing.T) {
	f := newFixture(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer fake.Close()
	f.s.config.MediaURL = fake.URL
	f.s.config.MediaKey = "test"
	f.s.config.MediaSecret = strings.Repeat("s", 32)
	var grant struct {
		Client, Token string
		Screen        screenState
	}
	f.request(t, f.guest, "POST", f.path("/screen"), map[string]string{"action": "start"}, 200, &grant)
	f.request(t, f.host, "POST", f.path("/screen"), map[string]string{"action": "start"}, 409, nil)
	if !f.s.validMediaClient(grant.Client) {
		t.Fatal("publisher invalid")
	}
	var view struct{ Client, Token string }
	f.request(t, f.host, "POST", f.path("/screen"), map[string]string{"action": "watch"}, 200, &view)
	claims := decodeClaims(t, view.Token)
	if claims["video"].(map[string]interface{})["canPublish"] != false {
		t.Fatal("viewer may publish")
	}
	proxy, err := http.Get(f.ts.URL + "/media/rtc/v1/validate?access_token=" + view.Token)
	if err != nil {
		t.Fatal(err)
	}
	proxy.Body.Close()
	if proxy.StatusCode != 200 {
		t.Fatal("valid token not proxied", proxy.StatusCode)
	}
	f.request(t, f.host, "POST", f.path("/screen"), map[string]string{"action": "heartbeat", "client": grant.Client}, 403, nil)
	f.request(t, f.host, "POST", f.path("/screen"), map[string]string{"action": "stop", "screen_id": "previous-screen"}, 409, nil)
	f.request(t, f.host, "POST", f.path("/screen"), map[string]string{"action": "stop", "screen_id": grant.Screen.ID}, 200, nil)
	proxy, err = http.Get(f.ts.URL + "/media/rtc/v1/validate?access_token=" + view.Token)
	if err != nil {
		t.Fatal(err)
	}
	proxy.Body.Close()
	if proxy.StatusCode != 403 {
		t.Fatal("old token accepted")
	}
	f.request(t, f.guest, "POST", f.path("/screen"), map[string]string{"action": "start"}, 200, &grant)
	_, _ = f.s.db.Exec("UPDATE participants SET revoked=1 WHERE room_id=?", f.room.ID)
	f.s.reconcileMedia()
	if f.s.getScreen(f.room.ID) != nil {
		t.Fatal("revoked publisher retained screen")
	}
}
func decodeClaims(t *testing.T, token string) map[string]interface{} {
	t.Helper()
	parts := strings.Split(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]interface{}
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
