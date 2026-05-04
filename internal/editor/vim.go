// Package editor wraps the user's $EDITOR (default vim) for inline edits.
package editor

import (
	"fmt"
	"os"
	"os/exec"
)

// Edit hands content to the user's editor and returns the edited bytes.
// readOnly true opens vim with -R. The temp file is removed before returning.
func Edit(suffix string, content []byte, readOnly bool) ([]byte, error) {
	tmp, err := os.CreateTemp("", "kstool-*"+suffix)
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	path := tmp.Name()
	defer os.Remove(path)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp: %w", err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	args := []string{path}
	if readOnly {
		args = []string{"-R", path}
	}
	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run %s: %w", editor, err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read edited: %w", err)
	}
	return out, nil
}
