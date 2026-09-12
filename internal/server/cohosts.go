package server

import (
	"encoding/json"
	"net/http"
)

var cohostScopes = []string{"content", "screen", "games", "display"}

func (s *Server) permissions(p *principal, room string) []string {
	if p == nil || p.Role == "display" {
		return []string{}
	}
	if p.Role == "host" {
		return cohostScopes
	}
	var raw string
	if p.RoomID != room || s.db.QueryRow("SELECT permissions FROM participants WHERE id=? AND room_id=? AND revoked=0 AND muted=0", p.ID, room).Scan(&raw) != nil {
		return []string{}
	}
	result := []string{}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}
func (s *Server) permitted(p *principal, room, scope string) bool {
	for _, v := range s.permissions(p, room) {
		if v == scope {
			return true
		}
	}
	return false
}
func (s *Server) setCohost(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role != "host" {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	if !room.Active() {
		apiError(w, 409, "ROOM_CLOSED")
		return
	}
	var in struct {
		Permissions []string `json:"permissions"`
		Scope       string   `json:"scope"`
		Enabled     *bool    `json:"enabled"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Scope != "" {
		valid := false
		for _, scope := range cohostScopes {
			if scope == in.Scope {
				valid = true
			}
		}
		if !valid || in.Enabled == nil {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
		var raw string
		if s.db.QueryRow("SELECT permissions FROM participants WHERE id=? AND room_id=? AND revoked=0", r.PathValue("participant"), room.ID).Scan(&raw) != nil {
			apiError(w, 404, "NOT_FOUND")
			return
		}
		current := []string{}
		_ = json.Unmarshal([]byte(raw), &current)
		in.Permissions = []string{}
		for _, scope := range current {
			if scope != in.Scope {
				in.Permissions = append(in.Permissions, scope)
			}
		}
		if *in.Enabled {
			in.Permissions = append(in.Permissions, in.Scope)
		}
	}
	if in.Permissions == nil {
		in.Permissions = []string{}
	}
	seen := map[string]bool{}
	for _, scope := range in.Permissions {
		valid := false
		for _, allowed := range cohostScopes {
			if scope == allowed {
				valid = true
			}
		}
		if !valid || seen[scope] {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
		seen[scope] = true
	}
	raw, _ := json.Marshal(in.Permissions)
	tx, err := s.db.Begin()
	if err != nil {
		s.internal(w, err)
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE participants SET permissions=? WHERE id=? AND room_id=? AND revoked=0", string(raw), r.PathValue("participant"), room.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if _, err = tx.Exec("UPDATE rooms SET state_revision=state_revision+1 WHERE id=?", room.ID); err != nil {
		s.internal(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(room.ID, "cohost.updated:"+r.PathValue("participant"), p.ID)
	s.hub.notify(room.ID)
	s.hub.notifyInteractions(room.ID)
	writeJSON(w, 200, map[string]interface{}{"ok": true, "permissions": in.Permissions})
}
