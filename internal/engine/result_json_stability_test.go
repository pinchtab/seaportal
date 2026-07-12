package engine

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// result_json_stability_test.go — locks the Result JSON wire format.
//
// The goldens pin BOTH the key set/order (encoding/json emits struct fields
// in declaration order, inlining anonymous embedded structs at the embedding
// position) and the omitempty behaviour (via the zero-value golden). Any
// restructuring of Result — such as the T13 embedded sub-struct decomposition
// — must keep both goldens byte-identical.
//
// Sentinel values are derived from Go field NAMES only (never from index or
// nesting path), so moving a field into an anonymous embedded struct does not
// change its sentinel — only a genuine wire-format change fails the test.
//
// Regenerate (only for an intentional wire-format change):
//
//	UPDATE_GOLDEN=1 go test ./internal/engine/ -run TestResultJSONWireStability

func TestResultJSONWireStability(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	cases := []struct {
		name   string
		golden string
		build  func() Result
	}{
		{
			// Every field populated with a distinct sentinel: locks the full
			// key ordering of the wire format.
			name:   "full",
			golden: "full.golden.json",
			build: func() Result {
				var r Result
				fillStructSentinels(reflect.ValueOf(&r).Elem(), "")
				return r
			},
		},
		{
			// Zero value: locks which keys survive omitempty when empty.
			name:   "zero",
			golden: "zero.golden.json",
			build:  func() Result { return Result{} },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.MarshalIndent(tc.build(), "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got = append(got, '\n')

			if tc.name == "full" {
				// Guard against the filler silently skipping fields.
				for _, key := range []string{`"ttfbMs"`, `"responseXAmzCfId"`, `"cacheCostAnalysis"`, `"uniqueBlockCount"`, `"traceCorrelation"`} {
					if !bytes.Contains(got, []byte(key)) {
						t.Fatalf("sentinel Result JSON is missing %s — filler regression?", key)
					}
				}
			}

			goldenPath := filepath.Join("..", "..", "testdata", "result_json", tc.golden)
			if update {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s (%d bytes)", goldenPath, len(got))
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden %q — regenerate with: UPDATE_GOLDEN=1 go test ./internal/engine/ -run TestResultJSONWireStability", goldenPath)
			}
			if !bytes.Equal(want, got) {
				t.Errorf("Result JSON wire format drifted from golden %s.\nThis breaks consumers — key order and omitempty are part of the contract.\nFirst diff:\n%s",
					tc.golden, firstDiffLines(string(want), string(got), 20))
			}
		})
	}
}

// fillStructSentinels sets every settable field of a struct to a non-zero
// sentinel derived from the field name. Anonymous embedded structs are
// traversed transparently (no prefix contribution), mirroring how both Go
// field promotion and encoding/json treat them.
func fillStructSentinels(v reflect.Value, prefix string) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			fillStructSentinels(fv, prefix)
			continue
		}
		setSentinel(fv, prefix+f.Name)
	}
}

// setSentinel writes a deterministic non-zero value derived from seed.
func setSentinel(v reflect.Value, seed string) {
	switch v.Kind() {
	case reflect.String:
		v.SetString(seed + "-v")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(seedNum(seed))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(seedNum(seed)))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(seedNum(seed)) + 0.5)
	case reflect.Slice:
		elem := reflect.New(v.Type().Elem()).Elem()
		setSentinel(elem, seed+"0")
		v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), elem))
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		setSentinel(key, seed+"K")
		val := reflect.New(v.Type().Elem()).Elem()
		setSentinel(val, seed+"V")
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.Struct:
		fillStructSentinels(v, seed+".")
	case reflect.Interface:
		v.Set(reflect.ValueOf(seed + "-iv"))
	case reflect.Ptr:
		p := reflect.New(v.Type().Elem())
		setSentinel(p.Elem(), seed)
		v.Set(p)
	}
}

// seedNum maps a seed string to a stable positive number (1000..9999).
func seedNum(seed string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return int64(h.Sum32()%9000) + 1000
}
