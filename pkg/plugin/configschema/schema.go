// Package configschema parses and applies the JSON Schema subset a plugin may
// declare in PluginInfo.config_schema: a flat object of scalar properties
// (string, integer, number, boolean) with titles, descriptions, defaults,
// enums, bounds, patterns and a "secret" string format. Anything beyond that
// (nested objects, arrays, $ref, combinators) is rejected at parse time so
// the admin form and the validator agree on what a schema can express.
package configschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Type is a supported property type.
type Type string

const (
	TypeString  Type = "string"
	TypeInteger Type = "integer"
	TypeNumber  Type = "number"
	TypeBoolean Type = "boolean"
)

// FormatSecret marks a string property whose value is stored encrypted and
// masked in the admin API.
const FormatSecret = "secret"

const (
	// MaxSchemaBytes bounds the schema text a manifest may declare.
	MaxSchemaBytes = 64 << 10
	// MaxProperties bounds the number of properties.
	MaxProperties = 100
	// MaxValueBytes bounds one string value.
	MaxValueBytes = 8 << 10
	// maxSafeInteger is the largest integer a JSON number round-trips exactly.
	maxSafeInteger = 1 << 53
)

// KeyPattern is what a property name (and a free-form configuration key) must
// match.
var KeyPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

var unsupportedRootKeywords = []string{
	"$ref", "oneOf", "anyOf", "allOf", "not", "patternProperties", "dependencies",
	"dependentSchemas", "if", "then", "else", "items", "prefixItems",
}

// Property describes one configuration key.
type Property struct {
	Name        string
	Type        Type
	Title       string
	Description string
	// Secret is set for string properties with "format": "secret".
	Secret bool
	// Default is typed: string, int64, float64 or bool; nil when absent.
	Default any
	// Enum holds typed values like Default.
	Enum      []any
	Minimum   *float64
	Maximum   *float64
	MinLength *int
	MaxLength *int
	Pattern   *regexp.Regexp
	Required  bool
}

// Schema is a parsed configuration schema; Properties keep the declaration
// order so the admin form renders fields the way the author listed them.
type Schema struct {
	Properties           []Property
	AdditionalProperties bool

	byName map[string]int
}

// Parse parses the schema text. An empty text yields a nil schema and no
// error (free-form configuration).
func Parse(text string) (*Schema, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil //nolint:nilnil
	}

	if len(text) > MaxSchemaBytes {
		return nil, errors.Errorf("schema exceeds %d bytes", MaxSchemaBytes)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		return nil, errors.WithMessage(err, "schema is not a JSON object")
	}

	for _, keyword := range unsupportedRootKeywords {
		if _, found := root[keyword]; found {
			return nil, errors.Errorf("unsupported keyword %q", keyword)
		}
	}

	if raw, found := root["type"]; found {
		var rootType string
		if err := json.Unmarshal(raw, &rootType); err != nil || rootType != "object" {
			return nil, errors.New(`root "type" must be "object"`)
		}
	}

	schema := &Schema{AdditionalProperties: true, byName: make(map[string]int)}

	if raw, found := root["additionalProperties"]; found {
		if err := json.Unmarshal(raw, &schema.AdditionalProperties); err != nil {
			return nil, errors.New(`"additionalProperties" must be a boolean`)
		}
	}

	if raw, found := root["properties"]; found {
		if err := schema.parseProperties(raw); err != nil {
			return nil, err
		}
	}

	if raw, found := root["required"]; found {
		if err := schema.parseRequired(raw); err != nil {
			return nil, err
		}
	}

	return schema, nil
}

// parseProperties reads the properties object token by token to keep the
// declaration order, which encoding/json maps would lose.
func (s *Schema) parseProperties(raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	open, err := dec.Token()
	if err != nil || open != json.Delim('{') {
		return errors.New(`"properties" must be an object`)
	}

	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return errors.WithMessage(err, `invalid "properties"`)
		}

		name, _ := keyToken.(string)
		if !KeyPattern.MatchString(name) {
			return errors.Errorf("property name %q must match %s", name, KeyPattern.String())
		}

		if _, dup := s.byName[name]; dup {
			return errors.Errorf("property %q is declared twice", name)
		}

		if len(s.Properties) >= MaxProperties {
			return errors.Errorf("schema declares more than %d properties", MaxProperties)
		}

		var definition map[string]json.RawMessage
		if err := dec.Decode(&definition); err != nil {
			return errors.Errorf("property %q must be an object", name)
		}

		property, err := parseProperty(name, definition)
		if err != nil {
			return errors.WithMessagef(err, "property %q", name)
		}

		s.byName[name] = len(s.Properties)
		s.Properties = append(s.Properties, property)
	}

	if _, err := dec.Token(); err != nil && !errors.Is(err, io.EOF) {
		return errors.WithMessage(err, `invalid "properties"`)
	}

	return nil
}

