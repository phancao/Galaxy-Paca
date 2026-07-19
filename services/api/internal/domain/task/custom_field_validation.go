package taskdom

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ValidateFieldDefinition enforces structural rules on a custom-field
// definition itself (independent of any task value): select / multi_select
// must carry at least one option, otherwise the field can never hold a valid
// value.
func ValidateFieldDefinition(ft FieldType, options []string) error {
	if ft == FieldTypeSelect || ft == FieldTypeMultiSelect {
		if len(options) == 0 {
			return ErrCustomFieldOptionsInvalid
		}
	}
	return nil
}

// ValidateCustomFields validates and normalizes a task's raw custom-field
// values against the project's definitions. It:
//   - drops keys with no matching definition (keeps the JSONB clean of junk),
//   - coerces/validates each provided value to its declared type (rejecting
//     bad types, out-of-range select options, malformed dates/URLs),
//   - fills in default values for absent fields.
//
// Required-field handling depends on enforceAllRequired:
//   - true  (task creation): every required field must end up non-empty.
//   - false (task update): a required field may stay absent (e.g. it was added
//     after the task existed, or a partial edit doesn't touch it) — but the
//     caller may NOT explicitly clear one it did send.
//
// The returned map is what should be persisted; a non-nil error means the
// caller supplied an invalid or incomplete value and the write must be refused.
func ValidateCustomFields(defs []*CustomFieldDefinition, values map[string]any, enforceAllRequired bool) (map[string]any, error) {
	byKey := make(map[string]*CustomFieldDefinition, len(defs))
	for _, d := range defs {
		byKey[d.FieldKey] = d
	}

	provided := make(map[string]bool, len(values)) // keys the caller explicitly sent
	out := make(map[string]any, len(values))
	for k, v := range values {
		provided[k] = true
		d, ok := byKey[k]
		if !ok {
			continue // no such field in this project — drop rather than persist junk
		}
		if v == nil {
			continue // explicit clear; required-check below still applies
		}
		cv, err := coerceFieldValue(d, v)
		if err != nil {
			return nil, err
		}
		if isEmptyFieldValue(cv) {
			continue // treat "" / [] as absent so defaults + required apply
		}
		out[k] = cv
	}

	// Fill defaults for fields the caller left out.
	for _, d := range defs {
		if _, present := out[d.FieldKey]; !present && !isEmptyFieldValue(d.DefaultValue) {
			out[d.FieldKey] = d.DefaultValue
		}
	}

	// Enforce required fields.
	for _, d := range defs {
		if !d.IsRequired {
			continue
		}
		if v, present := out[d.FieldKey]; present && !isEmptyFieldValue(v) {
			continue // satisfied
		}
		if enforceAllRequired || provided[d.FieldKey] {
			return nil, fmt.Errorf("%w: %q", ErrCustomFieldRequired, d.DisplayName)
		}
	}

	return out, nil
}

// CoerceDefaultValue validates a proposed default value against a field type,
// returning the normalized value (or nil when no default is set). Used when
// creating/updating a definition so a stored default is always type-correct.
func CoerceDefaultValue(ft FieldType, options []string, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	d := &CustomFieldDefinition{FieldType: ft, Options: options, DisplayName: "default"}
	cv, err := coerceFieldValue(d, v)
	if err != nil {
		return nil, err
	}
	if isEmptyFieldValue(cv) {
		return nil, nil
	}
	return cv, nil
}

// coerceFieldValue validates one value against a field definition and returns
// the value normalized to a canonical Go type (string / float64 / bool /
// []string) suitable for JSONB storage.
func coerceFieldValue(d *CustomFieldDefinition, v any) (any, error) {
	switch d.FieldType {
	case FieldTypeText:
		s, ok := v.(string)
		if !ok {
			return nil, fieldValueErr(d, "expected a string")
		}
		return s, nil

	case FieldTypeURL:
		s, ok := v.(string)
		if !ok {
			return nil, fieldValueErr(d, "expected a URL string")
		}
		s = strings.TrimSpace(s)
		if s != "" {
			u, err := url.ParseRequestURI(s)
			if err != nil || u.Scheme == "" || u.Host == "" {
				return nil, fieldValueErr(d, "is not a valid URL")
			}
		}
		return s, nil

	case FieldTypeNumber:
		f, ok := toFloat(v)
		if !ok {
			return nil, fieldValueErr(d, "expected a number")
		}
		return f, nil

	case FieldTypeBoolean:
		b, ok := v.(bool)
		if !ok {
			return nil, fieldValueErr(d, "expected a boolean")
		}
		return b, nil

	case FieldTypeDate:
		s, ok := v.(string)
		if !ok {
			return nil, fieldValueErr(d, "expected a date string")
		}
		s = strings.TrimSpace(s)
		if s != "" && !isValidDate(s) {
			return nil, fieldValueErr(d, "is not a valid date (expected YYYY-MM-DD)")
		}
		return s, nil

	case FieldTypeSelect:
		s, ok := v.(string)
		if !ok {
			return nil, fieldValueErr(d, "expected one of the allowed options")
		}
		if s != "" && !containsStr(d.Options, s) {
			return nil, fieldValueErr(d, "is not one of the allowed options")
		}
		return s, nil

	case FieldTypeMultiSelect:
		arr, ok := toStringSlice(v)
		if !ok {
			return nil, fieldValueErr(d, "expected an array of options")
		}
		for _, item := range arr {
			if !containsStr(d.Options, item) {
				return nil, fieldValueErr(d, fmt.Sprintf("value %q is not one of the allowed options", item))
			}
		}
		return arr, nil

	default:
		return v, nil
	}
}

func fieldValueErr(d *CustomFieldDefinition, msg string) error {
	return fmt.Errorf("%w: %q %s", ErrCustomFieldValueInvalid, d.DisplayName, msg)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toStringSlice(v any) ([]string, bool) {
	switch a := v.(type) {
	case []string:
		return a, true
	case []any:
		out := make([]string, 0, len(a))
		for _, item := range a {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func isValidDate(s string) bool {
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func isEmptyFieldValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []string:
		return len(x) == 0
	case []any:
		return len(x) == 0
	default:
		return false
	}
}
