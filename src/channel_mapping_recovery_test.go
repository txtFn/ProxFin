package src

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func setupChannelMappingRecoveryTest(t *testing.T) {
	t.Helper()

	originalChannels := Data.XEPG.Channels
	originalFiltered := Data.XEPG.ChannelsFiltered
	originalCount := Data.XEPG.XEPGCount
	originalActive := Data.Cache.Streams.Active
	originalMapping := Data.XMLTV.Mapping
	originalFiles := Data.XMLTV.Files

	Data.XEPG.Channels = make(map[string]interface{})
	Data.XEPG.ChannelsFiltered = make(map[string]interface{})
	Data.XEPG.XEPGCount = 0
	Data.Cache.Streams.Active = []string{}
	Data.XMLTV.Mapping = make(map[string]interface{})
	Data.XMLTV.Files = []string{}

	t.Cleanup(func() {
		Data.XEPG.Channels = originalChannels
		Data.XEPG.ChannelsFiltered = originalFiltered
		Data.XEPG.XEPGCount = originalCount
		Data.Cache.Streams.Active = originalActive
		Data.XMLTV.Mapping = originalMapping
		Data.XMLTV.Files = originalFiles
	})
}

func TestChannelMappingRecoveryRestore(t *testing.T) {
	setupChannelMappingRecoveryTest(t)

	Data.XEPG.Channels["channel-1"] = XEPGChannelStruct{
		XActive:        true,
		XName:          "Original channel",
		XMapping:       "original.epg",
		BackupChannel1: &BackupStream{},
	}

	Data.XEPG.ChannelsFiltered["channel-1"] =
		map[string]interface{}{
			"active": true,
		}

	Data.XEPG.XEPGCount = 1

	Data.Cache.Streams.Active = []string{
		"original-hash",
	}

	Data.XMLTV.Mapping["guide.xml"] =
		map[string]interface{}{
			"original.epg": "Original channel",
		}

	Data.XMLTV.Files = []string{"guide.xml"}

	snapshot, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	Data.XEPG.Channels =
		map[string]interface{}{
			"replacement": XEPGChannelStruct{},
		}

	Data.XEPG.ChannelsFiltered = nil
	Data.XEPG.XEPGCount = 99
	Data.Cache.Streams.Active = []string{"changed"}
	Data.XMLTV.Mapping = nil
	Data.XMLTV.Files = []string{"changed.xml"}

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	restored, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(snapshot, restored) {
		t.Fatal("Restored mapping state differs from snapshot")
	}

	channel, ok :=
		Data.XEPG.Channels["channel-1"].(XEPGChannelStruct)

	if !ok {
		t.Fatal("Concrete channel type was not preserved")
	}

	if !channel.XActive ||
		channel.XName != "Original channel" ||
		channel.XMapping != "original.epg" {

		t.Fatal("Original channel attributes were not restored")
	}

	if Data.XEPG.XEPGCount != 1 {
		t.Fatal("Channel count was not restored")
	}
}

func TestChannelMappingRecoveryNestedIndependence(t *testing.T) {
	setupChannelMappingRecoveryTest(t)

	nested := map[string]interface{}{
		"names": []string{"original"},
		"values": []interface{}{
			map[string]interface{}{
				"enabled": true,
			},
		},
	}

	Data.XEPG.Channels["nested"] = nested
	Data.Cache.Streams.Active = []string{"hash-1"}
	Data.XMLTV.Files = []string{"guide.xml"}

	snapshot, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	nested["names"].([]string)[0] = "changed"

	nested["values"].([]interface{})[0].(map[string]interface{})["enabled"] = false

	Data.Cache.Streams.Active[0] = "changed"
	Data.XMLTV.Files[0] = "changed.xml"

	saved := snapshot.Channels["nested"].(map[string]interface{})

	if saved["names"].([]string)[0] != "original" {
		t.Fatal("Snapshot shares original string slice")
	}

	enabled := saved["values"].([]interface{})[0].(map[string]interface{})["enabled"]

	if enabled != true {
		t.Fatal("Snapshot shares nested map")
	}

	if snapshot.ActiveStreams[0] != "hash-1" ||
		snapshot.XMLTVFiles[0] != "guide.xml" {

		t.Fatal("Snapshot shares original cache slices")
	}

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	restored := Data.XEPG.Channels["nested"].(map[string]interface{})

	restored["names"].([]string)[0] = "changed again"

	if saved["names"].([]string)[0] != "original" {
		t.Fatal("Restored state shares snapshot slice")
	}

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	repeated, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(snapshot, repeated) {
		t.Fatal("Repeated restoration failed")
	}
}

