package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPilotManifestPinsReviewedRevision(t *testing.T) {
	path := filepath.Join("..", "..", "catalogue", "personal-local-v1.json")
	got, err := readPilotManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "personal-local-v1" || len(got.Sources) != 5 || len(got.Feeds) != 5 || len(got.Excluded) != 5 {
		t.Fatalf("unexpected profile: %#v", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-2] ^= 1
	tampered := filepath.Join(t.TempDir(), "pilot.json")
	if err := os.WriteFile(tampered, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPilotManifest(tampered); err == nil {
		t.Fatal("tampered reviewed manifest accepted")
	}
}
