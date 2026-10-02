package routeros

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonIPsecKeyVersionedLifecycle(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	res := ResourceIpIpsecKey()
	for _, tc := range []struct{ version, path string }{
		{"7.19.4", "/ip/ipsec/key"},
		{"7.20", "/ip/ipsec/key/rsa"},
		{"7.20.1", "/ip/ipsec/key/rsa"},
		{"7.24.2", "/ip/ipsec/key/rsa"},
		{"7.19.4", "/ip/ipsec/key"}, // Reusing a resource cannot retain another version's path.
	} {
		t.Run(tc.version, func(t *testing.T) {
			RouterOSVersion = tc.version
			var row MikrotikItem
			failWrites, failReads := false, false
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				requests++
				want := "/rest" + tc.path
				switch r.Method {
				case http.MethodPost:
					want += "/generate-key"
				case http.MethodPatch, http.MethodDelete:
					want += "/*K"
				}
				if r.URL.Path != want || failWrites && r.Method != http.MethodGet || failReads && r.Method == http.MethodGet {
					if r.URL.Path != want {
						t.Errorf("RouterOS %s: %s targeted %s, want %s", tc.version, r.Method, r.URL.Path, want)
					}
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request","detail":"request refused"}`))
					return
				}
				switch r.Method {
				case http.MethodPost, http.MethodPatch:
					var body MikrotikItem
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if row == nil {
						row = MikrotikItem{".id": "*K", "private-key": "true", "rsa": "true"}
					}
					for key, value := range body {
						if key != "name" && key != "key-size" {
							t.Errorf("wrote unsupported RSA property %q", key)
						}
						row[key] = value
					}
					_ = json.NewEncoder(w).Encode(row)
				case http.MethodGet:
					rows := []MikrotikItem{}
					matches := row != nil
					for field, values := range r.URL.Query() {
						if row[field] != values[0] {
							matches = false
						}
					}
					if matches {
						rows = append(rows, row)
					}
					_ = json.NewEncoder(w).Encode(rows)
				case http.MethodDelete:
					row = nil
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected method %s", r.Method)
				}
			}))
			defer server.Close()
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
			data := bogonResourceData(t, res, map[string]interface{}{"name": "test-key", "key_size": 2048})
			if diags := res.CreateContext(context.Background(), data, client); diags.HasError() || data.Id() != "*K" || data.Get("key_size") != 2048 {
				t.Fatalf("generate/lookup failed: %v, id=%q", diags, data.Id())
			}
			for _, selector := range []string{"*K", "test-key", "name=test-key"} {
				imported := schema.TestResourceDataRaw(t, res.Schema, nil)
				imported.SetId(selector)
				if _, err := res.Importer.StateContext(context.Background(), imported, client); err != nil {
					t.Fatal(err)
				}
				if diags := res.ReadContext(context.Background(), imported, client); diags.HasError() || imported.Id() != "*K" || imported.Get("name") != "test-key" {
					t.Fatalf("import/refresh failed: %v", diags)
				}
			}
			if err := data.Set("name", "renamed-key"); err != nil {
				t.Fatal(err)
			}
			if diags := res.UpdateContext(context.Background(), data, client); diags.HasError() || row["name"] != "renamed-key" {
				t.Fatalf("update failed: %v", diags)
			}
			failWrites = true
			if diags := res.UpdateContext(context.Background(), data, client); !diags.HasError() || data.Id() != "*K" {
				t.Fatal("failed update lost state")
			}
			if diags := res.DeleteContext(context.Background(), data, client); !diags.HasError() || data.Id() != "*K" {
				t.Fatal("failed delete lost state")
			}
			fresh := bogonResourceData(t, res, map[string]interface{}{"name": "failed-key", "key_size": 2048})
			if diags := res.CreateContext(context.Background(), fresh, client); !diags.HasError() || fresh.Id() != "" {
				t.Fatal("failed generation claimed a key")
			}
			failWrites, failReads = false, true
			if diags := res.ReadContext(context.Background(), data, client); !diags.HasError() || data.Id() != "*K" {
				t.Fatal("failed refresh lost state")
			}
			failReads = false
			if diags := res.DeleteContext(context.Background(), data, client); diags.HasError() || data.Id() != "" || row != nil {
				t.Fatalf("delete failed: %v", diags)
			}
			data.SetId("*K")
			if diags := res.ReadContext(context.Background(), data, client); diags.HasError() || data.Id() != "" {
				t.Fatal("externally missing key remained in state")
			}
			if requests == 0 || res.Schema[MetaResourcePath].Default != "/ip/ipsec/key" {
				t.Fatal("runtime path selection mutated the provider schema")
			}
		})
	}
}

func TestBogonIPsecKeyRejectsUnknownVersion(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	for _, version := range []string{"", "7.bad", "7.20rc1", "0"} {
		t.Run(version, func(t *testing.T) {
			RouterOSVersion = version
			res := ResourceIpIpsecKey()
			data := bogonResourceData(t, res, map[string]interface{}{"name": "test-key", "key_size": 2048})
			data.SetId("*K")
			for _, operation := range []schema.CreateContextFunc{res.CreateContext, schema.CreateContextFunc(res.ReadContext), schema.CreateContextFunc(res.UpdateContext), schema.CreateContextFunc(res.DeleteContext)} {
				if diags := operation(context.Background(), data, nil); !diags.HasError() || data.Id() != "*K" {
					t.Fatal("unknown firmware did not fail safely before accessing the client")
				}
			}
			if _, err := res.Importer.StateContext(context.Background(), data, nil); err == nil || data.Id() != "*K" {
				t.Fatal("unknown firmware import did not fail safely")
			}
		})
	}
}
