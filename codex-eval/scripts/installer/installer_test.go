package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureBackupRejectsMissingEmptyAndDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := ensureBackup(filepath.Join(directory, "missing")); err == nil {
		t.Fatal("missing backup was accepted")
	}
	empty := filepath.Join(directory, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureBackup(empty); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty backup error = %v", err)
	}
	if err := ensureBackup(directory); err == nil {
		t.Fatal("directory was accepted as a backup")
	}
	valid := filepath.Join(directory, "backup")
	if err := os.WriteFile(valid, []byte("not-an-elf-but-non-empty"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureBackup(valid); err != nil {
		t.Fatalf("valid backup rejected: %v", err)
	}
}

func TestCheckExecutableRejectsTextFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate")
	if err := os.WriteFile(path, []byte("plain text"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkExecutable(path, "linux", "amd64"); err == nil {
		t.Fatal("text file was accepted as a Linux executable")
	}
}

func TestParseVersion(t *testing.T) {
	version, err := parseVersion("Sub2API 0.2.21 (commit: abcdef, built: 2026-11-13T10:00:00Z)\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if version != "0.2.21" {
		t.Fatalf("version = %q", version)
	}
}

func TestParseVersionRejectsMissingRecord(t *testing.T) {
	if _, err := parseVersion("starting server\n", ""); err == nil {
		t.Fatal("missing version record was accepted")
	}
}
