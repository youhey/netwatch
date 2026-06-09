package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameGroupInJSONLFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples-2026-06-09.jsonl")
	content := strings.Join([]string{
		`{"ts":"2026-06-09T12:00:00+09:00","type":"http","name":"riot_status","group":"pcgame","category":"game","ok":true}`,
		`{"ts":"2026-06-09T12:00:00+09:00","type":"http","name":"youtube_home","group":"youtube","category":"service","ok":true}`,
		"\x00bad-json",
		`{"ts":"2026-06-09T12:01:00+09:00","type":"http","name":"ea_status","group":"pcgame","extra":"kept"}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := RenameGroupInJSONLFile(path, "pcgame", "pc_game", JSONLGroupRenameOptions{BackupSuffix: ".bak"})
	if err != nil {
		t.Fatalf("RenameGroupInJSONLFile() error = %v", err)
	}
	if result.Lines != 4 || result.Changed != 2 || result.InvalidLines != 1 || result.BackupPath != path+".bak" {
		t.Fatalf("result = %+v, want line/change/invalid counts", result)
	}

	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(gotBytes)
	if strings.Count(got, `"group":"pc_game"`) != 2 {
		t.Fatalf("rewritten content = %q, want two pc_game groups", got)
	}
	if !strings.Contains(got, `"group":"youtube"`) {
		t.Fatalf("rewritten content = %q, want unrelated group preserved", got)
	}
	if !strings.Contains(got, "\x00bad-json") {
		t.Fatalf("rewritten content = %q, want invalid line preserved", got)
	}
	if !strings.Contains(got, `"extra":"kept"`) {
		t.Fatalf("rewritten content = %q, want extra field preserved", got)
	}

	backupBytes, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(backupBytes) != content {
		t.Fatalf("backup = %q, want original content", string(backupBytes))
	}
}

func TestRenameGroupInJSONLFileDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples-2026-06-09.jsonl")
	content := `{"type":"http","name":"riot_status","group":"pcgame"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := RenameGroupInJSONLFile(path, "pcgame", "pc_game", JSONLGroupRenameOptions{DryRun: true, BackupSuffix: ".bak"})
	if err != nil {
		t.Fatalf("RenameGroupInJSONLFile() error = %v", err)
	}
	if result.Changed != 1 || result.BackupPath != "" {
		t.Fatalf("result = %+v, want dry-run count without backup", result)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != content {
		t.Fatalf("content = %q, want unchanged", string(got))
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup exists or stat error = %v, want no backup", err)
	}
}

func TestRenameGroupInJSONLDataDir(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "samples-2026-06-08.jsonl")
	second := filepath.Join(dir, "samples-2026-06-09.jsonl")
	if err := os.WriteFile(first, []byte(`{"group":"pcgame"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(first) error = %v", err)
	}
	if err := os.WriteFile(second, []byte(`{"group":"youtube"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(second) error = %v", err)
	}

	result, err := RenameGroupInJSONLDataDir(dir, "samples-%Y-%m-%d.jsonl", "pcgame", "pc_game", JSONLGroupRenameOptions{DryRun: true})
	if err != nil {
		t.Fatalf("RenameGroupInJSONLDataDir() error = %v", err)
	}
	if len(result.Files) != 2 || result.TotalChanged != 1 {
		t.Fatalf("result = %+v, want two files and one changed line", result)
	}
}
