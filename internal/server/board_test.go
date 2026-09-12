package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func guestID(t *testing.T, f *fixture, c *http.Client) string {
	var snap struct {
		ID string `json:"participant_id"`
	}
	f.request(t, c, "GET", f.path("/snapshot"), nil, 200, &snap)
	return snap.ID
}
func TestCohostScopesAndRevocation(t *testing.T) {
	f := newFixture(t)
	id := guestID(t, f, f.guest)
	permissions := f.path("/participants/" + id + "/permissions")
	input := map[string]interface{}{"permissions": []string{"content", "games"}}
	f.request(t, f.guest, "PUT", permissions, input, 403, nil)
	f.request(t, f.host, "PUT", permissions, map[string]interface{}{"permissions": []string{"owner"}}, 400, nil)
	f.request(t, f.host, "PUT", permissions, input, 200, nil)
	f.request(t, f.host, "PUT", permissions, map[string]interface{}{"scope": "display", "enabled": true}, 200, nil)
	f.request(t, f.host, "PUT", permissions, map[string]interface{}{"scope": "screen", "enabled": true}, 200, nil)
	f.request(t, f.host, "PUT", permissions, map[string]interface{}{"scope": "screen", "enabled": false}, 200, nil)
	var scopes struct {
		Permissions []string `json:"permissions"`
	}
	f.request(t, f.guest, "GET", f.path("/snapshot"), nil, 200, &scopes)
	if len(scopes.Permissions) != 3 {
		t.Fatal("individual updates overwrote other permissions")
	}
	var c Content
	f.request(t, f.host, "POST", f.path("/contents"), map[string]string{"kind": "note", "body": "moderate"}, 201, &c)
	f.request(t, f.guest, "PATCH", f.path("/contents/"+c.ID), map[string]bool{"visible": false}, 200, nil)
	body := f.request(t, f.guest, "GET", f.path("/snapshot"), nil, 200, nil)
	if !strings.Contains(string(body), "moderate") || strings.Contains(string(body), `"invite":`) {
		t.Fatal("moderator scope leaks or lacks data")
	}
	f.request(t, f.guest, "POST", f.path("/screen/queue"), map[string]string{"action": "mode", "mode": "approval"}, 403, nil)
	f.request(t, f.guest, "DELETE", f.path(""), nil, 401, nil)
	f.request(t, f.guest, "POST", f.path("/exports"), nil, 401, nil)
	settings := f.room.Settings
	settings.DanmakuMode = "approval"
	update := map[string]interface{}{"name": f.room.Name, "settings": settings, "retention": f.room.Retention, "version": f.room.Version}
	f.request(t, f.guest, "PATCH", f.path(""), update, 200, nil)
	room, _ := f.s.getRoom(f.room.ID)
	settings.Quota++
	update["settings"] = settings
	update["version"] = room.Version
	f.request(t, f.guest, "PATCH", f.path(""), update, 403, nil)
	f.request(t, f.guest, "POST", f.path("/game"), gameCommand{Action: "create", Kind: "dice", Language: "en"}, 200, nil)
	f.request(t, f.host, "PUT", permissions, map[string]interface{}{"permissions": []string{}}, 200, nil)
	f.request(t, f.guest, "PATCH", f.path("/contents/"+c.ID), map[string]bool{"visible": true}, 403, nil)
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "create", Epoch: 1, Language: "zh"}, 403, nil)
	// A valid grant for one room never authorizes another room.
	var second Room
	f.request(t, f.host, "POST", "/rooms", map[string]interface{}{"name": "Other", "duration": 7200, "retention": 86400}, 201, &second)
	f.request(t, f.host, "PUT", "/rooms/"+second.ID+"/participants/"+id+"/permissions", input, 404, nil)
	f.request(t, f.host, "PUT", permissions, input, 200, nil)
	f.request(t, f.host, "PATCH", f.path("/participants/"+id), map[string]bool{"muted": true}, 200, nil)
	f.request(t, f.guest, "PATCH", f.path("/contents/"+c.ID), map[string]bool{"visible": true}, 403, nil)
}

func TestBoardAndCohostSurviveRestart(t *testing.T) {
	f := newFixture(t)
	id := guestID(t, f, f.guest)
	f.request(t, f.host, "PUT", f.path("/participants/"+id+"/permissions"), map[string]interface{}{"scope": "games", "enabled": true}, 200, nil)
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "stroke", Epoch: 1, Stroke: boardStroke{ID: "persist", Color: "#30664d", Width: 3, Points: []boardPoint{{10, 20}, {40, 50}}}}, 200, nil)
	config := f.s.config
	f.ts.Close()
	f.s.Close()
	restored, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	ts := httptest.NewServer(restored.Handler())
	defer ts.Close()
	f.ts = ts
	f.s = restored
	var board struct {
		Strokes []boardStroke `json:"strokes"`
	}
	f.request(t, f.guest, "GET", f.path("/board"), nil, 200, &board)
	if len(board.Strokes) != 1 || board.Strokes[0].ID != "persist" {
		t.Fatal("drawing lost on restart")
	}
	var snap struct {
		Permissions []string `json:"permissions"`
	}
	f.request(t, f.guest, "GET", f.path("/snapshot"), nil, 200, &snap)
	if len(snap.Permissions) != 1 || snap.Permissions[0] != "games" {
		t.Fatal("grant lost on restart")
	}
}

