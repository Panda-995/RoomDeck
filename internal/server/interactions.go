package server

import (
	"net/http"
	"time"
)

var allowedReactions = map[string]bool{"heart": true, "like": true, "laugh": true, "wow": true, "clap": true, "party": true}

func (s *Server) setReaction(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil || !s.canInteract(w, p, room) {
		return
	}
	if room.Settings.ReactionsDisabled {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	c, e := s.getContent(room.ID, r.PathValue("content"))
	if e != nil || !c.Visible || !room.Enabled(c.Kind) {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	var in struct {
		Emoji string `json:"emoji"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Emoji != "" && !allowedReactions[in.Emoji] {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	if s.limited(r, "reaction:"+room.ID+":"+p.ID, 120) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	if in.Emoji == "" {
		_, e = s.db.Exec("DELETE FROM reactions WHERE content_id=? AND owner=?", c.ID, p.ID)
	} else {
		_, e = s.db.Exec("INSERT INTO reactions VALUES(?,?,?) ON CONFLICT(content_id,owner) DO UPDATE SET emoji=excluded.emoji", c.ID, p.ID, in.Emoji)
	}
	if e != nil {
		s.internal(w, e)
		return
	}
	s.hub.notifyInteractions(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type danmakuMessage struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	State   string `json:"state"`
	Expires int64  `json:"expires_at"`
}

func (s *Server) interactions(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role == "display" && (!room.Enabled("display") || !room.Active() || room.Settings.DisplayMode == "blank") {
		writeJSON(w, 200, map[string]interface{}{"reactions": []interface{}{}, "messages": []interface{}{}})
		return
	}
	type reaction struct {
		Content string `json:"content"`
		Emoji   string `json:"emoji"`
		Count   int    `json:"count"`
		Mine    int    `json:"mine"`
	}
	reactions := []reaction{}
	if p.Role != "display" {
		rows, e := s.db.Query("SELECT c.id,r.emoji,COUNT(*),MAX(CASE WHEN r.owner=? THEN 1 ELSE 0 END),c.kind FROM reactions r JOIN contents c ON c.id=r.content_id WHERE c.room_id=? AND c.visible=1 GROUP BY c.id,r.emoji", p.ID, room.ID)
		if e != nil {
			s.internal(w, e)
			return
		}
		for rows.Next() {
			var v reaction
			var kind string
			if rows.Scan(&v.Content, &v.Emoji, &v.Count, &v.Mine, &kind) == nil && room.Enabled(kind) {
				reactions = append(reactions, v)
			}
		}
		rows.Close()
	}
	messages := []danmakuMessage{}
	if room.Active() && room.Settings.DanmakuMode != "off" && room.Settings.DanmakuMode != "" {
		rows, e := s.db.Query("SELECT id,name,body,state,expires_at FROM danmaku WHERE room_id=? AND expires_at>? AND (state='approved' OR (?<>'display' AND (owner=? OR ?=1))) ORDER BY created_at LIMIT 50", room.ID, time.Now().Unix(), p.Role, p.ID, s.permitted(p, room.ID, "content"))
		if e != nil {
			s.internal(w, e)
			return
		}
		for rows.Next() {
			var v danmakuMessage
			if rows.Scan(&v.ID, &v.Name, &v.Body, &v.State, &v.Expires) == nil {
				messages = append(messages, v)
			}
		}
		rows.Close()
	}
	writeJSON(w, 200, map[string]interface{}{"reactions": reactions, "messages": messages})
}
func (s *Server) sendDanmaku(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil || !s.canInteract(w, p, room) {
		return
	}
	if room.Settings.DanmakuMode != "direct" && room.Settings.DanmakuMode != "approval" {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !validText(in.Body, 1, 60) {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	now := time.Now().Unix()
	var recent, waiting, second int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM danmaku WHERE room_id=? AND owner=? AND created_at>?", room.ID, p.ID, now-5).Scan(&recent)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM danmaku WHERE room_id=? AND expires_at>? AND state IN ('approved','pending')", room.ID, now).Scan(&waiting)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM danmaku WHERE room_id=? AND created_at>=?", room.ID, now).Scan(&second)
	if recent > 0 || waiting >= 20 || second >= 5 || s.limited(r, "danmaku:"+room.ID+":"+p.ID, 10) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	state := "approved"
	expiry := now + 30
	if room.Settings.DanmakuMode == "approval" {
		state = "pending"
		expiry = now + 300
	}
	id := randomID(16)
	_, e := s.db.Exec("INSERT INTO danmaku VALUES(?,?,?,?,?,?,?,?)", id, room.ID, p.ID, p.Name, in.Body, state, now, expiry)
	if e != nil {
		s.internal(w, e)
		return
	}
	s.hub.notifyInteractions(room.ID)
	writeJSON(w, 201, map[string]string{"id": id, "state": state})
}
func (s *Server) moderateDanmaku(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if !s.permitted(p, room.ID, "content") {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	var in struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var e error
	switch in.Action {
	case "clear":
		_, e = s.db.Exec("DELETE FROM danmaku WHERE room_id=?", room.ID)
	case "approve", "reject":
		state := "approved"
		if in.Action == "reject" {
			state = "rejected"
		}
		result, err := s.db.Exec("UPDATE danmaku SET state=?,expires_at=? WHERE id=? AND room_id=? AND state='pending' AND expires_at>?", state, time.Now().Unix()+30, in.ID, room.ID, time.Now().Unix())
		e = err
		if e == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				apiError(w, 409, "VERSION_CONFLICT")
				return
			}
		}
	default:
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	if e != nil {
		s.internal(w, e)
		return
	}
	s.hub.notifyInteractions(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) canInteract(w http.ResponseWriter, p *principal, room *Room) bool {
	if !room.Active() {
		apiError(w, 409, "ROOM_CLOSED")
		return false
	}
	if p.Role == "display" {
		apiError(w, 403, "FORBIDDEN")
		return false
	}
	if p.Role == "guest" {
		var n int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM participants WHERE id=? AND room_id=? AND revoked=0 AND muted=0", p.ID, room.ID).Scan(&n)
		if n != 1 {
			apiError(w, 403, "FORBIDDEN")
			return false
		}
	}
	return true
}
