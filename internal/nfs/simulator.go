package nfs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

const defaultBasePath = "/tmp/cloudbridge-nfs"

// FileInfo holds metadata for a single file in the simulated NFS mount.
type FileInfo struct {
	Path      string    // path relative to the simulator's BasePath
	SizeBytes int64
	ModTime   time.Time
	Checksum  string // SHA-256 hex digest
}

// Simulator simulates an NFS mount point using the local filesystem.
// In production this would be replaced by a real NFS kernel module or driver.
// All paths are relative to BasePath and are guarded against directory traversal.
type Simulator struct {
	BasePath string
	logger   *zap.Logger
}

// New creates a Simulator rooted at basePath.
// If basePath is empty, /tmp/cloudbridge-nfs is used.
// The directory is created (with all parents) if it does not exist.
func New(basePath string, logger *zap.Logger) (*Simulator, error) {
	if basePath == "" {
		basePath = defaultBasePath
	}
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("nfs: create base path %q: %w", basePath, err)
	}
	logger.Info("NFS simulator initialised", zap.String("base_path", basePath))
	return &Simulator{BasePath: basePath, logger: logger}, nil
}

// fullPath resolves a relative path against BasePath and rejects traversal attempts.
func (s *Simulator) fullPath(relPath string) (string, error) {
	abs := filepath.Clean(filepath.Join(s.BasePath, relPath))
	// Ensure the result is still under BasePath.
	rel, err := filepath.Rel(s.BasePath, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("nfs: path %q escapes base directory", relPath)
	}
	return abs, nil
}

// ReadFile reads the entire file at path and returns its contents, byte count,
// SHA-256 hex checksum, and any error.
func (s *Simulator) ReadFile(path string) ([]byte, int64, string, error) {
	start := time.Now()

	full, err := s.fullPath(path)
	if err != nil {
		return nil, 0, "", err
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return nil, 0, "", fmt.Errorf("nfs: read %q: %w", path, err)
	}

	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	size := int64(len(data))

	s.logger.Debug("nfs: ReadFile",
		zap.String("path", path),
		zap.Int64("size_bytes", size),
		zap.String("checksum", checksum),
		zap.Duration("duration", time.Since(start)),
	)
	return data, size, checksum, nil
}

// WriteFile writes data to path, creating parent directories as needed.
// Existing files are overwritten atomically via a temp-file + rename pattern.
func (s *Simulator) WriteFile(path string, data []byte) error {
	start := time.Now()

	full, err := s.fullPath(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("nfs: mkdir for %q: %w", path, err)
	}

	// Write to a temp file in the same directory so the rename is atomic.
	tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-cloudbridge-*")
	if err != nil {
		return fmt.Errorf("nfs: create temp file for %q: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup; rename below may succeed before this runs.
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("nfs: write temp file for %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("nfs: close temp file for %q: %w", path, err)
	}

	if err := os.Rename(tmpName, full); err != nil {
		return fmt.Errorf("nfs: rename to %q: %w", path, err)
	}

	s.logger.Debug("nfs: WriteFile",
		zap.String("path", path),
		zap.Int("size_bytes", len(data)),
		zap.Duration("duration", time.Since(start)),
	)
	return nil
}

// DeleteFile removes the file at path. Returns nil if the file does not exist.
func (s *Simulator) DeleteFile(path string) error {
	full, err := s.fullPath(path)
	if err != nil {
		return err
	}

	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("nfs: delete %q: %w", path, err)
	}
	s.logger.Debug("nfs: DeleteFile", zap.String("path", path))
	return nil
}

// ListFiles returns metadata for all regular files directly inside dirPath.
// Checksums are not computed during listing (call Stat for per-file checksums).
func (s *Simulator) ListFiles(dirPath string) ([]FileInfo, error) {
	start := time.Now()

	full, err := s.fullPath(dirPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, fmt.Errorf("nfs: readdir %q: %w", dirPath, err)
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue // skip unreadable entries
		}
		files = append(files, FileInfo{
			Path:      filepath.Join(dirPath, entry.Name()),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime(),
			// Checksum omitted — too expensive on large directories.
		})
	}

	s.logger.Debug("nfs: ListFiles",
		zap.String("dir", dirPath),
		zap.Int("count", len(files)),
		zap.Duration("duration", time.Since(start)),
	)
	return files, nil
}

// Stat returns full metadata (including SHA-256 checksum) for the file at path.
// Returns an error if the path is a directory or does not exist.
func (s *Simulator) Stat(path string) (*FileInfo, error) {
	start := time.Now()

	full, err := s.fullPath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("nfs: stat %q: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("nfs: %q is a directory, not a file", path)
	}

	// Compute SHA-256 checksum by streaming the file.
	f, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("nfs: open for checksum %q: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("nfs: compute checksum %q: %w", path, err)
	}

	s.logger.Debug("nfs: Stat",
		zap.String("path", path),
		zap.Int64("size_bytes", info.Size()),
		zap.Duration("duration", time.Since(start)),
	)

	return &FileInfo{
		Path:      path,
		SizeBytes: info.Size(),
		ModTime:   info.ModTime(),
		Checksum:  hex.EncodeToString(h.Sum(nil)),
	}, nil
}
