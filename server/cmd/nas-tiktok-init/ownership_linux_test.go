//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDecodeMountPath(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: `/workspace/my\040media`, want: "/workspace/my media"},
		{input: `/workspace/tab\011line\012slash\134name`, want: "/workspace/tab\tline\nslash\\name"},
	} {
		got, err := decodeMountPath(test.input)
		if err != nil || got != test.want {
			t.Errorf("decodeMountPath(%q) = %q, %v; want %q, nil", test.input, got, err, test.want)
		}
	}
	for _, input := range []string{`/workspace/bad\04`, `/workspace/bad\09x`, `/workspace/bad\08x`} {
		if _, err := decodeMountPath(input); err == nil {
			t.Errorf("decodeMountPath(%q) succeeded; want malformed escape error", input)
		}
	}
}

func TestDatabaseFileNameResolvesTrimmedRelativePathAndRejectsOutsidePath(t *testing.T) {
	workspace := t.TempDir()
	databaseDir := filepath.Join(workspace, "db")
	if err := os.Mkdir(databaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(databaseDir, "custom.sqlite")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativePath, err := filepath.Rel(workingDirectory, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_PATH", " \t"+relativePath+" \n")
	if got, ok := databaseFileName(workspace); !ok || got != "custom.sqlite" {
		t.Fatalf("relative trimmed DB_PATH = %q, %v; want custom.sqlite, true", got, ok)
	}

	t.Setenv("DB_PATH", filepath.Join(workspace, "outside.sqlite"))
	if got, ok := databaseFileName(workspace); ok {
		t.Fatalf("outside DB_PATH accepted as %q", got)
	}

	t.Setenv("DB_PATH", " \t ")
	if got, ok := databaseFileName(workspace); !ok || got != "nas-tiktok.db" {
		t.Fatalf("empty DB_PATH fallback = %q, %v; want nas-tiktok.db, true", got, ok)
	}
}

func TestRepairWorkspaceTreeRejectsSymlinksAndSkipsMountsAndProtectedPaths(t *testing.T) {
	uid, gid := os.Getuid(), os.Getgid()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("rejects symlink without following it", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(workspace, "linked")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		fd, err := openAbsoluteDirectoryNoSymlinks(workspace)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fd)

		changed, err := repairWorkspaceTree(fd, workspace, nil, map[string]workspaceMount{}, uid, gid, false)
		if err == nil || changed {
			t.Fatalf("symlink walk = changed %v, err %v; want an error without changes", changed, err)
		}
	})

	t.Run("skips mounts and protected paths before opening entries", func(t *testing.T) {
		workspace := t.TempDir()
		mountedPath := filepath.Join(workspace, "mounted")
		protectedPath := filepath.Join(workspace, "statics")
		for _, directory := range []string{mountedPath, protectedPath} {
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(directory, "linked")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}
		fd, err := openAbsoluteDirectoryNoSymlinks(workspace)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fd)

		mounts := map[string]workspaceMount{mountedPath: {seen: true, writable: true}}
		changed, err := repairWorkspaceTree(fd, workspace, []string{protectedPath}, mounts, uid, gid, false)
		if err != nil || changed {
			t.Fatalf("excluded-path walk = changed %v, err %v; want no changes or error", changed, err)
		}
	})
}

func TestPrepareWorkspaceSkipsTreeWhenMarkerAndDatabaseOwnershipMatch(t *testing.T) {
	workspace := t.TempDir()
	databaseDir := filepath.Join(workspace, "db")
	if err := os.Mkdir(databaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(databaseDir, "test.sqlite")
	if err := os.WriteFile(databasePath, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(workspace, "unexpected-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("DB_PATH", databasePath)
	t.Setenv("MEDIA_ROOT", filepath.Join(workspace, "statics"))
	t.Setenv("TLS_CERT_FILE", "")
	t.Setenv("TLS_KEY_FILE", "")

	changed, err := prepareWorkspace(workspace, os.Getuid(), os.Getgid())
	if err != nil || changed {
		t.Fatalf("prepareWorkspace with matching ownership = changed %v, err %v; want no-op", changed, err)
	}
}
