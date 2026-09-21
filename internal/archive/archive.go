package archive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/apgul/thorsync/internal/ids"
)

type Archive struct {
	root      string
	softQuota int64
	reserve   int64
	mu        sync.Mutex
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
	a.mu.Lock()
	defer a.mu.Unlock()

	in, err := os.Open(source)
	if err != nil {
		return Blob{}, err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return Blob{}, err
	}
	if !info.Mode().IsRegular() {
		return Blob{}, errors.New("archive source is not a regular file")
	}
	if err := a.prepareWriteLocked(info.Size()); err != nil {
		return Blob{}, err
	}
	validateUnchanged := func() error {
		final, statErr := in.Stat()
		if statErr != nil {
			return statErr
		}
		if !final.Mode().IsRegular() || final.Size() != info.Size() || !final.ModTime().Equal(info.ModTime()) {
			return errors.New("archive source changed while it was being read")
		}
		return nil
	}
	return a.putReader(io.LimitReader(in, info.Size()+1), info.Size(), validateUnchanged)
}

// PutBytes stores data as an immutable content-addressed archive object.
func (a *Archive) PutBytes(data []byte) (Blob, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.prepareWriteLocked(int64(len(data))); err != nil {
		return Blob{}, err
	}
	return a.putReader(bytes.NewReader(data), int64(len(data)), nil)
}

// PutReader accepts bounded save data whose size is not known by the caller.
// Buffering it first lets the reserve check account for the complete write.
func (a *Archive) PutReader(reader io.Reader) (Blob, error) {
	if reader == nil {
		return Blob{}, errors.New("archive reader is nil")
	}
	const maxSaveBytes = 16 * 1024 * 1024
	var buffered bytes.Buffer
	if _, err := io.Copy(&buffered, io.LimitReader(reader, maxSaveBytes+1)); err != nil {
		return Blob{}, err
	}
	if buffered.Len() > maxSaveBytes {
		return Blob{}, errors.New("save exceeds 16 MiB safety limit")
	}
	return a.PutBytes(buffered.Bytes())
}

// prepareWriteLocked must be called while a.mu is held so multiple captures
// cannot each pass the reserve check and collectively consume protected space.
func (a *Archive) prepareWriteLocked(required int64) error {
	if err := a.Ensure(); err != nil {
		return err
	}
	capacity, err := a.Capacity()
	if err != nil {
		return fmt.Errorf("measure archive capacity: %w", err)
	}
	if !reserveAllows(capacity.FreeBytes, a.reserve, required) {
		if capacity.FreeBytes <= uint64(max64(a.reserve, 0)) {
			return errors.New("archive free-space reserve reached")
		}
		return errors.New("archive write would cross free-space reserve")
	}
	return nil
}

func reserveAllows(free uint64, reserve, required int64) bool {
	if reserve < 0 {
		reserve = 0
	}
	if required < 0 || free <= uint64(reserve) {
		return false
	}
	return uint64(required) <= free-uint64(reserve)
}

func max64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func (a *Archive) putReader(reader io.Reader, expectedSize int64, validate func() error) (Blob, error) {
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
	size, err := io.Copy(io.MultiWriter(out, hash), reader)
	if err != nil {
		return Blob{}, err
	}
	if size == 0 {
		return Blob{}, errors.New("refusing to archive zero-byte save")
	}
	if expectedSize >= 0 && size != expectedSize {
		return Blob{}, fmt.Errorf("archive source changed size while being read: read %d of %d bytes", size, expectedSize)
	}
	if validate != nil {
		if err := validate(); err != nil {
			return Blob{}, err
		}
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
	if err := syncDirTree(filepath.Dir(destination), a.root); err != nil {
		return Blob{}, err
	}
	return Blob{Hash: digest, Size: size, Path: destination}, nil
}

func syncDirTree(start, stop string) error {
	current, err := filepath.Abs(start)
	if err != nil {
		return err
	}
	stop, err = filepath.Abs(stop)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(stop, current)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("archive directory escaped its root")
	}
	for {
		if err := syncDir(current); err != nil {
			return err
		}
		if current == stop {
			return nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("archive directory escaped its root")
		}
		current = parent
	}
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
