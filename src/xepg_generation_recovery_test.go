package src

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"
)

func setupXMLTVGenerationRecovery(t *testing.T) {
	t.Helper()

	dir := usePipelineFolders(t)
	useSettings(t, dir)

	savedName := System.Name
	savedImagesFiles := Data.Cache.ImagesFiles
	savedImagesURLs := Data.Cache.ImagesURLS
	savedImagesCache := Data.Cache.ImagesCache

	t.Cleanup(func() {
		System.Name = savedName
		Data.Cache.ImagesFiles = savedImagesFiles
		Data.Cache.ImagesURLS = savedImagesURLs
		Data.Cache.ImagesCache = savedImagesCache
	})

	Data.XMLTV.Files = []string{}
	Data.Streams.Active = []interface{}{
		map[string]string{"name": "Test stream"},
	}

	Data.XEPG.Channels = make(map[string]interface{})

	Settings.EnableNonAscii = true
	Settings.XepgReplaceMissingImages = false
}

func recoveryTestChannel() XEPGChannelStruct {
	return XEPGChannelStruct{
		XEPG:       "x-ID.0",
		XActive:    true,
		XChannelID: "1000",
		TvgChno:    "1000",
		XName:      "Recovery Test Channel",
		TvgName:    "Recovery Test Channel",
		XmltvFile:  "-",
		XMapping:   "-",
	}
}

func TestXMLTVGenerationRecoveryPreservesMappings(t *testing.T) {
	setupXMLTVGenerationRecovery(t)

	Data.Streams.Active = nil
	Data.XMLTV.Files = nil

	channel := recoveryTestChannel()
	Data.XEPG.Channels["x-ID.0"] = channel

	err := createXMLTVFile()

	if err == nil {
		t.Fatal("expected generation to be blocked")
	}

	if !strings.Contains(err.Error(), "XMLTV generation blocked:") {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(Data.XEPG.Channels) != 1 {
		t.Fatal("existing channel mappings were lost")
	}

	if Data.XEPG.Channels["x-ID.0"] != channel {
		t.Fatal("existing channel mapping was modified")
	}

	t.Log("Existing channel mapping preserved")
}

func TestXMLTVGenerationRecoveryEmptyConfiguration(t *testing.T) {
	setupXMLTVGenerationRecovery(t)

	Data.Streams.Active = nil
	Data.XMLTV.Files = nil

	if err := createXMLTVFile(); err != nil {
		t.Fatalf("empty configuration failed: %v", err)
	}

	if len(Data.XEPG.Channels) != 0 {
		t.Fatal("unexpected channel mappings created")
	}

	t.Log("Empty configuration handled safely")
}

func TestXMLTVGenerationRecoveryInvalidChannel(t *testing.T) {
	setupXMLTVGenerationRecovery(t)

	Data.XEPG.Channels["x-ID.invalid"] = "invalid channel"

	err := createXMLTVFile()

	if err == nil {
		t.Fatal("expected channel decoding failure")
	}

	if !strings.Contains(err.Error(), "XMLTV channel decoding failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("Expected error received: %v", err)
}

func TestXMLTVGenerationRecoveryWithoutEPG(t *testing.T) {
	setupXMLTVGenerationRecovery(t)

	channel := recoveryTestChannel()
	Data.XEPG.Channels["x-ID.0"] = channel

	if err := createXMLTVFile(); err != nil {
		t.Fatalf("channel without EPG failed: %v", err)
	}

	raw, err := os.ReadFile(System.File.XML)
	if err != nil {
		t.Fatal(err)
	}

	var guide XMLTV

	if err := xml.Unmarshal(raw, &guide); err != nil {
		t.Fatalf("invalid generated XMLTV: %v", err)
	}

	if len(guide.Channel) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(guide.Channel))
	}

	if len(guide.Program) != 0 {
		t.Fatalf("expected no programmes, got %d", len(guide.Program))
	}

	t.Log("Channel without EPG generated successfully")
}

