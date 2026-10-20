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
)

var imageExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".heic": {}, ".heif": {}, ".avif": {},
	".webp": {}, ".png": {}, ".gif": {}, ".apng": {}, ".bmp": {},
}

var videoExtensions = map[string]struct{}{
	".mp4": {}, ".webm": {}, ".mov": {}, ".ogv": {}, ".ogg": {},
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
	extensions, ok := extensionsFor(mediaType)
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

	readDirectory := func(directory string) ([]string, error) {
		directoryFile, err := catalog.root.Open(directory)
		if err != nil {
			return nil, err
		}
		defer directoryFile.Close()

		subdirectories := make([]string, 0)
		for {
			entries, readErr := directoryFile.ReadDir(128)
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					return subdirectories, err
				}
				name := entry.Name()
				if entry.IsDir() {
					if name != "@eaDir" {
						subdirectories = append(subdirectories, path.Join(directory, name))
					}
					continue
				}
				if entry.Type()&fs.ModeSymlink != 0 {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					partial = true
					continue
				}
				if !info.Mode().IsRegular() {
					continue
				}
				extension := strings.ToLower(filepath.Ext(name))
				if _, matched := extensions[extension]; matched {
					selected.Add(path.Join(directory, name))
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return subdirectories, nil
				}
				return subdirectories, readErr
			}
			if len(entries) == 0 {
				return subdirectories, errors.New("directory read returned no entries without reaching EOF")
			}
		}
	}

	var walk func(string) error
	walk = func(directory string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		subdirectories, err := readDirectory(directory)
		if err != nil {
			if directory == "." {
				return fmt.Errorf("read media root: %w", err)
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
	if partial {
		return selected.Items(), errors.New("one or more media directories could not be read")
	}
	return selected.Items(), nil
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

func isMediaExtension(extension string) bool {
	extension = strings.ToLower(extension)
	if _, ok := imageExtensions[extension]; ok {
		return true
	}
	_, ok := videoExtensions[extension]
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
