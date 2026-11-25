package src

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

type channelMappingRecoverySnapshot struct {
	Channels         map[string]interface{}
	ChannelsFiltered map[string]interface{}
	XEPGCount        int64
	ActiveStreams    []string
	XMLTVMapping     map[string]interface{}
	XMLTVFiles       []string
}

func cloneRecoveryXEPGChannel(
	original XEPGChannelStruct,
) XEPGChannelStruct {

	cloned := original

	cloned.BackupChannel1 =
		cloneRecoveryBackupStream(
			original.BackupChannel1,
		)

	cloned.BackupChannel2 =
		cloneRecoveryBackupStream(
			original.BackupChannel2,
		)

	cloned.BackupChannel3 =
		cloneRecoveryBackupStream(
			original.BackupChannel3,
		)

	return cloned
}

func cloneRecoveryMappingValue(
	original interface{},
	depth int,
) (interface{}, error) {

	if depth > 128 {
		return nil, errors.New(
			"Recovery mapping exceeds maximum nesting depth",
		)
	}

	switch value := original.(type) {

	case nil:
		return nil, nil

	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:

		return value, nil

	case XEPGChannelStruct:

		return cloneRecoveryXEPGChannel(value), nil

	case *XEPGChannelStruct:

		if value == nil {
			return (*XEPGChannelStruct)(nil), nil
		}

		cloned := cloneRecoveryXEPGChannel(*value)

		return &cloned, nil

	case BackupStream:

		return *cloneRecoveryBackupStream(&value), nil

	case *BackupStream:

		return cloneRecoveryBackupStream(value), nil

	case map[string]interface{}:

		return cloneRecoveryMappingMap(
			value,
			depth+1,
		)

	case map[string]string:

		if value == nil {
			return map[string]string(nil), nil
		}

		cloned := make(
			map[string]string,
			len(value),
		)

		for key, item := range value {
			cloned[key] = item
		}

		return cloned, nil

	case []interface{}:

		if value == nil {
			return []interface{}(nil), nil
		}

		cloned := make(
			[]interface{},
			len(value),
		)

		for index, item := range value {

			copied, err :=
				cloneRecoveryMappingValue(
					item,
					depth+1,
				)

			if err != nil {
				return nil, err
			}

			cloned[index] = copied
		}

		return cloned, nil

	case []string:

		return slices.Clone(value), nil

	case json.RawMessage:

		return slices.Clone(value), nil

	case []byte:

		return slices.Clone(value), nil

	default:

		return nil, fmt.Errorf(
			"Unsupported recovery mapping type: %T",
			original,
		)
	}
}

func cloneRecoveryMappingMap(
	original map[string]interface{},
	depth int,
) (map[string]interface{}, error) {

	if depth > 128 {
		return nil, errors.New(
			"Recovery mapping exceeds maximum nesting depth",
		)
	}

	if original == nil {
		return nil, nil
	}

	cloned := make(
		map[string]interface{},
		len(original),
	)

	for key, value := range original {

		copied, err :=
			cloneRecoveryMappingValue(
				value,
				depth+1,
			)

		if err != nil {
			return nil, err
		}

		cloned[key] = copied
	}

	return cloned, nil
}

// Caller must ensure exclusive access to channel mapping state.
func captureChannelMappingRecoverySnapshot() (
	*channelMappingRecoverySnapshot,
	error,
) {

	channels, err :=
		cloneRecoveryMappingMap(
			Data.XEPG.Channels,
			0,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"Channel snapshot failed: %w",
			err,
		)
	}

	filtered, err :=
		cloneRecoveryMappingMap(
			Data.XEPG.ChannelsFiltered,
			0,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"Filtered channel snapshot failed: %w",
			err,
		)
	}

	xmltvMapping, err :=
		cloneRecoveryMappingMap(
			Data.XMLTV.Mapping,
			0,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"XMLTV mapping snapshot failed: %w",
			err,
		)
	}

	return &channelMappingRecoverySnapshot{

		Channels: channels,

		ChannelsFiltered: filtered,

		XEPGCount: Data.XEPG.XEPGCount,

		ActiveStreams: slices.Clone(
			Data.Cache.Streams.Active,
		),

		XMLTVMapping: xmltvMapping,

		XMLTVFiles: slices.Clone(
			Data.XMLTV.Files,
		),
	}, nil
}

// Caller must ensure exclusive access to channel mapping state.
func (snapshot *channelMappingRecoverySnapshot) Restore() error {

	if snapshot == nil {
		return errors.New(
			"Channel mapping recovery snapshot is missing",
		)
	}

	channels, err :=
		cloneRecoveryMappingMap(
			snapshot.Channels,
			0,
		)

	if err != nil {
		return fmt.Errorf(
			"Channel restoration preparation failed: %w",
			err,
		)
	}

	filtered, err :=
		cloneRecoveryMappingMap(
			snapshot.ChannelsFiltered,
			0,
		)

	if err != nil {
		return fmt.Errorf(
			"Filtered channel restoration preparation failed: %w",
			err,
		)
	}

	xmltvMapping, err :=
		cloneRecoveryMappingMap(
			snapshot.XMLTVMapping,
			0,
		)

	if err != nil {
		return fmt.Errorf(
			"XMLTV restoration preparation failed: %w",
			err,
		)
	}

	activeStreams :=
		slices.Clone(snapshot.ActiveStreams)

	xmltvFiles :=
		slices.Clone(snapshot.XMLTVFiles)

	// Publish only after every copy succeeds.

	Data.XEPG.Channels = channels

	Data.XEPG.ChannelsFiltered = filtered

	Data.XEPG.XEPGCount = snapshot.XEPGCount

	Data.Cache.Streams.Active = activeStreams

	Data.XMLTV.Mapping = xmltvMapping

	Data.XMLTV.Files = xmltvFiles

	return nil
}
