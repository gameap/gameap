package configschema

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// FieldError is one validation failure, attributed to a configuration key.
type FieldError struct {
	Field   string
	Message string
}

// ValidationErrors is the list of failures of one validation; nil when the
// values are valid.
type ValidationErrors []FieldError

func (e ValidationErrors) Error() string {
	parts := make([]string, 0, len(e))
	for _, fieldErr := range e {
		parts = append(parts, fieldErr.Field+" "+fieldErr.Message)
	}

	return strings.Join(parts, "; ")
}

// Map renders the failures as field → message, the shape the admin API
// answers with.
func (e ValidationErrors) Map() map[string]string {
	if len(e) == 0 {
		return nil
	}

	result := make(map[string]string, len(e))
	for _, fieldErr := range e {
		if _, dup := result[fieldErr.Field]; !dup {
			result[fieldErr.Field] = fieldErr.Message
		}
	}

	return result
}

// Validate checks operator-supplied values against the schema. Values are
// JSON-decoded (strings, booleans, json.Number or float64) or typed Go
// values; nil means "absent" — which satisfies a required property that
// declares a default. Keys the schema does not declare are allowed as
// strings unless additionalProperties is false. Failures are reported in a
// stable order (declared properties first, then unknown keys by name).
func (s *Schema) Validate(values map[string]any) ValidationErrors {
	var errs ValidationErrors

	if s == nil {
		return ValidateFreeForm(values)
	}

	for i := range s.Properties {
		property := &s.Properties[i]

		value, present := values[property.Name]
		if !present || value == nil {
			// The effective configuration is defaults ⊕ values, so a
			// required key with a default always holds a value.
			if property.Required && property.Default == nil {
				errs = append(errs, FieldError{Field: property.Name, Message: "is required"})
			}

			continue
		}

		if message := property.check(value); message != "" {
			errs = append(errs, FieldError{Field: property.Name, Message: message})
		}
	}

	unknown := make([]string, 0)
	for key := range values {
		if _, declared := s.byName[key]; !declared {
			unknown = append(unknown, key)
		}
	}

	sort.Strings(unknown)

	for _, key := range unknown {
		if !s.AdditionalProperties {
			errs = append(errs, FieldError{Field: key, Message: "unknown key"})

			continue
		}

		if message := checkFreeFormEntry(key, values[key]); message != "" {
			errs = append(errs, FieldError{Field: key, Message: message})
		}
	}

	return errs
}

// ValidateFreeForm checks values for a plugin without a schema: every key
// must be a valid configuration key and every value a bounded string.
func ValidateFreeForm(values map[string]any) ValidationErrors {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	var errs ValidationErrors

	for _, key := range keys {
		if values[key] == nil {
			continue
		}

		if message := checkFreeFormEntry(key, values[key]); message != "" {
			errs = append(errs, FieldError{Field: key, Message: message})
		}
	}

	return errs
}

func checkFreeFormEntry(key string, value any) string {
	if !KeyPattern.MatchString(key) {
		return "must match " + KeyPattern.String()
	}

	text, ok := value.(string)
	if !ok {
		return "must be a string"
	}

	if len(text) > MaxValueBytes {
		return "must be at most " + strconv.Itoa(MaxValueBytes) + " bytes"
	}

	return ""
}

// check validates one present value; "" means valid.
func (p *Property) check(value any) string {
	typed, err := coerce(value, p.Type)
	if err != nil {
		return err.Error()
	}

	switch p.Type {
	case TypeString:
		if message := p.checkString(typed.(string)); message != "" { //nolint:forcetypeassert
			return message
		}
	case TypeInteger, TypeNumber:
		f, _ := toFloat(typed)
		if message := p.checkBounds(f); message != "" {
			return message
		}
	case TypeBoolean:
	}

	if len(p.Enum) > 0 && !containsValue(p.Enum, typed) {
		return "must be one of: " + enumList(p.Enum)
	}

	return ""
}

func (p *Property) checkString(text string) string {
	if len(text) > MaxValueBytes {
		return "must be at most " + strconv.Itoa(MaxValueBytes) + " bytes"
	}

	length := utf8.RuneCountInString(text)

	switch {
	case p.MinLength != nil && p.MaxLength != nil && (length < *p.MinLength || length > *p.MaxLength):
		return "must be between " + strconv.Itoa(*p.MinLength) + " and " + strconv.Itoa(*p.MaxLength) + " characters"
	case p.MinLength != nil && length < *p.MinLength:
		return "must be at least " + strconv.Itoa(*p.MinLength) + " characters"
	case p.MaxLength != nil && length > *p.MaxLength:
		return "must be at most " + strconv.Itoa(*p.MaxLength) + " characters"
	}

	if p.Pattern != nil && !p.Pattern.MatchString(text) {
		return "must match " + p.Pattern.String()
	}

	return ""
}

