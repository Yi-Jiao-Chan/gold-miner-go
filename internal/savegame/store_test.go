package savegame

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testRecord(id string, updated time.Time, completed bool) Record {
	return Record{
		Version:    1,
		ID:         id,
		CreatedAt:  updated.Add(-time.Hour),
		UpdatedAt:  updated,
		Players:    1,
		Level:      2,
		Score:      10.5,
		Goal:       20,
		Completed:  completed,
		Checkpoint: json.RawMessage(`{"x":1,"opaque":[true,"value"]}`),
	}
}

func mustStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRestartRoundTripAndUpsert(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 10, 12, 10, 0, 0, 123, time.UTC)
	want := testRecord("campaign-1", when, false)
	if err := s.Put(want); err != nil {
		t.Fatal(err)
	}
	want.UpdatedAt = when.Add(time.Minute)
	want.Score = 99
	want.Checkpoint = json.RawMessage(`{"progress":7}`)
	if err := s.Put(want); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.List()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []Record{want}) {
		t.Fatalf("round trip/upsert mismatch:\n got %#v\nwant %#v", got, []Record{want})
	}
}

func TestFullStoreRefusesUnfinishedOverflow(t *testing.T) {
	s := mustStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < MaxRecords; i++ {
		if err := s.Put(testRecord("unfinished-"+itoa(i), base.Add(time.Duration(i)*time.Second), false)); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	if err := s.Put(testRecord("overflow", base.Add(24*time.Hour), false)); !errors.Is(err, ErrFull) {
		t.Fatalf("overflow error = %v, want ErrFull", err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxRecords {
		t.Fatalf("record count after refused overflow = %d, want %d", len(got), MaxRecords)
	}
}

func TestHistoryDoesNotConsumeActiveCapacity(t *testing.T) {
	s := mustStore(t)
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < MaxRecords+20; i++ {
		if err := s.Put(testRecord("finished-"+itoa(i), base.Add(time.Duration(i)*time.Second), true)); err != nil {
			t.Fatalf("archive %d: %v", i, err)
		}
	}
	for i := 0; i < MaxRecords; i++ {
		if err := s.Put(testRecord("unfinished-"+itoa(i), base.Add(time.Duration(i)*time.Second), false)); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	// Completing another campaign remains possible even with 100 active saves.
	if err := s.Put(testRecord("another-finished", base, true)); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(testRecord("overflow", base, false)); !errors.Is(err, ErrFull) {
		t.Fatalf("overflow error = %v, want ErrFull", err)
	}
	active, err := s.List()
	if err != nil || len(active) != MaxRecords {
		t.Fatalf("active count = %d, err = %v", len(active), err)
	}
	history, err := s.History()
	if err != nil || len(history) != MaxRecords+21 {
		t.Fatalf("history count = %d, err = %v", len(history), err)
	}
}

func TestExplicitDeleteFreesCapacity(t *testing.T) {
	s := mustStore(t)
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < MaxRecords; i++ {
		if err := s.Put(testRecord("campaign-"+itoa(i), base.Add(time.Duration(i)*time.Second), false)); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	if err := s.Delete("campaign-0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(testRecord("after-delete", base.Add(24*time.Hour), false)); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptFileSkippedAndWarned(t *testing.T) {
	s := mustStore(t)
	good := testRecord("good", time.Now().UTC(), false)
	if err := s.Put(good); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(s.Dir, "broken.json")
	if err := os.WriteFile(badPath, []byte("{not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []Record{good}) {
		t.Fatalf("records = %#v, want only good record", got)
	}
	if len(s.Warnings) == 0 || !strings.Contains(strings.Join(s.Warnings, "\n"), "broken.json") {
		t.Fatalf("warnings = %#v, want broken.json warning", s.Warnings)
	}
	if data, err := os.ReadFile(badPath); err != nil || string(data) != "{not-json" {
		t.Fatalf("corrupt file changed: data=%q err=%v", data, err)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	s := mustStore(t)
	when := time.Now().UTC()
	for _, id := range []string{"../escape", "a/b", "a\\b", "", ".", "-bad", "bad-"} {
		record := testRecord(id, when, false)
		if err := s.Put(record); err == nil {
			t.Errorf("Put(%q) succeeded", id)
		}
		if err := s.Delete(id); err == nil {
			t.Errorf("Delete(%q) succeeded", id)
		}
		record.Completed = true
		if err := s.Put(record); err == nil {
			t.Errorf("completed Put(%q) succeeded", id)
		}
		if err := s.DeleteHistory(id); err == nil {
			t.Errorf("DeleteHistory(%q) succeeded", id)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.Dir), "escape.json")); !os.IsNotExist(err) {
		t.Fatalf("path traversal created outside file: %v", err)
	}
}

func TestListReturnsOnlyUnfinishedRecords(t *testing.T) {
	s := mustStore(t)
	when := time.Now().UTC()
	finished := testRecord("finished", when, true)
	unfinished := testRecord("unfinished", when.Add(time.Second), false)
	if err := s.Put(finished); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(unfinished); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []Record{unfinished}) {
		t.Fatalf("List = %#v, want unfinished only", got)
	}
}

func TestFileAndDirectoryPermissions(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("Unix permission test")
	}
	s := mustStore(t)
	if err := s.Put(testRecord("permissions", time.Now().UTC(), false)); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0700 {
		t.Errorf("directory mode = %o, want 700", mode)
	}
	fileInfo, err := os.Stat(filepath.Join(s.Dir, "permissions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := fileInfo.Mode().Perm(); mode != 0600 {
		t.Errorf("file mode = %o, want 600", mode)
	}
}

func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var out [20]byte
	i := len(out)
	for n > 0 {
		i--
		out[i] = digits[n%10]
		n /= 10
	}
	return string(out[i:])
}
