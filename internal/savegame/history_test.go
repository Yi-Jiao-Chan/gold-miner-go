package savegame

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func summary(record Record) HistoryRecord {
	return HistoryRecord{Version: record.Version, ID: record.ID,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		Players: record.Players, Level: record.Level, Score: record.Score, Goal: record.Goal}
}

func writeLegacy(t *testing.T, dir string, record Record) []byte {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, record.ID+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCompletionArchivesSummaryAndSurvivesRestart(t *testing.T) {
	s := mustStore(t)
	record := testRecord("completed", time.Now().UTC(), false)
	if err := s.Put(record); err != nil {
		t.Fatal(err)
	}
	record.Completed = true
	record.Checkpoint = nil // Completed metadata does not need a checkpoint.
	record.UpdatedAt = record.UpdatedAt.Add(time.Minute)
	if err := s.Put(record); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.recordPath(record.ID)); !os.IsNotExist(err) {
		t.Fatalf("active checkpoint remains: %v", err)
	}
	data, err := os.ReadFile(s.historyPath(record.ID))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 8 || fields["checkpoint"] != nil || fields["completed"] != nil {
		t.Fatalf("unexpected summary fields: %s", data)
	}
	restarted, err := Open(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	active, err := restarted.List()
	if err != nil || len(active) != 0 {
		t.Fatalf("active = %#v, error = %v", active, err)
	}
	history, err := restarted.History()
	if err != nil || !reflect.DeepEqual(history, []HistoryRecord{summary(record)}) {
		t.Fatalf("history = %#v, error = %v", history, err)
	}
	// An archived checkpoint cannot be accidentally resurrected.
	record.Completed = false
	record.Checkpoint = json.RawMessage(`{}`)
	if err := restarted.Put(record); err == nil {
		t.Fatal("unfinished Put for an archived ID succeeded")
	}
}

func TestLegacyCompletedMigrationAndCorruptionPreservation(t *testing.T) {
	dir := t.TempDir()
	when := time.Now().UTC()
	finished := testRecord("legacy-finished", when, true)
	finished.Checkpoint = nil
	unfinished := testRecord("legacy-active", when.Add(time.Second), false)
	writeLegacy(t, dir, finished)
	writeLegacy(t, dir, unfinished)
	brokenPath := filepath.Join(dir, "legacy-broken.json")
	broken := []byte(`{"id":"legacy-broken","completed":true`)
	if err := os.WriteFile(brokenPath, broken, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Warnings) != 1 || !strings.Contains(s.Warnings[0], "legacy-broken.json") {
		t.Fatalf("migration warnings = %#v", s.Warnings)
	}
	if data, err := os.ReadFile(brokenPath); err != nil || !reflect.DeepEqual(data, broken) {
		t.Fatalf("corrupted legacy file changed: %q, %v", data, err)
	}
	if _, err := os.Stat(s.recordPath(finished.ID)); !os.IsNotExist(err) {
		t.Fatalf("completed legacy checkpoint remains: %v", err)
	}
	active, err := s.List()
	if err != nil || !reflect.DeepEqual(active, []Record{unfinished}) {
		t.Fatalf("migrated active = %#v, error = %v", active, err)
	}
	history, err := s.History()
	if err != nil || !reflect.DeepEqual(history, []HistoryRecord{summary(finished)}) {
		t.Fatalf("migrated history = %#v, error = %v", history, err)
	}

	// Recreate the state after a successful archive but before a successful
	// checkpoint removal. List must safely retry without duplicate histories.
	writeLegacy(t, dir, finished)
	if _, err := s.List(); err != nil {
		t.Fatal(err)
	}
	history, err = s.History()
	if err != nil || !reflect.DeepEqual(history, []HistoryRecord{summary(finished)}) {
		t.Fatalf("retried history = %#v, error = %v", history, err)
	}
	if _, err := os.Stat(s.recordPath(finished.ID)); !os.IsNotExist(err) {
		t.Fatalf("retried migration left active: %v", err)
	}
}

func TestArchiveFailurePreservesActiveCheckpoint(t *testing.T) {
	for _, block := range []string{"directory", "install"} {
		t.Run(block, func(t *testing.T) {
			s := mustStore(t)
			record := testRecord("protected", time.Now().UTC(), false)
			if err := s.Put(record); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(s.recordPath(record.ID))
			if err != nil {
				t.Fatal(err)
			}
			if block == "directory" {
				if err := os.WriteFile(s.historyDir(), []byte("block history"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(s.historyPath(record.ID), 0700); err != nil {
					t.Fatal(err)
				}
			}
			record.Completed = true
			if err := s.Put(record); err == nil {
				t.Fatal("archive with blocked target succeeded")
			}
			after, err := os.ReadFile(s.recordPath(record.ID))
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("active checkpoint changed after failed archive: %q, %v", after, err)
			}
		})
	}
}

func TestMigrationArchiveFailurePreservesLegacyCheckpoint(t *testing.T) {
	s := mustStore(t)
	record := testRecord("legacy-protected", time.Now().UTC(), true)
	before := writeLegacy(t, s.Dir, record)
	if err := os.WriteFile(s.historyDir(), []byte("block history"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil {
		t.Fatal("migration with blocked history succeeded")
	}
	after, err := os.ReadFile(s.recordPath(record.ID))
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("legacy checkpoint changed after failed archive: %q, %v", after, err)
	}
	if err := os.Remove(s.historyDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err != nil {
		t.Fatalf("migration retry: %v", err)
	}
	if _, err := os.Stat(s.recordPath(record.ID)); !os.IsNotExist(err) {
		t.Fatalf("legacy checkpoint remains after retry: %v", err)
	}
}

func TestArchiveRemovalFailureCanRetry(t *testing.T) {
	s := mustStore(t)
	record := testRecord("retry", time.Now().UTC(), true)
	// A nonempty directory deterministically makes active-path removal fail.
	if err := os.MkdirAll(s.recordPath(record.ID), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.recordPath(record.ID), "block"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(record); err == nil {
		t.Fatal("completion with blocked removal succeeded")
	}
	data, err := os.ReadFile(s.historyPath(record.ID))
	if err != nil {
		t.Fatalf("archive missing after failed removal: %v", err)
	}
	var got HistoryRecord
	if err := json.Unmarshal(data, &got); err != nil || !reflect.DeepEqual(got, summary(record)) {
		t.Fatalf("archive = %#v, error = %v", got, err)
	}
	if err := os.RemoveAll(s.recordPath(record.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(record); err != nil {
		t.Fatalf("completion retry failed: %v", err)
	}
	history, err := s.History()
	if err != nil || len(history) != 1 {
		t.Fatalf("retried history = %#v, error = %v", history, err)
	}
}

func TestDeleteHistoryIsIndependentAndIdempotent(t *testing.T) {
	s := mustStore(t)
	when := time.Now().UTC()
	if err := s.Put(testRecord("active", when, false)); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(testRecord("finished", when, true)); err != nil {
		t.Fatal(err)
	}
	// Active deletion leaves completed summaries alone.
	if err := s.Delete("finished"); err != nil {
		t.Fatal(err)
	}
	before, err := s.History()
	if err != nil || len(before) != 1 {
		t.Fatalf("history after active deletion = %#v, error = %v", before, err)
	}
	for i := 0; i < 2; i++ {
		if err := s.DeleteHistory("finished"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteHistory("active"); err != nil {
		t.Fatal(err)
	}
	history, err := s.History()
	if err != nil || len(history) != 0 {
		t.Fatalf("history after deletion = %#v, error = %v", history, err)
	}
	active, err := s.List()
	if err != nil || len(active) != 1 || active[0].ID != "active" {
		t.Fatalf("active after history deletion = %#v, error = %v", active, err)
	}
}

func TestHistoryPermissionsAndCorruptionPreservation(t *testing.T) {
	s := mustStore(t)
	when := time.Now().UTC()
	older := testRecord("older", when, true)
	newer := testRecord("newer", when.Add(time.Minute), true)
	for _, record := range []Record{older, newer} {
		if err := s.Put(record); err != nil {
			t.Fatal(err)
		}
	}
	broken := filepath.Join(s.historyDir(), "broken.json")
	if err := os.WriteFile(broken, []byte("bad-json"), 0600); err != nil {
		t.Fatal(err)
	}
	history, err := s.History()
	if err != nil || !reflect.DeepEqual(history, []HistoryRecord{summary(newer), summary(older)}) {
		t.Fatalf("history order/content = %#v, error = %v", history, err)
	}
	if len(s.Warnings) != 1 || !strings.Contains(s.Warnings[0], "history/broken.json") {
		t.Fatalf("history warnings = %#v", s.Warnings)
	}
	if data, err := os.ReadFile(broken); err != nil || string(data) != "bad-json" {
		t.Fatalf("corrupted history changed: %q, %v", data, err)
	}
	if os.PathSeparator != '/' {
		t.Skip("Unix permission test")
	}
	for path, want := range map[string]os.FileMode{s.historyDir(): 0700, s.historyPath(older.ID): 0600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != want {
			t.Errorf("%s permissions = %o, want %o", path, mode, want)
		}
	}
}

func TestRecoveryRemovesUnfinishedCheckpointAfterFinalArchive(t *testing.T) {
	s := mustStore(t)
	active := testRecord("crash-recovery", time.Now().UTC(), false)
	if err := s.Put(active); err != nil {
		t.Fatal(err)
	}
	final := active
	final.Completed = true
	final.Checkpoint = nil
	final.UpdatedAt = final.UpdatedAt.Add(time.Hour)
	final.Level, final.Score, final.Goal = 8, 900, 850
	if err := s.Put(final); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.historyPath(final.ID))
	if err != nil {
		t.Fatal(err)
	}
	// A real crash between archive sync and active removal leaves the prior
	// checkpoint unfinished, with stale score and timestamps.
	writeLegacy(t, s.Dir, active)
	restarted, err := Open(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.List()
	if err != nil || len(got) != 0 {
		t.Fatalf("recovered active saves = %#v, error = %v", got, err)
	}
	if _, err := os.Stat(s.recordPath(active.ID)); !os.IsNotExist(err) {
		t.Fatalf("stale checkpoint remains after recovery: %v", err)
	}
	history, err := restarted.History()
	if err != nil || !reflect.DeepEqual(history, []HistoryRecord{summary(final)}) {
		t.Fatalf("recovered history = %#v, error = %v", history, err)
	}
	after, err := os.ReadFile(s.historyPath(final.ID))
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("recovery changed final history bytes: %q, error = %v", after, err)
	}
}

func TestRecoveryPreservesActiveWhenMatchingHistoryIsInvalid(t *testing.T) {
	for _, invalid := range []string{"corrupt", "mismatched", "unreadable"} {
		t.Run(invalid, func(t *testing.T) {
			s := mustStore(t)
			active := testRecord("protected-recovery", time.Now().UTC(), false)
			final := active
			final.Completed = true
			if err := s.Put(final); err != nil {
				t.Fatal(err)
			}
			switch invalid {
			case "corrupt":
				if err := os.WriteFile(s.historyPath(active.ID), []byte("{broken"), 0600); err != nil {
					t.Fatal(err)
				}
			case "mismatched":
				wrong := summary(final)
				wrong.ID = "another-game"
				data, err := json.Marshal(wrong)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(s.historyPath(active.ID), data, 0600); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if err := os.Remove(s.historyPath(active.ID)); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(s.historyPath(active.ID), 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := writeLegacy(t, s.Dir, active)
			restarted, err := Open(s.Dir)
			if err != nil {
				t.Fatal(err)
			}
			got, err := restarted.List()
			if err != nil || !reflect.DeepEqual(got, []Record{active}) {
				t.Fatalf("active with invalid history = %#v, error = %v", got, err)
			}
			if len(restarted.Warnings) == 0 || !strings.Contains(strings.Join(restarted.Warnings, "\n"), "history/protected-recovery.json") {
				t.Fatalf("missing history recovery warning: %#v", restarted.Warnings)
			}
			after, err := os.ReadFile(s.recordPath(active.ID))
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("recovery changed active checkpoint: %q, error = %v", after, err)
			}
		})
	}
}