func (p *Property) checkBounds(f float64) string {
	switch {
	case p.Minimum != nil && p.Maximum != nil && (f < *p.Minimum || f > *p.Maximum):
		return "must be between " + formatFloat(*p.Minimum) + " and " + formatFloat(*p.Maximum)
	case p.Minimum != nil && f < *p.Minimum:
		return "must be at least " + formatFloat(*p.Minimum)
	case p.Maximum != nil && f > *p.Maximum:
		return "must be at most " + formatFloat(*p.Maximum)
	}

	return ""
}

func containsValue(values []any, wanted any) bool {
	return slices.Contains(values, wanted)
}

func enumList(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		text, _ := ValueToString(value)
		parts = append(parts, text)
	}

	return strings.Join(parts, ", ")
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Normalize converts validated values into their canonical stored form:
// declared properties become string, int64, float64 or bool according to
// their type, undeclared keys stay strings; nil values are dropped. Values
// that fail coercion are reported with the same messages as Validate.
func (s *Schema) Normalize(values map[string]any) (map[string]any, ValidationErrors) {
	result := make(map[string]any, len(values))

	var errs ValidationErrors

	for key, value := range values {
		if value == nil {
			continue
		}

		property, declared := s.Property(key)
		if !declared {
			text, ok := value.(string)
			if !ok {
				errs = append(errs, FieldError{Field: key, Message: "must be a string"})

				continue
			}

			result[key] = text

			continue
		}

		typed, err := coerce(value, property.Type)
		if err != nil {
			errs = append(errs, FieldError{Field: key, Message: err.Error()})

			continue
		}

		result[key] = typed
	}

	sort.Slice(errs, func(i, j int) bool { return errs[i].Field < errs[j].Field })

	return result, errs
}

// SummaryInfo describes a schema text for listings without exposing it whole.
type SummaryInfo struct {
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
	Properties int    `json:"properties"`
	Required   int    `json:"required"`
	Secrets    int    `json:"secrets"`
}

// Summary parses the schema text and counts what it declares; an invalid
// schema is reported with Valid false and the parse error.
func Summary(text string) SummaryInfo {
	schema, err := Parse(text)
	if err != nil {
		return SummaryInfo{Error: err.Error()}
	}

	info := SummaryInfo{Valid: true}
	if schema == nil {
		return info
	}

	info.Properties = len(schema.Properties)

	for _, property := range schema.Properties {
		if property.Required {
			info.Required++
		}

		if property.Secret {
			info.Secrets++
		}
	}

	return info
}

// PropertyJSON is the API rendering of a property.
type PropertyJSON struct {
	Name        string   `json:"name"`
	Type        Type     `json:"type"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Default     any      `json:"default,omitempty"`
	Enum        []any    `json:"enum,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
	MinLength   *int     `json:"min_length,omitempty"`
	MaxLength   *int     `json:"max_length,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
}

// SchemaJSON is the API rendering of a schema: an ordered property list
// instead of the JSON Schema object, so the form needs no schema parser.
type SchemaJSON struct {
	Properties           []PropertyJSON `json:"properties"`
	AdditionalProperties bool           `json:"additional_properties"`
}

// JSON renders the schema for the admin API.
func (s *Schema) JSON() *SchemaJSON {
	if s == nil {
		return nil
	}

	rendered := &SchemaJSON{
		Properties:           make([]PropertyJSON, 0, len(s.Properties)),
		AdditionalProperties: s.AdditionalProperties,
	}

	for _, property := range s.Properties {
		var pattern string
		if property.Pattern != nil {
			pattern = property.Pattern.String()
		}

		rendered.Properties = append(rendered.Properties, PropertyJSON{
			Name:        property.Name,
			Type:        property.Type,
			Title:       property.Title,
			Description: property.Description,
			Secret:      property.Secret,
			Required:    property.Required,
			Default:     property.Default,
			Enum:        property.Enum,
			Minimum:     property.Minimum,
			Maximum:     property.Maximum,
			MinLength:   property.MinLength,
			MaxLength:   property.MaxLength,
			Pattern:     pattern,
		})
	}

	return rendered
}
