package server

import (
	"encoding/json"
	"net/http"
	"time"
)

type screenRequest struct {
	ID      string `json:"id"`
	Owner   string `json:"owner"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Expires int64  `json:"expires_at"`
}

// Called while mediaMu is held. A stale media stage must be removed before calling the next person.
func (s *Server) advanceQueue(room *Room) {
	now := time.Now().Unix()
	_, _ = s.db.Exec("UPDATE screen_requests SET state='expired' WHERE room_id=? AND state='offered' AND expires_at<=?", room.ID, now)
	_, _ = s.db.Exec("UPDATE screen_requests SET state='expired' WHERE room_id=? AND state IN ('queued','pending','offered') AND role='guest' AND NOT EXISTS(SELECT 1 FROM participants WHERE participants.id=screen_requests.owner AND revoked=0 AND muted=0 AND last_seen>?)", room.ID, now-60)
	if !room.Active() {
		_, _ = s.db.Exec("UPDATE screen_requests SET state='expired' WHERE room_id=? AND state IN ('queued','pending','offered')", room.ID)
		return
	}
	if room.Settings.ScreenMode != "queue" && room.Settings.ScreenMode != "approval" {
		return
	}
	if s.getScreen(room.ID) != nil {
		return
	}
	var offers int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM screen_requests WHERE room_id=? AND state='offered'", room.ID).Scan(&offers)
	if offers > 0 {
		return
	}
	_, _ = s.db.Exec("UPDATE screen_requests SET state='offered',expires_at=? WHERE id=(SELECT id FROM screen_requests WHERE room_id=? AND state='queued' ORDER BY created_at,id LIMIT 1)", now+30, room.ID)
}
func (s *Server) screenQueue(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role == "display" {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	room, err := s.getRoom(room.ID)
	if err != nil {
		apiError(w, 404, "ROOM_NOT_FOUND")
		return
	}
	s.advanceQueue(room)
	rows, err := s.db.Query("SELECT id,owner,name,state,expires_at FROM screen_requests WHERE room_id=? AND state IN ('pending','queued','offered','presenting') AND (state<>'pending' OR owner=? OR ?='host') ORDER BY created_at,id", room.ID, p.ID, p.Role)
	if err != nil {
		s.internal(w, err)
		return
	}
	defer rows.Close()
	list := []screenRequest{}
	for rows.Next() {
		var v screenRequest
		if err = rows.Scan(&v.ID, &v.Owner, &v.Name, &v.State, &v.Expires); err != nil {
			s.internal(w, err)
			return
		}
		list = append(list, v)
	}
	mode := room.Settings.ScreenMode
	if mode == "" {
		mode = "free"
	}
	writeJSON(w, 200, map[string]interface{}{"mode": mode, "requests": list})
}
func (s *Server) screenQueueAction(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role == "display" {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	var in struct {
		Action string `json:"action"`
		ID     string `json:"id"`
		Mode   string `json:"mode"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	room, err := s.getRoom(room.ID)
	if err != nil || !room.Active() {
		apiError(w, 409, "ROOM_CLOSED")
		return
	}
	if !s.canInteract(w, p, room) {
		return
	}
	switch in.Action {
	case "mode":
		if !s.permitted(p, room.ID, "screen") {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if in.Mode != "free" && in.Mode != "queue" && in.Mode != "approval" {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		room, err = s.getRoom(room.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		room.Settings.ScreenMode = in.Mode
		data, _ := json.Marshal(room.Settings)
		_, err = s.db.Exec("UPDATE rooms SET settings=?,version=version+1 WHERE id=?", string(data), room.ID)
		if err == nil {
			_, err = s.db.Exec("UPDATE screen_requests SET state='cancelled' WHERE room_id=? AND state IN ('pending','queued','offered')", room.ID)
		}
	case "request":
		if room.Settings.ScreenMode != "queue" && room.Settings.ScreenMode != "approval" {
			apiError(w, 409, "INVALID_INPUT")
			return
		}
		if s.limited(r, "screen-request", 20) {
			apiError(w, 429, "RATE_LIMITED")
			return
		}
		state := "queued"
		if room.Settings.ScreenMode == "approval" {
			state = "pending"
		}
		_, err = s.db.Exec("INSERT INTO screen_requests(id,room_id,owner,role,name,state,created_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT DO NOTHING", randomID(16), room.ID, p.ID, p.Role, p.Name, state, time.Now().UnixNano())
	case "cancel", "approve", "reject":
		var owner, state string
		if s.db.QueryRow("SELECT owner,state FROM screen_requests WHERE id=? AND room_id=?", in.ID, room.ID).Scan(&owner, &state) != nil {
			apiError(w, 404, "NOT_FOUND")
			return
		}
		if !s.permitted(p, room.ID, "screen") && (in.Action != "cancel" || owner != p.ID) {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if state == "presenting" {
			apiError(w, 409, "SCREEN_BUSY")
			return
		}
		next := "cancelled"
		if in.Action == "approve" {
			if state != "pending" {
				apiError(w, 409, "VERSION_CONFLICT")
				return
			}
			next = "queued"
		}
		if in.Action == "reject" {
			next = "rejected"
		}
		_, err = s.db.Exec("UPDATE screen_requests SET state=? WHERE id=?", next, in.ID)
	case "next":
		if !s.permitted(p, room.ID, "screen") {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if s.getScreen(room.ID) != nil {
			apiError(w, 409, "SCREEN_BUSY")
			return
		}
		_, err = s.db.Exec("UPDATE screen_requests SET state='expired' WHERE room_id=? AND state='offered'", room.ID)
	default:
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	s.advanceQueue(room)
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
