//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type workspaceMount struct {
	writable bool
	seen     bool
}

// prepareWorkspace repairs image-owned workspace entries without following
// links or crossing mounts. The workspace directory owner is the completion
// marker and is changed only after a successful traversal.
func prepareWorkspace(workspace string, uid, gid int) (bool, error) {
	if uid < 0 || gid < 0 || uint64(uid) > uint64(^uint32(0)) || uint64(gid) > uint64(^uint32(0)) {
		return false, fmt.Errorf("invalid target uid:gid %d:%d", uid, gid)
	}

	workspacePath, err := filepath.Abs(workspace)
	if err != nil {
		return false, fmt.Errorf("resolve workspace path: %w", err)
	}
	workspacePath = filepath.Clean(workspacePath)
	if workspacePath == string(filepath.Separator) {
		return false, errors.New("refusing to initialize filesystem root")
	}
	workspaceFD, err := openAbsoluteDirectoryNoSymlinks(workspacePath)
	if err != nil {
		return false, fmt.Errorf("open workspace: %w", err)
	}
	defer unix.Close(workspaceFD)
	workspaceInfo, err := statFD(workspaceFD)
	if err != nil {
		return false, fmt.Errorf("stat workspace: %w", err)
	}

	databaseDir := filepath.Join(workspacePath, "db")
	databaseDirFD, err := openDirectoryAtNoFollow(workspaceFD, "db")
	if err != nil {
		return false, fmt.Errorf("open database directory: %w", err)
	}
	defer unix.Close(databaseDirFD)
	databaseDirInfo, err := statFD(databaseDirFD)
	if err != nil {
		return false, fmt.Errorf("stat database directory: %w", err)
	}
	databaseName, hasDatabasePath := databaseFileName(workspacePath)
	if ownershipMatches(workspaceInfo, uid, gid) && ownershipMatches(databaseDirInfo, uid, gid) {
		if !hasDatabasePath {
			return false, nil
		}
		databaseFilesMatch, err := databaseFilesOwnedBy(databaseDirFD, databaseDir, databaseName, uid, gid)
		if err != nil {
			return false, err
		}
		if databaseFilesMatch {
			return false, nil
		}
	}

	mounts, err := readWorkspaceMounts()
	if err != nil {
		return false, fmt.Errorf("read mount table: %w", err)
	}
	if mounts[workspacePath].seen {
		if mounts[databaseDir].seen && mounts[databaseDir].writable && hasDatabasePath {
			return repairDatabaseVolume(databaseDirFD, databaseDir, databaseName, mounts, uid, gid)
		}
		return false, nil
	}

	protectedPaths := protectedWorkspacePaths(workspacePath)
	changed, err := repairWorkspaceTree(workspaceFD, workspacePath, protectedPaths, mounts, uid, gid, false)
	if err != nil {
		return changed, err
	}
	if mounts[databaseDir].seen && mounts[databaseDir].writable && hasDatabasePath {
		databaseChanged, err := repairDatabaseVolume(databaseDirFD, databaseDir, databaseName, mounts, uid, gid)
		changed = changed || databaseChanged
		if err != nil {
			return changed, err
		}
	}

	rootChanged, err := chownIfNeeded(workspaceFD, workspaceInfo, uid, gid)
	if err != nil {
		return changed, fmt.Errorf("update workspace completion marker: %w", err)
	}
	return changed || rootChanged, nil
}

func repairWorkspaceTree(directoryFD int, absolutePath string, protected []string, mounts map[string]workspaceMount, uid, gid int, updateDirectory bool) (bool, error) {
	changed := false
	if updateDirectory {
		info, err := statFD(directoryFD)
		if err != nil {
			return false, fmt.Errorf("stat %q: %w", absolutePath, err)
		}
		didChange, err := chownIfNeeded(directoryFD, info, uid, gid)
		changed = changed || didChange
		if err != nil {
			return changed, fmt.Errorf("chown %q: %w", absolutePath, err)
		}
	}

	readFD, err := unix.Openat(directoryFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return changed, fmt.Errorf("open directory %q for reading: %w", absolutePath, err)
	}
	directory := os.NewFile(uintptr(readFD), absolutePath)
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return changed, fmt.Errorf("read directory %q: %w", absolutePath, readErr)
	}
	if closeErr != nil {
		return changed, fmt.Errorf("close directory %q: %w", absolutePath, closeErr)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(absolutePath, entry.Name())
		if mounts[entryPath].seen || isProtectedWorkspacePath(entryPath, protected) {
			continue
		}
		entryFD, err := unix.Openat(directoryFD, entry.Name(), unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return changed, fmt.Errorf("open entry %q without following links: %w", entryPath, err)
		}
		info, err := statFD(entryFD)
		if err != nil {
			_ = unix.Close(entryFD)
			return changed, fmt.Errorf("stat %q: %w", entryPath, err)
		}

		switch info.Mode & unix.S_IFMT {
		case unix.S_IFLNK:
			_ = unix.Close(entryFD)
			return changed, fmt.Errorf("refusing symlink in workspace tree: %q", entryPath)
		case unix.S_IFDIR:
			didChange, walkErr := repairWorkspaceTree(entryFD, entryPath, protected, mounts, uid, gid, true)
			closeErr := unix.Close(entryFD)
			changed = changed || didChange
			if walkErr != nil {
				return changed, walkErr
			}
			if closeErr != nil {
				return changed, fmt.Errorf("close directory %q: %w", entryPath, closeErr)
			}
		case unix.S_IFREG:
			if info.Nlink != 1 {
				_ = unix.Close(entryFD)
				return changed, fmt.Errorf("refusing hard-linked workspace file %q (link count %d)", entryPath, info.Nlink)
			}
			didChange, chownErr := chownIfNeeded(entryFD, info, uid, gid)
			closeErr := unix.Close(entryFD)
			changed = changed || didChange
			if chownErr != nil {
				return changed, fmt.Errorf("chown %q: %w", entryPath, chownErr)
			}
			if closeErr != nil {
				return changed, fmt.Errorf("close file %q: %w", entryPath, closeErr)
			}
		default:
			_ = unix.Close(entryFD)
			return changed, fmt.Errorf("refusing unsupported workspace entry %q (mode %#o)", entryPath, info.Mode)
		}
	}
	return changed, nil
}

