package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) displayControl(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if !s.permitted(p, room.ID, "display") {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	if !room.Active() {
		apiError(w, 409, "ROOM_CLOSED")
		return
	}
	if !room.Enabled("display") {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	var in struct {
		Mode    string `json:"mode"`
		Target  string `json:"target"`
		Source  string `json:"source"`
		Version int64  `json:"expected_version"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	settings := room.Settings
	settings.DisplayMode, settings.DisplayTarget, settings.DisplaySource = in.Mode, in.Target, in.Source
	if !validSettings(settings) {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	if room.Version != in.Version {
		apiError(w, 409, "VERSION_CONFLICT")
		return
	}
	if in.Mode == "poll" && in.Target != "" {
		c, err := s.getContent(room.ID, in.Target)
		if err != nil || c.Kind != "poll" || !c.Visible || !room.Enabled("poll") {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		s.internal(w, err)
		return
	}
	defer tx.Rollback()
	if in.Mode == "poll" && in.Target != "" {
		if _, err = tx.Exec("UPDATE contents SET selected=1 WHERE id=? AND room_id=?", in.Target, room.ID); err != nil {
			s.internal(w, err)
			return
		}
	}
	data, _ := json.Marshal(settings)
	if _, err = tx.Exec("UPDATE rooms SET settings=?,version=version+1 WHERE id=?", string(data), room.ID); err != nil {
		s.internal(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(room.ID, "display.changed", p.ID)
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
