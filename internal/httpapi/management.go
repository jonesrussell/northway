package httpapi

import (
	"embed"
	"encoding/json"
)

//go:embed collection_operations.json
var collectionContracts embed.FS

type ManagementOperation struct {
	ID       string         `json:"id"`
	State    string         `json:"state"`
	Contract map[string]any `json:"contract,omitempty"`
	Reason   string         `json:"reason,omitempty"`
}

func CollectionOperations(active bool) []ManagementOperation {
	raw, e := collectionContracts.ReadFile("collection_operations.json")
	if e != nil {
		panic("collection catalogue unavailable")
	}
	var ops []ManagementOperation
	if json.Unmarshal(raw, &ops) != nil {
		panic("invalid collection catalogue")
	}
	if !active {
		for i := range ops {
			ops[i].State = "planned"
			ops[i].Contract = nil
			ops[i].Reason = "Collection API opt-in is disabled."
		}
	}
	return ops
}
func collectionBinding(id string) (string, string) {
	for _, op := range CollectionOperations(true) {
		if op.ID == id {
			binding := op.Contract["binding"].(map[string]any)["name"].(string)
			scopes := op.Contract["required_scopes"].([]any)
			if len(scopes) != 1 {
				panic("invalid collection authority")
			}
			return binding, scopes[0].(string)
		}
	}
	panic("unregistered collection operation")
}
