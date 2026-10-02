package routeros

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonOpenVPNModernLifecycle(t *testing.T) {
	for _, version := range []string{"7.17", "7.24.5"} {
		t.Run(version, func(t *testing.T) { bogonOpenVPNModernLifecycle(t, version) })
	}
}

func bogonOpenVPNModernLifecycle(t *testing.T, version string) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = version
	res := ResourceOpenVPNServer()
	var row MikrotikItem
	failWrite, failRead := false, false
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/rest/interface/ovpn-server/server" {
				t.Errorf("unexpected read %s", r.URL)
			}
			if failRead {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request"}`))
				return
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
			want := "/rest/interface/ovpn-server/server"
			if r.Method == http.MethodPatch {
				want += "/*A"
			}
			if r.URL.Path != want {
				t.Errorf("unexpected write %s %s", r.Method, r.URL)
			}
			if failWrite {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request"}`))
				return
			}
			var body MikrotikItem
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if _, present := body["enabled"]; present {
				t.Error("modern server received removed enabled property")
			}
			if row == nil {
				row = MikrotikItem{".id": "*A", "name": "ovpn-server1", "inactive": "false", "disabled": "true", "vrf": "main"}
			}
			for key, value := range body {
				if key == "disabled" {
					if value == "yes" {
						value = "true"
					} else if value == "no" {
						value = "false"
					}
				}
				row[key] = value
			}
			writes++
			_ = json.NewEncoder(w).Encode(row)
		case http.MethodDelete:
			if r.URL.Path != "/rest/interface/ovpn-server/server/*A" {
				t.Errorf("unexpected delete %s", r.URL)
			}
			if failWrite {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request"}`))
				return
			}
			row = nil
			writes++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request"}`))
		}
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client(), extra: &ExtraParams{}}
	defaultServer := bogonResourceData(t, res, nil)
	if diags := res.CreateContext(context.Background(), defaultServer, client); diags.HasError() || row["disabled"] != "true" {
		t.Fatalf("unconfigured server was not disabled by default: %v row=%v", diags, row)
	}
	if diags := res.DeleteContext(context.Background(), defaultServer, client); diags.HasError() {
		t.Fatal(diags)
	}
	d := bogonResourceData(t, res, map[string]interface{}{"enabled": true})
	if diags := res.CreateContext(context.Background(), d, client); diags.HasError() || d.Id() != "*A" {
		t.Fatalf("modern server create failed: %v id=%q", diags, d.Id())
	}
	if row["disabled"] != "false" || d.Get("enabled") != true || d.Get("disabled") != false {
		t.Fatalf("enabled compatibility conversion failed: %v", row)
	}
	for _, selector := range []string{"*A", "ovpn-server1", "name=ovpn-server1", ".id=*A"} {
		imported := schema.TestResourceDataRaw(t, res.Schema, nil)
		imported.SetId(selector)
		if _, err := res.Importer.StateContext(context.Background(), imported, client); err != nil || imported.Id() != "*A" || imported.Get("name") != "ovpn-server1" {
			t.Fatalf("modern import %q failed: %v id=%q", selector, err, imported.Id())
		}
	}
	row["disabled"] = "true"
	if diags := res.ReadContext(context.Background(), d, client); diags.HasError() || d.Get("enabled") != false || d.Get("disabled") != true {
		t.Fatalf("refresh did not detect disabled drift: %v", diags)
	}
	d = bogonResourceData(t, res, map[string]interface{}{"disabled": false, "name": "managed-ovpn", "vrf": "main"})
	d.SetId("*A")
	if diags := res.UpdateContext(context.Background(), d, client); diags.HasError() || row["name"] != "managed-ovpn" || row["disabled"] != "false" {
		t.Fatalf("modern update failed: %v row=%v", diags, row)
	}
	failWrite = true
	if diags := res.UpdateContext(context.Background(), d, client); !diags.HasError() || d.Id() != "*A" {
		t.Fatal("failed update discarded owned identity")
	}
	if diags := res.DeleteContext(context.Background(), d, client); !diags.HasError() || d.Id() != "*A" {
		t.Fatal("failed delete discarded owned identity")
	}
	failedCreate := bogonResourceData(t, res, map[string]interface{}{"disabled": true})
	if diags := res.CreateContext(context.Background(), failedCreate, client); !diags.HasError() || failedCreate.Id() != "" {
		t.Fatal("failed create claimed ownership")
	}
	failWrite, failRead = false, true
	if diags := res.ReadContext(context.Background(), d, client); !diags.HasError() || d.Id() != "*A" {
		t.Fatal("failed read discarded owned identity")
	}
	failRead = false
	before := writes
	for _, operation := range []func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics{res.ReadContext, res.UpdateContext, res.DeleteContext} {
		d.SetId("interface.ovpn-server.server")
		if diags := operation(context.Background(), d, client); !diags.HasError() || d.Id() != "interface.ovpn-server.server" {
			t.Fatal("legacy ID silently selected or forgot a modern server")
		}
	}
	if writes != before {
		t.Fatal("legacy ID changed a modern server")
	}
	d.SetId("*A")
	if diags := res.DeleteContext(context.Background(), d, client); diags.HasError() || d.Id() != "" || row != nil {
		t.Fatalf("modern destroy failed: %v", diags)
	}
	d.SetId("*A")
	if diags := res.ReadContext(context.Background(), d, client); diags.HasError() || d.Id() != "" {
		t.Fatalf("missing modern server retained state: %v", diags)
	}
	missing := schema.TestResourceDataRaw(t, res.Schema, nil)
	missing.SetId("absent")
	if _, err := res.Importer.StateContext(context.Background(), missing, client); err == nil {
		t.Fatal("missing server imported")
	}
}

