package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Poll struct {
	Options     []string `json:"options"`
	Multiple    bool     `json:"multiple"`
	MaxChoices  int      `json:"max_choices"`
	HideResults bool     `json:"hide_results"`
	Closed      bool     `json:"closed"`
	ClosesAt    int64    `json:"closes_at"`
}
type Content struct {
	ID         string `json:"id"`
	RoomID     string `json:"room_id"`
	AuthorID   string `json:"author_id"`
	AuthorName string `json:"author_name"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Filename   string `json:"filename"`
	Mime       string `json:"mime"`
	Bytes      int64  `json:"bytes"`
	DiskBytes  int64  `json:"-"`
	Visible    bool   `json:"visible"`
	Selected   bool   `json:"selected"`
	CreatedAt  int64  `json:"created_at"`
	Poll       *Poll  `json:"poll,omitempty"`
	Counts     []int  `json:"counts,omitempty"`
	Voters     *int   `json:"voters,omitempty"`
	MyVote     []int  `json:"my_vote,omitempty"`
}

const contentSelect = "SELECT id,room_id,author_id,author_name,kind,title,body,filename,mime,bytes,disk_bytes,visible,selected,created_at,payload FROM contents"

func scanContent(row scanner) (*Content, error) {
	c := &Content{}
	var payload string
	e := row.Scan(&c.ID, &c.RoomID, &c.AuthorID, &c.AuthorName, &c.Kind, &c.Title, &c.Body, &c.Filename, &c.Mime, &c.Bytes, &c.DiskBytes, &c.Visible, &c.Selected, &c.CreatedAt, &payload)
	if e != nil {
		return nil, e
	}
	if c.Kind == "poll" {
		c.Poll = &Poll{}
		if e = json.Unmarshal([]byte(payload), c.Poll); e != nil {
			return nil, e
		}
	}
	return c, nil
}
func (s *Server) getContent(room, id string) (*Content, error) {
	return scanContent(s.db.QueryRow(contentSelect+" WHERE room_id=? AND id=?", room, id))
}
func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role == "display" && !room.Enabled("display") {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	if p.Role == "guest" {
		_, _ = s.db.Exec("UPDATE participants SET last_seen=? WHERE id=?", time.Now().Unix(), p.ID)
	}
	rows, e := s.db.Query(contentSelect+" WHERE room_id=? ORDER BY created_at DESC,id DESC LIMIT 2000", room.ID)
	if e != nil {
		s.internal(w, e)
		return
	}
	all := []*Content{}
	for rows.Next() {
		c, e := scanContent(rows)
		if e != nil {
			rows.Close()
			s.internal(w, e)
			return
		}
		all = append(all, c)
	}
	rows.Close()
	moderator := s.permitted(p, room.ID, "content")
	contents := []*Content{}
	for _, c := range all {
		if !moderator && (!c.Visible || !room.Enabled(c.Kind)) {
			continue
		}
		if p.Role == "display" {
			if room.State != "active" || room.Settings.DisplayMode == "blank" {
				continue
			}
			if !(c.Selected || (c.Kind == "photo" && room.Settings.AutoDisplay)) {
				continue
			}
			if c.Kind == "file" {
				continue
			}
		}
		if c.Kind == "poll" {
			s.pollResults(c, p, room)
		}
		if p.Role == "display" {
			c.AuthorID = ""
		}
		contents = append(contents, c)
	}
	type member struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Revoked     bool     `json:"revoked"`
		Muted       bool     `json:"muted"`
		Online      bool     `json:"online"`
		Permissions []string `json:"permissions"`
	}
	members := []member{}
	online := 0
	total := 0
	rows, e = s.db.Query("SELECT id,name,revoked,muted,last_seen,permissions FROM participants WHERE room_id=? ORDER BY joined_at", room.ID)
	if e != nil {
		s.internal(w, e)
		return
	}
	for rows.Next() {
		var m member
		var last int64
		var permissions string
		if e = rows.Scan(&m.ID, &m.Name, &m.Revoked, &m.Muted, &last, &permissions); e != nil {
			rows.Close()
			s.internal(w, e)
			return
		}
		_ = json.Unmarshal([]byte(permissions), &m.Permissions)
		m.Online = !m.Revoked && last > time.Now().Unix()-60
		if !m.Revoked {
			total++
		}
		if m.Online {
			online++
		}
		if p.Role == "host" {
			members = append(members, m)
		}
	}
	rows.Close()
	result := map[string]interface{}{"room": room, "contents": contents, "role": p.Role, "participant_id": p.ID, "name": p.Name, "online": online, "participants_total": total, "members": members, "display_online": s.hub.displayCount(room.ID)}
	result["permissions"] = s.permissions(p, room.ID)
	if p.Role == "host" {
		result["invite"] = room.Invite
		jobs, e := s.exportJobs(room.ID)
		if e != nil {
			s.internal(w, e)
			return
		}
		result["jobs"] = jobs
	}
	writeJSON(w, 200, result)
}
func (s *Server) createContent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
		Body  string `json:"body"`
		Poll  *Poll  `json:"poll"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil || !s.canWrite(w, p, room) {
		return
	}
	if !room.Enabled(in.Kind) {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	if s.limited(r, "publish", 30) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	if !validText(in.Title, 0, 120) || !validText(in.Body, 0, 2048) {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	switch in.Kind {
	case "note":
		if !validText(in.Body, 1, 2000) {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
	case "link":
		u, e := url.Parse(strings.TrimSpace(in.Body))
		if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			apiError(w, 400, "INVALID_URL")
			return
		}
		in.Body = u.String()
	case "poll":
		if p.Role != "host" {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		if !validText(in.Title, 1, 120) || !validPoll(in.Poll) {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
	default:
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM contents WHERE room_id=?", room.ID).Scan(&count)
	if count >= 2000 {
		apiError(w, 409, "CONTENT_LIMIT")
		return
	}
	payload := "{}"
	if in.Poll != nil {
		b, _ := json.Marshal(in.Poll)
		payload = string(b)
	}
	id := randomID(16)
	_, e := s.db.Exec("INSERT INTO contents(id,room_id,author_id,author_name,kind,title,body,created_at,payload) VALUES(?,?,?,?,?,?,?,?,?)", id, room.ID, p.ID, p.Name, in.Kind, strings.TrimSpace(in.Title), strings.TrimSpace(in.Body), time.Now().Unix(), payload)
	if e != nil {
		s.internal(w, e)
		return
	}
	s.hub.notify(room.ID)
	writeJSON(w, 201, map[string]string{"id": id})
}
func validPoll(p *Poll) bool {
	if p == nil || len(p.Options) < 2 || len(p.Options) > 8 || p.MaxChoices < 1 || p.MaxChoices > len(p.Options) || (!p.Multiple && p.MaxChoices != 1) || p.ClosesAt <= time.Now().Unix() {
		return false
	}
	for _, v := range p.Options {
		if !validText(v, 1, 60) {
			return false
		}
	}
	return !p.Closed
}
func (s *Server) updateContent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Visible   *bool `json:"visible"`
		Selected  *bool `json:"selected"`
		ClosePoll bool  `json:"close_poll"`
	}
	if !readJSON(w, r, &in) {
		return
	}
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
	c, e := s.getContent(room.ID, r.PathValue("content"))
	if e != nil {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if in.Visible != nil {
		c.Visible = *in.Visible
	}
	if in.Selected != nil {
		c.Selected = *in.Selected
	}
	if !c.Visible {
		c.Selected = false
	}
	payload := "{}"
	if c.Poll != nil {
		if in.ClosePoll {
			c.Poll.Closed = true
		}
		b, _ := json.Marshal(c.Poll)
		payload = string(b)
	}
	_, e = s.db.Exec("UPDATE contents SET visible=?,selected=?,payload=? WHERE id=?", c.Visible, c.Selected, payload, c.ID)
	if e != nil {
		s.internal(w, e)
		return
	}
	s.audit(room.ID, "content.updated", p.ID)
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) deleteContent(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	c, e := s.getContent(room.ID, r.PathValue("content"))
	if e != nil {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if !s.permitted(p, room.ID, "content") && (p.Role != "guest" || p.ID != c.AuthorID || !room.Active()) {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	tx, e := s.db.Begin()
	if e != nil {
		s.internal(w, e)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("DELETE FROM contents WHERE id=?", c.ID); e == nil {
		_, e = tx.Exec("UPDATE rooms SET used=MAX(0,used-?) WHERE id=?", c.DiskBytes, room.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.internal(w, e)
		return
	}
	for _, v := range []string{"original", "preview", "thumb"} {
		_ = os.Remove(s.assetPath(room.ID, c.ID, v))
	}
	s.audit(room.ID, "content.deleted", p.ID)
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) pollResults(c *Content, p *principal, room *Room) {
	if c.Poll == nil {
		return
	}
	if !room.Active() || c.Poll.ClosesAt <= time.Now().Unix() {
		c.Poll.Closed = true
	}
	var own string
	_ = s.db.QueryRow("SELECT choices FROM ballots WHERE poll_id=? AND participant_id=?", c.ID, p.ID).Scan(&own)
	if own != "" {
		_ = json.Unmarshal([]byte(own), &c.MyVote)
	}
	if c.Poll.HideResults && !c.Poll.Closed {
		return
	}
	counts := make([]int, len(c.Poll.Options))
	voters := 0
	rows, e := s.db.Query("SELECT choices FROM ballots WHERE poll_id=?", c.ID)
	if e != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var str string
		_ = rows.Scan(&str)
		var choices []int
		if json.Unmarshal([]byte(str), &choices) != nil {
			continue
		}
		voters++
		for _, i := range choices {
			if i >= 0 && i < len(counts) {
				counts[i]++
			}
		}
	}
	c.Counts = counts
	c.Voters = &voters
}
func (s *Server) vote(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Choices []int `json:"choices"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil || !s.canWrite(w, p, room) {
		return
	}
	if !room.Enabled("poll") {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	c, e := s.getContent(room.ID, r.PathValue("content"))
	if e != nil || c.Poll == nil || !c.Visible {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if c.Poll.Closed || c.Poll.ClosesAt <= time.Now().Unix() {
		apiError(w, 409, "POLL_CLOSED")
		return
	}
	if len(in.Choices) < 1 || len(in.Choices) > c.Poll.MaxChoices {
		apiError(w, 400, "INVALID_CHOICES")
		return
	}
	seen := map[int]bool{}
	for _, i := range in.Choices {
		if i < 0 || i >= len(c.Poll.Options) || seen[i] {
			apiError(w, 400, "INVALID_CHOICES")
			return
		}
		seen[i] = true
	}
	b, _ := json.Marshal(in.Choices)
	_, e = s.db.Exec("INSERT INTO ballots(poll_id,participant_id,choices) VALUES(?,?,?) ON CONFLICT(poll_id,participant_id) DO UPDATE SET choices=excluded.choices", c.ID, p.ID, string(b))
	if e != nil {
		s.internal(w, e)
		return
	}
	s.hub.notify(room.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) assetPath(room, id, variant string) string {
	return filepath.Join(s.config.DataDir, "rooms", room, id+"."+variant)
}
