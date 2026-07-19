package taskdom

import (
	"errors"
	"testing"
)

func defs() []*CustomFieldDefinition {
	return []*CustomFieldDefinition{
		{FieldKey: "sev", DisplayName: "Severity", FieldType: FieldTypeSelect, Options: []string{"low", "high"}, IsRequired: true},
		{FieldKey: "pts", DisplayName: "Points", FieldType: FieldTypeNumber},
		{FieldKey: "url", DisplayName: "Link", FieldType: FieldTypeURL},
		{FieldKey: "labels", DisplayName: "Labels", FieldType: FieldTypeMultiSelect, Options: []string{"a", "b"}},
		{FieldKey: "env", DisplayName: "Env", FieldType: FieldTypeText, DefaultValue: "prod"},
	}
}

func TestValidateCustomFields_CoerceAndOptions(t *testing.T) {
	out, err := ValidateCustomFields(defs(), map[string]any{
		"sev":    "high",
		"pts":    float64(3),
		"url":    "https://x.test/y",
		"labels": []any{"a", "b"},
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["sev"] != "high" || out["pts"].(float64) != 3 {
		t.Fatalf("bad coercion: %#v", out)
	}
	if out["env"] != "prod" {
		t.Fatalf("default not filled: %#v", out["env"])
	}
}

func TestValidateCustomFields_RejectsBadValues(t *testing.T) {
	cases := []map[string]any{
		{"sev": "critical"},          // not an allowed option
		{"sev": "low", "pts": "abc"}, // number is not numeric
		{"sev": "low", "url": "notaurl"},
		{"sev": "low", "labels": []any{"z"}}, // option not allowed
	}
	for i, c := range cases {
		if _, err := ValidateCustomFields(defs(), c, true); err == nil {
			t.Fatalf("case %d: expected validation error, got nil", i)
		}
	}
}

func TestValidateCustomFields_RequiredCreateVsUpdate(t *testing.T) {
	// Create: missing required "sev" must fail.
	if _, err := ValidateCustomFields(defs(), map[string]any{"pts": float64(1)}, true); !errors.Is(err, ErrCustomFieldRequired) {
		t.Fatalf("create: expected required error, got %v", err)
	}
	// Update (lenient): missing required is tolerated (legacy task).
	if _, err := ValidateCustomFields(defs(), map[string]any{"pts": float64(1)}, false); err != nil {
		t.Fatalf("update: unexpected error: %v", err)
	}
	// Update: explicitly clearing a required field must fail.
	if _, err := ValidateCustomFields(defs(), map[string]any{"sev": ""}, false); !errors.Is(err, ErrCustomFieldRequired) {
		t.Fatalf("update-clear: expected required error, got %v", err)
	}
}

func TestValidateCustomFields_DropsUnknownKeys(t *testing.T) {
	out, err := ValidateCustomFields(defs(), map[string]any{"sev": "low", "ghost": "x"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := out["ghost"]; ok {
		t.Fatalf("unknown key was not dropped: %#v", out)
	}
}

func TestValidateFieldDefinition_OptionsRequired(t *testing.T) {
	if err := ValidateFieldDefinition(FieldTypeSelect, nil); !errors.Is(err, ErrCustomFieldOptionsInvalid) {
		t.Fatalf("select without options should fail, got %v", err)
	}
	if err := ValidateFieldDefinition(FieldTypeText, nil); err != nil {
		t.Fatalf("text without options should pass, got %v", err)
	}
}

func TestCoerceDefaultValue(t *testing.T) {
	v, err := CoerceDefaultValue(FieldTypeNumber, nil, float64(5))
	if err != nil || v.(float64) != 5 {
		t.Fatalf("number default: %v %v", v, err)
	}
	if _, err := CoerceDefaultValue(FieldTypeSelect, []string{"a"}, "b"); err == nil {
		t.Fatalf("select default out of options should fail")
	}
	if v, err := CoerceDefaultValue(FieldTypeText, nil, nil); err != nil || v != nil {
		t.Fatalf("nil default should stay nil: %v %v", v, err)
	}
}
