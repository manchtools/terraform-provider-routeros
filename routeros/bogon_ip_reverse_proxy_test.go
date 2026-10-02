package routeros

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonIPReverseProxyLifecycle(t *testing.T) {
	res := Provider().ResourcesMap["routeros_ip_reverse_proxy"]
	if res == nil {
		t.Fatal("reverse proxy resource is not registered")
	}
	var row MikrotikItem
	failDelete := false
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/rest/ip/reverse-proxy" {
				t.Errorf("unexpected lookup: %s", r.URL)
			}
			rows := []MikrotikItem{}
			matches := row != nil
			for key, values := range r.URL.Query() {
				if row[key] != values[0] {
					matches = false
				}
			}
			if matches {
				rows = append(rows, row)
			}
			_ = json.NewEncoder(w).Encode(rows)
		case http.MethodPut, http.MethodPatch:
			want := "/rest/ip/reverse-proxy"
			if r.Method == http.MethodPatch {
				want += "/*P"
			}
			if r.URL.Path != want {
				t.Errorf("unexpected write target: %s", r.URL)
			}
			var body MikrotikItem
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if row == nil {
				row = MikrotikItem{".id": "*P", "dynamic": "false", "certificate": "none", "disabled": "false"}
			}
			for key, value := range body {
				switch key {
				case "sni", "ip-address", "port", "certificate", "disabled", "comment":
					if key == "disabled" {
						if value == "yes" {
							value = "true"
						} else if value == "no" {
							value = "false"
						}
					}
					if key == "comment" && value == "" {
						delete(row, key)
					} else {
						row[key] = value
					}
				default:
					t.Errorf("sent unsupported/native read-only property: %s", key)
				}
			}
			writes++
			_ = json.NewEncoder(w).Encode(row)
		case http.MethodDelete:
			if r.URL.Path != "/rest/ip/reverse-proxy/*P" {
				t.Errorf("unexpected delete target: %s", r.URL)
			}
			if failDelete {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request","detail":"delete refused"}`))
				return
			}
			row = nil
			writes++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
	config := map[string]interface{}{"sni": "nas.example.com", "ip_address": "192.0.2.10", "port": 8080, "certificate": "none", "disabled": true, "comment": "managed"}
	d := bogonResourceData(t, res, config)
	if diags := res.CreateContext(context.Background(), d, client); diags.HasError() {
		t.Fatal(diags)
	}
	if d.Id() != "*P" || d.Get("ip_address") != "192.0.2.10" {
		t.Fatal("create did not hydrate the native rule")
	}
	row["ip-address"] = "2001:db8::10"
	delete(row, "comment")
	if diags := res.ReadContext(context.Background(), d, client); diags.HasError() || d.Get("ip_address") != "2001:db8::10" || d.Get("comment") != "" {
		t.Fatalf("refresh retained stale native fields: %v address=%v comment=%v", diags, d.Get("ip_address"), d.Get("comment"))
	}
	config["ip_address"], config["port"], config["comment"] = "2001:db8::20", 8081, ""
	d = bogonResourceData(t, res, config)
	d.SetId("*P")
	if diags := res.UpdateContext(context.Background(), d, client); diags.HasError() || row["ip-address"] != "2001:db8::20" || row["port"] != "8081" {
		t.Fatalf("IPv6 update failed: %v row=%v", diags, row)
	}
	for _, selector := range []string{"*P", "sni=nas.example.com", "nas.example.com", "ip_address=2001:db8::20"} {
		imported := schema.TestResourceDataRaw(t, res.Schema, nil)
		imported.SetId(selector)
		if _, err := res.Importer.StateContext(context.Background(), imported, client); err != nil {
			t.Fatal(err)
		}
		if diags := res.ReadContext(context.Background(), imported, client); diags.HasError() || imported.Id() != "*P" || imported.Get("ip_address") != "2001:db8::20" || imported.Get("port") != 8081 {
			t.Fatalf("import lost rule identity or fields: %v id=%q", diags, imported.Id())
		}
	}
	row["dynamic"] = "true"
	before := writes
	if diags := res.ReadContext(context.Background(), d, client); !diags.HasError() || d.Id() == "" {
		t.Fatal("dynamic rule read discarded ownership state")
	}
	if diags := res.UpdateContext(context.Background(), d, client); !diags.HasError() {
		t.Fatal("dynamic rule update was accepted")
	}
	if diags := res.DeleteContext(context.Background(), d, client); !diags.HasError() || d.Id() == "" {
		t.Fatal("dynamic rule deletion lost ownership state")
	}
	if _, err := res.Importer.StateContext(context.Background(), d, client); err == nil {
		t.Fatal("dynamic container rule was imported")
	}
	if writes != before {
		t.Fatal("dynamic rule was modified")
	}
	row["dynamic"] = "false"
	failDelete = true
	if diags := res.DeleteContext(context.Background(), d, client); !diags.HasError() || d.Id() == "" {
		t.Fatal("failed delete discarded state")
	}
	failDelete = false
	if diags := res.DeleteContext(context.Background(), d, client); diags.HasError() || d.Id() != "" || row != nil {
		t.Fatalf("delete failed: %v id=%q", diags, d.Id())
	}
	d.SetId("*P")
	if diags := res.ReadContext(context.Background(), d, client); diags.HasError() || d.Id() != "" {
		t.Fatal("externally removed rule remains in state")
	}
}

func TestBogonIPReverseProxyValidation(t *testing.T) {
	res := Provider().ResourcesMap["routeros_ip_reverse_proxy"]
	if res == nil {
		t.Fatal("reverse proxy resource is not registered")
	}
	for _, tc := range []struct {
		field string
		value interface{}
		valid bool
	}{
		{"ip_address", "192.0.2.10", true},
		{"ip_address", "2001:db8::10", true},
		{"ip_address", "backend.example.com", false},
		{"port", 0, false},
		{"port", 65536, false},
		{"port", 8080, true},
		{"sni", "nas.example.com", true},
		{"sni", "*.example.com", false},
		{"sni", "", false},
	} {
		_, errs := res.Schema[tc.field].ValidateFunc(tc.value, tc.field)
		if (len(errs) == 0) != tc.valid {
			t.Errorf("%s=%v validation=%v, want valid=%v", tc.field, tc.value, errs, tc.valid)
		}
	}
	compare := res.Schema["ip_address"].DiffSuppressFunc
	if !compare("ip_address", "2001:db8::10", "2001:0db8:0:0:0:0:0:10", nil) || compare("ip_address", "2001:db8::10", "2001:db8::20", nil) {
		t.Fatal("IPv6 comparison suppresses real drift or misses equivalent spelling")
	}
}
