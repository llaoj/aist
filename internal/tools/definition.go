package tools

import "encoding/json"

// Definition describes a tool without depending on a wire protocol.
type Definition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

func definitionFromMap(def map[string]any) Definition {
	fn, _ := def["function"].(map[string]any)
	schema, _ := json.Marshal(fn["parameters"])
	name, _ := fn["name"].(string)
	description, _ := fn["description"].(string)
	return Definition{Name: name, Description: description, InputSchema: schema}
}
