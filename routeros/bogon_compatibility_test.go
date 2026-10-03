package routeros

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBogonBGPAddPathDefaultNotSent(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	for _, version := range []string{"7.21.3", "7.22", "7.24.2"} {
		t.Run(version, func(t *testing.T) {
			RouterOSVersion = version
			res := ResourceRoutingBgpConnection()
			data := bogonResourceData(t, res, map[string]interface{}{"name": "probe", "as": "64512", "address_families": "ip"})
			payload, _ := TerraformResourceDataToMikrotik(res.Schema, data)
			if _, ok := payload["add-path-out"]; ok {
				t.Fatal("omitted legacy attribute received an unsupported default")
			}
		})
	}
}

func TestBogonBGPAddPathLegacyStateNeverWritten(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	res := ResourceRoutingBgpConnection()
	data := bogonResourceData(t, res, map[string]interface{}{"name": "probe", "as": "64512"})
	RouterOSVersion = "7.21.3"
	if diags := MikrotikResourceDataToTerraform(MikrotikItem{"add-path-out": "none"}, res.Schema, data); diags.HasError() {
		t.Fatal(diags)
	}
	if got := data.Get("add_path_out"); got != "none" {
		t.Fatalf("legacy native state was not observed: %v", got)
	}
	for _, version := range []string{"7.21.3", "7.24.2"} {
		RouterOSVersion = version
		payload, _ := TerraformResourceDataToMikrotik(res.Schema, data)
		if _, ok := payload["add-path-out"]; ok {
			t.Fatalf("observed legacy state written on RouterOS %s", version)
		}
	}
}

func TestBogonAddressVRFIsReadOnly(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.2"
	res := ResourceIPAddress()
	data := bogonResourceData(t, res, map[string]interface{}{"address": "192.0.2.1/24", "interface": "ether2"})
	if diags := MikrotikResourceDataToTerraform(MikrotikItem{"vrf": "main"}, res.Schema, data); diags.HasError() {
		t.Fatal(diags)
	}
	if got := data.Get("vrf"); got != "main" {
		t.Fatalf("VRF was not read: %v", got)
	}
	payload, _ := TerraformResourceDataToMikrotik(res.Schema, data)
	if _, ok := payload["vrf"]; ok {
		t.Fatal("read-only VRF was serialized for address update")
	}
}

func bogonResourceData(t *testing.T, res *schema.Resource, config map[string]interface{}) *schema.ResourceData {
	t.Helper()
	attrs := map[string]cty.Value{}
	for name, ty := range res.CoreConfigSchema().ImpliedType().AttributeTypes() {
		attrs[name] = cty.NullVal(ty)
	}
	for name, value := range config {
		switch value := value.(type) {
		case string:
			attrs[name] = cty.StringVal(value)
		case bool:
			attrs[name] = cty.BoolVal(value)
		case []string:
			items := make([]cty.Value, len(value))
			for i, item := range value {
				items[i] = cty.StringVal(item)
			}
			if len(items) == 0 {
				attrs[name] = cty.ListValEmpty(cty.String)
			} else {
				attrs[name] = cty.ListVal(items)
			}
		default:
			t.Fatalf("unsupported test configuration type %T", value)
		}
	}
	data := res.Data(&terraform.InstanceState{RawConfig: cty.ObjectVal(attrs)})
	for name, value := range config {
		if err := data.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	return data
}

func TestBogonProviderSchemaValid(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatal(err)
	}
}

