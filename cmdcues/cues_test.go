package cmdcues

import "testing"

func TestBuildPutCue(t *testing.T) {
	got, err := BuildPutCue("/tempZone/home/test1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.GoCmd != "gocmd put <LOCAL_PATH> '/tempZone/home/test1'" {
		t.Fatalf("unexpected gocmd put cue: %q", got.GoCmd)
	}
	if got.ICommand != "iput <LOCAL_PATH> '/tempZone/home/test1'" {
		t.Fatalf("unexpected icommand put cue: %q", got.ICommand)
	}
}

func TestBuildGetCueUsesDestinationPlaceholder(t *testing.T) {
	got, err := BuildGetCue("/tempZone/home/test1/file.txt")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.GoCmd != "gocmd get '/tempZone/home/test1/file.txt' <DESTINATION_PATH>" {
		t.Fatalf("unexpected gocmd get cue: %q", got.GoCmd)
	}
	if got.ICommand != "iget '/tempZone/home/test1/file.txt' <DESTINATION_PATH>" {
		t.Fatalf("unexpected icommand get cue: %q", got.ICommand)
	}
}

func TestBuildCollectionCuesReturnsPutGetAndStorageOperations(t *testing.T) {
	got, err := BuildCollectionCues("/tempZone/home/test1/project")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 cues, got %d", len(got))
	}
	if got[0].Operation != OperationPut || got[0].GoCmd != "gocmd put -r <LOCAL_PATH> '/tempZone/home/test1/project'" {
		t.Fatalf("unexpected collection put cue: %+v", got[0])
	}
	if got[1].Operation != OperationGet || got[1].ICommand != "iget -r '/tempZone/home/test1/project' <DESTINATION_PATH>" {
		t.Fatalf("unexpected collection get cue: %+v", got[1])
	}
	if got[2].Operation != OperationPhyMove || got[2].GoCmd != "" || got[2].ICommand != "iphymv -r -S <srcResource> -R <targetResource> '/tempZone/home/test1/project'" {
		t.Fatalf("unexpected collection phymove cue: %+v", got[2])
	}
	if got[3].Operation != OperationReplicate || got[3].GoCmd != "" || got[3].ICommand != "irepl -r -S <srcResource> -R <targetResource> '/tempZone/home/test1/project'" {
		t.Fatalf("unexpected collection replicate cue: %+v", got[3])
	}
}

func TestBuildDataObjectCuesReturnsPutGetAndStorageOperations(t *testing.T) {
	got, err := BuildDataObjectCues("/tempZone/home/test1/file.txt")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 cues, got %d", len(got))
	}
	if got[0].Operation != OperationPut || got[0].GoCmd != "gocmd put <LOCAL_PATH> '/tempZone/home/test1'" {
		t.Fatalf("unexpected object put cue: %+v", got[0])
	}
	if got[1].Operation != OperationGet || got[1].ICommand != "iget '/tempZone/home/test1/file.txt' <DESTINATION_PATH>" {
		t.Fatalf("unexpected object get cue: %+v", got[1])
	}
	if got[2].Operation != OperationPhyMove || got[2].GoCmd != "" || got[2].ICommand != "iphymv -S <srcResource> -R <targetResource> '/tempZone/home/test1/file.txt'" {
		t.Fatalf("unexpected object phymove cue: %+v", got[2])
	}
	if got[3].Operation != OperationReplicate || got[3].GoCmd != "" || got[3].ICommand != "irepl -S <srcResource> -R <targetResource> '/tempZone/home/test1/file.txt'" {
		t.Fatalf("unexpected object replicate cue: %+v", got[3])
	}
}

func TestBuildCueDispatchCollection(t *testing.T) {
	got, err := BuildCue(CueRequest{
		OperationSet:        "put/get",
		CollectionIRODSPath: "/tempZone/home/test1",
		IsCollection:        true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 cues, got %d", len(got))
	}
	if got[0].GoCmd != "gocmd put -r <LOCAL_PATH> '/tempZone/home/test1'" {
		t.Fatalf("unexpected gocmd cue: %q", got[0].GoCmd)
	}
}

func TestBuildCueDispatchDefaultReturnsConsolidatedCues(t *testing.T) {
	got, err := BuildCue(CueRequest{
		ObjectIRODSPath: "/tempZone/home/test1/file.txt",
		IsCollection:    false,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 cues, got %d", len(got))
	}
}

func TestBuildGetCueRejectsCollectionPath(t *testing.T) {
	_, err := BuildGetCue("/tempZone/home/test1/folder/")
	if err == nil {
		t.Fatal("expected error for collection-like path")
	}
}
