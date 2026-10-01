package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/jonesrussell/northway/internal/catalogue"
	"os"
	"path/filepath"
	"testing"
)

func TestCataloguePrepareValidateAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	research := filepath.Join(dir, "research.json")
	output := filepath.Join(dir, "register.json")
	rows := []catalogue.Candidate{{URL: "https://publisher.example/feed", Title: "Example", Publisher: "Publisher", Topic: "science", Region: "Global", Language: "en", Rights: "not_assessed", Notes: "No rehosting rights assumed"}}
	b, _ := json.Marshal(rows)
	if e := os.WriteFile(research, b, 0600); e != nil {
		t.Fatal(e)
	}
	args := []string{"prepare", "--research", research, "--output", output}
	var out bytes.Buffer
	if e := executeCatalogue(context.Background(), args, &out); e != nil {
		t.Fatal(e)
	}
	original, e := os.ReadFile(output)
	if e != nil {
		t.Fatal(e)
	}
	if e := executeCatalogue(context.Background(), args, &out); e == nil {
		t.Fatal("existing operator file overwritten")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(original))
	if e := executeCatalogue(context.Background(), []string{"validate", "--manifest", output, "--sha256", digest}, &out); e != nil {
		t.Fatal(e)
	}
	if e := executeCatalogue(context.Background(), []string{"validate", "--manifest", output, "--sha256", digest, "--database", "unused"}, &out); e == nil {
		t.Fatal("validate accepted storage mutation flag")
	}
}
func TestCuratedConfigurationFailsWithoutExactRegister(t *testing.T) {
	c := Config{CustomerCatalogue: "curated-v1"}
	if c.Validate() == nil {
		t.Fatal("missing register accepted")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if e := os.WriteFile(path, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	c.CuratedManifest = path
	c.CuratedSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, e := catalogueRegister(c); e == nil {
		t.Fatal("wrong register accepted")
	}
}
