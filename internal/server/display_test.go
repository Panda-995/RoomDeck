package server

import (
	"testing"
	"time"
)

func TestExplicitDisplayPoll(t *testing.T) {
	f := newFixture(t)
	var c Content
	f.request(t, f.host, "POST", f.path("/contents"), map[string]interface{}{"kind": "poll", "title": "Which?", "poll": Poll{Options: []string{"One", "Two"}, MaxChoices: 1, HideResults: true, ClosesAt: time.Now().Unix() + 600}}, 201, &c)
	input := map[string]interface{}{"mode": "poll", "target": c.ID, "source": "content", "expected_version": f.room.Version}
	f.request(t, f.guest, "POST", f.path("/display/control"), input, 403, nil)
	f.request(t, f.host, "POST", f.path("/display/control"), input, 200, nil)
	fresh, _ := f.s.getRoom(f.room.ID)
	selected, _ := f.s.getContent(f.room.ID, c.ID)
	if !selected.Selected || fresh.Settings.DisplayTarget != c.ID || fresh.Settings.DisplaySource != "content" {
		t.Fatal("target not atomically selected")
	}
	if fresh.StateRevision <= f.room.StateRevision {
		t.Fatal("revision did not advance")
	}
	f.request(t, f.host, "POST", f.path("/display/control"), input, 409, nil)
	f.request(t, f.host, "PATCH", f.path("/contents/"+c.ID), map[string]bool{"visible": false}, 200, nil)
	fresh, _ = f.s.getRoom(f.room.ID)
	if fresh.Settings.DisplayTarget != "" {
		t.Fatal("hidden target retained")
	}
	input["expected_version"] = fresh.Version
	f.request(t, f.host, "POST", f.path("/display/control"), input, 400, nil)
}
