package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AnnotateConfig holds parsed annotation changes to apply to resources.
type AnnotateConfig struct {
	// Add maps annotation keys to their values (always overwrites existing).
	Add map[string]string
	// Remove lists annotation keys to delete.
	Remove []string
}

// IsEmpty returns true if no annotation changes are configured.
func (a AnnotateConfig) IsEmpty() bool {
	return len(a.Add) == 0 && len(a.Remove) == 0
}

// ParseAnnotateFlag parses an annotation flag value following kubectl annotate syntax.
// Supports:
//   - "k=v" to add/overwrite annotation k with value v
//   - "k=v,k2=v2" for multiple additions
//   - "k-" to remove annotation k
//   - "k=v,k2-" to mix additions and removals
func ParseAnnotateFlag(raw string) (AnnotateConfig, error) {
	cfg := AnnotateConfig{
		Add: make(map[string]string),
	}

	if raw == "" {
		return cfg, errors.New("annotate flag value cannot be empty")
	}

	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.HasSuffix(part, "-") {
			// Removal: "key-"
			key := strings.TrimSuffix(part, "-")
			if key == "" {
				return cfg, fmt.Errorf("invalid annotation removal %q: key cannot be empty", part)
			}
			cfg.Remove = append(cfg.Remove, key)
		} else if idx := strings.Index(part, "="); idx > 0 {
			// Addition: "key=value"
			key := part[:idx]
			value := part[idx+1:]
			cfg.Add[key] = value
		} else {
			return cfg, fmt.Errorf("invalid annotation format %q: expected key=value or key-", part)
		}
	}

	if cfg.IsEmpty() {
		return cfg, fmt.Errorf("no valid annotations found in %q", raw)
	}

	return cfg, nil
}

// ToMergePatch builds a JSON merge patch for the annotation changes.
// Additions set annotation values; removals set them to null.
func (a AnnotateConfig) ToMergePatch() ([]byte, error) {
	annotations := make(map[string]interface{})
	for k, v := range a.Add {
		annotations[k] = v
	}
	for _, k := range a.Remove {
		annotations[k] = nil
	}

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": annotations,
		},
	}

	return json.Marshal(patch)
}
