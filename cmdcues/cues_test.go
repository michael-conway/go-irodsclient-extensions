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

func TestBuildCueDispatch(t *testing.T) {
	got, err := BuildCue(CueRequest{
		Operation:           OperationPut,
		CollectionIRODSPath: "/tempZone/home/test1",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.GoCmd != "gocmd put <LOCAL_PATH> '/tempZone/home/test1'" {
		t.Fatalf("unexpected gocmd cue: %q", got.GoCmd)
	}
	if got.ICommand != "iput <LOCAL_PATH> '/tempZone/home/test1'" {
		t.Fatalf("unexpected icommand cue: %q", got.ICommand)
	}
}

func TestBuildGetCueRejectsCollectionPath(t *testing.T) {
	_, err := BuildGetCue("/tempZone/home/test1/folder/")
	if err == nil {
		t.Fatal("expected error for collection-like path")
	}
}