func (s *Schema) parseRequired(raw json.RawMessage) error {
	var required []string
	if err := json.Unmarshal(raw, &required); err != nil {
		return errors.New(`"required" must be an array of property names`)
	}

	for _, name := range required {
		index, found := s.byName[name]
		if !found {
			return errors.Errorf(`"required" names unknown property %q`, name)
		}

		s.Properties[index].Required = true
	}

	return nil
}

func parseProperty(name string, definition map[string]json.RawMessage) (Property, error) {
	property := Property{Name: name}

	rawType, found := definition["type"]
	if !found {
		return property, errors.New(`missing "type"`)
	}

	var typeName string
	if err := json.Unmarshal(rawType, &typeName); err != nil {
		return property, errors.New(`"type" must be a string`)
	}

	switch Type(typeName) {
	case TypeString, TypeInteger, TypeNumber, TypeBoolean:
		property.Type = Type(typeName)
	default:
		return property, errors.Errorf("unsupported type %q", typeName)
	}

	if err := decodeOptionalString(definition, "title", &property.Title); err != nil {
		return property, err
	}

	if err := decodeOptionalString(definition, "description", &property.Description); err != nil {
		return property, err
	}

	var format string
	if err := decodeOptionalString(definition, "format", &format); err != nil {
		return property, err
	}

	if format == FormatSecret {
		if property.Type != TypeString {
			return property, errors.New(`"format": "secret" requires type string`)
		}

		property.Secret = true
	}

	if err := parsePropertyConstraints(&property, definition); err != nil {
		return property, err
	}

	if err := parsePropertyValues(&property, definition); err != nil {
		return property, err
	}

	return property, nil
}

func parsePropertyConstraints(property *Property, definition map[string]json.RawMessage) error {
	if property.Type == TypeInteger || property.Type == TypeNumber {
		if err := decodeOptionalFloat(definition, "minimum", &property.Minimum); err != nil {
			return err
		}

		if err := decodeOptionalFloat(definition, "maximum", &property.Maximum); err != nil {
			return err
		}

		if property.Minimum != nil && property.Maximum != nil && *property.Minimum > *property.Maximum {
			return errors.New(`"minimum" exceeds "maximum"`)
		}
	}

	if property.Type != TypeString {
		return nil
	}

	if err := decodeOptionalLength(definition, "minLength", &property.MinLength); err != nil {
		return err
	}

	if err := decodeOptionalLength(definition, "maxLength", &property.MaxLength); err != nil {
		return err
	}

	if property.MinLength != nil && property.MaxLength != nil && *property.MinLength > *property.MaxLength {
		return errors.New(`"minLength" exceeds "maxLength"`)
	}

	var pattern string
	if err := decodeOptionalString(definition, "pattern", &pattern); err != nil {
		return err
	}

	if pattern != "" {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return errors.WithMessage(err, `invalid "pattern"`)
		}

		property.Pattern = compiled
	}

	return nil
}

func parsePropertyValues(property *Property, definition map[string]json.RawMessage) error {
	if raw, found := definition["default"]; found {
		if property.Secret {
			return errors.New("a secret must not declare a default")
		}

		value, err := decodeTypedValue(raw, property.Type)
		if err != nil {
			return errors.WithMessage(err, `"default"`)
		}

		property.Default = value
	}

	raw, found := definition["enum"]
	if !found {
		return nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return errors.New(`"enum" must be a non-empty array`)
	}

	if property.Secret {
		return errors.New("a secret must not declare an enum")
	}

	for _, item := range items {
		value, err := decodeTypedValue(item, property.Type)
		if err != nil {
			return errors.WithMessage(err, `"enum"`)
		}

		property.Enum = append(property.Enum, value)
	}

	return nil
}