func TestChannelMappingRecoveryChannelPointers(t *testing.T) {
	original := XEPGChannelStruct{
		BackupChannel1: &BackupStream{},
		BackupChannel2: &BackupStream{},
		BackupChannel3: &BackupStream{},
	}

	cloned := cloneRecoveryXEPGChannel(original)

	if !reflect.DeepEqual(original, cloned) {
		t.Fatal("Cloned channel differs from original")
	}

	if original.BackupChannel1 == cloned.BackupChannel1 ||
		original.BackupChannel2 == cloned.BackupChannel2 ||
		original.BackupChannel3 == cloned.BackupChannel3 {

		t.Fatal("Backup channel pointers were shared")
	}

	value, err :=
		cloneRecoveryMappingValue(&original, 0)

	if err != nil {
		t.Fatal(err)
	}

	pointer, ok := value.(*XEPGChannelStruct)

	if !ok || pointer == &original {
		t.Fatal("Channel pointer was not independently copied")
	}

	if pointer.BackupChannel1 == original.BackupChannel1 {
		t.Fatal("Nested backup pointer was shared")
	}
}

func TestChannelMappingRecoverySupportedTypes(t *testing.T) {
	values := []interface{}{
		nil,
		"text",
		true,
		int(1),
		int8(2),
		int16(3),
		int32(4),
		int64(5),
		uint(6),
		uint8(7),
		uint16(8),
		uint32(9),
		uint64(10),
		float32(1.5),
		float64(2.5),
		json.Number("123.45"),
		XEPGChannelStruct{},
		&XEPGChannelStruct{},
		(*XEPGChannelStruct)(nil),
		BackupStream{},
		&BackupStream{},
		(*BackupStream)(nil),
		map[string]interface{}{"key": "value"},
		map[string]interface{}(nil),
		map[string]string{"key": "value"},
		map[string]string(nil),
		[]interface{}{"value", true},
		[]interface{}(nil),
		[]string{"value"},
		[]string(nil),
		json.RawMessage(`{"valid":true}`),
		json.RawMessage(nil),
		[]byte{1, 2, 3},
		[]byte(nil),
	}

	for _, original := range values {
		cloned, err :=
			cloneRecoveryMappingValue(original, 0)

		if err != nil {
			t.Fatalf("Supported type %T failed: %v",
				original, err)
		}

		if !reflect.DeepEqual(original, cloned) {
			t.Fatalf("Cloned value differs for type %T",
				original)
		}

		if reflect.TypeOf(original) != reflect.TypeOf(cloned) {
			t.Fatalf("Concrete type changed for %T", original)
		}
	}
}

func TestChannelMappingRecoveryMutableValues(t *testing.T) {
	original := map[string]interface{}{
		"strings": map[string]string{"key": "original"},
		"bytes":   []byte{1, 2, 3},
		"json":    json.RawMessage(`{"valid":true}`),
	}

	cloned, err :=
		cloneRecoveryMappingMap(original, 0)

	if err != nil {
		t.Fatal(err)
	}

	original["strings"].(map[string]string)["key"] = "changed"
	original["bytes"].([]byte)[0] = 99
	original["json"].(json.RawMessage)[0] = 'X'

	if cloned["strings"].(map[string]string)["key"] != "original" {
		t.Fatal("String map was shared")
	}

	if cloned["bytes"].([]byte)[0] != 1 {
		t.Fatal("Byte slice was shared")
	}

	if !json.Valid(cloned["json"].(json.RawMessage)) {
		t.Fatal("Raw JSON was shared")
	}
}

func TestChannelMappingRecoveryNilState(t *testing.T) {
	setupChannelMappingRecoveryTest(t)

	Data.XEPG.Channels = nil
	Data.XEPG.ChannelsFiltered = nil
	Data.XEPG.XEPGCount = 0
	Data.Cache.Streams.Active = nil
	Data.XMLTV.Mapping = nil
	Data.XMLTV.Files = nil

	snapshot, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	Data.XEPG.Channels = make(map[string]interface{})
	Data.XEPG.ChannelsFiltered = make(map[string]interface{})
	Data.Cache.Streams.Active = []string{}
	Data.XMLTV.Mapping = make(map[string]interface{})
	Data.XMLTV.Files = []string{}

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	if Data.XEPG.Channels != nil ||
		Data.XEPG.ChannelsFiltered != nil ||
		Data.Cache.Streams.Active != nil ||
		Data.XMLTV.Mapping != nil ||
		Data.XMLTV.Files != nil {

		t.Fatal("Nil state was not preserved")
	}
}

