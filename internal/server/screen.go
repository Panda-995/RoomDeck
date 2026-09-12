package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type screenState struct {
	ID      string `json:"id"`
	Owner   string `json:"owner"`
	Name    string `json:"name"`
	Expires int64  `json:"expires"`
}

func (s *Server) mediaEnabled() bool {
	return s.config.MediaURL != "" && s.config.MediaKey != "" && len(s.config.MediaSecret) >= 32
}
func (s *Server) getScreen(room string) *screenState {
	v := &screenState{}
	if s.db.QueryRow("SELECT id,owner,name,expires FROM screens WHERE room_id=?", room).Scan(&v.ID, &v.Owner, &v.Name, &v.Expires) != nil {
		return nil
	}
	return v
}
func (s *Server) screenSnapshot(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	v := s.getScreen(room.ID)
	if !room.Active() || v != nil && v.Expires <= time.Now().Unix() {
		v = nil
	}
	writeJSON(w, 200, map[string]interface{}{"enabled": s.mediaEnabled(), "screen": v})
}
func (s *Server) mediaJWT(id, name, room string, admin, publish bool) string {
	grant := map[string]interface{}{"room": room, "roomJoin": !admin, "roomAdmin": admin, "canPublish": publish, "canSubscribe": true, "canPublishData": false}
	if admin {
		grant["roomCreate"] = true
	}
	if publish {
		grant["canPublishSources"] = []string{"screen_share", "screen_share_audio"}
	}
	payload, _ := json.Marshal(map[string]interface{}{"iss": s.config.MediaKey, "sub": id, "name": name, "nbf": time.Now().Unix() - 5, "exp": time.Now().Unix() + 60, "video": grant})
	enc := base64.RawURLEncoding.EncodeToString
	value := enc([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc(payload)
	mac := hmac.New(sha256.New, []byte(s.config.MediaSecret))
	mac.Write([]byte(value))
	return value + "." + enc(mac.Sum(nil))
}
func (s *Server) mediaCall(method, room string, body interface{}) error {
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.config.MediaURL, "/")+"/twirp/livekit.RoomService/"+method, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.mediaJWT("roomdeck", "", room, true, false))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	var result struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&result)
	if result.Code == "not_found" && (method == "DeleteRoom" || method == "RemoveParticipant") {
		return nil
	}
	return errors.New("media service unavailable")
}
func (s *Server) stopScreen(room string, v *screenState) error {
	// Deny reconnects before contacting the SFU; retain the row for retry on failure.
	_, _ = s.db.Exec("UPDATE screens SET expires=0 WHERE room_id=?", room)
	if err := s.mediaCall("DeleteRoom", v.ID, map[string]string{"room": v.ID}); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM media_clients WHERE screen_id=?", v.ID); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM screens WHERE room_id=? AND id=?", room, v.ID); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE screen_requests SET state='finished' WHERE room_id=? AND state='presenting'", room); err != nil {
		return err
	}
	err = tx.Commit()
	s.hub.notify(room)
	return err
}
func (s *Server) validMediaClient(id string) bool {
	var roomID, screenID, role, participant, session string
	var expiry int64
	if s.db.QueryRow("SELECT room_id,screen_id,session_hash,role,participant_id,expires FROM media_clients WHERE id=?", id).Scan(&roomID, &screenID, &session, &role, &participant, &expiry) != nil || expiry <= time.Now().Unix() {
		return false
	}
	room, err := s.getRoom(roomID)
	if err != nil || !room.Active() {
		return false
	}
	screen := s.getScreen(roomID)
	if screen == nil || screen.ID != screenID || screen.Expires <= time.Now().Unix() {
		return false
	}
	var n int
	if s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE hash=? AND expires_at>?", session, time.Now().Unix()).Scan(&n) != nil || n != 1 {
		return false
	}
	if role == "guest" {
		if s.db.QueryRow("SELECT COUNT(*) FROM participants WHERE id=? AND room_id=? AND revoked=0", participant, roomID).Scan(&n) != nil || n != 1 {
			return false
		}
		if strings.HasPrefix(id, "pub-") {
			var muted bool
			if s.db.QueryRow("SELECT muted FROM participants WHERE id=?", participant).Scan(&muted) != nil || muted {
				return false
			}
		}
	}
	if role == "display" && (!room.Enabled("display") || room.Settings.DisplayMode == "blank" || room.Settings.DisplaySource == "content") {
		return false
	}
	return true
}
func (s *Server) screenAction(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	var in struct {
		Action   string `json:"action"`
		Client   string `json:"client"`
		ScreenID string `json:"screen_id"`
		Offer    string `json:"offer"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !s.mediaEnabled() {
		apiError(w, 503, "MEDIA_UNAVAILABLE")
		return
	}
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	room, err := s.getRoom(room.ID)
	if err != nil || !room.Active() {
		apiError(w, 409, "ROOM_CLOSED")
		return
	}
	p = s.identity(r, p.Role)
	if p == nil {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	cookie, err := r.Cookie("rd_" + p.Role)
	if err != nil {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	session := digest(cookie.Value)
	v := s.getScreen(room.ID)
	if in.Action == "start" || in.Action == "watch" {
		var count int
		if err = s.db.QueryRow("SELECT COUNT(*) FROM media_clients WHERE room_id=? AND session_hash=? AND expires>?", room.ID, session, time.Now().Unix()).Scan(&count); err != nil {
			s.internal(w, err)
			return
		}
		if count >= 4 {
			apiError(w, 429, "RATE_LIMITED")
			return
		}
	}
	switch in.Action {
	case "start":
		if room.Settings.ScreenMode == "queue" || room.Settings.ScreenMode == "approval" {
			var count int
			_ = s.db.QueryRow("SELECT COUNT(*) FROM screen_requests WHERE id=? AND room_id=? AND owner=? AND state='offered' AND expires_at>?", in.Offer, room.ID, p.ID, time.Now().Unix()).Scan(&count)
			if count != 1 {
				apiError(w, 409, "SCREEN_NOT_YOUR_TURN")
				return
			}
		}
		if p.Role == "display" {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if p.Role == "guest" {
			var muted bool
			if s.db.QueryRow("SELECT muted FROM participants WHERE id=?", p.ID).Scan(&muted) != nil || muted {
				apiError(w, 403, "FORBIDDEN")
				return
			}
		}
		if v != nil {
			apiError(w, 409, "SCREEN_BUSY")
			return
		}
		v = &screenState{ID: "rd-" + randomID(16), Owner: p.ID, Name: p.Name, Expires: time.Now().Unix() + 45}
		if err = s.mediaCall("CreateRoom", v.ID, map[string]interface{}{"name": v.ID, "empty_timeout": 60, "max_participants": room.Settings.MaxMembers + 10}); err != nil {
			apiError(w, 503, "MEDIA_UNAVAILABLE")
			return
		}
		if _, err = s.db.Exec("INSERT INTO screens VALUES(?,?,?,?,?)", room.ID, v.ID, v.Owner, v.Name, v.Expires); err != nil {
			s.internal(w, err)
			return
		}
		_, _ = s.db.Exec("UPDATE screen_requests SET state='presenting' WHERE id=? AND owner=? AND room_id=?", in.Offer, p.ID, room.ID)
	case "watch":
		if v == nil || v.Expires <= time.Now().Unix() {
			apiError(w, 409, "SCREEN_MISSING")
			return
		}
		if p.Role == "display" && (!room.Enabled("display") || room.Settings.DisplayMode == "blank" || room.Settings.DisplaySource == "content") {
			apiError(w, 403, "FORBIDDEN")
			return
		}
	case "heartbeat":
		var own string
		if s.db.QueryRow("SELECT session_hash FROM media_clients WHERE id=? AND room_id=?", in.Client, room.ID).Scan(&own) != nil || own != session || !s.validMediaClient(in.Client) {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if v != nil && v.Owner == p.ID { // Only the actual publishing connection extends the stage lease.
			var publisher string
			_ = s.db.QueryRow("SELECT id FROM media_clients WHERE id=? AND id LIKE 'pub-%'", in.Client).Scan(&publisher)
			if publisher != "" {
				_, err = s.db.Exec("UPDATE screens SET expires=? WHERE room_id=?", time.Now().Unix()+45, room.ID)
			}
		}
		if err == nil {
			_, err = s.db.Exec("UPDATE media_clients SET expires=? WHERE id=?", time.Now().Unix()+45, in.Client)
		}
		if err != nil {
			s.internal(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	case "leave":
		var own, screen string
		if s.db.QueryRow("SELECT session_hash,screen_id FROM media_clients WHERE id=? AND room_id=?", in.Client, room.ID).Scan(&own, &screen) != nil || own != session {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		_, _ = s.db.Exec("UPDATE media_clients SET expires=0 WHERE id=?", in.Client)
		_ = s.mediaCall("RemoveParticipant", screen, map[string]string{"room": screen, "identity": in.Client})
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	case "stop":
		if v == nil {
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if !s.permitted(p, room.ID, "screen") && p.ID != v.Owner {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if in.ScreenID != v.ID {
			apiError(w, 409, "SCREEN_MISSING")
			return
		}
		if err = s.stopScreen(room.ID, v); err != nil {
			apiError(w, 503, "MEDIA_UNAVAILABLE")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	default:
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	prefix := "view-"
	if in.Action == "start" {
		prefix = "pub-"
	}
	id := prefix + randomID(16)
	_, err = s.db.Exec("INSERT INTO media_clients VALUES(?,?,?,?,?,?,?)", id, room.ID, v.ID, session, p.Role, p.ID, time.Now().Unix()+45)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]interface{}{"client": id, "token": s.mediaJWT(id, p.Name, v.ID, false, in.Action == "start"), "screen": v, "url": "/media"})
}

func (s *Server) mediaProxy(w http.ResponseWriter, r *http.Request) {
	// The SFU signaling port is private. All SDK connections, including refreshed tokens,
	// are checked here against current application sessions and the current stage.
	allowedPath := r.URL.Path == "/media/rtc" || r.URL.Path == "/media/rtc/validate" || r.URL.Path == "/media/rtc/v1" || r.URL.Path == "/media/rtc/v1/validate"
	if r.Method != "GET" || !allowedPath || !s.sameOrigin(r) || !s.mediaEnabled() {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	token := r.URL.Query().Get("access_token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	mac := hmac.New(sha256.New, []byte(s.config.MediaSecret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(mac.Sum(nil), signature) {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Sub   string `json:"sub"`
		Iss   string `json:"iss"`
		Exp   int64  `json:"exp"`
		Video struct {
			Room string `json:"room"`
		} `json:"video"`
	}
	if err != nil || json.Unmarshal(raw, &claims) != nil || claims.Iss != s.config.MediaKey || claims.Exp <= time.Now().Unix() || !s.validMediaClient(claims.Sub) {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	var screen string
	_ = s.db.QueryRow("SELECT screen_id FROM media_clients WHERE id=?", claims.Sub).Scan(&screen)
	if claims.Video.Room != screen {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	target, err := url.Parse(s.config.MediaURL)
	if err != nil {
		apiError(w, 503, "MEDIA_UNAVAILABLE")
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	original := proxy.Director
	proxy.Director = func(req *http.Request) {
		original(req)
		req.URL.Path = strings.TrimPrefix(r.URL.Path, "/media")
		req.Header.Del("Cookie")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) { apiError(w, 502, "MEDIA_UNAVAILABLE") }
	proxy.ServeHTTP(w, r)
}
func (s *Server) mediaMaintenance() {
	defer s.wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
		if !s.mediaEnabled() {
			continue
		}
		s.reconcileMedia()
	}
}
func (s *Server) reconcileMedia() {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	rows, err := s.db.Query("SELECT room_id FROM screens")
	if err != nil {
		return
	}
	rooms := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			rooms = append(rooms, id)
		}
	}
	rows.Close()
	for _, id := range rooms {
		v := s.getScreen(id)
		if v == nil {
			continue
		}
		room, err := s.getRoom(id)
		valid := err == nil && room.Active() && v.Expires > time.Now().Unix()
		var publisher string
		_ = s.db.QueryRow("SELECT id FROM media_clients WHERE screen_id=? AND id LIKE 'pub-%'", v.ID).Scan(&publisher)
		valid = valid && publisher != "" && s.validMediaClient(publisher)
		if !valid {
			if s.stopScreen(id, v) != nil {
				return
			}
			continue
		}
	}
	rows, err = s.db.Query("SELECT id,screen_id FROM media_clients")
	if err != nil {
		return
	}
	type client struct{ id, screen string }
	clients := []client{}
	for rows.Next() {
		var c client
		if rows.Scan(&c.id, &c.screen) == nil {
			clients = append(clients, c)
		}
	}
	rows.Close()
	for _, c := range clients {
		if !s.validMediaClient(c.id) {
			if s.mediaCall("RemoveParticipant", c.screen, map[string]string{"room": c.screen, "identity": c.id}) == nil {
				_, _ = s.db.Exec("DELETE FROM media_clients WHERE id=?", c.id)
			} else {
				return
			}
		}
	}
}
