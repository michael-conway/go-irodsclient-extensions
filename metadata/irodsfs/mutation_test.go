package irodsfs

import (
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
)

func TestIRODSMetaFromAVUStatIgnoresAVUIDSemantics(t *testing.T) {
	avu := metadata.AVUStat{Name: "source", Value: "before", Units: "fixture"}

	mapped := irodsMetaFromAVUStat(avu)

	if mapped.Name != "source" || mapped.Value != "before" || mapped.Units != "fixture" {
		t.Fatalf("unexpected mapped AVU: %+v", mapped)
	}
	if mapped.AVUID != 0 {
		t.Fatalf("expected AVUID to be omitted, got %d", mapped.AVUID)
	}
}

func TestReplaceMetadataRequestUsesModOperationAndFromToTuples(t *testing.T) {
	oldMetadata := &irodstypes.IRODSMeta{Name: "source", Value: "before", Units: "fixture"}
	newMetadata := &irodstypes.IRODSMeta{Name: "source", Value: "after", Units: "fixture"}

	request := newReplaceMetadataRequest(irodstypes.IRODSDataObjectMetaItemType, "/tempZone/home/test1/object.txt", oldMetadata, newMetadata)

	if request.Operation != "mod" {
		t.Fatalf("expected mod operation, got %q", request.Operation)
	}
	if request.ItemType != string(irodstypes.IRODSDataObjectMetaItemType) {
		t.Fatalf("unexpected item type %q", request.ItemType)
	}
	if request.ItemName != "/tempZone/home/test1/object.txt" {
		t.Fatalf("unexpected item name %q", request.ItemName)
	}
	if request.AttrName != "source" || request.AttrValue != "before" || request.AttrUnits != "fixture" {
		t.Fatalf("unexpected source tuple: name=%q value=%q units=%q", request.AttrName, request.AttrValue, request.AttrUnits)
	}
	if request.NewAttrName != "n:source" || request.NewAttrValue != "v:after" || request.NewAttrUnits != "u:fixture" {
		t.Fatalf("unexpected target tuple: name=%q value=%q units=%q", request.NewAttrName, request.NewAttrValue, request.NewAttrUnits)
	}
}

func TestFindAVUStatMatchesNameValueUnits(t *testing.T) {
	metadataList := []metadata.AVUStat{
		{ID: 41, Name: "source", Value: "before", Units: "fixture"},
		{ID: 42, Name: "source", Value: "after", Units: "fixture"},
	}

	matched, ok := findAVUStat(metadataList, metadata.AVUStat{Name: "source", Value: "after", Units: "fixture"})
	if !ok {
		t.Fatalf("expected replacement AVU to match")
	}
	if matched.Value != "after" {
		t.Fatalf("unexpected matched AVU: %+v", matched)
	}

	_, ok = findAVUStat(metadataList, metadata.AVUStat{Name: "source", Value: "after", Units: "other"})
	if ok {
		t.Fatalf("expected unit mismatch not to match")
	}
}
