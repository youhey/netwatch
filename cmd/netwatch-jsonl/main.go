package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/youhey/netwatch/internal/storage"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "rename-group":
		if err := runRenameGroup(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func runRenameGroup(args []string) error {
	fs := flag.NewFlagSet("rename-group", flag.ExitOnError)
	dataDir := fs.String("data-dir", "", "rotating JSONL data directory")
	filePattern := fs.String("pattern", "samples-%Y-%m-%d.jsonl", "rotating JSONL file pattern")
	path := fs.String("path", "", "single JSONL file path")
	dryRun := fs.Bool("dry-run", false, "scan files without rewriting them")
	backupSuffix := fs.String("backup-suffix", ".bak-"+time.Now().Format("20060102T150405"), "backup suffix for changed files")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s rename-group [flags] old_group new_group\n\n", os.Args[0])
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return fmt.Errorf("old_group and new_group are required")
	}
	if (*dataDir == "") == (*path == "") {
		return fmt.Errorf("exactly one of -data-dir or -path is required")
	}

	oldGroup := fs.Arg(0)
	newGroup := fs.Arg(1)
	opts := storage.JSONLGroupRenameOptions{
		DryRun:       *dryRun,
		BackupSuffix: *backupSuffix,
	}

	var result storage.JSONLGroupRenameResult
	var err error
	if *path != "" {
		result, err = storage.RenameGroupInJSONLFiles([]string{*path}, oldGroup, newGroup, opts)
	} else {
		result, err = storage.RenameGroupInJSONLDataDir(*dataDir, *filePattern, oldGroup, newGroup, opts)
	}
	if err != nil {
		return err
	}

	for _, file := range result.Files {
		if file.Changed == 0 {
			fmt.Printf("%s lines=%d changed=0 invalid=%d\n", file.Path, file.Lines, file.InvalidLines)
			continue
		}
		if *dryRun {
			fmt.Printf("%s lines=%d changed=%d invalid=%d dry_run=true\n", file.Path, file.Lines, file.Changed, file.InvalidLines)
			continue
		}
		fmt.Printf("%s lines=%d changed=%d invalid=%d backup=%s\n", file.Path, file.Lines, file.Changed, file.InvalidLines, file.BackupPath)
	}
	fmt.Printf("summary files=%d lines=%d changed=%d invalid=%d dry_run=%v\n", len(result.Files), result.TotalLines, result.TotalChanged, result.InvalidLines, *dryRun)
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <command> [args]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  rename-group    rename JSONL sample group values")
}
