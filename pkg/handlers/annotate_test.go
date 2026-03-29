package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAnnotateFlag(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantAdd   map[string]string
		wantRemov []string
		wantErr   bool
	}{
		{
			name:    "single addition",
			input:   "foo=bar",
			wantAdd: map[string]string{"foo": "bar"},
		},
		{
			name:    "multiple additions",
			input:   "foo=bar,baz=qux",
			wantAdd: map[string]string{"foo": "bar", "baz": "qux"},
		},
		{
			name:      "single removal",
			input:     "foo-",
			wantRemov: []string{"foo"},
		},
		{
			name:      "multiple removals",
			input:     "foo-,bar-",
			wantRemov: []string{"foo", "bar"},
		},
		{
			name:      "mixed additions and removals",
			input:     "foo=bar,baz-",
			wantAdd:   map[string]string{"foo": "bar"},
			wantRemov: []string{"baz"},
		},
		{
			name:    "value with equals sign",
			input:   "url=http://example.com?q=1",
			wantAdd: map[string]string{"url": "http://example.com?q=1"},
		},
		{
			name:    "empty value",
			input:   "foo=",
			wantAdd: map[string]string{"foo": ""},
		},
		{
			name:    "annotation with slash in key",
			input:   "kubernetes.io/name=test",
			wantAdd: map[string]string{"kubernetes.io/name": "test"},
		},
		{
			name:      "removal with slash in key",
			input:     "kubernetes.io/name-",
			wantRemov: []string{"kubernetes.io/name"},
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid format no equals or dash",
			input:   "foobar",
			wantErr: true,
		},
		{
			name:    "empty removal key",
			input:   "-",
			wantErr: true,
		},
		{
			name:    "equals at start",
			input:   "=value",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseAnnotateFlag(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantAdd != nil {
				assert.Equal(t, tt.wantAdd, cfg.Add)
			} else {
				assert.Empty(t, cfg.Add)
			}

			if tt.wantRemov != nil {
				assert.Equal(t, tt.wantRemov, cfg.Remove)
			} else {
				assert.Empty(t, cfg.Remove)
			}
		})
	}
}

func TestAnnotateConfig_ToMergePatch(t *testing.T) {
	tests := []struct {
		name    string
		cfg     AnnotateConfig
		wantErr bool
		check   func(t *testing.T, patch []byte)
	}{
		{
			name: "additions only",
			cfg: AnnotateConfig{
				Add: map[string]string{"foo": "bar"},
			},
			check: func(t *testing.T, patch []byte) {
				assert.Contains(t, string(patch), `"foo":"bar"`)
				assert.Contains(t, string(patch), `"metadata"`)
				assert.Contains(t, string(patch), `"annotations"`)
			},
		},
		{
			name: "removals only",
			cfg: AnnotateConfig{
				Add:    map[string]string{},
				Remove: []string{"foo"},
			},
			check: func(t *testing.T, patch []byte) {
				assert.Contains(t, string(patch), `"foo":null`)
			},
		},
		{
			name: "mixed",
			cfg: AnnotateConfig{
				Add:    map[string]string{"foo": "bar"},
				Remove: []string{"baz"},
			},
			check: func(t *testing.T, patch []byte) {
				assert.Contains(t, string(patch), `"foo":"bar"`)
				assert.Contains(t, string(patch), `"baz":null`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch, err := tt.cfg.ToMergePatch()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, patch)
		})
	}
}