func TestXMLTVGenerationRecoveryProgrammeFailure(t *testing.T) {
	setupXMLTVGenerationRecovery(t)

	channel := recoveryTestChannel()
	channel.XmltvFile = "missing-provider.xml"
	channel.XMapping = "missing.channel"

	Data.XEPG.Channels["x-ID.0"] = channel

	err := createXMLTVFile()

	if err == nil {
		t.Fatal("expected programme retrieval failure")
	}

	if !strings.Contains(err.Error(), "XMLTV programme retrieval failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("Expected error received: %v", err)
}

func TestXMLTVGenerationRecoveryPreservesPreviousFiles(t *testing.T) {
	setupXMLTVGenerationRecovery(t)

	previousXML := []byte(
		`<?xml version="1.0"?><tv><channel id="previous"/></tv>`,
	)

	if err := os.WriteFile(System.File.XML, previousXML, 0644); err != nil {
		t.Fatal(err)
	}

	if err := compressGZIPFile(
		System.File.XML,
		System.Compressed.GZxml,
	); err != nil {
		t.Fatal(err)
	}

	previousGZIP, err := os.ReadFile(System.Compressed.GZxml)
	if err != nil {
		t.Fatal(err)
	}

	channel := recoveryTestChannel()
	channel.XmltvFile = "missing-provider.xml"
	channel.XMapping = "missing.channel"

	Data.XEPG.Channels["x-ID.0"] = channel

	if err := createXMLTVFile(); err == nil {
		t.Fatal("expected XMLTV generation failure")
	}

	currentXML, err := os.ReadFile(System.File.XML)
	if err != nil {
		t.Fatal(err)
	}

	currentGZIP, err := os.ReadFile(System.Compressed.GZxml)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(previousXML, currentXML) {
		t.Fatal("previous XMLTV file was modified")
	}

	if !bytes.Equal(previousGZIP, currentGZIP) {
		t.Fatal("previous compressed XMLTV file was modified")
	}

	t.Log("Previous XMLTV and GZIP files preserved")
}

func TestXMLTVGenerationRecoveryEscapesXML(t *testing.T) {
	setupXMLTVGenerationRecovery(t)

	System.Name = "ProxFin & <Recovery>"

	channel := recoveryTestChannel()
	channel.XName = "News & Sports <HD>"

	Data.XEPG.Channels["x-ID.0"] = channel

	if err := createXMLTVFile(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(System.File.XML)
	if err != nil {
		t.Fatal(err)
	}

	var guide XMLTV

	if err := xml.Unmarshal(raw, &guide); err != nil {
		t.Fatalf("generated XML is invalid: %v", err)
	}

	if !bytes.Contains(raw, []byte("ProxFin &amp; &lt;Recovery&gt;")) {
		t.Fatal("generator text was not escaped")
	}

	if len(guide.Channel) != 1 {
		t.Fatal("expected exactly one channel")
	}

	if len(guide.Channel[0].DisplayName) == 0 {
		t.Fatal("channel display name missing")
	}

	if guide.Channel[0].DisplayName[0].Value != channel.XName {
		t.Fatal("channel display name was not preserved")
	}

	t.Log("Special XML characters escaped successfully")
}

func TestXMLTVGenerationRecoveryValidCompressedOutput(t *testing.T) {
	setupXMLTVGenerationRecovery(t)

	channel := recoveryTestChannel()
	channel.XmltvFile = "provider.xml"
	channel.XMapping = "ch0.example"

	Data.XEPG.Channels["x-ID.0"] = channel

	Data.Cache.XMLTV[System.Folder.Data+"provider.xml"] = testGuide(1, 2)

	if err := createXMLTVFile(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(System.File.XML)
	if err != nil {
		t.Fatal(err)
	}

	var guide XMLTV

	if err := xml.Unmarshal(raw, &guide); err != nil {
		t.Fatal(err)
	}

	if len(guide.Channel) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(guide.Channel))
	}

	if len(guide.Program) != 2 {
		t.Fatalf("expected 2 programmes, got %d", len(guide.Program))
	}

	for _, program := range guide.Program {
		if program.Channel != channel.XChannelID {
			t.Fatalf("incorrect programme channel: %s", program.Channel)
		}
	}

	file, err := os.Open(System.Compressed.GZxml)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	unpacked, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(raw, unpacked) {
		t.Fatal("compressed XMLTV does not match original XMLTV")
	}

	if !strings.HasSuffix(string(raw), "</tv>\n") {
		t.Fatal("generated XMLTV is incomplete")
	}

	t.Log("Valid XMLTV and matching compressed output generated")
}
