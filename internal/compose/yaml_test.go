package compose

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHealthcheckTest_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    healthcheckTest
		wantErr bool
	}{
		{
			name: "string scalar form",
			yaml: "\"curl -f http://localhost || exit 1\"",
			want: []string{"CMD-SHELL", "curl -f http://localhost || exit 1"},
		},
		{
			name: "list form",
			yaml: "[\"CMD\", \"curl\", \"-f\", \"http://localhost\"]",
			want: []string{"CMD", "curl", "-f", "http://localhost"},
		},
		{
			name: "list form multiline",
			yaml: "- CMD\n- curl\n- -f\n- http://localhost\n",
			want: []string{"CMD", "curl", "-f", "http://localhost"},
		},
		{
			name:    "invalid form map",
			yaml:    "{\"CMD\": \"curl\"}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got healthcheckTest
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("healthcheckTest.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("healthcheckTest.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvironment_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    Environment
		wantErr bool
	}{
		{
			name: "map form",
			yaml: "KEY1: val1\nKEY2: val2\n",
			want: Environment{"KEY1": "val1", "KEY2": "val2"},
		},
		{
			name: "list form with equals",
			yaml: "- KEY1=val1\n- KEY2=val2\n",
			want: Environment{"KEY1": "val1", "KEY2": "val2"},
		},
		{
			name: "list form without equals",
			yaml: "- KEY1\n- KEY2\n",
			want: Environment{"KEY1": "", "KEY2": ""},
		},
		{
			name:    "invalid form scalar",
			yaml:    "\"KEY1=val1\"",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Environment
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("Environment.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Environment.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworks_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    Networks
		wantErr bool
	}{
		{
			name: "list form",
			yaml: "- net1\n- net2\n",
			want: Networks{"net1", "net2"},
		},
		{
			name: "map form",
			yaml: "net1:\n  aliases:\n    - alias1\nnet2:\n  ipv4_address: 1.2.3.4\n",
			want: Networks{"net1", "net2"},
		},
		{
			name:    "invalid form scalar",
			yaml:    "\"net1\"",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Networks
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("Networks.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Networks.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDependsOn_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    DependsOn
		wantErr bool
	}{
		{
			name: "list form",
			yaml: "- db\n- redis\n",
			want: DependsOn{"db", "redis"},
		},
		{
			name: "map form",
			yaml: "db:\n  condition: service_healthy\nredis:\n  restart: true\n",
			want: DependsOn{"db", "redis"},
		},
		{
			name:    "invalid form scalar",
			yaml:    "\"db\"",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DependsOn
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("DependsOn.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DependsOn.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPort_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    Port
		wantErr bool
	}{
		{
			name: "short scalar form container only",
			yaml: "\"8080\"",
			want: Port{ContainerPort: 8080},
		},
		{
			name: "short scalar form host:container",
			yaml: "\"8000:80\"",
			want: Port{HostPort: 8000, ContainerPort: 80},
		},
		{
			name:    "short scalar form invalid format (ip:host:container)",
			yaml:    "\"127.0.0.1:8000:80\"",
			wantErr: true,
		},
		{
			name:    "short scalar form invalid port",
			yaml:    "\"abc:80\"",
			wantErr: true,
		},
		{
			name:    "short scalar form out of range",
			yaml:    "\"65536:80\"",
			wantErr: true,
		},
		{
			name: "long mapping form",
			yaml: "target: 80\npublished: 8000\nprotocol: tcp\nmode: host\n",
			want: Port{ContainerPort: 80, HostPort: 8000},
		},
		{
			name: "long mapping form string ports",
			yaml: "target: \"80\"\npublished: \"8000\"\n",
			want: Port{ContainerPort: 80, HostPort: 8000},
		},
		{
			name: "long mapping form no published",
			yaml: "target: 80\n",
			want: Port{ContainerPort: 80},
		},
		{
			name:    "long mapping form target missing",
			yaml:    "published: 8000\n",
			wantErr: true,
		},
		{
			name:    "long mapping form unsupported protocol",
			yaml:    "target: 80\nprotocol: udp\n",
			wantErr: true,
		},
		{
			name:    "long mapping form port range",
			yaml:    "target: 80\npublished: \"8000-8010\"\n",
			wantErr: true,
		},
		{
			name:    "invalid form list",
			yaml:    "[]",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Port
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("Port.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Port.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestYamlScalarString_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    yamlScalarString
		wantErr bool
	}{
		{
			name: "number",
			yaml: "80",
			want: "80",
		},
		{
			name: "string",
			yaml: "\"80\"",
			want: "80",
		},
		{
			name:    "invalid form list",
			yaml:    "[]",
			wantErr: true,
		},
		{
			name:    "invalid form map",
			yaml:    "{}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got yamlScalarString
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("yamlScalarString.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("yamlScalarString.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolume_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    Volume
		wantErr bool
	}{
		{
			name: "short scalar form named volume",
			yaml: "\"myvol:/data\"",
			want: Volume{Name: "myvol", ContainerPath: "/data"},
		},
		{
			name: "short scalar form host path",
			yaml: "\"/host/path:/data\"",
			want: Volume{HostPath: "/host/path", ContainerPath: "/data"},
		},
		{
			name: "short scalar form read only",
			yaml: "\"myvol:/data:ro\"",
			want: Volume{Name: "myvol", ContainerPath: "/data", ReadOnly: true},
		},
		{
			name: "short scalar form read write",
			yaml: "\"myvol:/data:rw\"",
			want: Volume{Name: "myvol", ContainerPath: "/data", ReadOnly: false},
		},
		{
			name:    "short scalar form missing path",
			yaml:    "\"myvol\"",
			wantErr: true,
		},
		{
			name:    "short scalar form relative path",
			yaml:    "\"./data:/data\"",
			wantErr: true,
		},
		{
			name: "long mapping form volume type",
			yaml: "type: volume\nsource: myvol\ntarget: /data\nread_only: true\n",
			want: Volume{Name: "myvol", ContainerPath: "/data", ReadOnly: true},
		},
		{
			name: "long mapping form volume type no source",
			yaml: "type: volume\ntarget: /data\n",
			wantErr: true,
		},
		{
			name: "long mapping form volume type absolute source",
			yaml: "type: volume\nsource: /host/path\ntarget: /data\n",
			wantErr: true,
		},
		{
			name: "long mapping form volume type relative source",
			yaml: "type: volume\nsource: ./path\ntarget: /data\n",
			wantErr: true,
		},
		{
			name: "long mapping form bind type",
			yaml: "type: bind\nsource: /host/path\ntarget: /data\n",
			want: Volume{HostPath: "/host/path", ContainerPath: "/data"},
		},
		{
			name: "long mapping form bind type missing source",
			yaml: "type: bind\ntarget: /data\n",
			wantErr: true,
		},
		{
			name: "long mapping form bind type relative source",
			yaml: "type: bind\nsource: ./path\ntarget: /data\n",
			wantErr: true,
		},
		{
			name: "long mapping form bind type non-absolute source",
			yaml: "type: bind\nsource: host/path\ntarget: /data\n",
			wantErr: true,
		},
		{
			name:    "long mapping form missing target",
			yaml:    "type: volume\nsource: myvol\n",
			wantErr: true,
		},
		{
			name:    "long mapping form unsupported type",
			yaml:    "type: tmpfs\ntarget: /data\n",
			wantErr: true,
		},
		{
			name:    "invalid form list",
			yaml:    "[]",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Volume
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("Volume.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Volume.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommand_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    Command
		wantErr bool
	}{
		{
			name: "scalar string form",
			yaml: "\"echo hello\"",
			want: Command{"/bin/sh", "-c", "echo hello"},
		},
		{
			name: "sequence list form",
			yaml: "- echo\n- hello\n",
			want: Command{"echo", "hello"},
		},
		{
			name:    "invalid form map",
			yaml:    "{}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Command
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Command.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRawBuild_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    rawBuild
		wantErr bool
	}{
		{
			name: "scalar form",
			yaml: "\".\"",
			want: rawBuild{Context: "."},
		},
		{
			name: "object form",
			yaml: "context: .\ndockerfile: Dockerfile.alt\n",
			want: rawBuild{Context: ".", Dockerfile: "Dockerfile.alt"},
		},
		{
			name:    "invalid form list",
			yaml:    "[]",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got rawBuild
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("rawBuild.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rawBuild.UnmarshalYAML() = %v, want %v", got, tt.want)
			}
		})
	}
}
