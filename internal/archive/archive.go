package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/apgul/thorsync/internal/ids"
)

type Archive struct {
	root      string
	softQuota int64
	reserve   int64
}

type Blob struct {
	Hash string
	Size int64
	Path string
}

type Capacity struct {
	UsedBytes      int64   `json:"usedBytes"`
	SoftQuotaBytes int64   `json:"softQuotaBytes"`
	FreeBytes      uint64  `json:"freeBytes"`
	ReserveBytes   int64   `json:"reserveBytes"`
	UsageRatio     float64 `json:"usageRatio"`
	State          string  `json:"state"`
}

func New(root string, softQuota, reserve int64) *Archive {
	return &Archive{root: root, softQuota: softQuota, reserve: reserve}
}

func (a *Archive) Ensure() error {
	return os.MkdirAll(filepath.Join(a.root, "blobs"), 0o750)
}

func (a *Archive) PutFile(source string) (Blob, error) {
	if err := a.Ensure(); err != nil {
		return Blob{}, err
	}
	capacity, err := a.Capacity()
	if err == nil && capacity.FreeBytes <= uint64(a.reserve) {
		return Blob{}, fmt.Errorf("archive free-space reserve reached")
	}

	in, err := os.Open(source)
	if err != nil {
		return Blob{}, err
	}
	defer in.Close()

	tmpDir := filepath.Join(a.root, "tmp")
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		return Blob{}, err
	}
	tmpPath := filepath.Join(tmpDir, ids.New()+".partial")
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return Blob{}, err
	}
	removeTmp := true
	defer func() {
		_ = out.Close()
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, hash), in)
	if err != nil {
		return Blob{}, err
	}
	if size == 0 {
		return Blob{}, errors.New("refusing to archive zero-byte save")
	}
	if err := out.Sync(); err != nil {
		return Blob{}, err
	}
	if err := out.Close(); err != nil {
		return Blob{}, err
	}

	digest := hex.EncodeToString(hash.Sum(nil))
	destination := a.Path(digest)
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return Blob{}, err
	}
	if _, err := os.Stat(destination); err == nil {
		if err := a.Verify(digest); err != nil {
			return Blob{}, fmt.Errorf("existing archive object is corrupt: %w", err)
		}
		return Blob{Hash: digest, Size: size, Path: destination}, nil
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return Blob{}, err
	}
	removeTmp = false
	if err := syncDir(filepath.Dir(destination)); err != nil {
		return Blob{}, err
	}
	return Blob{Hash: digest, Size: size, Path: destination}, nil
}

func (a *Archive) Path(hash string) string {
	if !validDigest(hash) {
		return filepath.Join(a.root, "blobs", "invalid")
	}
	return filepath.Join(a.root, "blobs", hash[:2], hash[2:4], hash)
}

func (a *Archive) Open(hash string) (*os.File, error) {
	if !validDigest(hash) {
		return nil, errors.New("invalid archive hash")
	}
	return os.Open(a.Path(hash))
}

// Verify re-hashes one immutable archive object and detects missing or
// corrupted bytes without trusting the filename.
func (a *Archive) Verify(hash string) error {
	if !validDigest(hash) {
		return errors.New("invalid archive hash")
	}
	file, err := os.Open(a.Path(hash))
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if actual != hash {
		return fmt.Errorf("archive integrity mismatch: expected %s, got %s", hash, actual)
	}
	return nil
}

func validDigest(hash string) bool {
	if len(hash) != sha256.Size*2 || strings.ToLower(hash) != hash {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

func (a *Archive) Capacity() (Capacity, error) {
	var used int64
	err := filepath.WalkDir(filepath.Join(a.root, "blobs"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			used += info.Size()
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return Capacity{}, err
	}

	free, err := diskFree(a.root)
	if err != nil {
		return Capacity{}, err
	}
	ratio := float64(0)
	if a.softQuota > 0 {
		ratio = float64(used) / float64(a.softQuota)
	}
	state := "healthy"
	if free <= uint64(a.reserve) {
		state = "blocked"
	} else if ratio >= 0.9 {
		state = "critical"
	} else if ratio >= 0.8 {
		state = "warning"
	}
	return Capacity{UsedBytes: used, SoftQuotaBytes: a.softQuota, FreeBytes: free, ReserveBytes: a.reserve, UsageRatio: ratio, State: state}, nil
}
