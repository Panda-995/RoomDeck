package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type fixture struct {
	s                  *Server
	ts                 *httptest.Server
	host, guest, other *http.Client
	room               Room
}

func TestDataDirectoryLockAndRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	s, e := New(Config{DataDir: dir})
	if e != nil {
		t.Fatal(e)
	}
	if second, err := New(Config{DataDir: dir}); err == nil {
		second.Close()
		t.Fatal("second instance acquired data directory")
	}
	token := s.setupToken
	if e = os.WriteFile(filepath.Join(dir, "tmp", "incomplete-upload"), []byte("partial"), 0600); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = New(Config{DataDir: dir})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.setupToken != token {
		t.Fatal("setup token unexpectedly changed")
	}
	if _, e = os.Stat(filepath.Join(dir, "tmp", "incomplete-upload")); !os.IsNotExist(e) {
		t.Fatal("incomplete upload not cleaned")
	}
}

func TestWebSocketInvalidation(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, e := websocket.Dial(ctx, strings.Replace(f.ts.URL, "http:", "ws:", 1)+"/api/v1"+f.path("/ws"), &websocket.DialOptions{HTTPClient: f.guest})
	if e != nil {
		t.Fatal(e)
	}
	defer conn.CloseNow()
	_, _, e = conn.Read(ctx)
	if e != nil {
		t.Fatal(e)
	}
	f.request(t, f.host, "POST", f.path("/contents"), map[string]string{"kind": "note", "body": "live update"}, 201, nil)
	_, body, e := conn.Read(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(body, []byte("refresh")) {
		t.Fatal("missing invalidation")
	}
}

func TestUploadCloseRaceReleasesReservation(t *testing.T) {
	f := newFixture(t)
	data := testImage()
	reader, writer := io.Pipe()
	req, _ := http.NewRequest("POST", f.ts.URL+"/api/v1"+f.path(fmt.Sprintf("/assets?kind=photo&name=test.png&size=%d", len(data))), reader)
	req.Header.Set("X-RoomDeck-Request", "1")
	done := make(chan int, 1)
	go func() {
		res, e := f.guest.Do(req)
		if e != nil {
			done <- 0
			return
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		done <- res.StatusCode
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		_ = f.s.db.QueryRow("SELECT reserved FROM rooms WHERE id=?", f.room.ID).Scan(&n)
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.request(t, f.host, "POST", f.path("/close"), nil, 200, nil)
	_, _ = writer.Write(data)
	writer.Close()
	if status := <-done; status != 409 {
		t.Fatalf("late upload status %d", status)
	}
	var count, reserved int64
	_ = f.s.db.QueryRow("SELECT COUNT(*) FROM contents WHERE room_id=?", f.room.ID).Scan(&count)
	_ = f.s.db.QueryRow("SELECT reserved FROM rooms WHERE id=?", f.room.ID).Scan(&reserved)
	if count != 0 || reserved != 0 {
		t.Fatalf("late upload published or leaked reservation: %d %d", count, reserved)
	}
}

func TestExpiredCloseKeepsOriginalDeadline(t *testing.T) {
	f := newFixture(t)
	expired := time.Now().Unix() - 120
	f.s.mu.Lock()
	_, _ = f.s.db.Exec("UPDATE rooms SET ends_at=? WHERE id=?", expired, f.room.ID)
	f.s.mu.Unlock()
	f.request(t, f.host, "POST", f.path("/close"), nil, 200, nil)
	r, e := f.s.getRoom(f.room.ID)
	if e != nil {
		t.Fatal(e)
	}
	if r.ClosedAt != expired || r.DeleteAt != expired+r.Retention {
		t.Fatal("automatic deadline was extended")
	}
}

func client() *http.Client {
	j, _ := cookiejar.New(nil)
	return &http.Client{Jar: j, Timeout: 10 * time.Second}
}
func newFixture(t *testing.T) *fixture {
	t.Helper()
	s, e := New(Config{DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Close() })
	f := &fixture{s: s, ts: ts, host: client(), guest: client(), other: client()}
	f.request(t, f.host, "POST", "/setup", map[string]string{"token": s.setupToken, "username": "host", "password": "a-strong-password-2026"}, 201, nil)
	f.request(t, f.host, "POST", "/rooms", map[string]interface{}{"name": "Gathering 聚会", "duration": 7200, "retention": 86400}, 201, &f.room)
	f.request(t, f.guest, "POST", "/join", joinInput{Code: f.room.Code, Name: "Guest 访客"}, 201, nil)
	return f
}
func (f *fixture) request(t *testing.T, c *http.Client, method, path string, body interface{}, want int, out interface{}) []byte {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	r, _ := http.NewRequest(method, f.ts.URL+"/api/v1"+path, reader)
	r.Header.Set("X-RoomDeck-Request", "1")
	r.Header.Set("Content-Type", "application/json")
	res, e := c.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	b, e := io.ReadAll(res.Body)
	if e != nil {
		t.Fatal(e)
	}
	if res.StatusCode != want {
		t.Fatalf("%s %s: status %d wanted %d, %s", method, path, res.StatusCode, want, b)
	}
	if out != nil {
		if e = json.Unmarshal(b, out); e != nil {
			t.Fatalf("decode: %v: %s", e, b)
		}
	}
	return b
}
func (f *fixture) path(suffix string) string { return "/rooms/" + f.room.ID + suffix }
func testImage() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 60, 40))
	for x := 0; x < 60; x++ {
		for y := 0; y < 40; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 50, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
func (f *fixture) upload(t *testing.T, c *http.Client, data []byte, kind, name string, want int) string {
	t.Helper()
	q := url.Values{"kind": {kind}, "name": {name}, "size": {fmt.Sprint(len(data))}}
	r, _ := http.NewRequest("POST", f.ts.URL+"/api/v1"+f.path("/assets?")+q.Encode(), bytes.NewReader(data))
	r.Header.Set("X-RoomDeck-Request", "1")
	res, e := c.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("upload: %d wanted %d: %s", res.StatusCode, want, b)
	}
	var result map[string]interface{}
	_ = json.Unmarshal(b, &result)
	id, _ := result["id"].(string)
	return id
}
func TestSetupAndCSRF(t *testing.T) {
	f := newFixture(t)
	f.request(t, f.other, "POST", "/setup", map[string]string{"token": "wrong", "username": "x", "password": "long-enough-password"}, 403, nil)
	f.request(t, f.other, "POST", "/login", map[string]string{"username": "host", "password": "wrong"}, 401, nil)
	r, _ := http.NewRequest("POST", f.ts.URL+"/api/v1/rooms", strings.NewReader("{}"))
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("X-RoomDeck-Request", "1")
	res, e := f.host.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("cross-origin write accepted")
	}
	f.request(t, f.other, "GET", "/rooms", nil, 401, nil)
}
func TestRoomAccessAndDisplayIsolation(t *testing.T) {
	f := newFixture(t)
	id := f.upload(t, f.guest, testImage(), "photo", "photo.png", 201)
	f.request(t, f.other, "GET", f.path("/snapshot"), nil, 401, nil)
	var pair map[string]string
	f.request(t, f.host, "POST", f.path("/display-session"), nil, 201, &pair)
	display := client()
	f.request(t, display, "POST", "/display-exchange", pair, 200, nil)
	f.request(t, display, "POST", "/display-exchange", pair, 403, nil)
	raw := f.request(t, display, "GET", f.path("/snapshot?display=1"), nil, 200, nil)
	if bytes.Contains(raw, []byte(id)) {
		t.Fatal("unselected photo exposed to display")
	}
	f.request(t, display, "GET", f.path("/assets/"+id+"/preview?display=1"), nil, 403, nil)
	f.request(t, f.host, "PATCH", f.path("/contents/"+id), map[string]bool{"selected": true}, 200, nil)
	raw = f.request(t, display, "GET", f.path("/snapshot?display=1"), nil, 200, nil)
	if !bytes.Contains(raw, []byte(id)) {
		t.Fatal("selected photo missing")
	}
	f.request(t, display, "GET", f.path("/assets/"+id+"/original?display=1"), nil, 403, nil)
	f.request(t, display, "POST", f.path("/close"), nil, 401, nil)
	f.request(t, f.host, "PATCH", f.path("/contents/"+id), map[string]bool{"visible": false}, 200, nil)
	f.request(t, display, "GET", f.path("/assets/"+id+"/preview?display=1"), nil, 404, nil)
}
func TestPhotoVariantsQuotaAndFormats(t *testing.T) {
	f := newFixture(t)
	id := f.upload(t, f.guest, testImage(), "photo", "../../photo.png", 201)
	for _, variant := range []string{"preview", "thumb"} {
		b := f.request(t, f.guest, "GET", f.path("/assets/"+id+"/"+variant), nil, 200, nil)
		if !bytes.HasPrefix(b, []byte{0xff, 0xd8}) {
			t.Fatal("not a JPEG derivative")
		}
	}
	f.request(t, f.guest, "GET", f.path("/assets/"+id+"/original"), nil, 403, nil)
	f.upload(t, f.guest, []byte("<svg onload=alert(1)>"), "photo", "attack.png", 415)
	f.s.mu.Lock()
	_, e := f.s.db.Exec("UPDATE rooms SET used=? WHERE id=?", f.room.Settings.Quota-100, f.room.ID)
	f.s.mu.Unlock()
	if e != nil {
		t.Fatal(e)
	}
	f.upload(t, f.guest, testImage(), "photo", "overflow.png", 507)
	var reserved int64
	_ = f.s.db.QueryRow("SELECT reserved FROM rooms WHERE id=?", f.room.ID).Scan(&reserved)
	if reserved != 0 {
		t.Fatalf("reservation leaked: %d", reserved)
	}
}
func TestFilesAreAttachments(t *testing.T) {
	f := newFixture(t)
	id := f.upload(t, f.guest, []byte("<html><script>alert(1)</script></html>"), "file", "test.html", 201)
	r, _ := http.NewRequest("GET", f.ts.URL+"/api/v1"+f.path("/assets/"+id+"/original"), nil)
	res, e := f.guest.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment;") || res.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatal("file executed inline")
	}
}
func TestCloseExpiryAndDeletion(t *testing.T) {
	f := newFixture(t)
	f.request(t, f.host, "POST", f.path("/close"), nil, 200, nil)
	f.request(t, f.guest, "POST", f.path("/contents"), map[string]string{"kind": "note", "body": "late"}, 409, nil)
	f.request(t, f.guest, "GET", f.path("/snapshot"), nil, 200, nil)
	f.request(t, f.other, "POST", "/join", joinInput{Code: f.room.Code, Name: "new"}, 404, nil)
	f.s.mu.Lock()
	_, _ = f.s.db.Exec("UPDATE rooms SET delete_at=? WHERE id=?", time.Now().Unix()-1, f.room.ID)
	f.s.mu.Unlock()
	f.request(t, f.guest, "GET", f.path("/snapshot"), nil, 410, nil)
	f.s.cleanup()
	var count int
	_ = f.s.db.QueryRow("SELECT COUNT(*) FROM rooms WHERE id=?", f.room.ID).Scan(&count)
	if count != 0 {
		t.Fatal("expired room not deleted")
	}
}
func TestJoinLockAndRevocation(t *testing.T) {
	f := newFixture(t)
	settings := f.room.Settings
	settings.JoinLocked = true
	f.request(t, f.host, "PATCH", f.path(""), map[string]interface{}{"name": f.room.Name, "settings": settings, "retention": 86400, "version": f.room.Version}, 200, nil)
	f.request(t, f.other, "POST", "/join", joinInput{Code: f.room.Code, Name: "new"}, 404, nil)
	f.request(t, f.guest, "POST", f.path("/contents"), map[string]string{"kind": "note", "body": "still sharing"}, 201, nil)
	var member string
	_ = f.s.db.QueryRow("SELECT id FROM participants WHERE room_id=?", f.room.ID).Scan(&member)
	f.request(t, f.host, "PATCH", f.path("/participants/"+member), map[string]bool{"revoked": true}, 200, nil)
	f.request(t, f.guest, "GET", f.path("/snapshot"), nil, 401, nil)
}
func TestPollIdempotencyAndHiddenResults(t *testing.T) {
	f := newFixture(t)
	var result map[string]string
	f.request(t, f.host, "POST", f.path("/contents"), map[string]interface{}{"kind": "poll", "title": "Where next?", "poll": Poll{Options: []string{"Park", "Cafe"}, MaxChoices: 1, HideResults: true, ClosesAt: time.Now().Add(time.Hour).Unix()}}, 201, &result)
	id := result["id"]
	f.request(t, f.guest, "PUT", f.path("/polls/"+id+"/vote"), map[string]interface{}{"choices": []int{0, 0}}, 400, nil)
	for i := 0; i < 2; i++ {
		f.request(t, f.guest, "PUT", f.path("/polls/"+id+"/vote"), map[string]interface{}{"choices": []int{0}}, 200, nil)
	}
	raw := f.request(t, f.host, "GET", f.path("/snapshot"), nil, 200, nil)
	if bytes.Contains(raw, []byte(`"counts"`)) {
		t.Fatal("hidden vote totals leaked")
	}
	f.request(t, f.host, "PATCH", f.path("/contents/"+id), map[string]bool{"close_poll": true}, 200, nil)
	c, e := f.s.getContent(f.room.ID, id)
	if e != nil {
		t.Fatal(e)
	}
	f.s.pollResults(c, &principal{Role: "host"}, &f.room)
	if c.Voters == nil || *c.Voters != 1 || c.Counts[0] != 1 {
		t.Fatal("duplicate votes counted")
	}
	f.request(t, f.guest, "PUT", f.path("/polls/"+id+"/vote"), map[string]interface{}{"choices": []int{1}}, 409, nil)
}
func TestCrossRoomAndVersionConflict(t *testing.T) {
	f := newFixture(t)
	var second Room
	f.request(t, f.host, "POST", "/rooms", map[string]interface{}{"name": "Other", "duration": 7200, "retention": 86400}, 201, &second)
	f.request(t, f.guest, "GET", "/rooms/"+second.ID+"/snapshot", nil, 401, nil)
	f.request(t, f.guest, "POST", "/rooms/"+second.ID+"/contents", map[string]string{"kind": "note", "body": "intrusion"}, 401, nil)
	f.request(t, f.host, "PATCH", f.path(""), map[string]interface{}{"name": f.room.Name, "settings": f.room.Settings, "retention": 86400, "version": 999}, 409, nil)
}
func TestExportArchiveAndPersistence(t *testing.T) {
	f := newFixture(t)
	f.upload(t, f.guest, []byte("first"), "file", "same.txt", 201)
	f.upload(t, f.guest, []byte("second"), "file", "same.txt", 201)
	f.request(t, f.guest, "POST", f.path("/contents"), map[string]string{"kind": "note", "title": "<script>", "body": "<script>alert(1)</script>"}, 201, nil)
	var job Job
	f.request(t, f.host, "POST", f.path("/exports"), nil, 202, &job)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var state string
		_ = f.s.db.QueryRow("SELECT state FROM jobs WHERE id=?", job.ID).Scan(&state)
		if state == "ready" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	archive := f.request(t, f.host, "GET", f.path("/exports/"+job.ID), nil, 200, nil)
	z, e := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if e != nil {
		t.Fatal(e)
	}
	files := 0
	for _, entry := range z.File {
		if strings.HasPrefix(entry.Name, "files/") {
			files++
		}
		if entry.Name == "notes.html" {
			r, _ := entry.Open()
			body, _ := io.ReadAll(r)
			r.Close()
			if bytes.Contains(body, []byte("<script>")) {
				t.Fatal("unescaped export")
			}
		}
	}
	if files != 2 {
		t.Fatal("same filename overwritten")
	}
	f.request(t, f.guest, "GET", f.path("/exports/"+job.ID), nil, 401, nil)
	if _, e = os.Stat(filepath.Join(f.s.config.DataDir, "roomdeck.db")); e != nil {
		t.Fatal(e)
	}
}
