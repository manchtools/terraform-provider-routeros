package routeros

import (
	"context"
	"encoding/json"
	"net/http"

	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func wifiLifecycleData(t *testing.T, res *schema.Resource, config map[string]interface{}, prior map[string]interface{}) *schema.ResourceData {
	t.Helper()
	if RouterOSVersion == "" {
		RouterOSVersion = "7.21.3"
		t.Cleanup(func() { RouterOSVersion = "" })
	}
	attrs := map[string]cty.Value{}
	for name, ty := range res.CoreConfigSchema().ImpliedType().AttributeTypes() {
		attrs[name] = cty.NullVal(ty)
	}
	for name, value := range config {
		switch v := value.(type) {
		case string:
			attrs[name] = cty.StringVal(v)
		case int:
			attrs[name] = cty.NumberIntVal(int64(v))
		case bool:
			attrs[name] = cty.BoolVal(v)
		default:
			t.Fatalf("unsupported %T", value)
		}
	}
	state := &terraform.InstanceState{RawConfig: cty.ObjectVal(attrs)}
	if prior != nil {
		previous := wifiLifecycleData(t, res, prior, nil)
		previous.SetId("*1")
		state = previous.State()
		state.RawConfig = cty.ObjectVal(attrs)
	}
	var d *schema.ResourceData
	if prior == nil {
		d = res.Data(state)
		for name, value := range config {
			if err := d.Set(name, value); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		cfg := terraform.NewResourceConfigRaw(config)
		cfg.CtyValue = cty.ObjectVal(attrs)
		diff, err := res.SimpleDiff(context.Background(), state, cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		diff.RawConfig = cfg.CtyValue
		d, err = schema.InternalMap(res.Schema).Data(state, diff)
		if err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestBogonWifiDatapathRemovalIsNotSuppressed(t *testing.T) {
	res := ResourceWifiDatapath()
	for _, field := range []string{"bridge", "traffic_processing", "vlan_id"} {
		if res.Schema[field].DiffSuppressFunc != nil {
			t.Errorf("%s removal is suppressed", field)
		}
	}
}

// The REST fixture rejects empty values in PATCH: a profile must use the
// RouterOS unset command to restore inheritance instead of storing a zero.
func TestBogonWifiDatapathUnsetREST(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			res := ResourceWifiDatapath()
			row := MikrotikItem{".id": "*1", "name": "profile", "bridge": "lan", "traffic-processing": "on-capsman", "vlan-id": "20"}
			unset := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet:
					_ = json.NewEncoder(w).Encode([]MikrotikItem{row})
				case r.Method == http.MethodPost && r.URL.Path == "/rest/interface/wifi/datapath/unset":
					var body MikrotikItem
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["numbers"] != "*1" {
						t.Errorf("unset targets %v", body)
					}
					field := body["value-name"]
					if field != "bridge" && field != "traffic-processing" && field != "vlan-id" {
						t.Errorf("unexpected unset %v", body)
					}
					unset[field]++
					if fail && field == "traffic-processing" {
						w.WriteHeader(400)
						_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request","detail":"unset failed"}`))
						return
					}
					delete(row, field)
					_, _ = w.Write([]byte(`[]`))
				case r.Method == http.MethodPatch && r.URL.Path == "/rest/interface/wifi/datapath/*1":
					var body MikrotikItem
					_ = json.NewDecoder(r.Body).Decode(&body)
					for _, field := range []string{"bridge", "traffic-processing", "vlan-id"} {
						if _, ok := body[field]; ok {
							t.Errorf("cleared %s sent as set: %v", field, body)
						}
						if _, ok := body["!"+field]; ok {
							t.Errorf("unset marker sent as PATCH: %v", body)
						}
					}
					for field, value := range body {
						row[field] = value
					}
					_ = json.NewEncoder(w).Encode(row)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
				}
			}))
			defer server.Close()
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, extra: &ExtraParams{}, Client: server.Client()}
			prior := map[string]interface{}{"name": "profile", "bridge": "lan", "traffic_processing": "on-capsman", "vlan_id": 20}
			d := wifiLifecycleData(t, res, map[string]interface{}{"name": "profile"}, prior)
			d.SetId("*1")
			diags := res.UpdateContext(context.Background(), d, client)
			if fail {
				if !diags.HasError() || d.Id() != "*1" {
					t.Fatalf("failed unset lost ownership: %v id=%q", diags, d.Id())
				}
				// The bridge unset succeeded before the mode failed. Retry must
				// skip the bridge and finish the remaining properties.
				fail = false
				retry := wifiLifecycleData(t, res, map[string]interface{}{"name": "profile"}, prior)
				if diags := res.UpdateContext(context.Background(), retry, client); diags.HasError() {
					t.Fatal(diags)
				}
				if unset["bridge"] != 1 || unset["traffic-processing"] != 2 || unset["vlan-id"] != 1 {
					t.Fatalf("partial retry duplicated or skipped unset: %v", unset)
				}
				return
			}
			if diags.HasError() {
				t.Fatal(diags)
			}
			for _, field := range []string{"bridge", "traffic-processing", "vlan-id"} {
				if unset[field] != 1 {
					t.Errorf("missing unset %s: %v", field, unset)
				}
			}
			if d.Get("vlan_id") != 0 || d.Get("bridge") != "" || d.Get("traffic_processing") != "" {
				t.Fatalf("stale profile state %v", d.State())
			}
			// Retry after successful remote unset but before Terraform saves state.
			if diags := res.UpdateContext(context.Background(), wifiLifecycleData(t, res, map[string]interface{}{"name": "profile"}, prior), client); diags.HasError() {
				t.Fatal(diags)
			}
		})
	}
}

