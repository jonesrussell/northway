package app

import (
	"bytes"
	"encoding/json"
	"github.com/jonesrussell/northway/internal/ingest"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectionFixtureExportEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "northway-collection.fixture.sqlite")
	var out bytes.Buffer
	args := []string{"collection", "fixture", "--database", path, "--input", "../../testdata/creator.html"}
	if e := Execute(t.Context(), args, os.LookupEnv, &out, &out); e != nil {
		t.Fatal(e)
	}
	var b ingest.Batch
	if e := json.Unmarshal(out.Bytes(), &b); e != nil {
		t.Fatal(e)
	}
	if len(b.Events) != 5 {
		t.Fatal(b)
	}
	out.Reset()
	if e := Execute(t.Context(), []string{"collection", "export", "--database", path}, os.LookupEnv, &out, &out); e != nil {
		t.Fatal(e)
	}
	var replay ingest.Batch
	if e := json.Unmarshal(out.Bytes(), &replay); e != nil {
		t.Fatal(e)
	}
	if replay.Next != b.Next {
		t.Fatal(replay)
	}
	if e := Execute(t.Context(), args, os.LookupEnv, &out, &out); e == nil {
		t.Fatal("existing fixture database overwritten")
	}
}
