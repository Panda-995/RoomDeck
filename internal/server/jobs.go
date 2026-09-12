package server

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Job struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	CreatedAt int64  `json:"created_at"`
	Error     string `json:"error"`
	Bytes     int64  `json:"bytes"`
}

func (s *Server) exportJobs(room string) ([]Job, error) {
	rows, e := s.db.Query("SELECT id,state,created_at,error,bytes FROM jobs WHERE room_id=? ORDER BY created_at DESC", room)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	list := []Job{}
	for rows.Next() {
		var j Job
		if e = rows.Scan(&j.ID, &j.State, &j.CreatedAt, &j.Error, &j.Bytes); e != nil {
			return nil, e
		}
		list = append(list, j)
	}
	return list, rows.Err()
}
func (s *Server) createExport(w http.ResponseWriter, r *http.Request) {
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
	jobs, e := s.exportJobs(room.ID)
	if e != nil {
		s.internal(w, e)
		return
	}
	for _, j := range jobs {
		if j.State == "queued" || j.State == "running" {
			writeJSON(w, 200, j)
			return
		}
	}
	// Keep the last successful archive while a replacement is built, but bound
	// repeated exports so they cannot accumulate until the retention sweep.
	keptReady := false
	for _, j := range jobs {
		if j.State == "ready" && !keptReady {
			keptReady = true
			continue
		}
		p := filepath.Join(s.config.DataDir, "rooms", room.ID, "export-"+j.ID+".zip")
		if e := os.Remove(p); e == nil || os.IsNotExist(e) {
			_, _ = s.db.Exec("DELETE FROM jobs WHERE id=?", j.ID)
		}
	}
	id := randomID(16)
	_, e = s.db.Exec("INSERT INTO jobs(id,room_id,state,created_at) VALUES(?,?,'queued',?)", id, room.ID, time.Now().Unix())
	if e != nil {
		s.internal(w, e)
		return
	}
	s.audit(room.ID, "export.created", p.ID)
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.runExport(room.ID, id) }()
	writeJSON(w, 202, Job{ID: id, State: "queued", CreatedAt: time.Now().Unix()})
}

var exportSlots = make(chan struct{}, 1)

