package src

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type rollbackExecutorTestSnapshot struct {
	calls        *[]string
	restoreErr   error
	discardErr   error
	restorePanic bool
	discardPanic bool
}

func (s *rollbackExecutorTestSnapshot) Restore() error {
	*s.calls = append(*s.calls, "restore")

	if s.restorePanic {
		panic("private restore details")
	}

	return s.restoreErr
}

func (s *rollbackExecutorTestSnapshot) Discard() error {
	*s.calls = append(*s.calls, "discard")

	if s.discardPanic {
		panic("private discard details")
	}

	return s.discardErr
}

func TestXEPGRollbackExecutorScenarios(t *testing.T) {

	operationFailure := errors.New("operation failure")
	captureFailure := errors.New("capture failure")
	restoreFailure := errors.New("restore failure")
	discardFailure := errors.New("discard failure")

	cases := []struct {
		name           string
		captureErr     error
		operationErr   error
		restoreErr     error
		discardErr     error
		capturePanic   bool
		operationPanic bool
		restorePanic   bool
		discardPanic   bool
		missing        bool
		partialCapture bool
		committed      bool
		restored       bool
		retained       bool
		wantError      bool
		expectedErrors []error
		expectedCalls  []string
	}{
		{
			name:      "Success",
			committed: true,
			expectedCalls: []string{
				"capture", "operation", "discard",
			},
		},
		{
			name:           "OperationFailure",
			operationErr:   operationFailure,
			restored:       true,
			wantError:      true,
			expectedErrors: []error{operationFailure},
			expectedCalls: []string{
				"capture", "operation", "restore", "discard",
			},
		},
		{
			name:           "OperationPanic",
			operationPanic: true,
			restored:       true,
			wantError:      true,
			expectedCalls: []string{
				"capture", "operation", "restore", "discard",
			},
		},
		{
			name:           "CaptureFailure",
			captureErr:     captureFailure,
			wantError:      true,
			expectedErrors: []error{captureFailure},
			expectedCalls:  []string{"capture"},
		},
		{
			name:          "CapturePanic",
			capturePanic:  true,
			wantError:     true,
			expectedCalls: []string{"capture"},
		},
		{
			name:          "MissingSnapshot",
			missing:       true,
			wantError:     true,
			expectedCalls: []string{"capture"},
		},
		{
			name:         "RestorationFailure",
			operationErr: operationFailure,
			restoreErr:   restoreFailure,
			retained:     true,
			wantError:    true,
			expectedErrors: []error{
				operationFailure, restoreFailure,
			},
			expectedCalls: []string{
				"capture", "operation", "restore",
			},
		},
		{
			name:           "RestorationPanic",
			operationErr:   operationFailure,
			restorePanic:   true,
			retained:       true,
			wantError:      true,
			expectedErrors: []error{operationFailure},
			expectedCalls: []string{
				"capture", "operation", "restore",
			},
		},
		{
			name:           "CommittedCleanupFailure",
			discardErr:     discardFailure,
			committed:      true,
			retained:       true,
			wantError:      true,
			expectedErrors: []error{discardFailure},
			expectedCalls: []string{
				"capture", "operation", "discard",
			},
		},
		{
			name:         "RestoredCleanupFailure",
			operationErr: operationFailure,
			discardErr:   discardFailure,
			restored:     true,
			retained:     true,
			wantError:    true,
			expectedErrors: []error{
				operationFailure, discardFailure,
			},
			expectedCalls: []string{
				"capture", "operation", "restore", "discard",
			},
		},
		{
			name:         "CleanupPanic",
			discardPanic: true,
			committed:    true,
			retained:     true,
			wantError:    true,
			expectedCalls: []string{
				"capture", "operation", "discard",
			},
		},
		{
			name:           "PartialCaptureFailure",
			captureErr:     captureFailure,
			partialCapture: true,
			retained:       true,
			wantError:      true,
			expectedErrors: []error{captureFailure},
			expectedCalls:  []string{"capture"},
		},
	}

	for _, tc := range cases {

		t.Run(tc.name, func(t *testing.T) {

			var calls []string

			snapshot := &rollbackExecutorTestSnapshot{
				calls:        &calls,
				restoreErr:   tc.restoreErr,
				discardErr:   tc.discardErr,
				restorePanic: tc.restorePanic,
				discardPanic: tc.discardPanic,
			}

			capture := func() (xepgRollbackSnapshot, error) {

				calls = append(calls, "capture")

				if tc.capturePanic {
					panic("private capture details")
				}

				if tc.captureErr != nil {

					if tc.partialCapture {
						return snapshot, tc.captureErr
					}

					return nil, tc.captureErr
				}

				if tc.missing {
					return nil, nil
				}

				return snapshot, nil
			}

			operation := func() error {

				calls = append(calls, "operation")

				if tc.operationPanic {
					panic("private operation details")
				}

				return tc.operationErr
			}

			result, err := executeXEPGWithRollback(
				capture,
				operation,
			)

			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Committed != tc.committed {
				t.Fatalf(
					"committed = %v; want %v",
					result.Committed,
					tc.committed,
				)
			}

			if result.Restored != tc.restored {
				t.Fatalf(
					"restored = %v; want %v",
					result.Restored,
					tc.restored,
				)
			}

			if (result.RetainedSnapshot != nil) != tc.retained {
				t.Fatal("unexpected retained snapshot state")
			}

			if tc.retained &&
				result.RetainedSnapshot != snapshot {
				t.Fatal("incorrect retained snapshot")
			}

			if result.Committed && result.Restored {
				t.Fatal("operation cannot be committed and restored")
			}

			for _, expected := range tc.expectedErrors {

				if !errors.Is(err, expected) {
					t.Fatalf(
						"missing expected error: %v",
						expected,
					)
				}
			}

			if !reflect.DeepEqual(calls, tc.expectedCalls) {
				t.Fatalf(
					"calls = %v; want %v",
					calls,
					tc.expectedCalls,
				)
			}

			if err != nil &&
				strings.Contains(err.Error(), "private") {
				t.Fatal("panic contents leaked")
			}

			if tc.capturePanic ||
				tc.operationPanic ||
				tc.restorePanic ||
				tc.discardPanic {

				if err == nil ||
					!strings.Contains(err.Error(), "panicked") {
					t.Fatal("panic was not reported")
				}
			}

			t.Logf("PASS: %s", tc.name)
		})
	}
}

func TestXEPGRollbackExecutorMissingCapture(t *testing.T) {

	called := false

	result, err := executeXEPGWithRollback(
		nil,
		func() error {
			called = true
			return nil
		},
	)

	if err == nil || called {
		t.Fatal("missing capture was not rejected")
	}

	if result.Committed ||
		result.Restored ||
		result.RetainedSnapshot != nil {
		t.Fatal("unexpected result")
	}
}

func TestXEPGRollbackExecutorMissingOperation(t *testing.T) {

	called := false

	result, err := executeXEPGWithRollback(
		func() (xepgRollbackSnapshot, error) {
			called = true
			return nil, nil
		},
		nil,
	)

	if err == nil || called {
		t.Fatal("missing operation was not rejected")
	}

	if result.Committed ||
		result.Restored ||
		result.RetainedSnapshot != nil {
		t.Fatal("unexpected result")
	}
}
