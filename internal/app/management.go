package app

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"github.com/jonesrussell/northway/internal/httpapi"
	"io"
)

//go:embed management_operations.json
var managementContracts embed.FS

func agentGrantOperation(action string) httpapi.ManagementOperation {
	raw, e := managementContracts.ReadFile("management_operations.json")
	if e != nil {
		panic("management catalogue unavailable")
	}
	var ops []httpapi.ManagementOperation
	if json.Unmarshal(raw, &ops) != nil {
		panic("invalid management catalogue")
	}
	for _, op := range ops {
		if op.ID == "collection.grant."+action {
			return op
		}
	}
	panic("unregistered grant operation")
}
func executeManagement(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "inventory" {
		return errors.New("expected management inventory")
	}
	fs := flag.NewFlagSet("management", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	active := false
	fs.BoolVar(&active, "collection-api", false, "project selected local API composition")
	if e := fs.Parse(args[1:]); e != nil || fs.NArg() != 0 {
		return errors.New("invalid management flags")
	}
	ops := httpapi.CollectionOperations(active)
	for _, action := range []string{"create", "revoke"} {
		ops = append(ops, agentGrantOperation(action))
	}
	ops = append(ops, httpapi.ManagementOperation{ID: "collection.schedule", State: "planned", Reason: "No collection schedule management or activation."})
	return json.NewEncoder(out).Encode(struct {
		Schema      string                        `json:"schema"`
		Version     int                           `json:"version"`
		Application string                        `json:"application"`
		Operations  []httpapi.ManagementOperation `json:"operations"`
	}{"waaseyaa.management", 1, "northway-api", ops})
}