// decodeTypedValue converts a JSON literal into the typed Go value the
// property expects (string, int64, float64 or bool).
func decodeTypedValue(raw json.RawMessage, propertyType Type) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, errors.New("invalid value")
	}

	return coerce(value, propertyType)
}

// coerce checks that a decoded value matches the property type and returns
// it in canonical form: string, int64, float64 or bool.
func coerce(value any, propertyType Type) (any, error) {
	switch propertyType {
	case TypeString:
		s, ok := value.(string)
		if !ok {
			return nil, errors.New("must be a string")
		}

		return s, nil
	case TypeBoolean:
		b, ok := value.(bool)
		if !ok {
			return nil, errors.New("must be a boolean")
		}

		return b, nil
	case TypeInteger:
		return coerceInteger(value)
	case TypeNumber:
		f, ok := toFloat(value)
		if !ok {
			return nil, errors.New("must be a number")
		}

		return f, nil
	default:
		return nil, errors.Errorf("unsupported type %q", propertyType)
	}
}

func coerceInteger(value any) (any, error) {
	switch v := value.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			if i > maxSafeInteger || i < -maxSafeInteger {
				return nil, errors.New("must be an integer within ±2^53")
			}

			return i, nil
		}

		f, err := v.Float64()
		if err != nil || f != float64(int64(f)) {
			return nil, errors.New("must be an integer")
		}

		return integerFromFloat(f)
	case float64:
		if v != float64(int64(v)) {
			return nil, errors.New("must be an integer")
		}

		return integerFromFloat(v)
	case float32:
		return coerceInteger(float64(v))
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return nil, errors.New("must be an integer")
	}
}

func integerFromFloat(f float64) (any, error) {
	if f > maxSafeInteger || f < -maxSafeInteger {
		return nil, errors.New("must be an integer within ±2^53")
	}

	return int64(f), nil
}

func toFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()

		return f, err == nil
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func decodeOptionalString(definition map[string]json.RawMessage, key string, target *string) error {
	raw, found := definition[key]
	if !found {
		return nil
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return errors.Errorf("%q must be a string", key)
	}

	return nil
}

func decodeOptionalFloat(definition map[string]json.RawMessage, key string, target **float64) error {
	raw, found := definition[key]
	if !found {
		return nil
	}

	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return errors.Errorf("%q must be a number", key)
	}

	*target = &value

	return nil
}

func decodeOptionalLength(definition map[string]json.RawMessage, key string, target **int) error {
	raw, found := definition[key]
	if !found {
		return nil
	}

	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value < 0 {
		return errors.Errorf("%q must be a non-negative integer", key)
	}

	*target = &value

	return nil
}

// Property looks a property up by name.
func (s *Schema) Property(name string) (*Property, bool) {
	if s == nil {
		return nil, false
	}

	index, found := s.byName[name]
	if !found {
		return nil, false
	}

	return &s.Properties[index], true
}

// Defaults renders the declared defaults as the strings the guest receives.
func (s *Schema) Defaults() map[string]string {
	if s == nil {
		return nil
	}

	defaults := make(map[string]string)

	for _, property := range s.Properties {
		if text, ok := ValueToString(property.Default); ok {
			defaults[property.Name] = text
		}
	}

	return defaults
}

// ApplyDefaults overlays the schema defaults under the given values: a key
// the values already hold wins. An empty or invalid schema leaves the values
// untouched; the result is a fresh map when anything was added.
func ApplyDefaults(schemaText string, values map[string]string) map[string]string {
	schema, err := Parse(schemaText)
	if err != nil {
		return values
	}

	return schema.Apply(values)
}

// Apply overlays the schema defaults under the given values (nil-safe).
func (s *Schema) Apply(values map[string]string) map[string]string {
	if s == nil {
		return values
	}

	defaults := s.Defaults()
	if len(defaults) == 0 {
		return values
	}

	merged := make(map[string]string, len(values)+len(defaults))
	maps.Copy(merged, defaults)
	maps.Copy(merged, values)

	return merged
}

// ValueToString renders a stored configuration value the way the guest
// receives it: strings as they are, booleans as true/false, numbers in
// their shortest decimal form, anything structured as compact JSON. ok is
// false for nil, which means "absent".
func ValueToString(value any) (string, bool) {
	switch v := value.(type) {
	case nil:
		return "", false
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), true
	case int:
		return strconv.Itoa(v), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	case json.Number:
		return v.String(), true
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v), true
		}

		return string(encoded), true
	}
}
