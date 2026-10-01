package routeros

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"testing"
)

// The omitted settings must survive a refresh followed by an unrelated write.
func TestBogonReviewedDHCPDefaults(t *testing.T) {
	oldVersion := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = oldVersion })
	RouterOSVersion = "7.24.5"
	tests := []struct {
		name                 string
		resource             *schema.Resource
		field, native, value string
	}{
		{"IPv4", ResourceDhcpServer(), "dynamic_lease_identifiers", "dynamic-lease-identifiers", "client-id,client-mac"},
		{"IPv6", ResourceIpv6DhcpServer(), "prefix_pool", "prefix-pool", "static-only"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := bogonResourceData(t, tc.resource, map[string]interface{}{"name": "probe", "interface": "ether2", "address_pool": "pool"})
			if diags := MikrotikResourceDataToTerraform(MikrotikItem{tc.native: tc.value}, tc.resource.Schema, d); diags.HasError() {
				t.Fatal(diags)
			}
			if err := d.Set("comment", "unrelated edit"); err != nil {
				t.Fatal(err)
			}
			payload, _ := TerraformResourceDataToMikrotik(tc.resource.Schema, d)
			if got := payload[tc.native]; got != tc.value {
				t.Fatalf("omitted %s cleared: got %q, want %q", tc.field, got, tc.value)
			}
			if !tc.resource.Schema[tc.field].DiffSuppressFunc(tc.field, tc.value, "", d) {
				t.Fatal("omitted setting diff was not suppressed")
			}
			configured := bogonResourceData(t, tc.resource, map[string]interface{}{tc.field: "explicit"})
			if tc.resource.Schema[tc.field].DiffSuppressFunc(tc.field, tc.value, "explicit", configured) {
				t.Fatal("explicit setting diff was suppressed")
			}
		})
	}
}

func TestBogonReviewedIPv6AddressList(t *testing.T) {
	oldVersion := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = oldVersion })
	RouterOSVersion = "7.24.5"
	res := ResourceIPv6FirewallFilter()
	for _, action := range []string{"add-src-to-address-list", "add-dst-to-address-list"} {
		t.Run(action, func(t *testing.T) {
			d := bogonResourceData(t, res, map[string]interface{}{"chain": "forward", "action": action, "address_list": "reviewed-list"})
			payload, _ := TerraformResourceDataToMikrotik(res.Schema, d)
			if got := payload["address-list"]; got != "reviewed-list" {
				t.Fatalf("address-list=%q", got)
			}
			if diags := MikrotikResourceDataToTerraform(MikrotikItem{"address-list": "native-list"}, res.Schema, d); diags.HasError() {
				t.Fatal(diags)
			}
			if got := d.Get("address_list"); got != "native-list" {
				t.Fatalf("native address list not refreshed: %v", got)
			}
		})
	}
}

func TestBogonReviewedMalformedDiffValues(t *testing.T) {
	res := ResourceInterfaceGre()
	d := bogonResourceData(t, res, map[string]interface{}{"name": "probe", "keepalive": "10s,10"})
	tests := []struct {
		name     string
		fn       schema.SchemaDiffSuppressFunc
		old, new string
	}{
		{"keepalive shape", PropKeepaliveRw.DiffSuppressFunc, "broken", "10s,10"},
		{"keepalive old interval", PropKeepaliveRw.DiffSuppressFunc, "broken,10", "10s,10"},
		{"keepalive new interval", PropKeepaliveRw.DiffSuppressFunc, "10s,10", "broken,10"},
		{"time", TimeEqual, "invalid", "10s"},
		{"hex", HexEqual, "invalid", "0x10"},
		{"bits", BitsEqual, "invalid", "30M"},
		{"bytes", BytesEqual, "invalid", "64M"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.fn("keepalive", tc.old, tc.new, d) {
				t.Fatal("malformed value unexpectedly suppressed")
			}
		})
	}
	if !PropKeepaliveRw.DiffSuppressFunc("keepalive", "10,10", "10s,10", d) {
		t.Fatal("equivalent valid intervals no longer suppressed")
	}
}
