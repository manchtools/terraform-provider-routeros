package routeros

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonIPServiceImport(t *testing.T) {
	for _, selector := range []string{"ssh", "name=ssh", "*4", ".id=*4", "port=22", "max_sessions=20"} {
		t.Run(selector, func(t *testing.T) {
			static := MikrotikItem{".id": "*4", "name": "ssh", "port": "22", "disabled": "false", "dynamic": "false", "address": "", "vrf": "main", "max-sessions": "20"}
			connection := MikrotikItem{".id": "*C", "name": "ssh", "port": "22", "dynamic": "true"}
			server := ipServiceTestServer(t, []*MikrotikItem{&static, &connection})
			defer server.Close()
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
			res := ResourceIpService()
			d := schema.TestResourceDataRaw(t, res.Schema, nil)
			d.SetId(selector)
			imported, err := res.Importer.StateContext(context.Background(), d, client)
			if err != nil {
				t.Fatal(err)
			}
			if len(imported) != 1 || imported[0] != d {
				t.Fatal("import did not return the selected service")
			}
			if diags := res.ReadContext(context.Background(), d, client); diags.HasError() {
				t.Fatal(diags)
			}
			if d.Id() != "ssh" || d.Get("numbers") != "ssh" || d.Get("name") != "ssh" || d.Get("port") != 22 {
				t.Fatalf("service import lost its identity/settings: id=%q numbers=%v name=%v port=%v", d.Id(), d.Get("numbers"), d.Get("name"), d.Get("port"))
			}
			static["port"] = "2222"
			if diags := res.ReadContext(context.Background(), d, client); diags.HasError() || d.Get("port") != 2222 {
				t.Fatalf("refresh failed to observe service drift: %v port=%v", diags, d.Get("port"))
			}
		})
	}
}

func TestBogonIPServiceImportRejectsMissingDynamicAndAmbiguous(t *testing.T) {
	for _, tc := range []struct {
		name, selector string
		rows           []MikrotikItem
	}{
		{"missing", "ssh", nil},
		{"dynamic ID", "*C", []MikrotikItem{{".id": "*C", "name": "ssh", "dynamic": "true"}}},
		{"ambiguous", "port=22", []MikrotikItem{{".id": "*4", "name": "ssh", "port": "22", "dynamic": "false"}, {".id": "*5", "name": "telnet", "port": "22", "dynamic": "false"}}},
		{"empty", "", nil},
		{"empty selector", "name=", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := make([]*MikrotikItem, len(tc.rows))
			for i := range tc.rows {
				rows[i] = &tc.rows[i]
			}
			server := ipServiceTestServer(t, rows)
			defer server.Close()
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
			res := ResourceIpService()
			d := schema.TestResourceDataRaw(t, res.Schema, nil)
			d.SetId(tc.selector)
			if _, err := res.Importer.StateContext(context.Background(), d, client); err == nil {
				t.Fatal("invalid service import was accepted")
			}
		})
	}
}

func TestBogonIPServiceReadExistingState(t *testing.T) {
	static := MikrotikItem{".id": "*8", "name": "www", "port": "80", "dynamic": "false", "disabled": "false"}
	connection := MikrotikItem{".id": "*C", "name": "www", "port": "80", "dynamic": "true", "disabled": "true"}
	// The dynamic connection precedes the service in the native table.
	server := ipServiceTestServer(t, []*MikrotikItem{&connection, &static})
	defer server.Close()
	res := ResourceIpService()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{"numbers": "www", "port": 80})
	d.SetId("www")
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
	if diags := res.ReadContext(context.Background(), d, client); diags.HasError() || d.Get("dynamic") != false || d.Get("disabled") != false || d.Id() != "www" {
		t.Fatalf("existing service state adopted a connection: %v dynamic=%v disabled=%v id=%q", diags, d.Get("dynamic"), d.Get("disabled"), d.Id())
	}
}

func TestBogonIPServiceReverseProxyValidation(t *testing.T) {
	property := ResourceIpService().Schema["numbers"]
	if property.ValidateDiagFunc != nil && property.ValidateDiagFunc("reverse-proxy", nil).HasError() {
		t.Fatal("reverse-proxy service was rejected")
	}
	if property.ValidateFunc != nil {
		if _, errs := property.ValidateFunc("reverse-proxy", "numbers"); len(errs) != 0 {
			t.Fatal(errs)
		}
	}
	if _, errs := property.ValidateFunc("ssh,winbox", "numbers"); len(errs) == 0 {
		t.Fatal("one service resource accepted multiple identities")
	}
}

func TestBogonIPServiceWriteTargetsStaticRecord(t *testing.T) {
	row := MikrotikItem{".id": "*4", "name": "ssh", "port": "22", "disabled": "false", "dynamic": "false"}
	connection := MikrotikItem{".id": "*C", "name": "ssh", "port": "22", "dynamic": "true"}
	failWrite := false
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/rest/ip/service" {
			_ = json.NewEncoder(w).Encode([]MikrotikItem{connection, row})
			return
		}
		if r.Method != http.MethodPatch || r.URL.Path != "/rest/ip/service/*4" {
			t.Errorf("service write did not target its static ID: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if failWrite {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request","detail":"write refused"}`))
			return
		}
		var body MikrotikItem
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for key, value := range body {
			switch key {
			case "port", "disabled", "address", "vrf":
				if key == "disabled" {
					if value == "yes" {
						value = "true"
					} else if value == "no" {
						value = "false"
					}
				}
				row[key] = value
			default:
				t.Errorf("wrote selector or read-only service field: %q", key)
			}
		}
		writes++
		_ = json.NewEncoder(w).Encode(row)
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client(), extra: &ExtraParams{SuppressSysODelWarn: true}}
	res := ResourceIpService()
	d := bogonResourceData(t, res, map[string]interface{}{"numbers": "ssh", "port": 2222, "disabled": true})
	if diags := res.CreateContext(context.Background(), d, client); diags.HasError() || d.Id() != "ssh" || row["port"] != "2222" || row["disabled"] != "true" || connection["port"] != "22" {
		t.Fatalf("static service update failed or altered a connection: %v row=%v", diags, row)
	}
	failWrite = true
	if diags := res.UpdateContext(context.Background(), d, client); !diags.HasError() || d.Id() != "ssh" {
		t.Fatal("failed update lost the service's stable identity")
	}
	fresh := bogonResourceData(t, res, map[string]interface{}{"numbers": "ssh", "port": 2222})
	if diags := res.CreateContext(context.Background(), fresh, client); !diags.HasError() || fresh.Id() != "" {
		t.Fatal("failed create claimed an unconfigured service")
	}
	before := writes
	if diags := res.DeleteContext(context.Background(), d, client); diags.HasError() || d.Id() != "" || writes != before || row["name"] != "ssh" {
		t.Fatal("destroy modified or removed a built-in service")
	}
}

func ipServiceTestServer(t *testing.T, rows []*MikrotikItem) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/rest/ip/service" {
			t.Errorf("unexpected service request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		result := []MikrotikItem{}
		for _, row := range rows {
			matches := true
			for key, values := range r.URL.Query() {
				if (*row)[key] != values[0] {
					matches = false
				}
			}
			if matches {
				result = append(result, *row)
			}
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
}
