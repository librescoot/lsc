package schema

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type EnumValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Setting struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Label       string      `json:"label,omitempty"`
	UserVisible bool        `json:"user-visible,omitempty"`
	Service     string      `json:"service,omitempty"`
	Default     any         `json:"default,omitempty"`
	Values      []EnumValue `json:"values,omitempty"`
	Unit        string      `json:"unit,omitempty"`
	Min         *float64    `json:"min,omitempty"`
	Max         *float64    `json:"max,omitempty"`
	Example     any         `json:"example,omitempty"`
	ReadOnly    bool        `json:"read-only,omitempty"`
	Pattern     string      `json:"pattern,omitempty"`
}

type Schema struct {
	Settings map[string]Setting
}

func Parse(data []byte) (*Schema, error) {
	var raw map[string]Setting
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	return &Schema{Settings: raw}, nil
}

func (s Setting) PossibleValues() []string {
	out := make([]string, len(s.Values))
	for i, v := range s.Values {
		out[i] = v.Value
	}
	return out
}

func (s *Schema) Get(key string) (Setting, bool) {
	setting, ok := s.Settings[key]
	return setting, ok
}

// Sort keys because Go map iteration is random.
func (s *Schema) Services() []string {
	keys := make([]string, 0, len(s.Settings))
	for k := range s.Settings {
		keys = append(keys, k)
	}
	sortStrings(keys)

	seen := make(map[string]bool)
	var services []string
	for _, k := range keys {
		svc := s.Settings[k].Service
		if svc != "" && !seen[svc] {
			seen[svc] = true
			services = append(services, svc)
		}
	}
	return services
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

func (s *Schema) IsKnown(key string) bool {
	if _, ok := s.Settings[key]; ok {
		return true
	}
	// Indexed settings use .0. as the schema placeholder.
	for schemaKey, setting := range s.Settings {
		if setting.Pattern != "indexed" {
			continue
		}
		if matchIndexedKey(schemaKey, key) {
			return true
		}
	}
	return false
}

func matchIndexedKey(schemaKey, candidateKey string) bool {
	parts := strings.SplitN(schemaKey, ".0.", 2)
	if len(parts) != 2 {
		return false
	}
	prefix := parts[0] + "."
	suffix := "." + parts[1]
	if !strings.HasPrefix(candidateKey, prefix) {
		return false
	}
	rest := candidateKey[len(prefix):]
	if !strings.HasSuffix(rest, suffix) {
		return false
	}
	mid := rest[:len(rest)-len(suffix)]
	_, err := strconv.Atoi(mid)
	return err == nil
}

func (s *Schema) ValidateValue(key, value string) error {
	setting, ok := s.Settings[key]
	if !ok {
		for schemaKey, schemaSetting := range s.Settings {
			if schemaSetting.Pattern == "indexed" && matchIndexedKey(schemaKey, key) {
				setting = schemaSetting
				ok = true
				break
			}
		}
	}
	if !ok {
		return nil
	}

	if setting.ReadOnly {
		return fmt.Errorf("setting is read-only (system-managed)")
	}

	if value == "" {
		return nil
	}

	switch setting.Type {
	case "bool":
		if value != "true" && value != "false" {
			return fmt.Errorf("must be 'true' or 'false'")
		}

	case "int":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("must be an integer")
		}
		if setting.Min != nil && float64(intVal) < *setting.Min {
			return fmt.Errorf("must be >= %.0f", *setting.Min)
		}
		if setting.Max != nil && float64(intVal) > *setting.Max {
			return fmt.Errorf("must be <= %.0f", *setting.Max)
		}

	case "float":
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("must be a number")
		}
		if setting.Min != nil && floatVal < *setting.Min {
			return fmt.Errorf("must be >= %.2f", *setting.Min)
		}
		if setting.Max != nil && floatVal > *setting.Max {
			return fmt.Errorf("must be <= %.2f", *setting.Max)
		}

	case "enum":
		if len(setting.Values) > 0 {
			valid := false
			for _, ev := range setting.Values {
				if value == ev.Value {
					valid = true
					break
				}
			}
			if !valid {
				vals := setting.PossibleValues()
				return fmt.Errorf("must be one of: %s", strings.Join(vals, ", "))
			}
		}

	case "url":
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return fmt.Errorf("must be a valid URL (http:// or https://)")
		}

	case "duration":
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("must be a valid duration (e.g., '1h', '30m', '1h30m')")
		}
		if setting.Min != nil || setting.Max != nil {
			d, _ := time.ParseDuration(value)
			seconds := d.Seconds()
			if setting.Min != nil && seconds < *setting.Min {
				return fmt.Errorf("must be >= %v", time.Duration(*setting.Min*float64(time.Second)))
			}
			if setting.Max != nil && seconds > *setting.Max {
				return fmt.Errorf("must be <= %v", time.Duration(*setting.Max*float64(time.Second)))
			}
		}
	}

	return nil
}
