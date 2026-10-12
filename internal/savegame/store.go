// Package savegame stores campaign checkpoints as small, durable JSON files.
package savegame

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MaxRecords is the maximum number of unfinished campaigns kept in a store.
const MaxRecords = 100

// MaxRecordBytes is the largest encoded record accepted by the store.
const MaxRecordBytes = 8 << 20

// ErrFull is returned when a store contains MaxRecords unfinished campaigns.
var ErrFull = errors.New("savegame: store is full")

// Record is one campaign checkpoint. Checkpoint is opaque to this package.
type Record struct {
	Version    int             `json:"version"`
	ID         string          `json:"id"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Players    int             `json:"players"`
	Level      int             `json:"level"`
	Score      float64         `json:"score"`
	Goal       float64         `json:"goal"`
	Completed  bool            `json:"completed"`
	Checkpoint json.RawMessage `json:"checkpoint"`
}

// HistoryRecord summarizes a completed campaign without retaining its checkpoint.
type HistoryRecord struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Players   int       `json:"players"`
	Level     int       `json:"level"`
	Score     float64   `json:"score"`
	Goal      float64   `json:"goal"`
}

// Store manages unfinished campaigns and completed summaries below Dir.
// Warnings contains problems found by the most recent List or History call;
// malformed files are left untouched.
type Store struct {
	Dir      string
	Warnings []string

	mu sync.Mutex
}

// Open creates or opens a save directory. Existing directories are tightened
// to mode 0700 where the operating system supports file permissions.
func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("savegame: empty directory")
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("savegame: create directory: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("savegame: stat directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("savegame: %q is not a directory", dir)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, fmt.Errorf("savegame: set directory permissions: %w", err)
	}
	s := &Store{Dir: dir}
	if _, err := s.List(); err != nil {
		return nil, err
	}
	return s, nil
}

// List returns valid unfinished records newest by UpdatedAt first, migrating
// legacy completions into history. Invalid files are skipped and described in
// Warnings so one damaged file cannot stop a game.
func (s *Store) List() ([]Record, error) {
	if s == nil {
		return nil, errors.New("savegame: nil store")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

// Put durably saves an unfinished campaign, or archives a completed summary
// before removing its checkpoint. Completed campaigns never use active slots.
func (s *Store) Put(record Record) error {
	if s == nil {
		return errors.New("savegame: nil store")
	}
	if err := validateRecord(record); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.listLocked()
	if err != nil {
		return err
	}
	if record.Completed {
		return s.archiveLocked(record)
	}
	if _, err := os.Lstat(s.historyPath(record.ID)); err == nil {
		return fmt.Errorf("savegame: campaign %q is already archived", record.ID)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("savegame: inspect history %q: %w", record.ID, err)
	}

	target := s.recordPath(record.ID)
	_, err = os.Lstat(target)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("savegame: inspect %q: %w", record.ID, err)
	}
	if os.IsNotExist(err) && len(records) >= MaxRecords {
		return ErrFull
	}
	payload, err := encodeRecord(record, record.ID)
	if err != nil {
		return err
	}
	return writeAtomic(s.Dir, target, payload)
}

// Delete removes a campaign by ID. Deleting an absent campaign is idempotent.
func (s *Store) Delete(id string) error {
	if s == nil {
		return errors.New("savegame: nil store")
	}
	if err := validateID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return removeRecord(s.Dir, s.recordPath(id), id)
}

// History returns completed summaries newest by UpdatedAt first. It also
// migrates legacy completed save files, just like List.
func (s *Store) History() ([]HistoryRecord, error) {
	if s == nil {
		return nil, errors.New("savegame: nil store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.listLocked(); err != nil {
		return nil, err
	}
	records := make([]HistoryRecord, 0)
	warnings, err := readRecords(s.historyDir(), func(id string, data []byte) error {
		record, err := decodeHistoryRecord(id, data)
		if err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if os.IsNotExist(err) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		s.Warnings = append(s.Warnings, "history/"+warning)
	}
	sort.Slice(records, func(i, j int) bool {
		return newestFirst(records[i].UpdatedAt, records[i].CreatedAt, records[i].ID,
			records[j].UpdatedAt, records[j].CreatedAt, records[j].ID)
	})
	return records, nil
}

// DeleteHistory removes a completed summary. Deleting an absent ID is idempotent.
func (s *Store) DeleteHistory(id string) error {
	if s == nil {
		return errors.New("savegame: nil store")
	}
	if err := validateID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return removeRecord(s.historyDir(), s.historyPath(id), id)
}

func (s *Store) listLocked() ([]Record, error) {
	records := make([]Record, 0)
	var migrationErr error
	warnings, err := readRecords(s.Dir, func(id string, data []byte) error {
		var record Record
		if err := json.Unmarshal(data, &record); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		if record.ID != id {
			return fmt.Errorf("record ID %q does not match filename", record.ID)
		}
		if err := validateRecord(record); err != nil {
			return err
		}
		archived, historyErr := s.hasHistoryLocked(id)
		if historyErr != nil {
			// Invalid history is not proof of completion. Preserve the active
			// checkpoint and warn rather than erasing potentially recoverable data.
			if !record.Completed {
				records = append(records, record)
			} else {
				migrationErr = errors.Join(migrationErr, historyErr)
			}
			return historyErr
		}
		if archived {
			// A crash after archive sync can leave an unfinished checkpoint.
			// The final summary is authoritative; never overwrite it with the
			// stale checkpoint's score or timestamps while finishing removal.
			if err := removeRecord(s.Dir, s.recordPath(id), id); err != nil {
				migrationErr = errors.Join(migrationErr, err)
				return err
			}
			return nil
		}
		if record.Completed {
			if err := s.archiveLocked(record); err != nil {
				migrationErr = errors.Join(migrationErr, err)
				return err
			}
			return nil
		}
		records = append(records, record)
		return nil
	})
	s.Warnings = warnings
	if err != nil {
		return nil, err
	}
	if migrationErr != nil {
		return nil, migrationErr
	}
	sort.Slice(records, func(i, j int) bool {
		return newestFirst(records[i].UpdatedAt, records[i].CreatedAt, records[i].ID,
			records[j].UpdatedAt, records[j].CreatedAt, records[j].ID)
	})
	return records, nil
}

// hasHistoryLocked only treats a readable, validated matching summary as
// authoritative. Its presence alone cannot justify deleting a checkpoint.
func (s *Store) hasHistoryLocked(id string) (bool, error) {
	info, err := os.Lstat(s.historyPath(id))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("not a regular file")
	}
	if err == nil && (info.Size() < 0 || info.Size() > MaxRecordBytes) {
		err = fmt.Errorf("exceeds %d-byte limit", MaxRecordBytes)
	}
	if err == nil {
		var data []byte
		data, err = readBounded(s.historyPath(id))
		if err == nil {
			_, err = decodeHistoryRecord(id, data)
		}
	}
	if err != nil {
		return false, fmt.Errorf("history/%s.json: %w", id, err)
	}
	return true, nil
}

func decodeHistoryRecord(id string, data []byte) (HistoryRecord, error) {
	var record HistoryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf("decode: %w", err)
	}
	if record.ID != id {
		return record, fmt.Errorf("record ID %q does not match filename", record.ID)
	}
	err := validateRecord(Record{Version: record.Version, ID: record.ID,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		Players: record.Players, Completed: true})
	return record, err
}

func newestFirst(aUpdated, aCreated time.Time, aID string, bUpdated, bCreated time.Time, bID string) bool {
	if !aUpdated.Equal(bUpdated) {
		return aUpdated.After(bUpdated)
	}
	if !aCreated.Equal(bCreated) {
		return aCreated.After(bCreated)
	}
	return aID < bID
}

// readRecords bounds reads and preserves malformed files for recovery.
func readRecords(dir string, visit func(string, []byte) error) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("savegame: list directory: %w", err)
	}
	warnings := make([]string, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if err := validateID(id); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		info, err := entry.Info()
		if err == nil && !info.Mode().IsRegular() {
			err = errors.New("not a regular file")
		}
		if err == nil && (info.Size() < 0 || info.Size() > MaxRecordBytes) {
			err = fmt.Errorf("exceeds %d-byte limit", MaxRecordBytes)
		}
		var data []byte
		if err == nil {
			data, err = readBounded(filepath.Join(dir, name))
		}
		if err == nil {
			err = visit(id, data)
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
		}
	}
	return warnings, nil
}

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxRecordBytes+1))
	if err == nil && len(data) > MaxRecordBytes {
		err = fmt.Errorf("exceeds %d-byte limit", MaxRecordBytes)
	}
	return data, err
}

func (s *Store) historyDir() string           { return filepath.Join(s.Dir, "history") }
func (s *Store) historyPath(id string) string { return filepath.Join(s.historyDir(), id+".json") }

func (s *Store) archiveLocked(record Record) error {
	history := HistoryRecord{Version: record.Version, ID: record.ID,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, Players: record.Players,
		Level: record.Level, Score: record.Score, Goal: record.Goal}
	payload, err := encodeRecord(history, record.ID)
	if err != nil {
		return err
	}
	// Sync the parent as well as the history directory so a newly created
	// history directory is durable before the active checkpoint is removed.
	if err := os.Mkdir(s.historyDir(), 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("savegame: create history directory: %w", err)
	}
	info, err := os.Lstat(s.historyDir())
	if err != nil {
		return fmt.Errorf("savegame: inspect history directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("savegame: history is not a directory")
	}
	if err := os.Chmod(s.historyDir(), 0700); err != nil {
		return fmt.Errorf("savegame: set history permissions: %w", err)
	}
	if err := syncDirectory(s.Dir); err != nil {
		return fmt.Errorf("savegame: sync history parent: %w", err)
	}
	if err := writeAtomic(s.historyDir(), s.historyPath(record.ID), payload); err != nil {
		return err
	}
	return removeRecord(s.Dir, s.recordPath(record.ID), record.ID)
}

func encodeRecord(record any, id string) ([]byte, error) {
	payload, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("savegame: encode %q: %w", id, err)
	}
	if len(payload) > MaxRecordBytes {
		return nil, fmt.Errorf("savegame: record %q exceeds %d-byte limit", id, MaxRecordBytes)
	}
	return payload, nil
}

func writeAtomic(dir, target string, payload []byte) error {
	tempName, err := stage(dir, payload)
	if err != nil {
		return err
	}
	defer os.Remove(tempName)
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("savegame: install %q: %w", target, err)
	}
	if err := syncDirectory(dir); err != nil {
		return fmt.Errorf("savegame: sync directory after %q: %w", target, err)
	}
	return nil
}

func removeRecord(dir, path, id string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("savegame: delete %q: %w", id, err)
	}
	// Sync even an absent target: this also makes a retry after a failed
	// directory sync durable. An absent directory has no deletion to sync.
	if err := syncDirectory(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("savegame: sync directory after deleting %q: %w", id, err)
	}
	return nil
}

func (s *Store) recordPath(id string) string {
	return filepath.Join(s.Dir, id+".json")
}

func stage(dir string, payload []byte) (string, error) {
	temp, err := os.CreateTemp(dir, ".savegame-*")
	if err != nil {
		return "", fmt.Errorf("savegame: create temporary file: %w", err)
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = temp.Close()
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0600); err != nil {
		return "", fmt.Errorf("savegame: set temporary file permissions: %w", err)
	}
	if _, err := temp.Write(payload); err != nil {
		return "", fmt.Errorf("savegame: write temporary file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return "", fmt.Errorf("savegame: sync temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("savegame: close temporary file: %w", err)
	}
	cleanup = false
	return tempName, nil
}

func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func validateRecord(record Record) error {
	if record.Version != 1 {
		return fmt.Errorf("savegame: record %q has unsupported version %d", record.ID, record.Version)
	}
	if err := validateID(record.ID); err != nil {
		return err
	}
	if record.Players != 1 && record.Players != 2 {
		return fmt.Errorf("savegame: record %q must have 1 or 2 players", record.ID)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return fmt.Errorf("savegame: record %q must have non-zero timestamps", record.ID)
	}
	if !record.Completed && (len(record.Checkpoint) == 0 || len(strings.TrimSpace(string(record.Checkpoint))) == 0 || !json.Valid(record.Checkpoint)) {
		return fmt.Errorf("savegame: record %q has invalid checkpoint JSON", record.ID)
	}
	return nil
}

func validateID(id string) error {
	if id == "" {
		return errors.New("savegame: empty ID")
	}
	for i, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		if r == '-' && i > 0 && i < len(id)-1 {
			continue
		}
		return fmt.Errorf("savegame: unsafe ID %q", id)
	}
	return nil
}
