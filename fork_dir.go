package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// フォークディレクトリの識別用プレフィックス
const ForkDirPrefix = "rerost-fork-"

func ForkDir(args []string, _ io.Writer) (io.Reader, error) {
	if len(args) > 0 {
		switch args[0] {
		case "clean":
			return cleanForkDirs()
		case "list":
			return listForkDirs(false)
		case "list-all":
			return listForkDirs(true)
		}
	}

	// デフォルトの動作（新しいフォークディレクトリを作成）
	return createForkDir()
}

// ソースディレクトリのパスをエンコードしてフォークディレクトリ名に埋め込む
func encodeSrcPath(srcPath string) string {
	encoded := base64.URLEncoding.EncodeToString([]byte(srcPath))
	return encoded
}

// フォークディレクトリ名からソースディレクトリのパスをデコードする
func decodeSrcPath(dirName string) (string, error) {
	// プレフィックスとタイムスタンプ部分を削除
	parts := strings.Split(dirName, "-")
	if len(parts) < 3 {
		return "", errors.New("invalid fork directory name format")
	}

	// 最後の部分がエンコードされたパス
	encodedPath := parts[len(parts)-1]

	decoded, err := base64.URLEncoding.DecodeString(encodedPath)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return string(decoded), nil
}

// フォークディレクトリ名を生成する
func generateForkDirName(baseName string, srcPath string) string {
	timestamp := time.Now().UnixNano()
	encodedSrc := encodeSrcPath(srcPath)
	return fmt.Sprintf("%s%s-%d-%s", ForkDirPrefix, baseName, timestamp, encodedSrc)
}

func createForkDir() (io.Reader, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	baseName := filepath.Base(currentDir)

	// フォークディレクトリ名を生成（ソースパスの情報を埋め込む）
	forkDirName := generateForkDirName(baseName, currentDir)

	// 一時ディレクトリのベースパスを取得
	tempBase := os.TempDir()
	destDir := filepath.Join(tempBase, forkDirName)

	// ディレクトリを作成
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := copyOnWrite(currentDir, destDir); err != nil {
		return nil, errors.WithStack(err)
	}

	out := filepath.Join(destDir, baseName)
	writer := strings.NewReader(out)

	return writer, nil
}

// すべてのフォークディレクトリを削除する
func cleanForkDirs() (io.Reader, error) {
	tempDir := os.TempDir()

	// 一時ディレクトリ内のエントリを取得
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var deleted []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// フォークディレクトリかどうか確認
		if strings.HasPrefix(entry.Name(), ForkDirPrefix) {
			forkPath := filepath.Join(tempDir, entry.Name())
			if err := os.RemoveAll(forkPath); err == nil {
				deleted = append(deleted, forkPath)
			}
		}
	}

	if len(deleted) == 0 {
		return strings.NewReader("No fork directories found.\n"), nil
	}

	output := fmt.Sprintf("Deleted %d fork directories.\n", len(deleted))
	return strings.NewReader(output), nil
}

// フォークディレクトリの一覧を表示する
func listForkDirs(listAll bool) (io.Reader, error) {
	tempDir := os.TempDir()

	// 一時ディレクトリ内のエントリを取得
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if listAll {
		// list-all: フォークディレクトリを1行ずつ出力
		var result bytes.Buffer
		found := false

		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ForkDirPrefix) {
				continue
			}

			forkPath := filepath.Join(tempDir, entry.Name())
			result.WriteString(fmt.Sprintf("%s\n", forkPath))
			found = true
		}

		if !found {
			return strings.NewReader("No fork directories found.\n"), nil
		}

		return &result, nil
	} else {
		// list: ソースディレクトリをキー、フォークディレクトリの配列を値とするマップを出力
		sourceToForks := make(map[string][]string)

		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ForkDirPrefix) {
				continue
			}

			forkPath := filepath.Join(tempDir, entry.Name())

			// フォークディレクトリ名からソースパスを抽出
			srcPath, err := decodeSrcPath(entry.Name())
			if err != nil {
				continue
			}

			// カレントディレクトリからのフォークのみを表示
			if srcPath != currentDir {
				continue
			}

			// マップにエントリを追加
			sourceToForks[srcPath] = append(sourceToForks[srcPath], forkPath)
		}

		if len(sourceToForks) == 0 {
			return strings.NewReader("No fork directories found for the current directory.\n"), nil
		}

		// JSON形式に変換
		var result bytes.Buffer
		result.WriteString("{\n")

		i := 0
		for srcDir, forkDirs := range sourceToForks {
			result.WriteString(fmt.Sprintf("  %q: [\n", srcDir))

			for j, forkDir := range forkDirs {
				result.WriteString(fmt.Sprintf("    %q", forkDir))
				if j < len(forkDirs)-1 {
					result.WriteString(",\n")
				} else {
					result.WriteString("\n")
				}
			}

			result.WriteString("  ]")
			if i < len(sourceToForks)-1 {
				result.WriteString(",\n")
			} else {
				result.WriteString("\n")
			}
			i++
		}

		result.WriteString("}\n")
		return &result, nil
	}
}

// NOTE: Mac Only
func copyOnWrite(srcDir, destDir string) error {
	cmd := exec.Command("cp", "-R", "-c", srcDir, destDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return errors.WithStack(cmd.Run())
}
