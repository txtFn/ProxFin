package src

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestXEPGRollbackIntegration(t *testing.T) {

	operationFailure := errors.New("test operation failure")

	cases := []struct {
		name          string
		fail          bool
		panic         bool
		pipeline      bool
		captureFail   bool
		wantCommitted bool
		wantRestored  bool
	}{
		{
			name:          "Commit",
			wantCommitted: true,
		},
		{
			name:         "RestoreAfterFailure",
			fail:         true,
			wantRestored: true,
		},
		{
			name:         "RestoreAfterPanic",
			panic:        true,
			wantRestored: true,
		},
		{
			name:         "RestoreAfterPipelineFailure",
			pipeline:     true,
			wantRestored: true,
		},
		{
			name:        "CaptureFailure",
			captureFail: true,
		},
	}

	for _, tc := range cases {

		t.Run(tc.name, func(t *testing.T) {

			root, path := setupCombinedRecoveryTest(t)

			originalMapping, err :=
				captureChannelMappingRecoverySnapshot()

			if err != nil {
				t.Fatal(err)
			}

			originalStreaming :=
				captureStreamingURLRecoverySnapshot()

			originalDisk, err := os.ReadFile(path)

			if err != nil {
				t.Fatal(err)
			}

			var snapshotDirectory string
			operationCalled := false
			laterStageCalled := false

			capture := func() (xepgRollbackSnapshot, error) {

				paths := []string{path}

				if tc.captureFail {
					paths = append(paths, path)
				}

				snapshot, err :=
					captureCombinedXEPGRecoverySnapshot(
						root,
						paths,
					)

				if err != nil {
					return nil, err
				}

				if snapshot == nil {
					return nil, errors.New(
						"Missing combined snapshot",
					)
				}

				snapshotDirectory = snapshot.Disk.Directory

				return snapshot, nil
			}

			operation := func() error {

				operationCalled = true

				if tc.pipeline {

					return executeXEPGRecoveryStages(
						[]xepgRecoveryStage{
							{
								Name: "modify-state",
								Run: func() error {
									mutateCombinedRecoveryTest(
										t, path,
									)
									return nil
								},
							},
							{
								Name: "fail-stage",
								Run: func() error {
									return operationFailure
								},
							},
							{
								Name: "must-not-run",
								Run: func() error {
									laterStageCalled = true
									return nil
								},
							},
						},
					)
				}

				mutateCombinedRecoveryTest(t, path)

				if tc.panic {
					panic("isolated integration panic")
				}

				if tc.fail {
					return operationFailure
				}

				return nil
			}

			result, err := executeXEPGWithRollback(
				capture,
				operation,
			)

			wantError := tc.fail ||
				tc.panic ||
				tc.pipeline ||
				tc.captureFail

			if (err != nil) != wantError {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tc.fail || tc.pipeline {

				if !errors.Is(err, operationFailure) {
					t.Fatal("Original error was lost")
				}
			}

			if result.Committed != tc.wantCommitted {
				t.Fatal("Incorrect committed status")
			}

			if result.Restored != tc.wantRestored {
				t.Fatal("Incorrect restored status")
			}

			if result.RetainedSnapshot != nil {
				t.Fatal("Unexpected retained snapshot")
			}

			if laterStageCalled {
				t.Fatal("Pipeline continued after failure")
			}

			if operationCalled == tc.captureFail {
				t.Fatal("Incorrect operation execution")
			}

			if tc.wantCommitted {

				assertCombinedRecoveryDisk(
					t, path, "modified",
				)

				if Data.XEPG.XEPGCount != 99 {
					t.Fatal("Committed memory changes lost")
				}

				t.Log("PASS: Disk changes committed")
				t.Log("PASS: Memory changes committed")

			} else {

				currentDisk, err := os.ReadFile(path)

				if err != nil {
					t.Fatal(err)
				}

				if !reflect.DeepEqual(
					originalDisk, currentDisk,
				) {
					t.Fatal("Original disk bytes not preserved")
				}

				currentMapping, err :=
					captureChannelMappingRecoverySnapshot()

				if err != nil {
					t.Fatal(err)
				}

				if !reflect.DeepEqual(
					originalMapping,
					currentMapping,
				) {
					t.Fatal("Channel mapping state differs")
				}

				currentStreaming :=
					captureStreamingURLRecoverySnapshot()

				if !reflect.DeepEqual(
					originalStreaming,
					currentStreaming,
				) {
					t.Fatal("Streaming URL state differs")
				}

				t.Log("PASS: Original disk bytes preserved")
				t.Log("PASS: Channel mapping state preserved")
				t.Log("PASS: Streaming URL state preserved")
			}

			if snapshotDirectory != "" {

				if _, err := os.Stat(
					snapshotDirectory,
				); !os.IsNotExist(err) {
					t.Fatal("Snapshot directory was not removed")
				}
			}

			leftovers, err := filepath.Glob(
				filepath.Join(root, ".xepg-rollback-*"),
			)

			if err != nil {
				t.Fatal(err)
			}

			if len(leftovers) != 0 {
				t.Fatal("Unexpected rollback directories remain")
			}

			t.Log("PASS: Snapshot cleanup verified")
			t.Logf("PASS: %s", tc.name)
		})
	}
}

func TestXEPGRollbackIntegrationMultipleFiles(t *testing.T) {

	root, path := setupCombinedRecoveryTest(t)

	secondPath := filepath.Join(root, "second.json")
	newPath := filepath.Join(root, "new.json")

	original := []byte(`{"original":true}`)

	if err := os.WriteFile(
		secondPath, original, 0600,
	); err != nil {
		t.Fatal(err)
	}

	operationFailure := errors.New("multi-file failure")

	result, err := executeXEPGWithRollback(
		func() (xepgRollbackSnapshot, error) {

			snapshot, err :=
				captureCombinedXEPGRecoverySnapshot(
					root,
					[]string{
						path,
						secondPath,
						newPath,
					},
				)

			if err != nil {
				return nil, err
			}

			return snapshot, nil
		},
		func() error {

			mutateCombinedRecoveryTest(t, path)

			if err := os.Remove(secondPath); err != nil {
				return err
			}

			if err := os.WriteFile(
				newPath,
				[]byte(`{"created":true}`),
				0600,
			); err != nil {
				return err
			}

			return operationFailure
		},
	)

	if !errors.Is(err, operationFailure) {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Restored ||
		result.Committed ||
		result.RetainedSnapshot != nil {
		t.Fatal("Unexpected rollback result")
	}

	assertCombinedRecoveryDisk(t, path, "original")

	restored, err := os.ReadFile(secondPath)

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(original, restored) {
		t.Fatal("Deleted file was not restored correctly")
	}

	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal("Newly created file was not removed")
	}

	t.Log("PASS: Modified file restored")
	t.Log("PASS: Deleted file recreated")
	t.Log("PASS: Newly created file removed")
}
