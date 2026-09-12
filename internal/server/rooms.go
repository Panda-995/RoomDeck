package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type Settings struct {
	Modules           []string `json:"modules"`
	JoinLocked        bool     `json:"join_locked"`
	UploadsPaused     bool     `json:"uploads_paused"`
	DownloadOriginal  bool     `json:"download_original"`
	AutoDisplay       bool     `json:"auto_display"`
	Quota             int64    `json:"quota"`
	MaxMembers        int      `json:"max_members"`
	DisplayMode       string   `json:"display_mode"`
	DisplayTarget     string   `json:"display_target"`
	DisplaySource     string   `json:"display_source"`
	ScreenMode        string   `json:"screen_mode"`
	ReactionsDisabled bool     `json:"reactions_disabled"`
	DanmakuMode       string   `json:"danmaku_mode"`
	DisplayPaused     bool     `json:"display_paused"`
	Interval          int      `json:"interval"`
}
type Room struct {
	ID            string   `json:"id"`
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	State         string   `json:"state"`
	Invite        string   `json:"-"`
	EndsAt        int64    `json:"ends_at"`
	ClosedAt      int64    `json:"closed_at"`
	DeleteAt      int64    `json:"delete_at"`
	Retention     int64    `json:"retention"`
	CreatedAt     int64    `json:"created_at"`
	Settings      Settings `json:"settings"`
	Version       int64    `json:"version"`
	Used          int64    `json:"used"`
	Reserved      int64    `json:"reserved"`
	StateRevision int64    `json:"state_revision"`
}

func (r *Room) Expired() bool {
	return r.State == "deleting" || (r.DeleteAt > 0 && r.DeleteAt <= time.Now().Unix())
}
func (r *Room) Active() bool { return r.State == "active" && r.EndsAt > time.Now().Unix() }
func (r *Room) Enabled(kind string) bool {
	for _, m := range r.Settings.Modules {
		if m == kind {
			return true
		}
	}
	return false
}

const roomSelect = "SELECT id,code,name,state,invite,ends_at,closed_at,delete_at,retention,created_at,settings,version,used,reserved,state_revision FROM rooms"

type scanner interface{ Scan(...interface{}) error }

