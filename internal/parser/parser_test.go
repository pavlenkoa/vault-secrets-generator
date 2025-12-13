package parser

import (
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "simple key",
			json: `{"key": "value"}`,
			path: ".key",
			want: "value",
		},
		{
			name: "nested key",
			json: `{"outputs": {"db_host": {"value": "localhost"}}}`,
			path: ".outputs.db_host.value",
			want: "localhost",
		},
		{
			name: "without leading dot",
			json: `{"outputs": {"db_host": {"value": "localhost"}}}`,
			path: "outputs.db_host.value",
			want: "localhost",
		},
		{
			name: "array access",
			json: `{"items": ["a", "b", "c"]}`,
			path: ".items[1]",
			want: "b",
		},
		{
			name: "nested array",
			json: `{"data": [{"name": "first"}, {"name": "second"}]}`,
			path: ".data[1].name",
			want: "second",
		},
		{
			name: "integer value",
			json: `{"port": 5432}`,
			path: ".port",
			want: "5432",
		},
		{
			name: "float value",
			json: `{"rate": 3.14}`,
			path: ".rate",
			want: "3.14",
		},
		{
			name: "boolean value",
			json: `{"enabled": true}`,
			path: ".enabled",
			want: "true",
		},
		{
			name: "null value",
			json: `{"value": null}`,
			path: ".value",
			want: "",
		},
		{
			name: "terraform state output",
			json: `{
				"version": 4,
				"outputs": {
					"endpoint": {
						"value": "mydb.123456.rds.amazonaws.com",
						"type": "string"
					}
				}
			}`,
			path: ".outputs.endpoint.value",
			want: "mydb.123456.rds.amazonaws.com",
		},
		{
			name:    "key not found",
			json:    `{"key": "value"}`,
			path:    ".missing",
			wantErr: true,
		},
		{
			name:    "array index out of bounds",
			json:    `{"items": ["a", "b"]}`,
			path:    ".items[5]",
			wantErr: true,
		},
		{
			name:    "invalid json",
			json:    `not json`,
			path:    ".key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractJSON([]byte(tt.json), tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "simple key",
			yaml: "key: value",
			path: ".key",
			want: "value",
		},
		{
			name: "nested key",
			yaml: `
database:
  host: localhost
  port: 5432
`,
			path: ".database.host",
			want: "localhost",
		},
		{
			name: "integer in yaml",
			yaml: `
database:
  port: 5432
`,
			path: ".database.port",
			want: "5432",
		},
		{
			name: "array access",
			yaml: `
items:
  - first
  - second
  - third
`,
			path: ".items[0]",
			want: "first",
		},
		{
			name: "nested object in array",
			yaml: `
servers:
  - name: server1
    ip: 10.0.0.1
  - name: server2
    ip: 10.0.0.2
`,
			path: ".servers[1].ip",
			want: "10.0.0.2",
		},
		{
			name:    "key not found",
			yaml:    "key: value",
			path:    ".missing",
			wantErr: true,
		},
		{
			name:    "invalid yaml",
			yaml:    "not: valid: yaml: here",
			path:    ".key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractYAML([]byte(tt.yaml), tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractYAML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		path string
		want []pathPart
	}{
		{
			path: "key",
			want: []pathPart{{key: "key"}},
		},
		{
			path: "a.b.c",
			want: []pathPart{{key: "a"}, {key: "b"}, {key: "c"}},
		},
		{
			path: "items[0]",
			want: []pathPart{{key: "items"}, {isArray: true, index: 0}},
		},
		{
			path: "data[1].name",
			want: []pathPart{{key: "data"}, {isArray: true, index: 1}, {key: "name"}},
		},
		{
			path: "[0]",
			want: []pathPart{{isArray: true, index: 0}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := parsePath(tt.path)
			if len(got) != len(tt.want) {
				t.Errorf("parsePath(%q) returned %d parts, want %d", tt.path, len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parsePath(%q)[%d] = %+v, want %+v", tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}
