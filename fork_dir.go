package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

func ForkDir(args []string, _ io.Writer) (io.Reader, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	baseName := filepath.Base(currentDir)

	tempDir, err := os.MkdirTemp("", baseName)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	destDir := tempDir
	if err := copyOnWrite(currentDir, destDir); err != nil {
		return nil, errors.WithStack(err)
	}

	out := filepath.Join(destDir, baseName)
	writer := strings.NewReader(out)

	return writer, nil
}

// NOTE: Mac Only
func copyOnWrite(srcDir, destDir string) error {
	cmd := exec.Command("cp", "-R", "-c", srcDir, destDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return errors.WithStack(cmd.Run())
}