func TestBogonRouteBlackholePresence(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.2"
	for name, res := range map[string]*schema.Resource{"ipv4": ResourceIPRoute(), "ipv6": ResourceIPv6Route()} {
		for _, tc := range []struct {
			name    string
			present bool
			value   string
			want    bool
		}{
			{"empty flag", true, "", true},
			{"true", true, "true", true},
			{"yes", true, "yes", true},
			{"false", true, "false", false},
			{"no", true, "no", false},
			{"absent", false, "", false},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				prefix := "192.0.2.0/24"
				if name == "ipv6" {
					prefix = "2001:db8::/64"
				}
				data := bogonResourceData(t, res, map[string]interface{}{"gateway": "", "dst_address": prefix})
				// A missing flag must clear previously observed true state on refresh.
				if err := data.Set("blackhole", true); err != nil {
					t.Fatal(err)
				}
				response := MikrotikItem{}
				if tc.present {
					response["blackhole"] = tc.value
				}
				if diags := MikrotikResourceDataToTerraform(response, res.Schema, data); diags.HasError() {
					t.Fatal(diags)
				}
				if got := data.Get("blackhole"); got != tc.want {
					t.Fatalf("blackhole read=%v want=%v", got, tc.want)
				}
				if value, present := response["blackhole"]; present != tc.present || value != tc.value {
					t.Fatal("normalization mutated native response")
				}
			})
		}
	}
}

func TestBogonOtherBooleanEmptyValueUnchanged(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.2"
	res := ResourceIPAddress()
	data := bogonResourceData(t, res, map[string]interface{}{"address": "192.0.2.1/24", "interface": "ether2"})
	if diags := MikrotikResourceDataToTerraform(MikrotikItem{"disabled": ""}, res.Schema, data); diags.HasError() {
		t.Fatal(diags)
	}
	if data.Get("disabled") != false {
		t.Fatal("unrelated empty boolean behavior changed")
	}
}

// Validate both the selected channel and the provisioning matcher. Supporting
// only one leaves 6 GHz configuration unusable before any RouterOS request.
func TestBogonWifi6GHzSchema(t *testing.T) {
	for _, tc := range []struct {
		name     string
		property *schema.Schema
	}{
		{"channel.band", ResourceWifiChannel().Schema["band"]},
		{"provisioning.supported_bands", ResourceWifiProvisioning().Schema["supported_bands"].Elem.(*schema.Schema)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, band := range []string{"2ghz-ax", "5ghz-ac", "5ghz-ax", "6ghz-ax"} {
				if _, errs := tc.property.ValidateFunc(band, tc.name); len(errs) != 0 {
					t.Fatalf("supported band %q rejected: %v", band, errs)
				}
			}
			if _, errs := tc.property.ValidateFunc("not-a-band", tc.name); len(errs) == 0 {
				t.Fatal("unknown band accepted")
			}
		})
	}
}

