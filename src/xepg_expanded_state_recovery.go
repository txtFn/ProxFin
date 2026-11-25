package src

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
)

type xepgExpandedStateRecoverySnapshot struct {
	Settings        SettingsStruct
	AllStreams      []interface{}
	ActiveStreams   []interface{}
	InactiveStreams []interface{}
	Filters         []Filter
	GroupText       []string
	GroupValue      []string
	ActivePreview   []string
	InactivePreview []string
	TempDirectory   string
	DeviceID        string
}

func cloneExpandedSettings(
	original SettingsStruct,
) (SettingsStruct, error) {

	cloned := original

	cloned.FileM3U = slices.Clone(original.FileM3U)
	cloned.FileXMLTV = slices.Clone(original.FileXMLTV)
	cloned.Update = slices.Clone(original.Update)

	var err error

	cloned.Files.M3U, err =
		cloneRecoveryMappingMap(original.Files.M3U, 0)

	if err != nil {
		return SettingsStruct{}, fmt.Errorf(
			"M3U settings snapshot failed: %w", err,
		)
	}

	cloned.Files.HDHR, err =
		cloneRecoveryMappingMap(original.Files.HDHR, 0)

	if err != nil {
		return SettingsStruct{}, fmt.Errorf(
			"HDHR settings snapshot failed: %w", err,
		)
	}

	cloned.Files.XMLTV, err =
		cloneRecoveryMappingMap(original.Files.XMLTV, 0)

	if err != nil {
		return SettingsStruct{}, fmt.Errorf(
			"XMLTV settings snapshot failed: %w", err,
		)
	}

	if original.Filter != nil {
		cloned.Filter = make(
			map[int64]interface{},
			len(original.Filter),
		)

		for id, value := range original.Filter {

			var copied interface{}

			switch filter := value.(type) {

			case FilterStruct:
				copied = filter

			case *FilterStruct:
				if filter == nil {
					copied = (*FilterStruct)(nil)
				} else {
					duplicate := *filter
					copied = &duplicate
				}

			default:
				copied, err =
					cloneRecoveryMappingValue(value, 0)

				if err != nil {
					return SettingsStruct{}, fmt.Errorf(
						"Filter settings snapshot failed: %w",
						err,
					)
				}
			}

			cloned.Filter[id] = copied
		}
	}

	return cloned, nil
}

// One memo is shared across all three stream lists.
// Repeated references to the same channel map remain shared.
type expandedStreamCloneMemo struct {
	stringMaps map[uintptr]map[string]string
}

func cloneExpandedStreamList(
	original []interface{},
	memo *expandedStreamCloneMemo,
) ([]interface{}, error) {

	if original == nil {
		return nil, nil
	}

	cloned := make([]interface{}, len(original))

	for index, item := range original {

		switch stream := item.(type) {

		case nil:
			cloned[index] = nil

		case map[string]string:

			if stream == nil {
				cloned[index] = map[string]string(nil)
				continue
			}

			identity := uintptr(
				reflect.ValueOf(stream).UnsafePointer(),
			)

			copied, exists := memo.stringMaps[identity]

			if !exists {
				copied = maps.Clone(stream)
				memo.stringMaps[identity] = copied
			}

			cloned[index] = copied

		default:
			return nil, fmt.Errorf(
				"Unsupported stream snapshot type: %T",
				item,
			)
		}
	}

	return cloned, nil
}

func cloneExpandedState(
	original xepgExpandedStateRecoverySnapshot,
) (*xepgExpandedStateRecoverySnapshot, error) {

	cloned := original

	var err error

	cloned.Settings, err =
		cloneExpandedSettings(original.Settings)

	if err != nil {
		return nil, err
	}

	memo := &expandedStreamCloneMemo{
		stringMaps: make(map[uintptr]map[string]string),
	}

	cloned.AllStreams, err =
		cloneExpandedStreamList(original.AllStreams, memo)

	if err != nil {
		return nil, err
	}

	cloned.ActiveStreams, err =
		cloneExpandedStreamList(original.ActiveStreams, memo)

	if err != nil {
		return nil, err
	}

	cloned.InactiveStreams, err =
		cloneExpandedStreamList(original.InactiveStreams, memo)

	if err != nil {
		return nil, err
	}

	cloned.Filters = slices.Clone(original.Filters)
	cloned.GroupText = slices.Clone(original.GroupText)
	cloned.GroupValue = slices.Clone(original.GroupValue)
	cloned.ActivePreview = slices.Clone(original.ActivePreview)
	cloned.InactivePreview = slices.Clone(original.InactivePreview)

	return &cloned, nil
}

// Caller must own exclusive refresh state.
// Caller must not already hold providerMutex.
func captureXEPGExpandedStateRecoverySnapshot() (
	*xepgExpandedStateRecoverySnapshot,
	error,
) {

	providerMutex.RLock()
	defer providerMutex.RUnlock()

	original := xepgExpandedStateRecoverySnapshot{
		Settings:        Settings,
		AllStreams:      Data.Streams.All,
		ActiveStreams:   Data.Streams.Active,
		InactiveStreams: Data.Streams.Inactive,
		Filters:         Data.Filter,
		GroupText:       Data.Playlist.M3U.Groups.Text,
		GroupValue:      Data.Playlist.M3U.Groups.Value,
		ActivePreview:   Data.StreamPreviewUI.Active,
		InactivePreview: Data.StreamPreviewUI.Inactive,
		TempDirectory:   System.Folder.Temp,
		DeviceID:        System.DeviceID,
	}

	return cloneExpandedState(original)
}

// Caller must own exclusive refresh state.
// Caller must not already hold providerMutex.
func (snapshot *xepgExpandedStateRecoverySnapshot) Restore() error {

	if snapshot == nil {
		return errors.New("Expanded XEPG snapshot is missing")
	}

	// Prepare every independent copy before publishing.
	restored, err := cloneExpandedState(*snapshot)

	if err != nil {
		return fmt.Errorf(
			"Expanded XEPG restoration preparation failed: %w",
			err,
		)
	}

	providerMutex.Lock()

	Settings = restored.Settings

	Data.Streams.All = restored.AllStreams
	Data.Streams.Active = restored.ActiveStreams
	Data.Streams.Inactive = restored.InactiveStreams

	Data.Filter = restored.Filters

	Data.Playlist.M3U.Groups.Text = restored.GroupText
	Data.Playlist.M3U.Groups.Value = restored.GroupValue

	Data.StreamPreviewUI.Active = restored.ActivePreview
	Data.StreamPreviewUI.Inactive = restored.InactivePreview

	System.Folder.Temp = restored.TempDirectory
	System.DeviceID = restored.DeviceID

	providerMutex.Unlock()

	// Rebuild derived EPG data on demand.
	clearXMLTVCache()

	return nil
}