func TestChannelMappingRecoveryEmptyState(t *testing.T) {
	setupChannelMappingRecoveryTest(t)

	snapshot, err :=
		captureChannelMappingRecoverySnapshot()

	if err != nil {
		t.Fatal(err)
	}

	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}

	if Data.XEPG.Channels == nil ||
		Data.XEPG.ChannelsFiltered == nil ||
		Data.Cache.Streams.Active == nil ||
		Data.XMLTV.Mapping == nil ||
		Data.XMLTV.Files == nil {

		t.Fatal("Empty state was incorrectly converted to nil")
	}
}

func TestChannelMappingRecoveryUnsupportedTypes(t *testing.T) {
	unsupported := []interface{}{
		make(chan int),
		func() {},
		complex128(1 + 2i),
		map[int]string{1: "value"},
	}

	for _, value := range unsupported {
		_, err :=
			cloneRecoveryMappingValue(value, 0)

		if err == nil {
			t.Fatalf("Unsupported type %T was accepted", value)
		}
	}
}

func TestChannelMappingRecoveryCircularReference(t *testing.T) {
	circular := make(map[string]interface{})
	circular["self"] = circular

	_, err :=
		cloneRecoveryMappingMap(circular, 0)

	if err == nil {
		t.Fatal("Circular reference was accepted")
	}

	if !strings.Contains(err.Error(), "nesting depth") {
		t.Fatalf("Unexpected error: %v", err)
	}

	circularSlice := make([]interface{}, 1)
	circularSlice[0] = circularSlice

	_, err =
		cloneRecoveryMappingValue(circularSlice, 0)

	if err == nil {
		t.Fatal("Circular slice was accepted")
	}
}

func TestChannelMappingRecoveryCaptureFailure(t *testing.T) {
	cases := []string{
		"channels",
		"filtered",
		"xmltv",
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			setupChannelMappingRecoveryTest(t)

			switch name {
			case "channels":
				Data.XEPG.Channels["invalid"] = make(chan int)
			case "filtered":
				Data.XEPG.ChannelsFiltered["invalid"] = make(chan int)
			case "xmltv":
				Data.XMLTV.Mapping["invalid"] = make(chan int)
			}

			snapshot, err :=
				captureChannelMappingRecoverySnapshot()

			if err == nil || snapshot != nil {
				t.Fatal("Invalid snapshot was accepted")
			}
		})
	}
}

func TestChannelMappingRecoveryRestoreFailure(t *testing.T) {
	cases := []string{
		"channels",
		"filtered",
		"xmltv",
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			setupChannelMappingRecoveryTest(t)

			Data.XEPG.Channels["live"] = "current"
			Data.XEPG.ChannelsFiltered["live"] = true
			Data.XEPG.XEPGCount = 12
			Data.Cache.Streams.Active = []string{"live-hash"}
			Data.XMLTV.Mapping["live.xml"] = "current"
			Data.XMLTV.Files = []string{"live.xml"}

			before, err :=
				captureChannelMappingRecoverySnapshot()

			if err != nil {
				t.Fatal(err)
			}

			invalid := &channelMappingRecoverySnapshot{
				Channels: map[string]interface{}{
					"replacement": "new",
				},
				ChannelsFiltered: make(map[string]interface{}),
				XEPGCount:        99,
				XMLTVMapping:     make(map[string]interface{}),
			}

			switch name {
			case "channels":
				invalid.Channels["invalid"] = make(chan int)
			case "filtered":
				invalid.ChannelsFiltered["invalid"] = make(chan int)
			case "xmltv":
				invalid.XMLTVMapping["invalid"] = make(chan int)
			}

			if err := invalid.Restore(); err == nil {
				t.Fatal("Invalid restoration was accepted")
			}

			after, err :=
				captureChannelMappingRecoverySnapshot()

			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(before, after) {
				t.Fatal("Failed restoration modified live state")
			}
		})
	}
}

func TestChannelMappingRecoveryNilSnapshot(t *testing.T) {
	var snapshot *channelMappingRecoverySnapshot

	if err := snapshot.Restore(); err == nil {
		t.Fatal("Nil snapshot was accepted")
	}
}
