package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const uploadChunkSize int64 = 4 << 20

type uploadContextKey struct{}
type durableUpload struct {
	ID          string   `json:"id"`
	Room        string   `json:"-"`
	Owner       string   `json:"-"`
	RequestID   string   `json:"request_id"`
	Filename    string   `json:"filename"`
	Kind        string   `json:"kind"`
	Bytes       int64    `json:"bytes"`
	Reservation int64    `json:"-"`
	Hashes      []string `json:"hashes"`
	State       string   `json:"state"`
	Created     int64    `json:"-"`
	Expires     int64    `json:"expires_at"`
	Parts       []int    `json:"parts"`
	ChunkSize   int64    `json:"chunk_size"`
}

const uploadSelect = "SELECT id,room_id,owner,request_id,filename,kind,bytes,reservation,hashes,state,created_at,expires_at FROM uploads"

func scanUpload(row scanner) (*durableUpload, error) {
	u := &durableUpload{ChunkSize: uploadChunkSize, Parts: []int{}}
	var hashes string
	err := row.Scan(&u.ID, &u.Room, &u.Owner, &u.RequestID, &u.Filename, &u.Kind, &u.Bytes, &u.Reservation, &hashes, &u.State, &u.Created, &u.Expires)
	if err == nil {
		err = json.Unmarshal([]byte(hashes), &u.Hashes)
	}
	return u, err
}
func (s *Server) uploadDir(u *durableUpload) string {
	return filepath.Join(s.config.DataDir, "uploads", u.Room, u.ID)
}
func (s *Server) uploadParts(u *durableUpload) {
	for i := range u.Hashes {
		f, err := os.Open(filepath.Join(s.uploadDir(u), strconv.Itoa(i)))
		if err != nil {
			continue
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err == nil && n == u.partSize(i) && hex.EncodeToString(h.Sum(nil)) == u.Hashes[i] {
			u.Parts = append(u.Parts, i)
		}
	}
}
func (u *durableUpload) partSize(i int) int64 {
	if i == len(u.Hashes)-1 {
		return u.Bytes - int64(i)*uploadChunkSize
	}
	return uploadChunkSize
}
func (s *Server) uploadAccess(w http.ResponseWriter, r *http.Request, write bool) (*durableUpload, *Room) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return nil, nil
	}
	if p.Role == "display" {
		apiError(w, 403, "FORBIDDEN")
		return nil, nil
	}
	u, err := scanUpload(s.db.QueryRow(uploadSelect+" WHERE id=? AND room_id=? AND owner=?", r.PathValue("upload"), room.ID, p.Role+":"+p.ID))
	if err != nil {
		apiError(w, 404, "NOT_FOUND")
		return nil, nil
	}
	if write && (!s.canWrite(w, p, room)) {
		return nil, nil
	}
	if write && !room.Enabled(u.Kind) {
		apiError(w, 403, "MODULE_DISABLED")
		return nil, nil
	}
	if u.State != "ready" && u.Expires <= time.Now().Unix() {
		apiError(w, 410, "UPLOAD_EXPIRED")
		return nil, nil
	}
	return u, room
}
func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	p, room := s.roomAccess(w, r)
	if p == nil || !s.canWrite(w, p, room) {
		return
	}
	var in struct {
		RequestID string   `json:"request_id"`
		Filename  string   `json:"filename"`
		Kind      string   `json:"kind"`
		Bytes     int64    `json:"bytes"`
		Hashes    []string `json:"hashes"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !validText(in.RequestID, 1, 80) || !validText(in.Filename, 1, 240) || in.Bytes <= 0 || in.Bytes > 200<<20 || (in.Kind != "photo" && in.Kind != "file") || (in.Kind == "photo" && in.Bytes > 20<<20) || len(in.Hashes) != int((in.Bytes+uploadChunkSize-1)/uploadChunkSize) {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	if !room.Enabled(in.Kind) {
		apiError(w, 403, "MODULE_DISABLED")
		return
	}
	for _, h := range in.Hashes {
		b, e := hex.DecodeString(h)
		if e != nil || len(b) != 32 {
			apiError(w, 400, "INVALID_INPUT")
			return
		}
	}
	in.Filename = filepath.Base(strings.ReplaceAll(in.Filename, "\\", "/"))
	if strings.ContainsAny(in.Filename, "\r\n") {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	hashes, _ := json.Marshal(in.Hashes)
	owner := p.Role + ":" + p.ID
	if old, err := scanUpload(s.db.QueryRow(uploadSelect+" WHERE room_id=? AND owner=? AND request_id=?", room.ID, owner, in.RequestID)); err == nil {
		oldHashes, _ := json.Marshal(old.Hashes)
		if old.Filename != in.Filename || old.Bytes != in.Bytes || old.Kind != in.Kind || string(oldHashes) != string(hashes) {
			apiError(w, 409, "UPLOAD_CONFLICT")
			return
		}
		if old.State != "ready" && old.Expires <= time.Now().Unix() {
			apiError(w, 410, "UPLOAD_EXPIRED")
			return
		}
		s.uploadParts(old)
		writeJSON(w, 200, old)
		return
	}
	if s.limited(r, "upload-init", 30) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.getRoom(room.ID)
	if err != nil || !room.Active() {
		apiError(w, 409, "ROOM_CLOSED")
		return
	}
	var own, all, content int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM uploads WHERE owner=? AND state='uploading'", owner).Scan(&own)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM uploads WHERE state='uploading'").Scan(&all)
	_ = s.db.QueryRow("SELECT (SELECT COUNT(*) FROM contents WHERE room_id=?)+(SELECT COUNT(*) FROM uploads WHERE room_id=? AND state='uploading')", room.ID, room.ID).Scan(&content)
	if own >= 3 || all >= 50 {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	if content >= 2000 {
		apiError(w, 409, "CONTENT_LIMIT")
		return
	}
	reserve := in.Bytes
	if in.Kind == "photo" {
		reserve += 16 << 20
	}
	if room.Used+room.Reserved+reserve > room.Settings.Quota {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	if free, e := freeBytes(s.config.DataDir); e != nil || free < uint64(reserve+in.Bytes)+(256<<20) {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	now := time.Now().Unix()
	expiry := min(now+86400, room.EndsAt)
	u := &durableUpload{ID: randomID(16), Room: room.ID, Owner: owner, RequestID: in.RequestID, Filename: in.Filename, Kind: in.Kind, Bytes: in.Bytes, Reservation: reserve, Hashes: in.Hashes, State: "uploading", Created: now, Expires: expiry, Parts: []int{}, ChunkSize: uploadChunkSize}
	tx, err := s.db.Begin()
	if err != nil {
		s.internal(w, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec("INSERT INTO uploads(id,room_id,owner,request_id,filename,kind,bytes,reservation,hashes,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", u.ID, u.Room, u.Owner, u.RequestID, u.Filename, u.Kind, u.Bytes, reserve, string(hashes), now, expiry)
	if err == nil {
		_, err = tx.Exec("UPDATE rooms SET reserved=reserved+? WHERE id=?", reserve, room.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 201, u)
}
func (s *Server) listUploads(w http.ResponseWriter, r *http.Request) {
	p, room := s.roomAccess(w, r)
	if p == nil {
		return
	}
	if p.Role == "display" {
		apiError(w, 403, "FORBIDDEN")
		return
	}
	rows, err := s.db.Query(uploadSelect+" WHERE room_id=? AND owner=? ORDER BY created_at DESC LIMIT 50", room.ID, p.Role+":"+p.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	defer rows.Close()
	result := []*durableUpload{}
	for rows.Next() {
		u, e := scanUpload(rows)
		if e != nil {
			s.internal(w, e)
			return
		}
		s.uploadParts(u)
		result = append(result, u)
	}
	writeJSON(w, 200, result)
}
func (s *Server) getUploadStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := s.uploadAccess(w, r, false)
	if u == nil {
		return
	}
	s.uploadParts(u)
	writeJSON(w, 200, u)
}
func (s *Server) uploadPart(w http.ResponseWriter, r *http.Request) {
	if u, _ := s.uploadAccess(w, r, true); u == nil {
		return
	}
	// Read a bounded chunk before acquiring the upload lock so slow clients cannot block all uploads.
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, uploadChunkSize))
	if err != nil {
		apiError(w, 413, "FILE_TOO_LARGE")
		return
	}
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	u, room := s.uploadAccess(w, r, true)
	if u == nil {
		return
	}
	i, err := strconv.Atoi(r.PathValue("part"))
	if err != nil || i < 0 || i >= len(u.Hashes) || int64(len(data)) != u.partSize(i) || u.State != "uploading" {
		apiError(w, 400, "INVALID_INPUT")
		return
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != u.Hashes[i] {
		apiError(w, 409, "UPLOAD_CONFLICT")
		return
	}
	if s.limited(r, "upload-part", 300) {
		apiError(w, 429, "RATE_LIMITED")
		return
	}
	if free, e := freeBytes(s.config.DataDir); e != nil || free < uint64(len(data))+(256<<20) {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	dir := s.uploadDir(u)
	if err = os.MkdirAll(dir, 0700); err != nil {
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	dest := filepath.Join(dir, strconv.Itoa(i))
	if old, e := os.ReadFile(dest); e == nil {
		h := sha256.Sum256(old)
		if h == hash {
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	temp := dest + ".part"
	f, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err == nil {
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err == nil {
			err = ce
		}
	}
	if err == nil {
		err = os.Rename(temp, dest)
	}
	if err != nil {
		_ = os.Remove(temp)
		apiError(w, 507, "STORAGE_FULL")
		return
	}
	_, err = s.db.Exec("UPDATE uploads SET expires_at=? WHERE id=?", min(time.Now().Unix()+86400, u.Created+72*3600, room.EndsAt), u.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	u, _ := s.uploadAccess(w, r, true)
	if u == nil {
		return
	}
	if u.State == "ready" {
		writeJSON(w, 200, map[string]string{"id": u.ID})
		return
	}
	readers := []io.Reader{}
	files := []*os.File{}
	defer func() {
		for _, f := range files {
			_ = f.Close()
		}
	}()
	for i, h := range u.Hashes {
		f, err := os.Open(filepath.Join(s.uploadDir(u), strconv.Itoa(i)))
		if err != nil {
			apiError(w, 409, "UPLOAD_INCOMPLETE")
			return
		}
		files = append(files, f)
		hash := sha256.New()
		n, err := io.Copy(hash, f)
		if err != nil || n != u.partSize(i) || hex.EncodeToString(hash.Sum(nil)) != h {
			apiError(w, 409, "UPLOAD_CONFLICT")
			return
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			s.internal(w, err)
			return
		}
		readers = append(readers, f)
	}
	request := r.Clone(context.WithValue(r.Context(), uploadContextKey{}, u.ID))
	query := request.URL.Query()
	query.Set("kind", u.Kind)
	query.Set("name", u.Filename)
	query.Set("size", fmt.Sprint(u.Bytes))
	request.URL.RawQuery = query.Encode()
	request.Body = io.NopCloser(io.MultiReader(readers...))
	s.uploadAsset(w, request)
	// Files are removed by maintenance after handles close (also safe on Windows).
}
func (s *Server) cancelUpload(w http.ResponseWriter, r *http.Request) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	u, _ := s.uploadAccess(w, r, false)
	if u == nil {
		return
	}
	if u.State == "ready" {
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if err := s.removeUpload(u); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) removeUpload(u *durableUpload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM uploads WHERE id=?", u.ID); err != nil {
		return err
	}
	if u.State == "uploading" {
		if _, err = tx.Exec("UPDATE rooms SET reserved=MAX(0,reserved-?) WHERE id=?", u.Reservation, u.Room); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return os.RemoveAll(s.uploadDir(u))
}
func (s *Server) uploadMaintenance() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		s.cleanUploads()
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) cleanUploads() {
	_, _ = s.db.Exec("DELETE FROM danmaku WHERE created_at<?", time.Now().Unix()-86400)
	_, _ = s.db.Exec("DELETE FROM screen_requests WHERE state NOT IN ('pending','queued','offered','presenting') AND created_at<?", time.Now().Add(-24*time.Hour).UnixNano())
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	rows, err := s.db.Query(uploadSelect)
	if err != nil {
		return
	}
	list := []*durableUpload{}
	for rows.Next() {
		u, e := scanUpload(rows)
		if e == nil {
			list = append(list, u)
		}
	}
	rows.Close()
	keep := map[string]bool{}
	for _, u := range list {
		room, e := s.getRoom(u.Room)
		if e != nil || !room.Active() || u.Expires <= time.Now().Unix() {
			_ = s.removeUpload(u)
			continue
		}
		keep[s.uploadDir(u)] = true
		if u.State == "ready" {
			_ = os.RemoveAll(s.uploadDir(u))
		}
	}
	roots, _ := os.ReadDir(filepath.Join(s.config.DataDir, "uploads"))
	for _, room := range roots {
		if !room.IsDir() {
			continue
		}
		entries, _ := os.ReadDir(filepath.Join(s.config.DataDir, "uploads", room.Name()))
		for _, entry := range entries {
			dir := filepath.Join(s.config.DataDir, "uploads", room.Name(), entry.Name())
			if !keep[dir] {
				_ = os.RemoveAll(dir)
			}
		}
	}
}
