package app

import (
	"bytes"
	"encoding/json"
	"github.com/jonesrussell/northway/internal/httpapi"
	"os"
	"testing"
)

func TestManagementInventoryProjectsSelectedBindings(t *testing.T) {
	for _, active := range []bool{false, true} {
		var out bytes.Buffer
		args := []string{"management", "inventory"}
		if active {
			args = append(args, "--collection-api")
		}
		if e := Execute(t.Context(), args, os.LookupEnv, &out, &out); e != nil {
			t.Fatal(e)
		}
		var inventory struct {
			Schema      string
			Version     int
			Application string
			Operations  []httpapi.ManagementOperation
		}
		if e := json.Unmarshal(out.Bytes(), &inventory); e != nil {
			t.Fatal(e)
		}
		if inventory.Schema != "waaseyaa.management" || inventory.Version != 1 || inventory.Application != "northway-api" || len(inventory.Operations) != 6 {
			t.Fatal(inventory)
		}
		for _, op := range inventory.Operations[:3] {
			if active && op.State != "supported" || !active && op.State != "planned" {
				t.Fatal("activation projection", op)
			}
		}
		for _, op := range inventory.Operations[3:5] {
			if op.State != "supported" || op.Contract["dry_run"] != "supported" {
				t.Fatal("operator CLI projection", op)
			}
		}
	}
}
