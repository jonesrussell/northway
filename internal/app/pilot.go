package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite"
)

const personalLocalV1SHA256 = "7dcf59ee285ad179f0062a7f075a0de93dc7e75cc1a30bbf2ef699c4ce76b503"

const pilotHelp = `Usage: northway pilot provision --database PATH --tenant UUID --manifest PATH
Atomically provision the exact reviewed personal-news profile. Stop serve first.
The manifest must be a local regular file, version 1, explicitly enabled for personal
metadata use, and must keep provider/commercial use false. It cannot contain secrets.
This creates saved feeds and enables bounded feed-document polling; it never starts a timer.
`

type pilotManifest struct {
	SchemaVersion         int    `json:"schema_version"`
	Profile               string `json:"profile"`
	Enabled               bool   `json:"enabled"`
	UseScope              string `json:"use_scope"`
	ProviderExportAllowed bool   `json:"provider_export_allowed"`
	CommercialUseApproved bool   `json:"commercial_use_approved"`
	ApprovalRecord        string `json:"approval_record"`
	Feeds                 []struct {
		ID           string   `json:"id"`
		Title        string   `json:"title"`
		Categories   []string `json:"categories"`
		PublisherCap int      `json:"publisher_cap"`
		UseContext   bool     `json:"use_context"`
	} `json:"feeds"`
	Sources []struct {
		ID               string   `json:"id"`
		Title            string   `json:"title"`
		FeedURL          string   `json:"feed_url"`
		FeedIDs          []string `json:"feed_ids"`
		IntervalSeconds  int      `json:"interval_seconds"`
		MaxBytes         int64    `json:"max_bytes"`
		PersonalUseBasis string   `json:"personal_use_basis"`
		PublisherGroup   string   `json:"publisher_group"`
		Categories       []string `json:"categories"`
	} `json:"sources"`
	Excluded []struct {
		CatalogueID string `json:"catalogue_id"`
		Reason      string `json:"reason"`
	} `json:"excluded"`
}

func executePilot(ctx context.Context, args []string, lookup func(string) (string, bool), out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, e := io.WriteString(out, pilotHelp)
		return e
	}
	if len(args) == 0 || args[0] != "provision" {
		return errors.New("expected pilot provision; use northway pilot --help")
	}
	path, _ := lookup("NORTHWAY_DATABASE_PATH")
	var tenant, manifest string
	fs := flag.NewFlagSet("pilot provision", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&path, "database", path, "local database")
	fs.StringVar(&tenant, "tenant", "", "tenant UUID")
	fs.StringVar(&manifest, "manifest", "", "reviewed local manifest")
	if err := fs.Parse(args[1:]); err != nil {
		return errors.New("invalid pilot flags")
	}
	if path == "" || manifest == "" || fs.NArg() != 0 {
		return errors.New("pilot provisioning requires database, manifest and no positional arguments")
	}
	p, err := identity.Operator(identity.TenantID(tenant))
	if err != nil {
		return errors.New("pilot provisioning requires canonical tenant UUID")
	}
	v, err := readPilotManifest(manifest)
	if err != nil {
		return err
	}
	feeds := make([]sqlite.PilotFeed, len(v.Feeds))
	for i, f := range v.Feeds {
		feeds[i] = sqlite.PilotFeed{ID: f.ID, Title: f.Title, Categories: f.Categories, PublisherCap: f.PublisherCap, UseContext: f.UseContext}
	}
	sources := make([]sqlite.PilotSource, len(v.Sources))
	for i, s := range v.Sources {
		sources[i] = sqlite.PilotSource{ID: s.ID, URL: s.FeedURL, Title: s.Title, FeedIDs: s.FeedIDs, Interval: time.Duration(s.IntervalSeconds) * time.Second, MaxBytes: s.MaxBytes, PublisherGroup: s.PublisherGroup, Categories: s.Categories}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		return errors.New("cannot open pilot storage; migrate and create tenant first")
	}
	defer store.Close()
	if err = store.ProvisionPilot(ctx, p, sources, feeds); err != nil {
		return errors.New("pilot profile conflicts with storage or failed validation")
	}
	return json.NewEncoder(out).Encode(struct {
		Status  string `json:"status"`
		Profile string `json:"profile"`
		Sources int    `json:"sources"`
		Feeds   int    `json:"feeds"`
	}{"complete", v.Profile, len(sources), len(feeds)})
}
func readPilotManifest(path string) (pilotManifest, error) {
	var v pilotManifest
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return v, errors.New("pilot manifest must be a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != personalLocalV1SHA256 {
		return v, errors.New("pilot manifest is not the reviewed personal-local-v1 revision")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&v); err != nil {
		return v, errors.New("invalid pilot manifest")
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return v, errors.New("pilot manifest must contain one JSON value")
	}
	if v.SchemaVersion != 1 || v.Profile != "personal-local-v1" || !v.Enabled || v.UseScope != "personal_metadata_feed_aggregation" || v.ProviderExportAllowed || v.CommercialUseApproved || v.ApprovalRecord != "docs/source-approval-2026-08-31.md" || len(v.Excluded) != 5 {
		return v, errors.New("pilot manifest does not carry the reviewed personal-use decision")
	}
	if len(v.Sources) != 5 || len(v.Feeds) != 5 {
		return v, fmt.Errorf("pilot profile requires 5 active sources and 5 saved feeds")
	}
	return v, nil
}