// Exercise the real upstream REST and resource CRUD paths. The compatibility
// patch must preserve 6 GHz strings on create/refresh/update, native IDs and
// deletion, while keeping a previous 5 GHz configuration valid.
func TestBogonWifi6GHzCRUD(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.2"
	for _, tc := range []struct {
		name          string
		resource      *schema.Resource
		config        map[string]interface{}
		attribute     string
		wireAttribute string
		updated       interface{}
	}{
		{"channel", ResourceWifiChannel(), map[string]interface{}{"name": "six", "band": "6ghz-ax", "width": "20/40/80mhz", "frequency": []string{"5955"}}, "band", "band", "5ghz-ax"},
		{"provisioning", ResourceWifiProvisioning(), map[string]interface{}{"action": "create-enabled", "master_configuration": "six", "radio_mac": "02:00:00:00:00:06", "supported_bands": []string{"6ghz-ax"}}, "supported_bands", "supported-bands", []string{"5ghz-ax"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/rest" + tc.resource.Schema[MetaResourcePath].Default.(string)
			var row MikrotikItem
			creates, updates, deletes := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != path && r.URL.Path != path+"/*6" {
					http.Error(w, "unexpected menu", http.StatusNotFound)
					return
				}
				switch r.Method {
				case http.MethodPut, http.MethodPatch:
					var payload MikrotikItem
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						http.Error(w, "invalid payload", http.StatusBadRequest)
						return
					}
					if r.Method == http.MethodPut {
						creates++
						row = MikrotikItem{".id": "*6"}
						if payload[tc.wireAttribute] != "6ghz-ax" {
							t.Errorf("6 GHz create payload=%v", payload)
						}
					} else {
						updates++
						if payload[tc.wireAttribute] != "5ghz-ax" {
							t.Errorf("band update payload=%v", payload)
						}
					}
					for key, value := range payload {
						row[key] = value
					}
					_ = json.NewEncoder(w).Encode(row)
				case http.MethodGet:
					if row == nil {
						_ = json.NewEncoder(w).Encode([]MikrotikItem{})
					} else {
						_ = json.NewEncoder(w).Encode([]MikrotikItem{row})
					}
				case http.MethodDelete:
					if !strings.HasSuffix(r.URL.Path, "/*6") {
						t.Error("delete did not target native resource ID")
					}
					deletes++
					row = nil
					w.WriteHeader(http.StatusNoContent)
				default:
					http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
				}
			}))
			t.Cleanup(server.Close)
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, extra: &ExtraParams{}, Client: server.Client()}
			data := bogonResourceData(t, tc.resource, tc.config)
			if diags := tc.resource.CreateContext(context.Background(), data, client); diags.HasError() {
				t.Fatal(diags)
			}
			if data.Id() != "*6" || creates != 1 {
				t.Fatalf("create id=%q count=%d", data.Id(), creates)
			}
			if diags := tc.resource.ReadContext(context.Background(), data, client); diags.HasError() {
				t.Fatal(diags)
			}
			payload, _ := TerraformResourceDataToMikrotik(tc.resource.Schema, data)
			if payload[tc.wireAttribute] != "6ghz-ax" {
				t.Fatalf("6 GHz did not survive refresh: %v", payload)
			}
			tc.config[tc.attribute] = tc.updated
			updated := bogonResourceData(t, tc.resource, tc.config)
			updated.SetId(data.Id())
			if diags := tc.resource.UpdateContext(context.Background(), updated, client); diags.HasError() {
				t.Fatal(diags)
			}
			if updates != 1 || updated.Id() != "*6" {
				t.Fatalf("update replaced resource: id=%q count=%d", updated.Id(), updates)
			}
			if diags := tc.resource.DeleteContext(context.Background(), updated, client); diags.HasError() {
				t.Fatal(diags)
			}
			if deletes != 1 || updated.Id() != "" || row != nil {
				t.Fatal("delete did not clear native resource and state")
			}
		})
	}
}

func TestBogonModernServiceAddress(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	for _, version := range []string{"7.23.7", "7.24.5"} {
		RouterOSVersion = version
		res := ResourceIpService()
		d := bogonResourceData(t, res, map[string]interface{}{"numbers": "ssh", "address": "192.0.2.0/24"})
		_ = d.Set("port", 22)
		body, _ := TerraformResourceDataToMikrotik(res.Schema, d)
		native := "address"
		if version == "7.24.5" {
			native = "available-from"
		}
		if body[native] != "192.0.2.0/24" {
			t.Fatalf("%s: wrong service payload %v", version, body)
		}
		if diags := MikrotikResourceDataToTerraform(MikrotikItem{native: "198.51.100.0/24"}, res.Schema, d); diags.HasError() {
			t.Fatal(diags)
		}
		if d.Get("address") != "198.51.100.0/24" {
			t.Fatal("service address refresh failed")
		}
	}
}

func TestBogonBridgeDoesNotOwnMLAG(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.5"
	res := ResourceInterfaceBridge()
	d := bogonResourceData(t, res, map[string]interface{}{"name": "bridge-site"})
	if diags := MikrotikResourceDataToTerraform(MikrotikItem{"mlag-peer-port": "bond-peer", "mlag-priority": "50", "mlag-heartbeat": "1s"}, res.Schema, d); diags.HasError() {
		t.Fatal(diags)
	}
	body, _ := TerraformResourceDataToMikrotik(res.Schema, d)
	for _, key := range []string{"mlag-peer-port", "mlag-priority", "mlag-heartbeat"} {
		if _, ok := body[key]; ok {
			t.Fatalf("bridge replays MLAG %q", key)
		}
	}
}
