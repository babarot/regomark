package input

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/invopop/jsonschema"
	validator "github.com/santhosh-tekuri/jsonschema/v6"
)

// Schema returns the JSON Schema of Input, generated from the Go types: every
// field is required, and no other field is allowed. It has no references, so
// that OPA can type-check policies against it.
func Schema() []byte {
	schemaOnce.Do(func() {
		r := jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true}
		s := r.Reflect(&Input{})
		s.ID = jsonschema.ID(schemaID)
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			panic(err) // the schema is made of plain values
		}
		schemaJSON = b
	})
	return schemaJSON
}

const schemaID = "https://github.com/babarot/regomark/input.schema.json"

var (
	schemaOnce sync.Once
	schemaJSON []byte

	compileOnce sync.Once
	compiled    *validator.Schema
	compileErr  error
)

// Validate checks the input against Schema, as it will reach the policies.
// The input is made by this program, so an error here is a bug in it; the
// check keeps such a bug from reaching the policies as data they misread.
func Validate(in *Input) error {
	compileOnce.Do(func() {
		doc, err := validator.UnmarshalJSON(bytes.NewReader(Schema()))
		if err != nil {
			compileErr = err
			return
		}
		c := validator.NewCompiler()
		if err := c.AddResource(schemaID, doc); err != nil {
			compileErr = err
			return
		}
		compiled, compileErr = c.Compile(schemaID)
	})
	if compileErr != nil {
		return fmt.Errorf("input schema: %w", compileErr)
	}
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	v, err := validator.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return err
	}
	if err := compiled.Validate(v); err != nil {
		return fmt.Errorf("input does not match its schema: %w", err)
	}
	return nil
}