func repairDatabaseVolume(databaseDirFD int, databaseDir, databaseName string, mounts map[string]workspaceMount, uid, gid int) (bool, error) {
	changed := false
	directoryInfo, err := statFD(databaseDirFD)
	if err != nil {
		return false, fmt.Errorf("stat database directory %q: %w", databaseDir, err)
	}
	directoryChanged, err := chownIfNeeded(databaseDirFD, directoryInfo, uid, gid)
	changed = changed || directoryChanged
	if err != nil {
		return changed, fmt.Errorf("chown database directory %q: %w", databaseDir, err)
	}

	for _, name := range []string{databaseName, databaseName + "-wal", databaseName + "-shm", databaseName + "-journal"} {
		entryPath := filepath.Join(databaseDir, name)
		if mounts[entryPath].seen {
			continue
		}
		entryFD, err := unix.Openat(databaseDirFD, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return changed, fmt.Errorf("open database file %q without following links: %w", entryPath, err)
		}
		info, err := statFD(entryFD)
		if err != nil {
			_ = unix.Close(entryFD)
			return changed, fmt.Errorf("stat database file %q: %w", entryPath, err)
		}
		if info.Mode&unix.S_IFMT != unix.S_IFREG {
			_ = unix.Close(entryFD)
			return changed, fmt.Errorf("refusing non-regular database file %q", entryPath)
		}
		if info.Nlink != 1 {
			_ = unix.Close(entryFD)
			return changed, fmt.Errorf("refusing hard-linked database file %q (link count %d)", entryPath, info.Nlink)
		}
		didChange, chownErr := chownIfNeeded(entryFD, info, uid, gid)
		closeErr := unix.Close(entryFD)
		changed = changed || didChange
		if chownErr != nil {
			return changed, fmt.Errorf("chown database file %q: %w", entryPath, chownErr)
		}
		if closeErr != nil {
			return changed, fmt.Errorf("close database file %q: %w", entryPath, closeErr)
		}
	}

	return changed, nil
}

func databaseFilesOwnedBy(databaseDirFD int, databaseDir, databaseName string, uid, gid int) (bool, error) {
	for _, name := range []string{databaseName, databaseName + "-wal", databaseName + "-shm", databaseName + "-journal"} {
		entryPath := filepath.Join(databaseDir, name)
		entryFD, err := unix.Openat(databaseDirFD, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("open database file %q without following links: %w", entryPath, err)
		}
		info, err := statFD(entryFD)
		_ = unix.Close(entryFD)
		if err != nil {
			return false, fmt.Errorf("stat database file %q: %w", entryPath, err)
		}
		if info.Mode&unix.S_IFMT != unix.S_IFREG {
			return false, fmt.Errorf("refusing non-regular database file %q", entryPath)
		}
		if info.Nlink != 1 {
			return false, fmt.Errorf("refusing hard-linked database file %q (link count %d)", entryPath, info.Nlink)
		}
		if !ownershipMatches(info, uid, gid) {
			return false, nil
		}
	}
	return true, nil
}

func chownIfNeeded(fd int, info *unix.Stat_t, uid, gid int) (bool, error) {
	if ownershipMatches(info, uid, gid) {
		return false, nil
	}
	if info.Mode&unix.S_IFMT == unix.S_IFLNK {
		return false, errors.New("refusing to chown symlink")
	}
	if err := unix.Fchownat(fd, "", uid, gid, unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false, err
	}
	return true, nil
}

func ownershipMatches(info *unix.Stat_t, uid, gid int) bool {
	return info.Uid == uint32(uid) && info.Gid == uint32(gid)
}

