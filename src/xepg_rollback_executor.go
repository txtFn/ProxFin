package src

import (
	"errors"
	"fmt"
)

type xepgRollbackSnapshot interface {
	Restore() error
	Discard() error
}

type xepgRollbackResult struct {
	Committed        bool
	Restored         bool
	RetainedSnapshot xepgRollbackSnapshot
}

// Convert an operation panic into an error without
// exposing potentially sensitive panic contents.
func executeXEPGRollbackOperation(
	operation func() error,
) (err error) {

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"XEPG operation panicked (%T)",
				recovered,
			)
		}
	}()

	return operation()
}

// Caller must maintain exclusive access to all affected
// application state throughout this operation.
func executeXEPGWithRollback(
	capture func() (xepgRollbackSnapshot, error),
	operation func() error,
) (result xepgRollbackResult, err error) {

	if capture == nil {
		return result, errors.New(
			"XEPG snapshot capture function is missing",
		)
	}

	if operation == nil {
		return result, errors.New(
			"XEPG protected operation is missing",
		)
	}

	var snapshot xepgRollbackSnapshot

	captureErr := executeXEPGRollbackOperation(
		func() error {
			var err error
			snapshot, err = capture()
			return err
		},
	)

	if captureErr != nil {
		result.RetainedSnapshot = snapshot

		return result, fmt.Errorf(
			"XEPG snapshot capture failed: %w",
			captureErr,
		)
	}

	if snapshot == nil {
		return result, errors.New(
			"XEPG snapshot capture returned no snapshot",
		)
	}

	result.RetainedSnapshot = snapshot

	operationErr := executeXEPGRollbackOperation(
		operation,
	)

	if operationErr == nil {

		result.Committed = true

		if discardErr := executeXEPGRollbackOperation(
			snapshot.Discard,
		); discardErr != nil {

			return result, fmt.Errorf(
				"XEPG committed but snapshot cleanup failed: %w",
				discardErr,
			)
		}

		result.RetainedSnapshot = nil

		return result, nil
	}

	operationErr = fmt.Errorf(
		"XEPG protected operation failed: %w",
		operationErr,
	)

	restoreErr := executeXEPGRollbackOperation(
		snapshot.Restore,
	)

	if restoreErr != nil {

		return result, errors.Join(
			operationErr,
			fmt.Errorf(
				"XEPG automatic restoration failed: %w",
				restoreErr,
			),
		)
	}

	result.Restored = true

	if discardErr := executeXEPGRollbackOperation(
		snapshot.Discard,
	); discardErr != nil {

		return result, errors.Join(
			operationErr,
			fmt.Errorf(
				"XEPG restored but snapshot cleanup failed: %w",
				discardErr,
			),
		)
	}

	result.RetainedSnapshot = nil

	return result, operationErr
}