func TestBoardSharedStrokesGamePrivacyAndTimeout(t *testing.T) {
	f := newFixture(t)
	f.request(t, f.other, "POST", "/join", joinInput{Code: f.room.Code, Name: "Third"}, 201, nil)
	hostID := guestID(t, f, f.host)
	guest := guestID(t, f, f.guest)
	get := func() *boardState {
		t.Helper()
		b, e := f.s.loadBoard(f.room.ID)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	command := func(c *http.Client, action string, want int) {
		t.Helper()
		b := get()
		f.request(t, c, "POST", f.path("/board"), boardCommand{Action: action, Epoch: b.Epoch, Version: b.Version, Language: "en"}, want, nil)
	}
	stroke := boardStroke{ID: "stroke-1", Owner: "forged", Color: "#263b33", Width: 7, Points: []boardPoint{{20, 30}, {90, 80}}}
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "stroke", Epoch: 1, Stroke: stroke}, 200, nil)
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "stroke", Epoch: 1, Stroke: stroke}, 200, nil)
	if b := get(); len(b.Strokes) != 1 || b.Strokes[0].Owner != guest {
		t.Fatal("stroke idempotency or identity failed")
	}
	stroke.ID = "invalid"
	stroke.Points[0].X = 1001
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "stroke", Epoch: 1, Stroke: stroke}, 400, nil)
	command(f.guest, "clear", 403)
	command(f.host, "clear", 200)
	stroke.Points[0].X = 1
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "stroke", Epoch: 1, Stroke: stroke}, 409, nil)
	command(f.host, "create", 200)
	command(f.host, "join", 200)
	command(f.guest, "join", 200)
	command(f.other, "join", 200)
	command(f.host, "start", 200)
	b := get()
	if b.Players[b.Turn].ID != hostID || b.Word == "" {
		t.Fatal("missing artist/word")
	}
	var hidden struct {
		Word string `json:"word"`
	}
	f.request(t, f.guest, "GET", f.path("/board"), nil, 200, &hidden)
	if hidden.Word != "" {
		t.Fatal("secret leaked to guesser")
	}
	settings := f.room.Settings
	settings.DisplayMode = "board"
	f.request(t, f.host, "PATCH", f.path(""), map[string]interface{}{"name": f.room.Name, "settings": settings, "retention": f.room.Retention, "version": f.room.Version}, 200, nil)
	var pair map[string]string
	f.request(t, f.host, "POST", f.path("/display-session"), nil, 201, &pair)
	display := client()
	f.request(t, display, "POST", "/display-exchange", map[string]string{"token": pair["token"]}, 200, nil)
	f.request(t, display, "GET", f.path("/board?display=1"), nil, 200, &hidden)
	if hidden.Word != "" {
		t.Fatal("secret leaked to display")
	}
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "stroke", Epoch: b.Epoch, Stroke: stroke}, 403, nil)
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "guess", Epoch: b.Epoch, Text: "  " + strings.ToUpper(b.Word) + "  "}, 200, nil)
	data := f.request(t, f.other, "GET", f.path("/board"), nil, 200, nil)
	if strings.Contains(string(data), `"word":"`+b.Word+`"`) {
		t.Fatal("correct guess leaked before reveal")
	}
	b = get()
	if b.Players[1].Score != 100 || b.Players[0].Score != 50 {
		t.Fatal("incorrect scores")
	}
	f.request(t, f.guest, "POST", f.path("/board"), boardCommand{Action: "guess", Epoch: b.Epoch, Text: b.Word}, 409, nil)
	b.Deadline = time.Now().Unix() - 1
	if e := f.s.saveBoard(f.room.ID, b); e != nil {
		t.Fatal(e)
	}
	f.request(t, f.other, "GET", f.path("/board"), nil, 200, &hidden)
	if hidden.Word == "" {
		t.Fatal("timeout did not reveal")
	}
	previous := b.Epoch
	command(f.host, "next", 200)
	f.request(t, f.other, "POST", f.path("/board"), boardCommand{Action: "guess", Epoch: previous, Text: b.Word}, 409, nil)
	f.request(t, f.host, "PATCH", f.path("/participants/"+guest), map[string]bool{"revoked": true}, 200, nil)
	if b = get(); b.Phase != "finished" || b.Word != "" {
		t.Fatal("removed participant did not cancel game")
	}
}
