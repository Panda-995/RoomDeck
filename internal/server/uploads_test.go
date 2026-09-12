package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestUploadResumeRestartAndIdempotency(t *testing.T) {
	f := newFixture(t)
	data := bytes.Repeat([]byte("z"), int(uploadChunkSize)+37)
	hashes := []string{}
	for offset := 0; offset < len(data); offset += int(uploadChunkSize) {
		h := sha256.Sum256(data[offset:min(len(data), offset+int(uploadChunkSize))])
		hashes = append(hashes, hex.EncodeToString(h[:]))
	}
	var u durableUpload
	input := map[string]interface{}{"request_id": "resume-test", "filename": "test.bin", "kind": "file", "bytes": len(data), "hashes": hashes}
	f.request(t, f.guest, "POST", f.path("/uploads"), input, 201, &u)
	put := func(part string, data []byte, want int) {
		t.Helper()
		r, _ := http.NewRequest("PUT", f.ts.URL+"/api/v1"+f.path("/uploads/"+u.ID+"/parts/"+part), bytes.NewReader(data))
		r.Header.Set("X-RoomDeck-Request", "1")
		resp, e := f.guest.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("part status %d: %s", resp.StatusCode, body)
		}
	}
	put("0", data[:uploadChunkSize], 200)
	put("0", data[:uploadChunkSize], 200)
	put("1", bytes.Repeat([]byte("x"), 37), 409)
	f.request(t, f.host, "GET", f.path("/uploads/"+u.ID), nil, 404, nil)
	f.request(t, f.guest, "POST", f.path("/uploads/"+u.ID+"/complete"), nil, 409, nil)
	// Restart the application over the same mapped directory and preserve session cookies.
	oldURL := f.ts.URL
	config := f.s.config
	f.ts.Close()
	f.s.Close()
	s, e := New(config)
	if e != nil {
		t.Fatal(e)
	}
	f.s = s
	f.ts = httptest.NewServer(s.Handler())
	defer f.ts.Close()
	defer s.Close()
	old, _ := http.NewRequest("GET", oldURL, nil)
	next, _ := http.NewRequest("GET", f.ts.URL, nil)
	for _, c := range []*http.Client{f.host, f.guest} {
		c.Jar.SetCookies(next.URL, c.Jar.Cookies(old.URL))
	}
	f.request(t, f.guest, "GET", f.path("/uploads/"+u.ID), nil, 200, &u)
	if len(u.Parts) != 1 || u.Parts[0] != 0 {
		t.Fatal("lost confirmed part", u.Parts)
	}
	room, _ := f.s.getRoom(f.room.ID)
	if room.Reserved != int64(len(data)) {
		t.Fatal("lost reservation", room.Reserved)
	}
	put("1", data[uploadChunkSize:], 200)
	var first, second map[string]string
	f.request(t, f.guest, "POST", f.path("/uploads/"+u.ID+"/complete"), nil, 201, &first)
	f.request(t, f.guest, "POST", f.path("/uploads/"+u.ID+"/complete"), nil, 200, &second)
	if first["id"] != second["id"] {
		t.Fatal("duplicate content")
	}
	actual, e := os.ReadFile(s.assetPath(f.room.ID, u.ID, "original"))
	if e != nil || !bytes.Equal(data, actual) {
		t.Fatal("incorrect assembled file", e)
	}
	room, _ = s.getRoom(f.room.ID)
	if room.Reserved != 0 || room.Used != int64(len(data)) {
		t.Fatal("incorrect accounting")
	}
}
