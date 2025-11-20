package agent

import (
	"context"
	"os"
)

// Storage provides an interface to interact with "persistent" storage.
// This interface is used by set of tools which allow agent to create persistent objects, like content or code snippets.
type Storage interface {
	Exists(ctx context.Context, filename string) (bool, error)
	Read(ctx context.Context, filename string) ([]byte, error)
	Write(ctx context.Context, filename string, content []byte) error
	Delete(ctx context.Context, filename string) error
}

type InMemoryStorage struct {
	files map[string][]byte
}

func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{files: make(map[string][]byte)}
}

func (s *InMemoryStorage) Read(ctx context.Context, filename string) ([]byte, error) {
	if f, ok := s.files[filename]; ok {
		return f, nil
	}

	return nil, os.ErrNotExist
}

func (s *InMemoryStorage) Write(ctx context.Context, filename string, content []byte) error {
	s.files[filename] = content
	return nil
}

func (s *InMemoryStorage) Delete(ctx context.Context, filename string) error {
	if _, ok := s.files[filename]; ok {
		delete(s.files, filename)
	}

	return nil
}

func (s *InMemoryStorage) Exists(ctx context.Context, filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}