func TestBogonWifiCapsmanDeleteREST(t *testing.T) {
	for _, mode := range []string{"success", "write-error", "still-enabled", "read-error"} {
		t.Run(mode, func(t *testing.T) {
			res := ResourceWifiCapsman()
			row := MikrotikItem{"enabled": "yes", "generated-certificate": "keep-me", "generated-ca-certificate": "keep-ca"}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "certificate") || strings.Contains(r.URL.Path, "radio") || r.Method == http.MethodDelete {
					t.Errorf("delete touched unmanaged asset %s %s", r.Method, r.URL.Path)
				}
				if r.URL.Path == "/rest/interface/wifi/capsman/set" && r.Method == http.MethodPost {
					writes++
					var body MikrotikItem
					_ = json.NewDecoder(r.Body).Decode(&body)
					if len(body) != 1 || body["enabled"] != "false" {
						t.Errorf("delete payload %v", body)
					}
					if mode == "write-error" {
						w.WriteHeader(400)
						_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request","detail":"failed"}`))
						return
					}
					if mode != "still-enabled" {
						row["enabled"] = "no"
					}
					_, _ = w.Write([]byte(`[]`))
					return
				}
				if r.URL.Path == "/rest/interface/wifi/capsman" && r.Method == http.MethodGet {
					if mode == "read-error" {
						w.WriteHeader(400)
						_, _ = w.Write([]byte(`{"error":400,"message":"Bad Request","detail":"failed"}`))
						return
					}
					_ = json.NewEncoder(w).Encode(row)
					return
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(400)
			}))
			defer server.Close()
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, extra: &ExtraParams{}, Client: server.Client()}
			d := wifiLifecycleData(t, res, map[string]interface{}{"enabled": true}, nil)
			d.SetId("interface.wifi.capsman")
			diags := res.DeleteContext(context.Background(), d, client)
			if mode != "success" {
				if !diags.HasError() || d.Id() == "" {
					t.Fatalf("delete failure forgot singleton: %v id=%q", diags, d.Id())
				}
				return
			}
			if diags.HasError() || d.Id() != "" || writes != 1 || row["enabled"] != "no" {
				t.Fatalf("CAPsMAN remained enabled: %v id=%q writes=%d row=%v", diags, d.Id(), writes, row)
			}
			if row["generated-certificate"] != "keep-me" {
				t.Fatal("certificate removed")
			}
			d.SetId("interface.wifi.capsman")
			if diags := res.DeleteContext(context.Background(), d, client); diags.HasError() {
				t.Fatal(diags)
			}
		})
	}
}

func TestBogonIPv6MangleJumpTargetRoundTrip(t *testing.T) {
	res := ResourceIPv6FirewallMangle()
	if res.Schema["jump_target"] == nil {
		t.Fatal("IPv6 jump action has no jump_target schema")
	}
	d := wifiLifecycleData(t, res, map[string]interface{}{"action": "jump", "chain": "prerouting", "jump_target": "custom"}, nil)
	payload, _ := TerraformResourceDataToMikrotik(res.Schema, d)
	if payload["jump-target"] != "custom" {
		t.Fatalf("missing jump target %v", payload)
	}
	if diags := MikrotikResourceDataToTerraform(MikrotikItem{"jump-target": "custom"}, res.Schema, d); diags.HasError() {
		t.Fatal(diags)
	}
	cleared := wifiLifecycleData(t, res, map[string]interface{}{"action": "accept", "chain": "prerouting"}, map[string]interface{}{"action": "jump", "chain": "prerouting", "jump_target": "custom"})
	payload, _ = TerraformResourceDataToMikrotik(res.Schema, cleared)
	if _, ok := payload["!jump-target"]; !ok {
		t.Fatalf("removed jump target was not cleared: %v", payload)
	}
}

func TestBogonWifiDatapathDirectValuesAndRefresh(t *testing.T) {
	res := ResourceWifiDatapath()
	row := MikrotikItem{".id": "*1", "name": "direct"}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]MikrotikItem{row})
		case http.MethodPatch:
			writes++
			var body MikrotikItem
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["bridge"] != "none" || body["traffic-processing"] != "on-cap" || body["vlan-id"] != "0" {
				t.Errorf("direct override omitted: %v", body)
			}
			for field, value := range body {
				row[field] = value
			}
			_ = json.NewEncoder(w).Encode(row)
		default:
			t.Errorf("explicit overrides triggered unset: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, extra: &ExtraParams{}, Client: server.Client()}
	config := map[string]interface{}{"name": "direct", "bridge": "none", "traffic_processing": "on-cap", "vlan_id": 0}
	d := wifiLifecycleData(t, res, config, nil)
	d.SetId("*1")
	if diags := res.UpdateContext(context.Background(), d, client); diags.HasError() {
		t.Fatal(diags)
	}
	if writes != 1 || d.Get("bridge") != "none" || d.Get("traffic_processing") != "on-cap" {
		t.Fatal("direct override did not survive update/read")
	}
	delete(row, "bridge")
	delete(row, "traffic-processing")
	delete(row, "vlan-id")
	if diags := res.ReadContext(context.Background(), d, client); diags.HasError() {
		t.Fatal(diags)
	}
	if d.Get("bridge") != "" || d.Get("traffic_processing") != "" || d.Get("vlan_id") != 0 {
		t.Fatal("refresh retained remote properties that were removed")
	}
}
