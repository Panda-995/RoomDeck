package server

import (
	"database/sql"
	"fmt"
)

const schemaVersion = 7

func migrate(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;"); err != nil {
		return err
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("database version %d is newer than this application", version)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if version < 1 {
		if _, err = tx.Exec(schema); err != nil {
			return err
		}
	}
	if version < 2 {
		_, err = tx.Exec(`
 CREATE TABLE games(room_id TEXT PRIMARY KEY REFERENCES rooms(id) ON DELETE CASCADE, state TEXT NOT NULL);
 CREATE TABLE screens(room_id TEXT PRIMARY KEY, id TEXT NOT NULL, owner TEXT NOT NULL, name TEXT NOT NULL, expires INTEGER NOT NULL);
 CREATE TABLE media_clients(id TEXT PRIMARY KEY, room_id TEXT NOT NULL, screen_id TEXT NOT NULL, session_hash TEXT NOT NULL, role TEXT NOT NULL, participant_id TEXT NOT NULL, expires INTEGER NOT NULL);
 CREATE INDEX media_clients_room ON media_clients(room_id);
 PRAGMA user_version=2;`)
		if err != nil {
			return err
		}
	}
	if version < 3 {
		_, err = tx.Exec(`
ALTER TABLE rooms ADD COLUMN state_revision INTEGER NOT NULL DEFAULT 1;
CREATE TRIGGER room_revision AFTER UPDATE OF settings,state,name,used,reserved ON rooms BEGIN
 UPDATE rooms SET state_revision=state_revision+1 WHERE id=NEW.id;
END;
CREATE TRIGGER content_insert_revision AFTER INSERT ON contents BEGIN
 UPDATE rooms SET state_revision=state_revision+1 WHERE id=NEW.room_id;
END;
CREATE TRIGGER content_update_revision AFTER UPDATE ON contents BEGIN
 UPDATE rooms SET state_revision=state_revision+1 WHERE id=NEW.room_id;
 UPDATE rooms SET settings=json_set(settings,'$.display_target',''),version=version+1 WHERE id=NEW.room_id AND json_extract(settings,'$.display_target')=NEW.id AND (NEW.visible=0 OR NEW.selected=0);
END;
CREATE TRIGGER content_delete_revision AFTER DELETE ON contents BEGIN
 UPDATE rooms SET state_revision=state_revision+1 WHERE id=OLD.room_id;
 UPDATE rooms SET settings=json_set(settings,'$.display_target',''),version=version+1 WHERE id=OLD.room_id AND json_extract(settings,'$.display_target')=OLD.id;
END;
CREATE TRIGGER ballot_insert_revision AFTER INSERT ON ballots BEGIN
 UPDATE rooms SET state_revision=state_revision+1 WHERE id=(SELECT room_id FROM contents WHERE id=NEW.poll_id);
END;
CREATE TRIGGER ballot_update_revision AFTER UPDATE ON ballots BEGIN
 UPDATE rooms SET state_revision=state_revision+1 WHERE id=(SELECT room_id FROM contents WHERE id=NEW.poll_id);
END;
PRAGMA user_version=3;`)
		if err != nil {
			return err
		}
	}
	if version < 4 {
		_, err = tx.Exec(`
CREATE TABLE uploads(id TEXT PRIMARY KEY,room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,owner TEXT NOT NULL,request_id TEXT NOT NULL,filename TEXT NOT NULL,kind TEXT NOT NULL,bytes INTEGER NOT NULL,reservation INTEGER NOT NULL,hashes TEXT NOT NULL,state TEXT NOT NULL DEFAULT 'uploading',created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,UNIQUE(room_id,owner,request_id));
CREATE INDEX uploads_expiry ON uploads(state,expires_at);
PRAGMA user_version=4;`)
		if err != nil {
			return err
		}
	}
	if version < 5 {
		_, err = tx.Exec(`
CREATE TABLE screen_requests(id TEXT PRIMARY KEY,room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,owner TEXT NOT NULL,role TEXT NOT NULL,name TEXT NOT NULL,state TEXT NOT NULL,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL DEFAULT 0);
CREATE UNIQUE INDEX screen_request_active ON screen_requests(room_id,owner) WHERE state IN ('pending','queued','offered','presenting');
CREATE INDEX screen_request_room ON screen_requests(room_id,state,created_at);
PRAGMA user_version=5;`)
		if err != nil {
			return err
		}
	}
	if version < 6 {
		_, err = tx.Exec(`
CREATE TABLE reactions(content_id TEXT NOT NULL REFERENCES contents(id) ON DELETE CASCADE,owner TEXT NOT NULL,emoji TEXT NOT NULL,PRIMARY KEY(content_id,owner));
CREATE TABLE danmaku(id TEXT PRIMARY KEY,room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,owner TEXT NOT NULL,name TEXT NOT NULL,body TEXT NOT NULL,state TEXT NOT NULL,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL);
CREATE INDEX danmaku_room ON danmaku(room_id,state,expires_at);
PRAGMA user_version=6;`)
		if err != nil {
			return err
		}
	}
	if version < 7 {
		_, err = tx.Exec(`ALTER TABLE participants ADD COLUMN permissions TEXT NOT NULL DEFAULT '[]';
CREATE TABLE boards(room_id TEXT PRIMARY KEY REFERENCES rooms(id) ON DELETE CASCADE,state TEXT NOT NULL);
PRAGMA user_version=7;`)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
