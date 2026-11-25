package src

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestXEPGRecoveryPipelineSuccess(t *testing.T) {

	var executed []string

	names := []string{
		"create-mapping",
		"create-database",
		"map-channels",
		"cleanup-database",
		"generate-xmltv",
		"generate-m3u",
	}

	var stages []xepgRecoveryStage

	for _, name := range names {

		stageName := name

		stages = append(stages, xepgRecoveryStage{
			Name: stageName,
			Run: func() error {
				executed = append(executed, stageName)
				return nil
			},
		})
	}

	if err := executeXEPGRecoveryStages(stages); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(executed, names) {
		t.Fatalf(
			"Incorrect execution order: %v",
			executed,
		)
	}
}

func TestXEPGRecoveryPipelineFailure(t *testing.T) {

	names := []string{
		"create-mapping",
		"create-database",
		"map-channels",
		"cleanup-database",
		"generate-xmltv",
		"generate-m3u",
	}

	expectedError := errors.New("simulated stage failure")

	for failureIndex := range names {

		t.Run(names[failureIndex], func(t *testing.T) {

			var executed []string
			var stages []xepgRecoveryStage

			for index, name := range names {

				stageIndex := index
				stageName := name

				stages = append(stages, xepgRecoveryStage{
					Name: stageName,
					Run: func() error {

						executed = append(executed, stageName)

						if stageIndex == failureIndex {
							return expectedError
						}

						return nil
					},
				})
			}

			err := executeXEPGRecoveryStages(stages)

			if err == nil {
				t.Fatal("Expected pipeline failure")
			}

			if !errors.Is(err, expectedError) {
				t.Fatal("Original error was not preserved")
			}

			if !strings.Contains(
				err.Error(),
				names[failureIndex],
			) {
				t.Fatal("Failed stage name missing")
			}

			expected := names[:failureIndex+1]

			if !reflect.DeepEqual(executed, expected) {
				t.Fatalf(
					"Unexpected stages executed: %v; expected %v",
					executed,
					expected,
				)
			}
		})
	}
}

func TestXEPGRecoveryPipelineEmpty(t *testing.T) {

	if err := executeXEPGRecoveryStages(nil); err == nil {
		t.Fatal("Empty pipeline was accepted")
	}

	if err := executeXEPGRecoveryStages(
		[]xepgRecoveryStage{},
	); err == nil {
		t.Fatal("Empty stage list was accepted")
	}
}

func TestXEPGRecoveryPipelineInvalidStage(t *testing.T) {

	cases := []struct {
		name  string
		stage xepgRecoveryStage
	}{
		{
			name: "missing-name",
			stage: xepgRecoveryStage{
				Run: func() error {
					return nil
				},
			},
		},
		{
			name: "missing-executor",
			stage: xepgRecoveryStage{
				Name: "invalid-stage",
				Run:  nil,
			},
		},
	}

	for _, tc := range cases {

		t.Run(tc.name, func(t *testing.T) {

			executed := false

			stages := []xepgRecoveryStage{
				{
					Name: "valid-stage",
					Run: func() error {
						executed = true
						return nil
					},
				},
				tc.stage,
			}

			if err := executeXEPGRecoveryStages(
				stages,
			); err == nil {
				t.Fatal("Invalid pipeline was accepted")
			}

			if executed {
				t.Fatal(
					"Pipeline executed before validation completed",
				)
			}
		})
	}
}

func TestXEPGRecoveryPipelineStructure(t *testing.T) {

	cases := []struct {
		name     string
		stages   []xepgRecoveryStage
		expected []string
	}{
		{
			name:   "full",
			stages: fullXEPGRecoveryStages(),
			expected: []string{
				"create-mapping",
				"create-database",
				"map-channels",
				"cleanup-database",
				"generate-xmltv",
				"generate-m3u",
			},
		},
		{
			name:   "incremental",
			stages: incrementalXEPGRecoveryStages(),
			expected: []string{
				"create-database",
				"map-channels",
				"cleanup-database",
				"generate-xmltv",
				"generate-m3u",
			},
		},
	}

	for _, tc := range cases {

		t.Run(tc.name, func(t *testing.T) {

			var names []string

			for _, stage := range tc.stages {

				if stage.Run == nil {
					t.Fatalf(
						"Missing executor for %s",
						stage.Name,
					)
				}

				names = append(names, stage.Name)
			}

			if !reflect.DeepEqual(names, tc.expected) {
				t.Fatalf(
					"Incorrect pipeline structure: %v",
					names,
				)
			}
		})
	}
}
