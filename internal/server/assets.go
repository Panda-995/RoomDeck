package server

import (
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

var imageSlots = make(chan struct{}, 1)

func (s *Server) uploadAsset(w http.ResponseWriter, r *http.Request) {
	resumeID, _ := r.Context().Value(uploadContextKey{}).(string)
	p, room := s.roomAccess(w, r)
	if p == nil || !s.canWrite(w, p, room) {
		return
	}
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	size, e := strconv.ParseInt(r.URL.Query().Get("size"), 10, 64)
	if e != nil || size <= 0 || size > 200<<20 || !validText(name, 1, 240) || (kind != "photo" && kind != "file") {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	if !room.Enabled(kind) {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	if kind == "photo" && size > 20<<20 {
		apiError(w, 413, "FILE_TOO_LARGE")
		return
	}
	if resumeID == "" && s.limited(r, "upload", 30) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if strings.ContainsAny(name, "\r\n") {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	reservation := size
	if kind == "photo" {
		reservation += 16 << 20
	}
	if free, err := freeBytes(s.config.DataDir); err != nil || free < uint64(reservation)+(256<<20) {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	s.mu.Lock()
	fresh, e := s.getRoom(room.ID)
	if e != nil || !fresh.Active() {
		s.mu.Unlock()
		apiError(w, 409, "ROOM_CLOSED")
		return
	}
	var count int
	_ = s.db.QueryRow("SELECT (SELECT COUNT(*) FROM contents WHERE room_id=?)+(SELECT COUNT(*) FROM uploads WHERE room_id=? AND state='uploading' AND id<>?)", room.ID, room.ID, resumeID).Scan(&count)
	if count >= 2000 {
		s.mu.Unlock()
		apiError(w, 409, "CONTENT_LIMIT")
		return
	}
	extra := reservation
	if resumeID != "" {
		extra = 0
	}
	if fresh.Used+fresh.Reserved+extra > fresh.Settings.Quota {
		s.mu.Unlock()
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	if resumeID == "" {
		_, e = s.db.Exec("UPDATE rooms SET reserved=reserved+? WHERE id=?", reservation, room.ID)
	}
	s.mu.Unlock()
	if e != nil {
		s.internal(w, e)
		return
	}
	defer func() {
		if resumeID != "" {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		_, _ = s.db.Exec("UPDATE rooms SET reserved=MAX(0,reserved-?) WHERE id=?", reservation, room.ID)
	}()
	id := randomID(16)
	if resumeID != "" {
		id = resumeID
	}
	temp := filepath.Join(s.config.DataDir, "tmp", id)
	f, e := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	defer os.Remove(temp)
	n, copyErr := io.Copy(f, io.LimitReader(r.Body, size+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n != size {
		apiError(w, 400, "UPLOAD_INCOMPLETE")
		return
	}
	header := make([]byte, 512)
	read, e := os.Open(temp)
	if e != nil {
		s.internal(w, e)
		return
	}
	nhead, _ := read.Read(header)
	read.Close()
	detected := http.DetectContentType(header[:nhead])
	if kind == "photo" && detected != "image/jpeg" && detected != "image/png" && detected != "image/webp" {
		apiError(w, 415, "UNSUPPORTED_IMAGE")
		return
	}
	roomDir := filepath.Join(s.config.DataDir, "rooms", room.ID)
	if e = os.MkdirAll(roomDir, 0700); e != nil {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	committed := false
	defer func() {
		if !committed {
			for _, v := range []string{"original", "preview", "thumb"} {
				_ = os.Remove(s.assetPath(room.ID, id, v))
			}
		}
	}()
	total := size
	if kind == "photo" {
		select {
		case imageSlots <- struct{}{}:
			defer func() { <-imageSlots }()
		case <-r.Context().Done():
			return
		}
		f, e = os.Open(temp)
		if e != nil {
			s.internal(w, e)
			return
		}
		cfg, _, decodeErr := image.DecodeConfig(f)
		f.Close()
		if decodeErr != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 60_000_000 {
			apiError(w, 415, "IMAGE_TOO_LARGE")
			return
		}
		img, e := imaging.Open(temp, imaging.AutoOrientation(true))
		if e != nil {
			apiError(w, 415, "UNSUPPORTED_IMAGE")
			return
		}
		for _, v := range []struct {
			name string
			size int
		}{{"preview", 1920}, {"thumb", 360}} {
			output := imaging.Fit(img, v.size, v.size, imaging.Lanczos)
			dest, e := os.OpenFile(s.assetPath(room.ID, id, v.name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if e != nil {
				apiError(w, 507, "STORAGE_FULL")
				return
			}
			e = jpeg.Encode(dest, output, &jpeg.Options{Quality: 85})
			ce := dest.Close()
			if e != nil || ce != nil {
				apiError(w, 507, "STORAGE_FULL")
				return
			}
			st, e := os.Stat(s.assetPath(room.ID, id, v.name))
			if e != nil {
				s.internal(w, e)
				return
			}
			total += st.Size()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, room = s.roomAccess(w, r)
	if p == nil || !s.canWrite(w, p, room) {
		return
	}
	if !room.Enabled(kind) {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	if room.Used+room.Reserved-reservation+total > room.Settings.Quota {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	if e = os.Rename(temp, s.assetPath(room.ID, id, "original")); e != nil {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	tx, e := s.db.Begin()
	if e != nil {
		s.internal(w, e)
		return
	}
	defer tx.Rollback()
	_, e = tx.Exec("INSERT INTO contents(id,room_id,author_id,author_name,kind,filename,mime,bytes,disk_bytes,created_at) VALUES(?,?,?,?,?,?,?,?,?,unixepoch())", id, room.ID, p.ID, p.Name, kind, name, detected, size, total)
	if e == nil {
		_, e = tx.Exec("UPDATE rooms SET used=used+? WHERE id=?", total, room.ID)
	}
	if e == nil && resumeID != "" {
		_, e = tx.Exec("UPDATE uploads SET state='ready' WHERE id=?", resumeID)
		if e == nil {
			_, e = tx.Exec("UPDATE rooms SET reserved=MAX(0,reserved-?) WHERE id=?", reservation, room.ID)
		}
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.internal(w, e)
		return
	}
	committed = true
	s.hub.notify(room.ID)
	writeJSON(w, 201, map[string]string{"id": id})
}
func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	c, e := s.getContent(room.ID, r.PathValue("content"))
	if e != nil || (c.Kind != "file" && c.Kind != "photo") {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if !s.permitted(p, room.ID, "content") && (!c.Visible || !room.Enabled(c.Kind)) {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	v := r.PathValue("variant")
	if v != "original" && v != "preview" && v != "thumb" {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if c.Kind == "file" && v != "original" {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	if p.Role == "display" {
		if !room.Enabled("display") || !room.Active() || room.Settings.DisplayMode == "blank" || c.Kind != "photo" || v == "original" || !(c.Selected || room.Settings.AutoDisplay) {
			apiError(w, 403, "FORBIDDEN")
			return
		}
	}
	if c.Kind == "photo" && v == "original" && p.Role != "host" && !room.Settings.DownloadOriginal {
		apiError(w, 403, "ORIGINAL_DISABLED")
		return
	}
	f, e := os.Open(s.assetPath(room.ID, c.ID, v))
	if e != nil {
		apiError(w, 404, "NOT_FOUND")
		return
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		s.internal(w, e)
		return
	}
	if v == "original" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": c.Filename}))
	} else {
		w.Header().Set("Content-Type", "image/jpeg")
		if r.URL.Query().Get("download") == "1" {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fmt.Sprintf("%s.jpg", strings.TrimSuffix(c.Filename, filepath.Ext(c.Filename)))}))
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, c.Filename, st.ModTime(), f)
}
