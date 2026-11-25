package src

import (
	"errors"
	"fmt"
)

type combinedXEPGRecoverySnapshot struct {
	Disk      *xepgRecoverySnapshot
	Streaming *streamingURLRecoverySnapshot
	Mapping   *channelMappingRecoverySnapshot
	Expanded  *xepgExpandedStateRecoverySnapshot

	restorationFailed bool
	discarded         bool
}

// Caller must ensure exclusive access to all affected
// application state and prevent concurrent file mutations.
// Caller must not already hold providerMutex.
func captureCombinedXEPGRecoverySnapshot(
	root string,
	paths []string,
) (*combinedXEPGRecoverySnapshot, error) {

	disk, err :=
		captureXEPGRecoverySnapshot(root, paths)

	if err != nil {
		return nil, fmt.Errorf(
			"Combined disk snapshot failed: %w",
			err,
		)
	}

	cleanupCaptureFailure := func(
		captureErr error,
	) (*combinedXEPGRecoverySnapshot, error) {

		if discardErr := disk.Discard(); discardErr != nil {

			return nil, errors.Join(
				captureErr,
				fmt.Errorf(
					"Incomplete snapshot cleanup failed: %w",
					discardErr,
				),
			)
		}

		return nil, captureErr
	}

	mapping, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		return cleanupCaptureFailure(
			fmt.Errorf(
				"Combined channel mapping snapshot failed: %w",
				err,
			),
		)
	}

	if mapping == nil {
		return cleanupCaptureFailure(
			errors.New(
				"Combined mapping snapshot is missing",
			),
		)
	}

	expanded, err :=
		captureXEPGExpandedStateRecoverySnapshot()

	if err != nil {
		return cleanupCaptureFailure(
			fmt.Errorf(
				"Combined expanded state snapshot failed: %w",
				err,
			),
		)
	}

	if expanded == nil {
		return cleanupCaptureFailure(
			errors.New(
				"Combined expanded snapshot is missing",
			),
		)
	}

	streaming :=
		captureStreamingURLRecoverySnapshot()

	if streaming == nil {
		return cleanupCaptureFailure(
			errors.New(
				"Combined streaming snapshot is missing",
			),
		)
	}

	return &combinedXEPGRecoverySnapshot{
		Disk:      disk,
		Streaming: streaming,
		Mapping:   mapping,
		Expanded:  expanded,
	}, nil
}

func (snapshot *combinedXEPGRecoverySnapshot) validate() error {

	if snapshot == nil {
		return errors.New(
			"Combined recovery snapshot is missing",
		)
	}

	if snapshot.discarded {
		return errors.New(
			"Combined recovery snapshot was discarded",
		)
	}

	if snapshot.Disk == nil {
		return errors.New(
			"Combined disk snapshot is missing",
		)
	}

	if snapshot.Streaming == nil {
		return errors.New(
			"Combined streaming snapshot is missing",
		)
	}

	if snapshot.Mapping == nil {
		return errors.New(
			"Combined mapping snapshot is missing",
		)
	}

	if snapshot.Expanded == nil {
		return errors.New(
			"Combined expanded snapshot is missing",
		)
	}

	return nil
}

// Caller must maintain exclusive access throughout
// restoration and must not already hold providerMutex.
func (snapshot *combinedXEPGRecoverySnapshot) Restore() error {

	if err := snapshot.validate(); err != nil {
		return err
	}

	snapshot.restorationFailed = true

	var recoveryErrors error

	if err := snapshot.Disk.Restore(); err != nil {

		recoveryErrors = errors.Join(
			recoveryErrors,
			fmt.Errorf(
				"Combined disk restoration failed: %w",
				err,
			),
		)
	}

	if err := snapshot.Streaming.Restore(); err != nil {

		recoveryErrors = errors.Join(
			recoveryErrors,
			fmt.Errorf(
				"Combined streaming restoration failed: %w",
				err,
			),
		)
	}

	if err := snapshot.Mapping.Restore(); err != nil {

		recoveryErrors = errors.Join(
			recoveryErrors,
			fmt.Errorf(
				"Combined mapping restoration failed: %w",
				err,
			),
		)
	}

	if err := snapshot.Expanded.Restore(); err != nil {

		recoveryErrors = errors.Join(
			recoveryErrors,
			fmt.Errorf(
				"Combined expanded state restoration failed: %w",
				err,
			),
		)
	}

	if recoveryErrors != nil {
		return recoveryErrors
	}

	snapshot.restorationFailed = false

	return nil
}

// Discard releases memory and removes the disk snapshot.
// Caller must not discard before the rebuild is committed
// or a required restoration has completed successfully.
func (snapshot *combinedXEPGRecoverySnapshot) Discard() error {

	if snapshot == nil {
		return errors.New(
			"Combined recovery snapshot is missing",
		)
	}

	if snapshot.discarded {
		return nil
	}

	if snapshot.restorationFailed {
		return errors.New(
			"Cannot discard snapshot after failed restoration",
		)
	}

	if err := snapshot.validate(); err != nil {
		return err
	}

	if err := snapshot.Disk.Discard(); err != nil {

		return fmt.Errorf(
			"Combined disk snapshot cleanup failed: %w",
			err,
		)
	}

	snapshot.Disk = nil
	snapshot.Streaming = nil
	snapshot.Mapping = nil
	snapshot.Expanded = nil

	snapshot.discarded = true

	return nil
}
