package src

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type xepgSnapshotFile struct {
	Path    string
	Backup  string
	Existed bool
	Mode    os.FileMode
}

type xepgRecoverySnapshot struct {
	Directory string
	Files     []xepgSnapshotFile
}

func xepgRecoveryFilePaths() []string {

	return []string{
		System.File.Settings,
		System.File.XEPG,
		System.File.URLS,
		System.File.M3U,
		System.File.XML,
		System.Compressed.GZxml,
	}
}

func copyXEPGSnapshotFile(
	source string,
	destination string,
	mode os.FileMode,
) error {

	input, err := os.Open(source)

	if err != nil {
		return err
	}

	defer input.Close()

	output, err := os.OpenFile(
		destination,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0600,
	)

	if err != nil {
		return err
	}

	success := false

	defer func() {
		output.Close()

		if !success {
			os.Remove(destination)
		}
	}()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}

	if err := output.Chmod(mode.Perm()); err != nil {
		return err
	}

	if err := output.Sync(); err != nil {
		return err
	}

	if err := output.Close(); err != nil {
		return err
	}

	success = true

	return nil
}

func captureXEPGRecoverySnapshot(
	root string,
	paths []string,
) (*xepgRecoverySnapshot, error) {

	if root == "" {
		return nil, errors.New(
			"Missing XEPG snapshot directory",
		)
	}

	if len(paths) == 0 {
		return nil, errors.New(
			"No XEPG snapshot files specified",
		)
	}

	directory, err := os.MkdirTemp(
		root,
		".xepg-rollback-",
	)

	if err != nil {
		return nil, fmt.Errorf(
			"Cannot create XEPG snapshot directory: %w",
			err,
		)
	}

	snapshot := &xepgRecoverySnapshot{
		Directory: directory,
	}

	success := false

	defer func() {
		if !success {
			os.RemoveAll(directory)
		}
	}()

	seen := make(map[string]bool)

	for index, path := range paths {

		if path == "" {
			return nil, errors.New(
				"Empty XEPG snapshot file path",
			)
		}

		absolute, err := filepath.Abs(path)

		if err != nil {
			return nil, err
		}

		if seen[absolute] {
			return nil, errors.New(
				"Duplicate XEPG snapshot file path",
			)
		}

		seen[absolute] = true

		entry := xepgSnapshotFile{
			Path: absolute,
		}

		info, err := os.Lstat(absolute)

		if errors.Is(err, os.ErrNotExist) {

			snapshot.Files = append(
				snapshot.Files,
				entry,
			)

			continue
		}

		if err != nil {
			return nil, fmt.Errorf(
				"Cannot inspect XEPG snapshot file %d: %w",
				index,
				err,
			)
		}

		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf(
				"XEPG snapshot file %d is not a regular file",
				index,
			)
		}

		entry.Existed = true
		entry.Mode = info.Mode().Perm()

		entry.Backup = filepath.Join(
			directory,
			fmt.Sprintf("%03d.snapshot", index),
		)

		if err := copyXEPGSnapshotFile(
			absolute,
			entry.Backup,
			0600,
		); err != nil {

			return nil, fmt.Errorf(
				"Cannot capture XEPG snapshot file %d: %w",
				index,
				err,
			)
		}

		snapshot.Files = append(
			snapshot.Files,
			entry,
		)
	}

	success = true

	return snapshot, nil
}

func restoreXEPGSnapshotFile(
	entry xepgSnapshotFile,
) error {

	if !entry.Existed {

		info, err := os.Lstat(entry.Path)

		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return errors.New(
				"Refusing to remove non-regular rollback target",
			)
		}

		return os.Remove(entry.Path)
	}

	info, err := os.Lstat(entry.Backup)

	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return errors.New(
			"Invalid XEPG rollback snapshot file",
		)
	}

	targetInfo, err := os.Lstat(entry.Path)

	if err == nil && !targetInfo.Mode().IsRegular() {
		return errors.New(
			"Invalid XEPG rollback destination",
		)
	}

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	temporary, err := os.MkdirTemp(
		filepath.Dir(entry.Path),
		".xepg-restore-",
	)

	if err != nil {
		return err
	}

	defer os.RemoveAll(temporary)

	staged := filepath.Join(temporary, "restored")

	if err := copyXEPGSnapshotFile(
		entry.Backup,
		staged,
		entry.Mode,
	); err != nil {

		return err
	}

	if err := os.Rename(
		staged,
		entry.Path,
	); err != nil {

		return err
	}

	return nil
}

func (snapshot *xepgRecoverySnapshot) Restore() error {

	if snapshot == nil {
		return errors.New(
			"Missing XEPG rollback snapshot",
		)
	}

	if snapshot.Directory == "" ||
		len(snapshot.Files) == 0 {

		return errors.New(
			"Invalid XEPG rollback snapshot",
		)
	}

	var restoreErrors error

	for index, entry := range snapshot.Files {

		if err := restoreXEPGSnapshotFile(
			entry,
		); err != nil {

			restoreErrors = errors.Join(
				restoreErrors,
				fmt.Errorf(
					"XEPG rollback file %d failed: %w",
					index,
					err,
				),
			)
		}
	}

	return restoreErrors
}

func (snapshot *xepgRecoverySnapshot) Discard() error {

	if snapshot == nil {
		return errors.New(
			"Missing XEPG rollback snapshot",
		)
	}

	if snapshot.Directory == "" {
		return errors.New(
			"Missing XEPG rollback directory",
		)
	}

	return os.RemoveAll(snapshot.Directory)
}
