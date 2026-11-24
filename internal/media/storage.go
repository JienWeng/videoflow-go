package media

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func StoragePath(storageRoot, path string) (string, error) {
	root, err := filepath.Abs(storageRoot)
	if err != nil {
		return "", err
	}
	var full string
	if filepath.IsAbs(path) && !strings.HasPrefix(path, "/storage/") {
		full = filepath.Clean(path)
	} else {
		if strings.HasPrefix(filepath.Clean(path), filepath.Clean(storageRoot)+string(filepath.Separator)) {
			path, err = filepath.Rel(storageRoot, path)
			if err != nil {
				return "", err
			}
		}
		path = strings.TrimPrefix(path, "/")
		path = strings.TrimPrefix(path, "storage/")
		full = filepath.Join(root, path)
	}
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file is outside storage")
	}
	// Reject symlinks that escape storage, too. Nonexistent destination paths are handled by their writer.
	if actual, err := filepath.EvalSymlinks(full); err == nil {
		resolvedRoot, e := filepath.EvalSymlinks(root)
		if e != nil {
			return "", e
		}
		rel, e = filepath.Rel(resolvedRoot, actual)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("file symlink is outside storage")
		}
	}
	return full, nil
}
func ImageDataURL(storageRoot, path string) (string, error) {
	full, err := StoragePath(storageRoot, path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	mediaType := http.DetectContentType(data)
	if !strings.HasPrefix(mediaType, "image/") {
		return "", fmt.Errorf("file is not a supported image")
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// WriteDataURLs saves base64 image data URLs as files in dir, in order, so a local AI
// session can read them.
func WriteDataURLs(dir string, urls []string) ([]string, error) {
	paths := make([]string, 0, len(urls))
	for i, url := range urls {
		header, encoded, ok := strings.Cut(url, ",")
		if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
			return nil, fmt.Errorf("image %d is not an image data URL", i+1)
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("image %d could not be decoded: %w", i+1, err)
		}
		ext := "." + strings.TrimSuffix(strings.TrimPrefix(header, "data:image/"), ";base64")
		if ext == ".jpeg" {
			ext = ".jpg"
		}
		path := filepath.Join(dir, fmt.Sprintf("image-%02d%s", i+1, ext))
		if err = os.WriteFile(path, data, 0o600); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}
