package api_test

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestSpecExamplesValid checks every request and response example in
// api/openapi.yaml against its schema (no database needed).
func TestSpecExamplesValid(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	check := func(where string, mt *openapi3.MediaType) {
		if mt == nil || mt.Example == nil || mt.Schema == nil || mt.Schema.Value == nil {
			return
		}
		// Round-trip through JSON so numbers are float64 as in a decoded body.
		raw, err := json.Marshal(mt.Example)
		if err != nil {
			t.Fatalf("%s: marshal example: %v", where, err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: unmarshal example: %v", where, err)
		}
		if err := mt.Schema.Value.VisitJSON(v, openapi3.MultiErrors()); err != nil {
			t.Errorf("%s: example violates schema: %v", where, err)
		}
	}
	n := 0
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for ct, mt := range op.RequestBody.Value.Content {
					check(method+" "+path+" request "+ct, mt)
					n++
				}
			}
			for status, res := range op.Responses.Map() {
				if res.Value == nil {
					continue
				}
				for ct, mt := range res.Value.Content {
					check(method+" "+path+" "+status+" "+ct, mt)
					n++
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no examples checked")
	}
}
