package src

import (
	"errors"
	"fmt"
)

// runRefreshTransaction acquires the existing refresh mutex.
//
// Only outer transaction entry points may call this function.
// Internal functions must not acquire refreshMutex again.
//
// The supplied operation must complete all configuration
// mutations before returning. Background work is not covered.
func runRefreshTransaction(
	operation func() error,
) error {

	if operation == nil {
		return errors.New(
			"Refresh transaction operation is missing",
		)
	}

	refreshMutex.Lock()
	defer refreshMutex.Unlock()

	return runRefreshTransactionLocked(operation)
}

// runRefreshTransactionLocked executes an operation whose
// caller already owns refreshMutex.
//
// This function does not acquire or release the mutex.
// Lock ownership is a caller responsibility.
func runRefreshTransactionLocked(
	operation func() error,
) (err error) {

	if operation == nil {
		return errors.New(
			"Refresh transaction operation is missing",
		)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"Refresh transaction panicked (%T)",
				recovered,
			)
		}
	}()

	return operation()
}
