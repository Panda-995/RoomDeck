package server

import (
	"crypto/subtle"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	var n int
	if e := s.db.QueryRow("SELECT COUNT(*) FROM admins").Scan(&n); e != nil {
		s.internal(w, e)
		return
	}
	p := s.identity(r, "host")
	writeJSON(w, 200, map[string]interface{}{"setup_required": n == 0, "authenticated": p != nil, "username": func() string {
		if p != nil {
			return p.Name
		}
		return ""
	}(), "base_url": s.config.BaseURL})
}
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if s.limited(r, "setup", 5) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	var in struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setupToken == "" || subtle.ConstantTimeCompare([]byte(in.Token), []byte(s.setupToken)) != 1 {
		apiError(w, 403, "SETUP_TOKEN_INVALID")
		return
	}
	if !validText(in.Username, 1, 40) || len(in.Password) < 12 || len(in.Password) > 72 {
		apiError(w, 400, "CREDENTIALS_INVALID")
		return
	}
	hashed, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		s.internal(w, e)
		return
	}
	id := randomID(16)
	_, e = s.db.Exec("INSERT INTO admins VALUES(?,?,?)", id, strings.TrimSpace(in.Username), hashed)
	if e != nil {
		s.internal(w, e)
		return
	}
	s.setupToken = ""
	_ = os.Remove(filepath.Join(s.config.DataDir, "setup-token"))
	if e = s.issueSession(w, r, "host", id, "", 30*24*time.Hour); e != nil {
		s.internal(w, e)
		return
	}
	writeJSON(w, 201, map[string]bool{"ok": true})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.limited(r, "login", 10) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var id string
	var hash []byte
	e := s.db.QueryRow("SELECT id,password FROM admins WHERE username=?", in.Username).Scan(&id, &hash)
	if e != nil || bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) != nil {
		apiError(w, 401, "LOGIN_FAILED")
		return
	}
	if e = s.issueSession(w, r, "host", id, "", 30*24*time.Hour); e != nil {
		s.internal(w, e)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	for _, role := range []string{"host", "guest", "display"} {
		if c, e := r.Cookie("rd_" + role); e == nil {
			_, _ = s.db.Exec("DELETE FROM sessions WHERE hash=?", digest(c.Value))
		}
		http.SetCookie(w, &http.Cookie{Name: "rd_" + role, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, role, id, room string, duration time.Duration) error {
	token := randomID(32)
	expires := time.Now().Add(duration)
	_, e := s.db.Exec("INSERT INTO sessions VALUES(?,?,?,?,?)", digest(token), role, id, room, expires.Unix())
	if e != nil {
		return e
	}
	http.SetCookie(w, &http.Cookie{Name: "rd_" + role, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil || strings.HasPrefix(s.config.BaseURL, "https://"), Expires: expires})
	return nil
}
func (s *Server) identity(r *http.Request, role string) *principal {
	cookie, e := r.Cookie("rd_" + role)
	if e != nil {
		return nil
	}
	p := &principal{}
	e = s.db.QueryRow("SELECT role,participant_id,room_id FROM sessions WHERE hash=? AND role=? AND expires_at>?", digest(cookie.Value), role, time.Now().Unix()).Scan(&p.Role, &p.ID, &p.RoomID)
	if e != nil {
		return nil
	}
	if role == "host" {
		e = s.db.QueryRow("SELECT username FROM admins WHERE id=?", p.ID).Scan(&p.Name)
	} else if role == "guest" {
		e = s.db.QueryRow("SELECT name FROM participants WHERE id=? AND revoked=0", p.ID).Scan(&p.Name)
	}
	if e != nil {
		return nil
	}
	return p
}
func (s *Server) host(w http.ResponseWriter, r *http.Request) *principal {
	p := s.identity(r, "host")
	if p == nil {
		apiError(w, 401, "AUTH_REQUIRED")
	}
	return p
}
func (s *Server) roomAccess(w http.ResponseWriter, r *http.Request) (*principal, *Room) {
	room, e := s.getRoom(r.PathValue("room"))
	if e != nil {
		if e == sql.ErrNoRows {
			apiError(w, 404, "ROOM_NOT_FOUND")
		} else {
			s.internal(w, e)
		}
		return nil, nil
	}
	if room.Expired() {
		apiError(w, 410, "ROOM_DELETED")
		return nil, nil
	}
	roles := []string{"host", "guest", "display"}
	if r.URL.Query().Get("display") == "1" {
		roles = []string{"display"}
	}
	for _, role := range roles {
		p := s.identity(r, role)
		if p != nil && (p.Role == "host" || p.RoomID == room.ID) {
			return p, room
		}
	}
	apiError(w, 401, "AUTH_REQUIRED")
	return nil, nil
}
func (s *Server) canWrite(w http.ResponseWriter, p *principal, room *Room) bool {
	if !room.Active() {
		apiError(w, 409, "ROOM_CLOSED")
		return false
	}
	if p.Role == "display" {
		apiError(w, 403, "FORBIDDEN")
		return false
	}
	if p.Role == "guest" {
		var muted bool
		e := s.db.QueryRow("SELECT muted FROM participants WHERE id=?", p.ID).Scan(&muted)
		if e != nil || muted || room.Settings.UploadsPaused {
			apiError(w, 403, "UPLOADS_PAUSED")
			return false
		}
	}
	return true
}
