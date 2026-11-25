package src

import (
	"fmt"
)

type xepgRecoveryStage struct {
	Name string
	Run  func() error
}

func executeXEPGRecoveryStages(
	stages []xepgRecoveryStage,
) error {

	if len(stages) == 0 {
		return fmt.Errorf("XEPG recovery pipeline has no stages")
	}

	// Validate all stages before performing any mutations.
	for _, stage := range stages {

		if stage.Name == "" {
			return fmt.Errorf(
				"XEPG recovery stage has no name",
			)
		}

		if stage.Run == nil {
			return fmt.Errorf(
				"XEPG recovery stage %s has no executor",
				stage.Name,
			)
		}
	}

	for _, stage := range stages {

		if err := stage.Run(); err != nil {
			return fmt.Errorf(
				"XEPG recovery stage %s failed: %w",
				stage.Name,
				err,
			)
		}
	}

	return nil
}

func fullXEPGRecoveryStages() []xepgRecoveryStage {

	return []xepgRecoveryStage{
		{
			Name: "create-mapping",
			Run:  createXEPGMapping,
		},
		{
			Name: "create-database",
			Run:  createXEPGDatabase,
		},
		{
			Name: "map-channels",
			Run:  mapping,
		},
		{
			Name: "cleanup-database",
			Run:  cleanupXEPG,
		},
		{
			Name: "generate-xmltv",
			Run:  createXMLTVFile,
		},
		{
			Name: "generate-m3u",
			Run:  createM3UFile,
		},
	}
}

func incrementalXEPGRecoveryStages() []xepgRecoveryStage {

	return []xepgRecoveryStage{
		{
			Name: "create-database",
			Run:  createXEPGDatabase,
		},
		{
			Name: "map-channels",
			Run:  mapping,
		},
		{
			Name: "cleanup-database",
			Run:  cleanupXEPG,
		},
		{
			Name: "generate-xmltv",
			Run:  createXMLTVFile,
		},
		{
			Name: "generate-m3u",
			Run:  createM3UFile,
		},
	}
}
