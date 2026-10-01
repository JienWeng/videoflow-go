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
