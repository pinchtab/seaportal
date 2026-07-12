package tools

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestRequiredURLArg(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]interface{}
		key     string
		want    string
		wantErr string
	}{
		{
			name: "present",
			args: map[string]interface{}{"url": "https://example.com"},
			key:  "url",
			want: "https://example.com",
		},
		{
			name:    "missing key",
			args:    map[string]interface{}{},
			key:     "url",
			wantErr: "missing required argument: url",
		},
		{
			name:    "empty string",
			args:    map[string]interface{}{"url": ""},
			key:     "url",
			wantErr: "missing required argument: url",
		},
		{
			name:    "non-string value",
			args:    map[string]interface{}{"base_url": 42.0},
			key:     "base_url",
			wantErr: "missing required argument: base_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requiredURLArg(tt.args, tt.key)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestSecurityFromArgs(t *testing.T) {
	t.Run("default is secure", func(t *testing.T) {
		sec := securityFromArgs(map[string]interface{}{})
		if sec == nil {
			t.Fatalf("expected non-nil policy")
		}
		if !sec.BlockPrivateIPs {
			t.Errorf("BlockPrivateIPs must default to true")
		}
		if sec.MaxResponseBytes == 0 || sec.MaxDecompressedBytes == 0 {
			t.Errorf("size caps must stay set: %+v", sec)
		}
	})

	t.Run("allow_internal lifts only the private-IP block", func(t *testing.T) {
		sec := securityFromArgs(map[string]interface{}{"allow_internal": true})
		if sec.BlockPrivateIPs {
			t.Errorf("BlockPrivateIPs must be lifted")
		}
		if sec.MaxResponseBytes == 0 || sec.MaxRedirects == 0 {
			t.Errorf("other guardrails must remain: %+v", sec)
		}
	})

	t.Run("allow_internal false keeps the block", func(t *testing.T) {
		sec := securityFromArgs(map[string]interface{}{"allow_internal": false})
		if !sec.BlockPrivateIPs {
			t.Errorf("BlockPrivateIPs must stay true")
		}
	})

	t.Run("non-bool allow_internal is ignored", func(t *testing.T) {
		sec := securityFromArgs(map[string]interface{}{"allow_internal": "yes"})
		if !sec.BlockPrivateIPs {
			t.Errorf("BlockPrivateIPs must stay true for non-bool arg")
		}
	})
}

func TestArgInt(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
		want int
	}{
		{"present positive", map[string]interface{}{"n": 7.0}, 7},
		{"absent uses default", map[string]interface{}{}, 42},
		{"zero uses default", map[string]interface{}{"n": 0.0}, 42},
		{"negative uses default", map[string]interface{}{"n": -3.0}, 42},
		{"non-numeric uses default", map[string]interface{}{"n": "9"}, 42},
		{"fraction truncates", map[string]interface{}{"n": 7.9}, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := argInt(tt.args, "n", 42); got != tt.want {
				t.Errorf("argInt = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestArgString(t *testing.T) {
	args := map[string]interface{}{"s": "hello", "n": 3.0}
	if got := argString(args, "s"); got != "hello" {
		t.Errorf("got %q", got)
	}
	if got := argString(args, "missing"); got != "" {
		t.Errorf("missing key: got %q want empty", got)
	}
	if got := argString(args, "n"); got != "" {
		t.Errorf("non-string: got %q want empty", got)
	}
}

func TestMarshalResult(t *testing.T) {
	t.Run("marshals payload", func(t *testing.T) {
		got, err := marshalResult(map[string]int{"a": 1}, "thing")
		if err != nil {
			t.Fatalf("marshalResult: %v", err)
		}
		if got != `{"a":1}` {
			t.Errorf("got %q", got)
		}
	})

	t.Run("names the payload on failure", func(t *testing.T) {
		_, err := marshalResult(math.NaN(), "snapshot")
		if err == nil {
			t.Fatalf("expected marshal error for NaN")
		}
		if !strings.HasPrefix(err.Error(), "marshal snapshot:") {
			t.Errorf("error should name the payload: %v", err)
		}
	})
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ", []string{"a", "b"}},
		{"a,,b,", []string{"a", "b"}},
	}
	for _, tt := range tests {
		if got := splitCSV(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", tt.in, got, tt.want)
		}
	}
}