func statFD(fd int) (*unix.Stat_t, error) {
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func openAbsoluteDirectoryNoSymlinks(absolutePath string) (int, error) {
	if !filepath.IsAbs(absolutePath) {
		return -1, fmt.Errorf("path %q is not absolute", absolutePath)
	}
	currentFD, err := unix.Open(string(filepath.Separator), unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if absolutePath == string(filepath.Separator) {
		return currentFD, nil
	}
	for _, segment := range strings.Split(strings.TrimPrefix(absolutePath, string(filepath.Separator)), string(filepath.Separator)) {
		if segment == "" || segment == "." || segment == ".." {
			_ = unix.Close(currentFD)
			return -1, fmt.Errorf("unsafe path segment in %q", absolutePath)
		}
		nextFD, openErr := unix.Openat(currentFD, segment, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if openErr != nil {
			_ = unix.Close(currentFD)
			return -1, openErr
		}
		info, statErr := statFD(nextFD)
		if statErr != nil {
			_ = unix.Close(nextFD)
			_ = unix.Close(currentFD)
			return -1, statErr
		}
		if info.Mode&unix.S_IFMT == unix.S_IFLNK {
			_ = unix.Close(nextFD)
			_ = unix.Close(currentFD)
			return -1, fmt.Errorf("refusing symlink path component %q", segment)
		}
		if info.Mode&unix.S_IFMT != unix.S_IFDIR {
			_ = unix.Close(nextFD)
			_ = unix.Close(currentFD)
			return -1, fmt.Errorf("path component %q is not a directory", segment)
		}
		_ = unix.Close(currentFD)
		currentFD = nextFD
	}
	return currentFD, nil
}

func openDirectoryAtNoFollow(parentFD int, name string) (int, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	info, err := statFD(fd)
	if err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	if info.Mode&unix.S_IFMT == unix.S_IFLNK {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("refusing symlink directory %q", name)
	}
	if info.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("%q is not a directory", name)
	}
	return fd, nil
}

func readWorkspaceMounts() (map[string]workspaceMount, error) {
	contents, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	mounts := make(map[string]workspaceMount)
	for lineNumber, line := range strings.Split(string(contents), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			return nil, fmt.Errorf("malformed mountinfo line %d", lineNumber+1)
		}
		mountPath, err := decodeMountPath(fields[4])
		if err != nil {
			return nil, fmt.Errorf("decode mountinfo line %d: %w", lineNumber+1, err)
		}
		mountPath = filepath.Clean(mountPath)
		if !filepath.IsAbs(mountPath) {
			return nil, fmt.Errorf("mountpoint on line %d is not absolute", lineNumber+1)
		}
		readWrite := false
		for _, option := range strings.Split(fields[5], ",") {
			if option == "rw" {
				readWrite = true
				break
			}
		}
		previous, exists := mounts[mountPath]
		if exists {
			previous.writable = previous.writable && readWrite
			mounts[mountPath] = previous
		} else {
			mounts[mountPath] = workspaceMount{writable: readWrite, seen: true}
		}
	}
	return mounts, nil
}

func decodeMountPath(value string) (string, error) {
	var decoded strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' {
			decoded.WriteByte(value[index])
			continue
		}
		if index+3 >= len(value) {
			return "", errors.New("incomplete octal escape")
		}
		var octal byte
		for offset := 1; offset <= 3; offset++ {
			character := value[index+offset]
			if character < '0' || character > '7' {
				return "", errors.New("invalid octal escape")
			}
			octal = octal*8 + (character - '0')
		}
		decoded.WriteByte(octal)
		index += 3
	}
	return decoded.String(), nil
}

func databaseFileName(workspace string) (string, bool) {
	databasePath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if databasePath == "" {
		databasePath = filepath.Join(workspace, "db", "nas-tiktok.db")
	} else if !filepath.IsAbs(databasePath) {
		absolutePath, err := filepath.Abs(databasePath)
		if err != nil {
			return "", false
		}
		databasePath = absolutePath
	}
	databasePath = filepath.Clean(databasePath)
	databaseDir := filepath.Join(workspace, "db")
	if !filepath.IsAbs(databasePath) || filepath.Dir(databasePath) != databaseDir {
		return "", false
	}
	name := filepath.Base(databasePath)
	if name == "." || name == ".." || name == "" {
		return "", false
	}
	return name, true
}

func protectedWorkspacePaths(workspace string) []string {
	paths := []string{
		filepath.Join(workspace, "static"),
		filepath.Join(workspace, "certs"),
	}
	mediaRoot := os.Getenv("MEDIA_ROOT")
	if mediaRoot != "" {
		if !filepath.IsAbs(mediaRoot) {
			mediaRoot = filepath.Join(workspace, mediaRoot)
		}
		paths = append(paths, filepath.Clean(mediaRoot))
	}
	for _, name := range []string{"TLS_CERT_FILE", "TLS_KEY_FILE"} {
		certPath := os.Getenv(name)
		if certPath == "" {
			continue
		}
		if !filepath.IsAbs(certPath) {
			certPath = filepath.Join(workspace, certPath)
		}
		paths = append(paths, filepath.Clean(certPath))
	}
	return paths
}

func isProtectedWorkspacePath(path string, protected []string) bool {
	for _, protectedPath := range protected {
		if path == protectedPath {
			return true
		}
		relative, err := filepath.Rel(protectedPath, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
