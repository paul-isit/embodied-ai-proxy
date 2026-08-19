package validator

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Validator checks LLM-generated action recipes against the shared
// configs/json_schema.json contract (the same schema used to prompt the
// LLM in the first place).
type Validator struct {
	schema *jsonschema.Schema
}

// New compiles the JSON Schema given by raw. id is used as the schema's
// resource URL (e.g. the path it was read from) for error messages and
// $ref resolution - it does not need to exist on disk.
func New(id string, raw []byte) (*Validator, error) {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(id, bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("add schema resource %s: %w", id, err)
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("compile schema %s: %w", id, err)
	}
	return &Validator{schema: schema}, nil
}

// Validate checks raw JSON bytes (an LLM's generated recipe or error
// response) against the schema, returning the decoded document on success
// so callers don't need to re-parse the same bytes.
func (v *Validator) Validate(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var doc any
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}

	if err := v.schema.Validate(doc); err != nil {
		return nil, fmt.Errorf("schema validation failed: %w", err)
	}
	return doc, nil
}