func TestBogonOpenVPNSuccessfulCreateRetainsIDOnMalformedReadback(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.5"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/rest/interface/ovpn-server/server" {
			t.Errorf("unexpected operation %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{".id":"*B","name":"owned-server","port":"invalid","disabled":"true"}`))
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
	res := ResourceOpenVPNServer()
	d := bogonResourceData(t, res, map[string]interface{}{"disabled": true})
	if diags := res.CreateContext(context.Background(), d, client); !diags.HasError() || d.Id() != "*B" {
		t.Fatalf("readback failure lost successful creation: %v id=%q", diags, d.Id())
	}
}

func TestBogonOpenVPNImportRejectsAmbiguousServers(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.5"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Query().Get("name") != "duplicate" {
			t.Errorf("unexpected import lookup %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{".id":"*A","name":"duplicate"},{".id":"*B","name":"duplicate"}]`))
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
	res := ResourceOpenVPNServer()
	d := schema.TestResourceDataRaw(t, res.Schema, nil)
	d.SetId("duplicate")
	if _, err := res.Importer.StateContext(context.Background(), d, client); err == nil || d.Id() != "duplicate" {
		t.Fatal("ambiguous import claimed a server")
	}
	for _, selector := range []string{"", ".", legacyOpenVPNServerID, "name=", "port=1194"} {
		d.SetId(selector)
		if _, err := res.Importer.StateContext(context.Background(), d, client); err == nil {
			t.Fatalf("invalid selector accepted: %q", selector)
		}
	}
	if requests != 1 {
		t.Fatal("invalid import selectors reached the router")
	}
}

func TestBogonOpenVPNLegacyLifecycle(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.16.2"
	res := ResourceOpenVPNServer()
	row := MikrotikItem{"enabled": "false"}
	writes := 0
	failRead := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/rest/interface/ovpn-server/server" {
			if failRead {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(row)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/rest/interface/ovpn-server/server/set" {
			t.Errorf("unexpected legacy operation %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body MikrotikItem
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, unsupported := range []string{"disabled", "name", "vrf", "inactive"} {
			if _, present := body[unsupported]; present {
				t.Errorf("legacy server received %s", unsupported)
			}
		}
		if body["enabled"] == "yes" {
			row["enabled"] = "true"
		} else {
			row["enabled"] = "false"
		}
		writes++
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client(), extra: &ExtraParams{}}
	d := bogonResourceData(t, res, map[string]interface{}{"disabled": false})
	if diags := res.CreateContext(context.Background(), d, client); diags.HasError() || d.Id() != "interface.ovpn-server.server" || d.Get("enabled") != true || d.Get("disabled") != false {
		t.Fatalf("legacy enable conversion failed: %v id=%q", diags, d.Id())
	}
	before := writes
	if diags := res.DeleteContext(context.Background(), d, client); diags.HasError() || d.Id() != "" || writes != before || row["enabled"] != "true" {
		t.Fatal("legacy destroy changed singleton")
	}
	imported := schema.TestResourceDataRaw(t, res.Schema, nil)
	imported.SetId(".")
	if _, err := res.Importer.StateContext(context.Background(), imported, client); err != nil || imported.Id() != "interface.ovpn-server.server" {
		t.Fatalf("legacy singleton import failed: %v", err)
	}
	invalid := bogonResourceData(t, res, map[string]interface{}{"name": "new-server"})
	if diags := res.CreateContext(context.Background(), invalid, client); !diags.HasError() || writes != before {
		t.Fatal("legacy firmware silently ignored modern server selection")
	}
	failRead = true
	created := bogonResourceData(t, res, map[string]interface{}{"enabled": false})
	if diags := res.CreateContext(context.Background(), created, client); !diags.HasError() || created.Id() != "interface.ovpn-server.server" {
		t.Fatal("successful singleton write followed by failed read lost ownership")
	}
}

func TestBogonOpenVPNUnknownVersionMakesNoRequests(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = ""
	res := ResourceOpenVPNServer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unknown firmware issued a native request")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client(), extra: &ExtraParams{}}
	d := bogonResourceData(t, res, map[string]interface{}{"enabled": true})
	if diags := res.CreateContext(context.Background(), d, client); !diags.HasError() || d.Id() != "" {
		t.Fatal("unknown firmware did not reject create")
	}
}
