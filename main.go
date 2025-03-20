package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

func main() {
	if err := run(os.Args, os.Stdin, os.Stdout); err != nil {
		fmt.Printf("%+v\n", err) // TODO Debug mode only
		os.Exit(1)
	}
}

func run(
	args []string,
	stdin io.Writer,
	stdout io.Writer,
) error {
	commands := map[string]Command{
		"fork-dir": {
			Description: "Copy current direcotry to a new directory",
			Run:         ForkDir,
		},
	}

	if len(args) < 2 {
		stdout.Write([]byte("Usage: <command> <args>\n\n"))
		for k, v := range commands {
			stdout.Write([]byte(fmt.Sprintf("%s: %s\n", k, v.Description)))
		}
		return nil
	}
	command, ok := commands[args[1]]
	if !ok {
		return errors.Newf("command not found: %s", args[1])
	}

	out, err := command.Run(args[2:], stdin)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := io.Copy(stdout, out); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

type CommandFunc func(args []string, in io.Writer) (io.Reader, error)
type Command struct {
	Description string
	Run         CommandFunc
}

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