func scanRoom(row scanner) (*Room, error) {
	v := &Room{}
	var settings string
	e := row.Scan(&v.ID, &v.Code, &v.Name, &v.State, &v.Invite, &v.EndsAt, &v.ClosedAt, &v.DeleteAt, &v.Retention, &v.CreatedAt, &settings, &v.Version, &v.Used, &v.Reserved, &v.StateRevision)
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal([]byte(settings), &v.Settings); e != nil {
		return nil, e
	}
	if v.State == "active" && v.EndsAt <= time.Now().Unix() {
		v.State = "closed"
		v.ClosedAt = v.EndsAt
		v.DeleteAt = v.EndsAt + v.Retention
	}
	return v, nil
}
func (s *Server) getRoom(id string) (*Room, error) {
	return scanRoom(s.db.QueryRow(roomSelect+" WHERE id=?", id))
}
func validSettings(v Settings) bool {
	if v.DanmakuMode != "" && v.DanmakuMode != "off" && v.DanmakuMode != "direct" && v.DanmakuMode != "approval" {
		return false
	}
	if v.ScreenMode != "" && v.ScreenMode != "free" && v.ScreenMode != "queue" && v.ScreenMode != "approval" {
		return false
	}
	if v.DisplaySource != "" && v.DisplaySource != "content" && v.DisplaySource != "screen" {
		return false
	}
	if v.Quota < 20<<20 || v.Quota > 100<<30 || v.MaxMembers < 1 || v.MaxMembers > 200 || v.Interval < 5 || v.Interval > 60 {
		return false
	}
	if v.DisplayMode != "welcome" && v.DisplayMode != "photos" && v.DisplayMode != "pinned" && v.DisplayMode != "poll" && v.DisplayMode != "blank" && v.DisplayMode != "board" {
		return false
	}
	allowed := map[string]bool{"photo": true, "file": true, "note": true, "link": true, "poll": true, "display": true}
	seen := map[string]bool{}
	for _, m := range v.Modules {
		if !allowed[m] || seen[m] {
			return false
		}
		seen[m] = true
	}
	return len(v.Modules) > 0
}
func defaultSettings() Settings {
	return Settings{Modules: []string{"photo", "file", "note", "link", "poll", "display"}, Quota: 2 << 30, MaxMembers: 50, DisplayMode: "welcome", Interval: 10}
}
func (s *Server) listRooms(w http.ResponseWriter, r *http.Request) {
	if s.host(w, r) == nil {
		return
	}
	rows, e := s.db.Query(roomSelect + " ORDER BY created_at DESC")
	if e != nil {
		s.internal(w, e)
		return
	}
	defer rows.Close()
	list := []*Room{}
	for rows.Next() {
		v, e := scanRoom(rows)
		if e != nil {
			s.internal(w, e)
			return
		}
		list = append(list, v)
	}
	writeJSON(w, 200, list)
}
func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	p := s.host(w, r)
	if p == nil {
		return
	}
	var in struct {
		Name      string   `json:"name"`
		Duration  int64    `json:"duration"`
		Retention int64    `json:"retention"`
		Modules   []string `json:"modules"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !validText(in.Name, 1, 40) || in.Duration < 3600 || in.Duration > 7*86400 || in.Retention < 3600 || in.Retention > 30*86400 {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	settings := defaultSettings()
	if len(in.Modules) > 0 {
		settings.Modules = in.Modules
	}
	if !validSettings(settings) {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	b, _ := json.Marshal(settings)
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count)
	if count >= 100 {
		apiError(w, 409, "ROOM_LIMIT")
		return
	}
	id := randomID(16)
	code := ""
	chars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for attempts := 0; attempts < 10; attempts++ {
		bytes := make([]byte, 6)
		_, _ = rand.Read(bytes)
		for i := range bytes {
			bytes[i] = chars[int(bytes[i])%len(chars)]
		}
		code = string(bytes)
		var exists int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM rooms WHERE code=?", code).Scan(&exists)
		if exists == 0 {
			break
		}
	}
	now := time.Now().Unix()
	_, e := s.db.Exec("INSERT INTO rooms(id,code,name,invite,ends_at,retention,created_at,settings) VALUES(?,?,?,?,?,?,?,?)", id, code, strings.TrimSpace(in.Name), randomID(24), now+in.Duration, in.Retention, now, string(b))
	if e != nil {
		s.internal(w, e)
		return
	}
	s.audit(id, "room.created", p.ID)
	room, _ := s.getRoom(id)
	writeJSON(w, 201, room)
}
func (s *Server) updateRoom(w http.ResponseWriter, r *http.Request) {
	p, _ := s.roomAccess(w, r)
	if p == nil {
		return
	}
	var in struct {
		Name      string   `json:"name"`
		Settings  Settings `json:"settings"`
		Version   int64    `json:"version"`
		Retention int64    `json:"retention"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !validText(in.Name, 1, 40) || !validSettings(in.Settings) || in.Retention < 3600 || in.Retention > 30*86400 {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if room == nil {
		return
	}
	if p.Role != "host" {
		allowed := room.Settings
		if s.permitted(p, room.ID, "display") {
			allowed.DisplayMode = in.Settings.DisplayMode
			allowed.DisplayTarget = in.Settings.DisplayTarget
			allowed.DisplaySource = in.Settings.DisplaySource
			allowed.DisplayPaused = in.Settings.DisplayPaused
			allowed.Interval = in.Settings.Interval
		}
		if s.permitted(p, room.ID, "content") {
			allowed.DanmakuMode = in.Settings.DanmakuMode
			allowed.ReactionsDisabled = in.Settings.ReactionsDisabled
		}
		if !room.Active() || in.Name != room.Name || in.Retention != room.Retention || !reflect.DeepEqual(in.Settings, allowed) || (!s.permitted(p, room.ID, "content") && !s.permitted(p, room.ID, "display")) {
			apiError(w, 403, "FORBIDDEN")
			return
		}
	}
	if in.Settings.Quota < room.Used+room.Reserved {
		apiError(w, 409, "QUOTA_TOO_SMALL")
		return
	}
	if room.State == "closed" && room.ClosedAt+in.Retention <= time.Now().Unix() {
		apiError(w, 409, "RETENTION_EXPIRED")
		return
	}
	data, _ := json.Marshal(in.Settings)
	result, e := s.db.Exec("UPDATE rooms SET name=?,settings=?,retention=?,delete_at=CASE WHEN closed_at>0 THEN closed_at+? ELSE delete_at END,version=version+1 WHERE id=? AND version=?", strings.TrimSpace(in.Name), string(data), in.Retention, in.Retention, room.ID, in.Version)
	if e != nil {
		s.internal(w, e)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		apiError(w, 409, "VERSION_CONFLICT")
		return
	}
	s.audit(room.ID, "room.updated", p.ID)
	if in.Settings.DanmakuMode == "" || in.Settings.DanmakuMode == "off" {
		_, _ = s.db.Exec("DELETE FROM danmaku WHERE room_id=?", room.ID)
	}
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) closeRoom(w http.ResponseWriter, r *http.Request) {
	p := s.host(w, r)
	if p == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, room := s.roomAccess(w, r)
	if room == nil {
		return
	}
	now := time.Now().Unix()
	if room.State == "closed" && room.ClosedAt > 0 {
		now = room.ClosedAt
	}
	_, e := s.db.Exec("UPDATE rooms SET state='closed',closed_at=?,delete_at=?+retention,version=version+1 WHERE id=? AND state='active'", now, now, room.ID)
	if e != nil {
		s.internal(w, e)
		return
	}
	s.audit(room.ID, "room.closed", p.ID)
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) deleteRoom(w http.ResponseWriter, r *http.Request) {
	p := s.host(w, r)
	if p == nil {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room, e := s.getRoom(r.PathValue("room"))
	if e != nil {
		apiError(w, 404, "ROOM_NOT_FOUND")
		return
	}
	if in.Name != room.Name {
		apiError(w, 400, "NAME_MISMATCH")
		return
	}
	_, e = s.db.Exec("UPDATE rooms SET state='deleting',delete_at=?,version=version+1 WHERE id=?", time.Now().Unix(), room.ID)
	if e != nil {
		s.internal(w, e)
		return
	}
	_, _ = s.db.Exec("DELETE FROM sessions WHERE room_id=?", room.ID)
	s.audit(room.ID, "room.deleting", p.ID)
	s.hub.notify(room.ID)
	writeJSON(w, 202, map[string]bool{"ok": true})
}
func (s *Server) rotateInvite(w http.ResponseWriter, r *http.Request) {
	if s.host(w, r) == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, room := s.roomAccess(w, r)
	if room == nil {
		return
	}
	_, e := s.db.Exec("UPDATE rooms SET invite=?,version=version+1 WHERE id=?", randomID(24), room.ID)
	if e != nil {
		s.internal(w, e)
		return
	}
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type joinInput struct {
	Code  string `json:"code"`
	Token string `json:"token"`
	Name  string `json:"name"`
}

func (s *Server) resolveJoin(in joinInput) (*Room, error) {
	if in.Token != "" {
		return scanRoom(s.db.QueryRow(roomSelect+" WHERE invite=?", in.Token))
	}
	return scanRoom(s.db.QueryRow(roomSelect+" WHERE code=?", strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(in.Code), " ", ""))))
}
func (s *Server) inviteInfo(w http.ResponseWriter, r *http.Request) {
	if s.limited(r, "join", 20) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	var in joinInput
	if !readJSON(w, r, &in) {
		return
	}
	room, e := s.resolveJoin(in)
	if e != nil || !room.Active() || room.Settings.JoinLocked {
		apiError(w, 404, "JOIN_UNAVAILABLE")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"name": room.Name, "ends_at": room.EndsAt, "retention": room.Retention, "auto_display": room.Settings.AutoDisplay})
}
func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	if s.limited(r, "join", 20) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	var in joinInput
	if !readJSON(w, r, &in) {
		return
	}
	if !validText(in.Name, 1, 20) {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room, e := s.resolveJoin(in)
	if e != nil {
		apiError(w, 404, "JOIN_UNAVAILABLE")
		return
	}
	if p := s.identity(r, "guest"); p != nil && p.RoomID == room.ID && !room.Expired() {
		writeJSON(w, 200, map[string]string{"room_id": room.ID})
		return
	}
	if !room.Active() || room.Settings.JoinLocked {
		apiError(w, 404, "JOIN_UNAVAILABLE")
		return
	}
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM participants WHERE room_id=? AND revoked=0", room.ID).Scan(&count)
	if count >= room.Settings.MaxMembers {
		apiError(w, 409, "ROOM_FULL")
		return
	}
	id := randomID(16)
	now := time.Now().Unix()
	_, e = s.db.Exec("INSERT INTO participants(id,room_id,name,joined_at,last_seen) VALUES(?,?,?,?,?)", id, room.ID, strings.TrimSpace(in.Name), now, now)
	if e != nil {
		s.internal(w, e)
		return
	}
	if e = s.issueSession(w, r, "guest", id, room.ID, 38*24*time.Hour); e != nil {
		s.internal(w, e)
		return
	}
	s.hub.notify(room.ID)
	writeJSON(w, 201, map[string]string{"room_id": room.ID})
}
func (s *Server) updateParticipant(w http.ResponseWriter, r *http.Request) {
	if s.host(w, r) == nil {
		return
	}
	var in struct {
		Revoked bool `json:"revoked"`
		Muted   bool `json:"muted"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, room := s.roomAccess(w, r)
	if room == nil {
		return
	}
	id := r.PathValue("participant")
	result, e := s.db.Exec("UPDATE participants SET revoked=?,muted=? WHERE id=? AND room_id=?", in.Revoked, in.Muted, id, room.ID)
	if e != nil {
		s.internal(w, e)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if in.Revoked {
		_, _ = s.db.Exec("DELETE FROM sessions WHERE participant_id=?", id)
	}
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) displaySession(w http.ResponseWriter, r *http.Request) {
	if s.host(w, r) == nil {
		return
	}
	_, room := s.roomAccess(w, r)
	if room == nil {
		return
	}
	if !room.Enabled("display") {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	token := randomID(24)
	_, e := s.db.Exec("INSERT INTO sessions(hash,role,room_id,expires_at) VALUES(?,?,?,?)", digest(token), "pair", room.ID, time.Now().Add(2*time.Minute).Unix())
	if e != nil {
		s.internal(w, e)
		return
	}
	writeJSON(w, 201, map[string]string{"token": token})
}
func (s *Server) displayExchange(w http.ResponseWriter, r *http.Request) {
	if s.limited(r, "pair", 10) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var roomID string
	e := s.db.QueryRow("DELETE FROM sessions WHERE hash=? AND role='pair' AND expires_at>? RETURNING room_id", digest(in.Token), time.Now().Unix()).Scan(&roomID)
	if e != nil {
		apiError(w, 403, "PAIR_INVALID")
		return
	}
	room, e := s.getRoom(roomID)
	if e != nil || room.Expired() {
		apiError(w, 410, "ROOM_DELETED")
		return
	}
	if e = s.issueSession(w, r, "display", randomID(16), roomID, 38*24*time.Hour); e != nil {
		s.internal(w, e)
		return
	}
	writeJSON(w, 200, map[string]string{"room_id": roomID})
}

var _ = sql.ErrNoRows
