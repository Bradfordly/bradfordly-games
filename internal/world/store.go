package world

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Store persists world records in SQLite on the data volume.
type Store struct {
	db *sql.DB
}

// OpenStore opens (and creates) the SQLite file at path.
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite dir: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS worlds (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	game TEXT NOT NULL,
	image TEXT NOT NULL,
	allocation_host TEXT NOT NULL UNIQUE,
	allocation_port INTEGER NOT NULL,
	allocation_protocol TEXT NOT NULL,
	backend_container TEXT NOT NULL UNIQUE,
	idle_timeout TEXT NOT NULL,
	start_timeout TEXT NOT NULL,
	stop_timeout TEXT NOT NULL,
	occupy_mode TEXT NOT NULL,
	asleep_motd TEXT NOT NULL DEFAULT '',
	starting_motd TEXT NOT NULL DEFAULT '',
	wake_whitelist TEXT NOT NULL DEFAULT '[]',
	env TEXT NOT NULL DEFAULT '{}',
	volume TEXT NOT NULL UNIQUE
);`)
	return err
}

func (s *Store) Insert(w World) error {
	whitelist, env, err := encodeLists(w)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO worlds (
	id, name, game, image, allocation_host, allocation_port, allocation_protocol,
	backend_container, idle_timeout, start_timeout, stop_timeout, occupy_mode,
	asleep_motd, starting_motd, wake_whitelist, env, volume
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Game, w.Image, w.Allocation.Host, w.Allocation.Port, w.Allocation.Protocol,
		w.Backend.Container, w.IdleTimeout, w.StartTimeout, w.StopTimeout, w.OccupyMode,
		w.AsleepMOTD, w.StartingMOTD, whitelist, env, w.Volume,
	)
	if err != nil {
		return mapSQLiteErr(err)
	}
	return nil
}

func (s *Store) Update(w World) error {
	whitelist, env, err := encodeLists(w)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`
UPDATE worlds SET
	name=?, game=?, image=?, allocation_host=?, allocation_port=?, allocation_protocol=?,
	backend_container=?, idle_timeout=?, start_timeout=?, stop_timeout=?, occupy_mode=?,
	asleep_motd=?, starting_motd=?, wake_whitelist=?, env=?, volume=?
WHERE id=?`,
		w.Name, w.Game, w.Image, w.Allocation.Host, w.Allocation.Port, w.Allocation.Protocol,
		w.Backend.Container, w.IdleTimeout, w.StartTimeout, w.StopTimeout, w.OccupyMode,
		w.AsleepMOTD, w.StartingMOTD, whitelist, env, w.Volume, w.ID,
	)
	if err != nil {
		return mapSQLiteErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Get(id string) (World, error) {
	w, err := scanWorld(s.db.QueryRow(`SELECT `+worldColumns+` FROM worlds WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return World{}, ErrNotFound
	}
	return w, err
}

func (s *Store) List() ([]World, error) {
	rows, err := s.db.Query(`SELECT ` + worldColumns + ` FROM worlds ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []World
	for rows.Next() {
		w, err := scanWorld(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	if out == nil {
		out = []World{}
	}
	return out, rows.Err()
}

func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM worlds WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const worldColumns = `id, name, game, image, allocation_host, allocation_port, allocation_protocol,
	backend_container, idle_timeout, start_timeout, stop_timeout, occupy_mode,
	asleep_motd, starting_motd, wake_whitelist, env, volume`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWorld(row rowScanner) (World, error) {
	var w World
	var whitelist, env string
	err := row.Scan(
		&w.ID, &w.Name, &w.Game, &w.Image, &w.Allocation.Host, &w.Allocation.Port, &w.Allocation.Protocol,
		&w.Backend.Container, &w.IdleTimeout, &w.StartTimeout, &w.StopTimeout, &w.OccupyMode,
		&w.AsleepMOTD, &w.StartingMOTD, &whitelist, &env, &w.Volume,
	)
	if err != nil {
		return World{}, err
	}
	if err := json.Unmarshal([]byte(whitelist), &w.WakeWhitelist); err != nil {
		return World{}, fmt.Errorf("wake_whitelist: %w", err)
	}
	if w.WakeWhitelist == nil {
		w.WakeWhitelist = []string{}
	}
	if err := json.Unmarshal([]byte(env), &w.Env); err != nil {
		return World{}, fmt.Errorf("env: %w", err)
	}
	if w.Env == nil {
		w.Env = map[string]string{}
	}
	return w, nil
}

func encodeLists(w World) (string, string, error) {
	if w.WakeWhitelist == nil {
		w.WakeWhitelist = []string{}
	}
	if w.Env == nil {
		w.Env = map[string]string{}
	}
	whitelist, err := json.Marshal(w.WakeWhitelist)
	if err != nil {
		return "", "", err
	}
	env, err := json.Marshal(w.Env)
	if err != nil {
		return "", "", err
	}
	return string(whitelist), string(env), nil
}

func mapSQLiteErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed") {
		return ErrConflict
	}
	return err
}
