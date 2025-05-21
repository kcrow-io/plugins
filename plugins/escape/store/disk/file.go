package disk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	gofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/kcrow-io/plugins/plugins/escape/store"
)

const (
	dfeaultPerm = os.FileMode(0644)
	prefix      = ".tmp-"
)

var _ store.Store = (*fileStore)(nil)

type fileStore struct {
	dir string
}

func New(dir string) (*fileStore, error) {
	if err := os.MkdirAll(dir, dfeaultPerm); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return &fileStore{dir: dir}, nil
}

func (fs *fileStore) Save(ctx context.Context, key string, proc *store.Process) error {
	var (
		filename = filepath.Join(fs.dir, key)
	)
	data, err := proc.Encode()
	if err != nil {
		return err
	}
	dataSize := int64(len(data))
	buf := bytes.NewBuffer(data)
	f, err := os.CreateTemp(fs.dir, prefix+key)
	if err != nil {
		return err
	}
	needClose := true
	defer func() {
		if needClose {
			f.Close()
		}
	}()

	err = os.Chmod(f.Name(), dfeaultPerm)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, buf)
	if err == nil && n < dataSize {
		return io.ErrShortWrite
	}
	if err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}

	needClose = false
	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(f.Name(), filename)
}

func (fs *fileStore) Get(ctx context.Context, key string) (*store.Process, error) {
	return fs.getbykey(key)
}

func (fs *fileStore) Delete(ctx context.Context, key string) error {
	// discard path error
	os.Remove(filepath.Join(fs.dir, key))
	return nil
}

func (fs *fileStore) Walk(ctx context.Context, fn func(key string, data *store.Process) error) error {

	return filepath.WalkDir(fs.dir, func(path string, d gofs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || strings.HasPrefix(prefix, d.Name()) {
			return nil
		}

		relPath, err := filepath.Rel(fs.dir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}
		key := relPath

		proc, err := fs.getbykey(key)
		if err != nil {
			return fmt.Errorf("failed to get process for key %s: %w", key, err)
		}
		if err := fn(key, proc); err != nil {
			return fmt.Errorf("callback function failed for key %s: %w", key, err)
		}

		return nil
	})
}

func (fs *fileStore) getbykey(key string) (*store.Process, error) {
	data, err := os.ReadFile(path.Join(fs.dir, key))
	if err != nil {
		return nil, err
	}
	proc := &store.Process{}
	err = proc.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode process for key %s: %w", key, err)
	}
	return proc, nil
}
