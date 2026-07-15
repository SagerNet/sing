package filemanager

import (
	"context"
	"io"
	"os"
	"sort"

	"github.com/sagernet/sing/service"
)

type Manager interface {
	BasePath(name string) string
	TempPath() string
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Create(name string) (*os.File, error)
	CreateTemp(pattern string) (*os.File, error)
	Chown(name string) error
	Mkdir(path string, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error
	Rename(oldPath string, newPath string) error
}

func BasePath(ctx context.Context, name string) string {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return name
	}
	return manager.BasePath(name)
}

func TempPath(ctx context.Context) string {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.TempDir()
	}
	return manager.TempPath()
}

func OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (*os.File, error) {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.OpenFile(name, flag, perm)
	}
	return manager.OpenFile(name, flag, perm)
}

func Create(ctx context.Context, name string) (*os.File, error) {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.Create(name)
	}
	return manager.Create(name)
}

func CreateTemp(ctx context.Context, pattern string) (*os.File, error) {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.CreateTemp("", pattern)
	}
	return manager.CreateTemp(pattern)
}

func Chown(ctx context.Context, name string) error {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return nil
	}
	return manager.Chown(name)
}

func Mkdir(ctx context.Context, path string, perm os.FileMode) error {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.Mkdir(path, perm)
	}
	return manager.Mkdir(path, perm)
}

func MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.MkdirAll(path, perm)
	}
	return manager.MkdirAll(path, perm)
}

func Remove(ctx context.Context, path string) error {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.Remove(path)
	}
	return manager.Remove(path)
}

func RemoveAll(ctx context.Context, path string) error {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.RemoveAll(path)
	}
	return manager.RemoveAll(path)
}

func Rename(ctx context.Context, oldPath string, newPath string) error {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.Rename(oldPath, newPath)
	}
	return manager.Rename(oldPath, newPath)
}

func WriteFile(ctx context.Context, name string, data []byte, perm os.FileMode) error {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.WriteFile(name, data, perm)
	}
	file, err := manager.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err1 := file.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}

func ReadFile(ctx context.Context, name string) ([]byte, error) {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.ReadFile(name)
	}
	file, err := manager.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	content, err := io.ReadAll(file)
	if err1 := file.Close(); err1 != nil && err == nil {
		err = err1
	}
	if err != nil {
		return nil, err
	}
	return content, nil
}

func Open(ctx context.Context, name string) (*os.File, error) {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.Open(name)
	}
	return manager.OpenFile(name, os.O_RDONLY, 0)
}

func ReadDir(ctx context.Context, name string) ([]os.DirEntry, error) {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.ReadDir(name)
	}
	directory, err := manager.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(-1)
	if err1 := directory.Close(); err1 != nil && err == nil {
		err = err1
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func Stat(ctx context.Context, name string) (os.FileInfo, error) {
	manager := service.FromContext[Manager](ctx)
	if manager == nil {
		return os.Stat(name)
	}
	file, err := manager.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return info, nil
}
