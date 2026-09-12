package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMediaNotFoundDoesNotCreateStage(t *testing.T) {
	f := newFixture(t)
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(`{"code":"not_found"}`))
	}))
	defer media.Close()
	f.s.config.MediaURL = media.URL
	f.s.config.MediaKey = "test"
	f.s.config.MediaSecret = strings.Repeat("s", 32)
	f.request(t, f.host, "POST", f.path("/screen"), map[string]string{"action": "start"}, 503, nil)
	if f.s.getScreen(f.room.ID) != nil {
		t.Fatal("failed media creation left a stage")
	}
	for _, method := range []string{"DeleteRoom", "RemoveParticipant"} {
		if err := f.s.mediaCall(method, "missing", map[string]string{"room": "missing"}); err != nil {
			t.Fatal("missing cleanup target should be idempotent", err)
		}
	}
}

func TestMediaFailureKeepsRevocationAndSharingAvailable(t *testing.T) {
	f := newFixture(t)
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "DeleteRoom") {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer media.Close()
	f.s.config.MediaURL = media.URL
	f.s.config.MediaKey = "test"
	f.s.config.MediaSecret = strings.Repeat("s", 32)
	var grant struct {
		Client string
		Screen screenState
	}
	f.request(t, f.host, "POST", f.path("/screen"), map[string]string{"action": "start"}, 200, &grant)
	f.request(t, f.host, "POST", f.path("/screen"), map[string]string{"action": "stop", "screen_id": grant.Screen.ID}, 503, nil)
	if f.s.validMediaClient(grant.Client) {
		t.Fatal("revoked client accepted after failed cleanup")
	}
	if v := f.s.getScreen(f.room.ID); v == nil || v.Expires != 0 {
		t.Fatal("cleanup retry state lost")
	}
	f.request(t, f.guest, "POST", f.path("/contents"), map[string]string{"kind": "note", "body": "Sharing still works"}, 201, nil)
}

func TestMediaCallHonorsShutdown(t *testing.T) {
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer media.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &Server{ctx: ctx, config: Config{MediaURL: media.URL, MediaKey: "test", MediaSecret: strings.Repeat("s", 32)}}
	if err := s.mediaCall("CreateRoom", "test", map[string]string{"name": "test"}); err == nil {
		t.Fatal("shutdown request succeeded")
	}
}
