package routeros

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"testing"
)

func TestBogonRouterOSResponseFields(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.2"
	cases := []struct {
		name     string
		resource *schema.Resource
		config   map[string]interface{}
		native   string
		values   []string
		writable bool
	}{
		{"bridge managed", ResourceInterfaceBridge(), map[string]interface{}{"name": "probe"}, "managed", []string{"false", "true"}, false},
		{"bridge port managed", ResourceInterfaceBridgePort(), map[string]interface{}{"bridge": "probe", "interface": "ether2"}, "managed", []string{"false", "true"}, false},
		{"bridge VLAN managed", ResourceInterfaceBridgeVlan(), map[string]interface{}{"bridge": "probe", "vlan_ids": []string{"80"}}, "managed", []string{"false", "true"}, false},
		{"bridge DHCPv6 snooping", ResourceInterfaceBridge(), map[string]interface{}{"name": "probe"}, "dhcpv6-snooping", []string{"false", "true"}, true},
		{"port DHCPv6 trust", ResourceInterfaceBridgePort(), map[string]interface{}{"bridge": "probe", "interface": "ether2"}, "trusted-dhcpv6", []string{"false", "true"}, true},
		{"DHCP DNS suffix", ResourceDhcpServer(), map[string]interface{}{"name": "probe", "interface": "probe"}, "add-dns-entries-suffix", []string{"lan", "office.example", ""}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := KebabToSnake(tc.native)
			for _, value := range tc.values {
				t.Run(value, func(t *testing.T) {
					data := bogonResourceData(t, tc.resource, tc.config)
					diags := MikrotikResourceDataToTerraform(MikrotikItem{tc.native: value}, tc.resource.Schema, data)
					if len(diags) != 0 {
						t.Fatalf("RouterOS response produced diagnostics: %v", diags)
					}
					got := data.Get(key)
					if value == "true" || value == "false" {
						if got != (value == "true") {
							t.Fatalf("field not refreshed: %v", got)
						}
					} else if got != value {
						t.Fatalf("field not refreshed: %v", got)
					}
					if !tc.writable {
						payload, _ := TerraformResourceDataToMikrotik(tc.resource.Schema, data)
						if _, ok := payload[tc.native]; ok {
							t.Fatal("read-only managed flag was sent to RouterOS")
						}
					}
				})
			}
			data := bogonResourceData(t, tc.resource, tc.config)
			payload, _ := TerraformResourceDataToMikrotik(tc.resource.Schema, data)
			if _, ok := payload[tc.native]; ok {
				t.Fatal("omitted field sent before RouterOS supplies it")
			}
			if tc.writable {
				meta := tc.resource.Schema[key]
				if meta == nil || !meta.Optional || !meta.Computed || meta.Default != nil {
					t.Fatal("native default must remain observed, without a provider default")
				}
				if !meta.DiffSuppressFunc(key, "native", "", data) {
					t.Fatal("unconfigured native value creates drift")
				}
				var requested interface{} = "office.example"
				expected := "office.example"
				if tc.native != "add-dns-entries-suffix" {
					requested = false
					expected = "no"
				}
				explicit := map[string]interface{}{}
				for k, v := range tc.config {
					explicit[k] = v
				}
				explicit[key] = requested
				data = bogonResourceData(t, tc.resource, explicit)
				payload, _ = TerraformResourceDataToMikrotik(tc.resource.Schema, data)
				if payload[tc.native] != expected {
					t.Fatalf("explicit value was not serialized: %v", payload)
				}
			}
		})
	}
}
