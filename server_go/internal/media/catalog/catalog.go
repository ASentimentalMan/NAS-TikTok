package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/sampling"
)

const SampleLimit = 10

var (
	ErrInvalidType = errors.New("media type must be image or video")
	ErrUnsafePath  = errors.New("invalid media path")
	ErrUnsupported = errors.New("unsupported media file")
	ErrRootScan    = errors.New("media root could not be scanned")
	ErrPartialScan = errors.New("media scan was incomplete")
)

var imageExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".jfif": {}, ".png": {}, ".webp": {},
	".avif": {}, ".heic": {}, ".heif": {}, ".gif": {}, ".apng": {},
	".bmp": {}, ".jxl": {},
}

var videoExtensions = map[string]struct{}{
	".mp4": {}, ".mov": {}, ".m4v": {}, ".webm": {},
}

// MediaFile describes an image or video file found under the media root.
// RelativePath always uses slash separators and is relative to that root.
type MediaFile struct {
	RelativePath string
	MediaType    string
}

type Catalog struct {
	root     *os.Root
	rootPath string
	scanGate chan struct{}
	max      int
}

func New(rootPath string) (*Catalog, error) {
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, errors.New("media root path is invalid")
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("open media root: %w", err)
	}
	return &Catalog{root: root, rootPath: absolute, scanGate: make(chan struct{}, 1), max: SampleLimit}, nil
}

func (catalog *Catalog) Close() error {
	return catalog.root.Close()
}

func (catalog *Catalog) RootPath() string {
	return catalog.rootPath
}

func (catalog *Catalog) List(ctx context.Context, mediaType string) ([]string, error) {
	_, ok := extensionsFor(mediaType)
	if !ok {
		return nil, ErrInvalidType
	}
	select {
	case catalog.scanGate <- struct{}{}:
		defer func() { <-catalog.scanGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	selected, err := sampling.New(catalog.max)
	if err != nil {
		return nil, err
	}
	var partial bool
	var walk func(string) error
	walk = func(directory string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		files, subdirectories, err, incomplete := catalog.scanDirectory(ctx, directory)
		for _, file := range files {
			if file.MediaType == mediaType {
				selected.Add(file.RelativePath)
			}
		}
		if incomplete != nil {
			partial = true
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if directory == "." {
				return fmt.Errorf("%w: %w", ErrRootScan, err)
			}
			partial = true
		}
		for _, subdirectory := range subdirectories {
			if err := walk(subdirectory); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk("."); err != nil {
		return selected.Items(), err
	}
	if err := ctx.Err(); err != nil {
		return selected.Items(), err
	}
	if partial {
		return selected.Items(), ErrPartialScan
	}
	return selected.Items(), nil
}

// ScanDirectory returns media files and immediate child directories for one
// directory under the media root. Pass "." to scan the root. A failed or
// incomplete scan returns no results, so callers cannot mistake partial data
// for a complete directory listing.
func (catalog *Catalog) ScanDirectory(ctx context.Context, relativeDir string) ([]MediaFile, []string, error) {
	if relativeDir != "." && !validRelativePath(relativeDir) {
		return nil, nil, ErrUnsafePath
	}
	files, childDirs, err, incomplete := catalog.scanDirectory(ctx, relativeDir)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, nil, ctxErr
	}
	if err != nil {
		return nil, nil, fmt.Errorf("scan directory %q: %w", relativeDir, err)
	}
	if incomplete != nil {
		return nil, nil, fmt.Errorf("%w: scan directory %q: %w", ErrPartialScan, relativeDir, incomplete)
	}
	return files, childDirs, nil
}

// scanDirectory is shared by List and ScanDirectory. It retains entries
// discovered before a read failure so List can preserve its partial-scan
// behavior; ScanDirectory discards those results and reports the error.
func (catalog *Catalog) scanDirectory(ctx context.Context, relativeDir string) ([]MediaFile, []string, error, error) {
	if relativeDir != "." && !validRelativePath(relativeDir) {
		return nil, nil, ErrUnsafePath, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err, nil
	}

	if relativeDir != "." {
		current := "."
		for _, segment := range strings.Split(relativeDir, "/") {
			current = path.Join(current, segment)
			info, err := catalog.root.Lstat(current)
			if err != nil {
				return nil, nil, err, nil
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return nil, nil, ErrUnsafePath, nil
			}
			if !info.IsDir() {
				return nil, nil, fmt.Errorf("%q is not a directory", relativeDir), nil
			}
		}
	}

	directoryFile, err := catalog.root.Open(relativeDir)
	if err != nil {
		return nil, nil, err, nil
	}
	defer directoryFile.Close()
	info, err := directoryFile.Stat()
	if err != nil {
		return nil, nil, err, nil
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%q is not a directory", relativeDir), nil
	}

	files := make([]MediaFile, 0)
	childDirs := make([]string, 0)
	var incomplete error
	for {
		if err := ctx.Err(); err != nil {
			return files, childDirs, err, incomplete
		}
		entries, readErr := directoryFile.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return files, childDirs, err, incomplete
			}
			name := entry.Name()
			if entry.IsDir() {
				if name != "@eaDir" {
					childDirs = append(childDirs, path.Join(relativeDir, name))
				}
				continue
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			entryInfo, err := entry.Info()
			if err != nil {
				if incomplete == nil {
					incomplete = fmt.Errorf("inspect %q: %w", path.Join(relativeDir, name), err)
				}
				continue
			}
			if entryInfo.Mode()&fs.ModeSymlink != 0 {
				continue
			}
			if !entryInfo.Mode().IsRegular() {
				continue
			}
			mediaType, supported := mediaTypeForExtension(filepath.Ext(name))
			if supported {
				files = append(files, MediaFile{
					RelativePath: path.Join(relativeDir, name),
					MediaType:    mediaType,
				})
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return files, childDirs, nil, incomplete
			}
			return files, childDirs, readErr, incomplete
		}
		if len(entries) == 0 {
			return files, childDirs, errors.New("directory read returned no entries without reaching EOF"), incomplete
		}
	}
}

func (catalog *Catalog) Open(relative string) (*os.File, fs.FileInfo, error) {
	if !validRelativePath(relative) {
		return nil, nil, ErrUnsafePath
	}
	current := "."
	for _, segment := range strings.Split(relative, "/") {
		current = path.Join(current, segment)
		info, err := catalog.root.Lstat(current)
		if err != nil {
			return nil, nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, nil, ErrUnsafePath
		}
	}
	file, err := catalog.root.Open(relative)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, ErrUnsupported
	}
	if !isMediaExtension(filepath.Ext(relative)) {
		_ = file.Close()
		return nil, nil, ErrUnsupported
	}
	return file, info, nil
}

func extensionsFor(mediaType string) (map[string]struct{}, bool) {
	switch mediaType {
	case "image":
		return imageExtensions, true
	case "video":
		return videoExtensions, true
	default:
		return nil, false
	}
}

func mediaTypeForExtension(extension string) (string, bool) {
	if _, ok := imageExtensions[strings.ToLower(extension)]; ok {
		return "image", true
	}
	if _, ok := videoExtensions[strings.ToLower(extension)]; ok {
		return "video", true
	}
	return "", false
}

func isMediaExtension(extension string) bool {
	_, ok := mediaTypeForExtension(extension)
	return ok
}

func validRelativePath(value string) bool {
	if value == "" || strings.ContainsRune(value, '\\') || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}
