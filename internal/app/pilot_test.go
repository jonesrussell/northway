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

func TestReviewedPilotPolicyMatchesPinnedV1(t *testing.T) {
	const hash = "80f8d68e0825b6006e9769589a8d6c974f6d3be98a153f5121c02b30a4a04005"
	policy, ok := reviewedPilotProfiles[hash]
	if !ok {
		t.Fatal("reviewed v1 profile is not pinned")
	}
	if policy.Profile != "personal-local-v1" || policy.Sources != 5 || policy.Feeds != 5 || policy.Excluded != 5 {
		t.Fatalf("unexpected v1 policy: %#v", policy)
	}
}
