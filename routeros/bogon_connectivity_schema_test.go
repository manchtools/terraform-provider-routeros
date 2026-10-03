package routeros

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonIPv6FirewallDatasourceJumpTarget(t *testing.T) {
	res := DatasourceIPv6Firewall()
	res.Schema[MetaResourcePath] = PropResourcePath("/ipv6/firewall/filter")
	for _, target := range []string{"bogon-forward", ""} {
		t.Run(target, func(t *testing.T) {
			data := schema.TestResourceDataRaw(t, res.Schema, nil)
			rows := []MikrotikItem{{".id": "*7", "chain": "forward", "action": "jump", "jump-target": target}}
			if diags := MikrotikResourceDataToTerraformDatasource(&rows, "rules", res.Schema, data); len(diags) != 0 {
				t.Fatalf("firewall response produced diagnostics: %v", diags)
			}
			got := data.Get("rules").([]interface{})
			if len(got) != 1 || got[0].(map[string]interface{})["jump_target"] != target {
				t.Fatalf("jump target lost from observed rule: %v", got)
			}
		})
	}
}

func TestBogonWireGuardClientAllowedAddress(t *testing.T) {
	res := ResourceInterfaceWireguardPeer()
	config := map[string]interface{}{"interface": "bogon-links", "public_key": "fixture", "allowed_address": []string{"198.18.64.126/32"}}
	for _, native := range []string{"", "0.0.0.0/0,::/0", "10.82.10.0/24"} {
		t.Run(native, func(t *testing.T) {
			data := bogonResourceData(t, res, config)
			if diags := MikrotikResourceDataToTerraform(MikrotikItem{"client-allowed-address": native}, res.Schema, data); len(diags) != 0 {
				t.Fatalf("WireGuard response produced diagnostics: %v", diags)
			}
			want := []interface{}{}
			if native != "" {
				for _, address := range strings.Split(native, ",") {
					want = append(want, address)
				}
			}
			if got := data.Get("client_allowed_address"); !reflect.DeepEqual(got, want) {
				t.Fatalf("client addresses not refreshed: got %v, want %v", got, want)
			}
			if got := data.Get("allowed_address"); !reflect.DeepEqual(got, []interface{}{"198.18.64.126/32"}) {
				t.Fatalf("client export addresses changed tunnel allowed addresses: %v", got)
			}
		})
	}
	data := bogonResourceData(t, res, config)
	payload, _ := TerraformResourceDataToMikrotik(res.Schema, data)
	if _, exists := payload["client-allowed-address"]; exists {
		t.Fatal("omitted client addresses written to RouterOS")
	}
	config["client_allowed_address"] = []string{"10.82.10.0/24", "fd00::/64"}
	data = bogonResourceData(t, res, config)
	payload, _ = TerraformResourceDataToMikrotik(res.Schema, data)
	if got := payload["client-allowed-address"]; got != "10.82.10.0/24,fd00::/64" {
		t.Fatalf("explicit client addresses not serialized: %v", got)
	}
	meta := res.Schema["client_allowed_address"]
	if meta == nil || !meta.Optional || !meta.Computed || meta.Default != nil {
		t.Fatal("omitted native client addresses must be observed without a provider default")
	}
	omitted := bogonResourceData(t, res, map[string]interface{}{"interface": "bogon-links", "public_key": "fixture", "allowed_address": []string{"198.18.64.126/32"}})
	if !meta.DiffSuppressFunc("client_allowed_address.#", "2", "0", omitted) {
		t.Fatal("omitted client export addresses create drift")
	}
	if meta.DiffSuppressFunc("client_allowed_address.#", "1", "2", data) {
		t.Fatal("explicit client export address changes were suppressed")
	}
}
