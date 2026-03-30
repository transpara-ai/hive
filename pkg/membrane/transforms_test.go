package membrane

import (
	"encoding/json"
	"testing"
)

func TestTransformRegistry(t *testing.T) {
	reg := NewTransformRegistry()

	reg.Register("uppercase_name", func(in json.RawMessage) (json.RawMessage, error) {
		var data map[string]string
		if err := json.Unmarshal(in, &data); err != nil {
			return nil, err
		}
		data["name"] = "TRANSFORMED"
		return json.Marshal(data)
	})

	fn, ok := reg.Get("uppercase_name")
	if !ok {
		t.Fatal("transform not found")
	}

	in := json.RawMessage(`{"name":"original"}`)
	out, err := fn(in)
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	var result map[string]string
	json.Unmarshal(out, &result)
	if result["name"] != "TRANSFORMED" {
		t.Errorf("name = %q, want TRANSFORMED", result["name"])
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent transform")
	}
}

func TestPassthroughTransform(t *testing.T) {
	in := json.RawMessage(`{"a":"b"}`)
	out, err := PassthroughTransform(in)
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	if string(out) != string(in) {
		t.Errorf("passthrough changed data: %s != %s", out, in)
	}
}
