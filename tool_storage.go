package agent

import (
	"context"
)

func WithStorageTools(storage Storage) Option {
	type Filename struct {
		Filename string `json:"filename"`
	}

	type File struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}

	return WithOptions(
		WithInlineTool("read_file", "Read a file from the storage using its filename", func(ctx context.Context, in Filename) (string, error) {
			content, err := storage.Read(ctx, in.Filename)
			if err != nil {
				return "", err
			}

			return string(content), nil
		}),
		WithInlineTool("write_file", "Write a file in the storage. You MUST always pass entire file content, never partial.", func(ctx context.Context, in File) (string, error) {
			exists, _ := storage.Exists(ctx, in.Filename)
			if err := storage.Write(ctx, in.Filename, []byte(in.Content)); err != nil {
				return "", err
			}

			if exists {
				return "File updated", nil
			}

			return "File created", nil
		}),
		WithInlineTool("delete_file", "Delete a file in the storage using its filename", func(ctx context.Context, in File) (string, error) {
			return "File deleted", storage.Delete(ctx, in.Filename)
		}),
	)
}

func WithStorageReadTool(storage Storage) Option {
	type Filename struct {
		Filename string `json:"filename"`
	}

	return WithInlineTool("read_file", "Read a file from the storage using its filename", func(ctx context.Context, in Filename) (string, error) {
		content, err := storage.Read(ctx, in.Filename)
		if err != nil {
			return "", err
		}

		return string(content), nil
	})
}