// Snapshot file references under a short mutation lock, then compress outside it.
// Hard links keep deleted originals alive for this job; filesystems without hard
// links fall back to a private copy, preserving the same export capability.
func (s *Server) exportSnapshot(roomID, id string) (*Room, []*Content, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.config.DataDir, "tmp", "export-"+id)
	room, e := s.getRoom(roomID)
	if e != nil {
		return nil, nil, dir, e
	}
	if room.Expired() {
		return nil, nil, dir, fmt.Errorf("ROOM_DELETED")
	}
	rows, e := s.db.Query(contentSelect+" WHERE room_id=? ORDER BY created_at", roomID)
	if e != nil {
		return nil, nil, dir, e
	}
	list := []*Content{}
	var sourceBytes int64
	for rows.Next() {
		c, e := scanContent(rows)
		if e != nil {
			rows.Close()
			return nil, nil, dir, e
		}
		list = append(list, c)
		sourceBytes += c.Bytes
	}
	rows.Close()
	free, e := freeBytes(s.config.DataDir)
	if e != nil || free < uint64(sourceBytes*2)+(256<<20) {
		return nil, nil, dir, fmt.Errorf("STORAGE_FULL")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return nil, nil, dir, e
	}
	for _, c := range list {
		if c.Kind == "poll" {
			s.pollResults(c, &principal{Role: "host"}, room)
		}
		if c.Kind != "photo" && c.Kind != "file" {
			continue
		}
		src := s.assetPath(roomID, c.ID, "original")
		dest := filepath.Join(dir, c.ID)
		if e = os.Link(src, dest); e != nil {
			input, e := os.Open(src)
			if e != nil {
				return nil, nil, dir, e
			}
			output, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				input.Close()
				return nil, nil, dir, e
			}
			_, e = io.Copy(output, input)
			input.Close()
			ce := output.Close()
			if e != nil {
				return nil, nil, dir, e
			}
			if ce != nil {
				return nil, nil, dir, ce
			}
		}
	}
	_, e = s.db.Exec("UPDATE jobs SET state='running' WHERE id=?", id)
	return room, list, dir, e
}
func (s *Server) runExport(roomID, id string) {
	select {
	case exportSlots <- struct{}{}:
		defer func() { <-exportSlots }()
	case <-s.ctx.Done():
		return
	}
	room, list, snapshotDir, e := s.exportSnapshot(roomID, id)
	defer os.RemoveAll(snapshotDir)
	dest := filepath.Join(s.config.DataDir, "rooms", roomID, "export-"+id+".zip")
	defer os.Remove(dest + ".part")
	fail := func(code string, err error) {
		_, _ = s.db.Exec("UPDATE jobs SET state='failed',error=? WHERE id=?", code, id)
		log.Printf("export %s: %v", id, err)
		s.hub.notify(roomID)
	}
	if e != nil {
		code := "ASSET_MISSING"
		if e.Error() == "STORAGE_FULL" || e.Error() == "ROOM_DELETED" {
			code = e.Error()
		}
		fail(code, e)
		return
	}
	s.hub.notify(roomID)
	if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
		fail("STORAGE_FULL", e)
		return
	}
	f, e := os.OpenFile(dest+".part", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		fail("STORAGE_FULL", e)
		return
	}
	z := zip.NewWriter(f)
	ok := false
	defer func() {
		if !ok {
			_ = z.Close()
			_ = f.Close()
		}
	}()
	type entry struct {
		Path   string `json:"path"`
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	manifest := []entry{}
	add := func(name string, reader io.Reader) error {
		out, e := z.Create(name)
		if e != nil {
			return e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(out, h), reader)
		if e == nil {
			manifest = append(manifest, entry{name, n, hex.EncodeToString(h.Sum(nil))})
		}
		return e
	}
	notes := "<!doctype html><html><meta charset=utf-8><title>RoomDeck Notes</title><body>"
	links := "<!doctype html><html><meta charset=utf-8><title>RoomDeck Links</title><body>"
	polls := []*Content{}
	for _, c := range list {
		if s.ctx.Err() != nil {
			fail("SERVER_RESTARTED", s.ctx.Err())
			return
		}
		if room.DeleteAt > 0 && time.Now().Unix() >= room.DeleteAt {
			fail("ROOM_DELETED", nil)
			return
		}
		switch c.Kind {
		case "photo", "file":
			src, e := os.Open(filepath.Join(snapshotDir, c.ID))
			if e != nil {
				fail("ASSET_MISSING", e)
				return
			}
			folder := "files/"
			if c.Kind == "photo" {
				folder = "photos/"
			}
			safe := strings.Map(func(r rune) rune {
				if r < ' ' || strings.ContainsRune(`<>:"/\|?*`, r) {
					return '_'
				}
				return r
			}, c.Filename)
			e = add(folder+c.ID[:8]+"-"+safe, src)
			src.Close()
			if e != nil {
				fail("STORAGE_FULL", e)
				return
			}
		case "note":
			notes += "<article><h2>" + html.EscapeString(c.Title) + "</h2><pre>" + html.EscapeString(c.Body) + "</pre></article>"
		case "link":
			links += "<p><a rel=\"noreferrer noopener\" href=\"" + html.EscapeString(c.Body) + "\">" + html.EscapeString(c.Title+" "+c.Body) + "</a></p>"
		case "poll":
			polls = append(polls, c)
		}
	}
	files := map[string]interface{}{"room.json": room, "poll-results.json": polls}
	for name, value := range files {
		b, _ := json.MarshalIndent(value, "", "  ")
		if e = add(name, strings.NewReader(string(b))); e != nil {
			fail("STORAGE_FULL", e)
			return
		}
	}
	if e = add("notes.html", strings.NewReader(notes+"</body></html>")); e == nil {
		e = add("links.html", strings.NewReader(links+"</body></html>"))
	}
	if e != nil {
		fail("STORAGE_FULL", e)
		return
	}
	b, _ := json.MarshalIndent(map[string]interface{}{"schema_version": 1, "files": manifest}, "", "  ")
	if e = add("manifest.json", strings.NewReader(string(b))); e != nil {
		fail("STORAGE_FULL", e)
		return
	}
	if e = z.Close(); e == nil {
		e = f.Close()
	}
	if e != nil {
		fail("STORAGE_FULL", e)
		return
	}
	ok = true
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh, roomErr := s.getRoom(roomID)
	if roomErr != nil || fresh.Expired() {
		fail("ROOM_DELETED", roomErr)
		return
	}
	if e = os.Rename(dest+".part", dest); e != nil {
		fail("STORAGE_FULL", e)
		return
	}
	st, e := os.Stat(dest)
	if e != nil {
		fail("STORAGE_FULL", e)
		return
	}
	_, e = s.db.Exec("UPDATE jobs SET state='ready',bytes=? WHERE id=?", st.Size(), id)
	if e != nil {
		fail("INTERNAL_ERROR", e)
		return
	}
	s.hub.notify(roomID)
}
func (s *Server) downloadExport(w http.ResponseWriter, r *http.Request) {
	if s.host(w, r) == nil {
		return
	}
	_, room := s.roomAccess(w, r)
	if room == nil {
		return
	}
	var state string
	e := s.db.QueryRow("SELECT state FROM jobs WHERE id=? AND room_id=?", r.PathValue("job"), room.ID).Scan(&state)
	if e != nil || state != "ready" {
		apiError(w, 404, "EXPORT_NOT_READY")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="RoomDeck-`+room.Code+`.zip"`)
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, filepath.Join(s.config.DataDir, "rooms", room.ID, "export-"+r.PathValue("job")+".zip"))
}
func (s *Server) maintenance() {
	defer s.wg.Done()
	s.cleanup()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}
func (s *Server) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	_, e := s.db.Exec("UPDATE rooms SET state='closed',closed_at=ends_at,delete_at=ends_at+retention,version=version+1 WHERE state='active' AND ends_at<=?", now)
	if e != nil {
		log.Printf("cleanup: %v", e)
		return
	}
	rows, e := s.db.Query("SELECT id FROM rooms WHERE state='deleting' OR (delete_at>0 AND delete_at<=?)", now)
	if e != nil {
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		_, _ = s.db.Exec("UPDATE rooms SET state='deleting' WHERE id=?", id)
		_, _ = s.db.Exec("DELETE FROM sessions WHERE room_id=?", id)
		s.hub.notify(id)
		if e = os.RemoveAll(filepath.Join(s.config.DataDir, "rooms", id)); e != nil {
			log.Printf("cleanup room %s: %v", id, e)
			continue
		}
		_, _ = s.db.Exec("DELETE FROM rooms WHERE id=?", id)
	}
	rows, e = s.db.Query("SELECT id,room_id FROM jobs WHERE created_at<? AND state IN ('ready','failed')", now-6*3600)
	if e == nil {
		type oldJob struct{ id, room string }
		old := []oldJob{}
		for rows.Next() {
			var j oldJob
			_ = rows.Scan(&j.id, &j.room)
			old = append(old, j)
		}
		rows.Close()
		for _, j := range old {
			if e = os.Remove(filepath.Join(s.config.DataDir, "rooms", j.room, "export-"+j.id+".zip")); e == nil || os.IsNotExist(e) {
				_, _ = s.db.Exec("DELETE FROM jobs WHERE id=?", j.id)
			}
		}
	}
	_, _ = s.db.Exec("DELETE FROM sessions WHERE expires_at<?", now)
	_, _ = s.db.Exec("DELETE FROM audit WHERE created_at<?", now-30*86400)
}
