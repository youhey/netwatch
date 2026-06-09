package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

type JSONLGroupRenameOptions struct {
	DryRun       bool
	BackupSuffix string
}

type JSONLGroupRenameResult struct {
	Files        []JSONLGroupRenameFileResult
	TotalLines   int
	TotalChanged int
	InvalidLines int
}

type JSONLGroupRenameFileResult struct {
	Path         string
	Lines        int
	Changed      int
	InvalidLines int
	BackupPath   string
}

func RenameGroupInJSONLDataDir(dataDir, filePattern, oldGroup, newGroup string, opts JSONLGroupRenameOptions) (JSONLGroupRenameResult, error) {
	if dataDir == "" {
		return JSONLGroupRenameResult{}, errors.New("data_dir is required")
	}
	if filePattern == "" {
		return JSONLGroupRenameResult{}, errors.New("file_pattern is required")
	}

	glob := filepath.Join(dataDir, patternGlob(filePattern))
	paths, err := filepath.Glob(glob)
	if err != nil {
		return JSONLGroupRenameResult{}, err
	}
	sort.Strings(paths)
	return RenameGroupInJSONLFiles(paths, oldGroup, newGroup, opts)
}

func RenameGroupInJSONLFiles(paths []string, oldGroup, newGroup string, opts JSONLGroupRenameOptions) (JSONLGroupRenameResult, error) {
	if oldGroup == "" {
		return JSONLGroupRenameResult{}, errors.New("old group is required")
	}
	if newGroup == "" {
		return JSONLGroupRenameResult{}, errors.New("new group is required")
	}
	if oldGroup == newGroup {
		return JSONLGroupRenameResult{}, errors.New("old group and new group must be different")
	}

	var result JSONLGroupRenameResult
	for _, path := range paths {
		fileResult, err := RenameGroupInJSONLFile(path, oldGroup, newGroup, opts)
		if err != nil {
			return result, err
		}
		result.Files = append(result.Files, fileResult)
		result.TotalLines += fileResult.Lines
		result.TotalChanged += fileResult.Changed
		result.InvalidLines += fileResult.InvalidLines
	}
	return result, nil
}

func RenameGroupInJSONLFile(path, oldGroup, newGroup string, opts JSONLGroupRenameOptions) (JSONLGroupRenameFileResult, error) {
	if path == "" {
		return JSONLGroupRenameFileResult{}, errors.New("path is required")
	}
	if oldGroup == "" {
		return JSONLGroupRenameFileResult{}, errors.New("old group is required")
	}
	if newGroup == "" {
		return JSONLGroupRenameFileResult{}, errors.New("new group is required")
	}
	if oldGroup == newGroup {
		return JSONLGroupRenameFileResult{}, errors.New("old group and new group must be different")
	}

	f, err := os.Open(path)
	if err != nil {
		return JSONLGroupRenameFileResult{}, err
	}
	defer f.Close()

	result := JSONLGroupRenameFileResult{Path: path}
	tmpPath := ""
	var tmp *os.File
	if !opts.DryRun {
		tmp, err = os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
		if err != nil {
			return JSONLGroupRenameFileResult{}, err
		}
		tmpPath = tmp.Name()
		defer func() {
			if tmpPath != "" {
				_ = os.Remove(tmpPath)
			}
		}()
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		result.Lines++

		nextLine := line
		var payload map[string]any
		if len(line) > 0 {
			if err := json.Unmarshal(line, &payload); err != nil {
				result.InvalidLines++
			} else if group, ok := payload["group"].(string); ok && group == oldGroup {
				payload["group"] = newGroup
				nextLine, err = json.Marshal(payload)
				if err != nil {
					return result, err
				}
				result.Changed++
			}
		}

		if tmp != nil {
			if _, err := tmp.Write(append(nextLine, '\n')); err != nil {
				return result, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	if opts.DryRun || result.Changed == 0 {
		if tmp != nil {
			if err := tmp.Close(); err != nil {
				return result, err
			}
		}
		return result, nil
	}

	if err := tmp.Close(); err != nil {
		return result, err
	}
	if err := preserveFileMetadata(path, tmpPath); err != nil {
		return result, err
	}

	if opts.BackupSuffix != "" {
		result.BackupPath = path + opts.BackupSuffix
		if _, err := os.Stat(result.BackupPath); err == nil {
			return result, fmt.Errorf("backup already exists: %s", result.BackupPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		if err := os.Rename(path, result.BackupPath); err != nil {
			return result, err
		}
		if err := os.Rename(tmpPath, path); err != nil {
			_ = os.Rename(result.BackupPath, path)
			return result, err
		}
		tmpPath = ""
		return result, nil
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return result, err
	}
	tmpPath = ""
	return result, nil
}

func preserveFileMetadata(srcPath, dstPath string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(dstPath, info.Mode()); err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if err := os.Chown(dstPath, int(stat.Uid), int(stat.Gid)); err != nil && os.Geteuid() == 0 {
		return err
	}
	return nil
}
