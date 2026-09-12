package server

import "testing"

func TestReactionsAndModerationPermissions(t *testing.T) {
	f := newFixture(t)
	var c Content
	f.request(t, f.guest, "POST", f.path("/contents"), map[string]string{"kind": "note", "body": "hello"}, 201, &c)
	for i := 0; i < 2; i++ {
		f.request(t, f.guest, "PUT", f.path("/contents/"+c.ID+"/reaction"), map[string]string{"emoji": "heart"}, 200, nil)
	}
	var n int
	_ = f.s.db.QueryRow("SELECT COUNT(*) FROM reactions WHERE content_id=?", c.ID).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate reactions")
	}
	f.request(t, f.guest, "PUT", f.path("/contents/"+c.ID+"/reaction"), map[string]string{"emoji": "party"}, 200, nil)
	var emoji string
	_ = f.s.db.QueryRow("SELECT emoji FROM reactions WHERE content_id=?", c.ID).Scan(&emoji)
	if emoji != "party" {
		t.Fatal("did not replace reaction")
	}
	room, _ := f.s.getRoom(f.room.ID)
	room.Settings.DanmakuMode = "approval"
	f.request(t, f.host, "PATCH", f.path(""), map[string]interface{}{"name": room.Name, "retention": room.Retention, "version": room.Version, "settings": room.Settings}, 200, nil)
	var message map[string]string
	f.request(t, f.guest, "POST", f.path("/danmaku"), map[string]string{"body": "<script>not HTML</script>"}, 201, &message)
	f.request(t, f.other, "POST", "/join", joinInput{Code: f.room.Code, Name: "Other"}, 201, nil)
	var result struct{ Messages []danmakuMessage }
	f.request(t, f.other, "GET", f.path("/interactions"), nil, 200, &result)
	if len(result.Messages) != 0 {
		t.Fatal("pending message leaked")
	}
	f.request(t, f.guest, "POST", f.path("/danmaku/moderate"), map[string]string{"action": "approve", "id": message["id"]}, 403, nil)
	f.request(t, f.host, "POST", f.path("/danmaku/moderate"), map[string]string{"action": "approve", "id": message["id"]}, 200, nil)
	f.request(t, f.other, "GET", f.path("/interactions"), nil, 200, &result)
	if len(result.Messages) != 1 || result.Messages[0].State != "approved" {
		t.Fatal("approved message missing")
	}
	f.request(t, f.host, "POST", f.path("/danmaku/moderate"), map[string]string{"action": "clear"}, 200, nil)
	f.request(t, f.other, "GET", f.path("/interactions"), nil, 200, &result)
	if len(result.Messages) != 0 {
		t.Fatal("clear failed")
	}
}

func TestScreenQueueApprovalAndExpiry(t *testing.T) {
	f := newFixture(t)
	f.request(t, f.host, "POST", f.path("/screen/queue"), map[string]string{"action": "mode", "mode": "approval"}, 200, nil)
	for i := 0; i < 2; i++ {
		f.request(t, f.guest, "POST", f.path("/screen/queue"), map[string]string{"action": "request"}, 200, nil)
	}
	var q struct{ Requests []screenRequest }
	f.request(t, f.host, "GET", f.path("/screen/queue"), nil, 200, &q)
	if len(q.Requests) != 1 || q.Requests[0].State != "pending" {
		t.Fatal("duplicate or premature offer")
	}
	id := q.Requests[0].ID
	f.request(t, f.guest, "POST", f.path("/screen/queue"), map[string]string{"action": "approve", "id": id}, 403, nil)
	f.request(t, f.host, "POST", f.path("/screen/queue"), map[string]string{"action": "approve", "id": id}, 200, nil)
	f.request(t, f.guest, "GET", f.path("/screen/queue"), nil, 200, &q)
	if len(q.Requests) != 1 || q.Requests[0].State != "offered" {
		t.Fatal("offer missing")
	}
	_, _ = f.s.db.Exec("UPDATE screen_requests SET expires_at=1 WHERE id=?", id)
	f.request(t, f.guest, "GET", f.path("/screen/queue"), nil, 200, &q)
	if len(q.Requests) != 0 {
		t.Fatal("expired offer retained")
	}
}
