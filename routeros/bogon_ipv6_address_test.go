package routeros

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonIPv6AddressVRFReadOnly(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.5"
	res := ResourceIPv6Address()
	d := bogonResourceData(t, res, map[string]interface{}{
		"address":   "2001:db8::1/64",
		"interface": "ether2",
	})
	if diags := MikrotikResourceDataToTerraform(MikrotikItem{"vrf": "main"}, res.Schema, d); diags.HasError() {
		t.Fatal(diags)
	}
	if got := d.Get("vrf"); got != "main" {
		t.Fatalf("native VRF was not refreshed: %v", got)
	}
	if err := d.Set("comment", "unrelated edit"); err != nil {
		t.Fatal(err)
	}
	payload, _ := TerraformResourceDataToMikrotik(res.Schema, d)
	if got := payload["comment"]; got != "unrelated edit" {
		t.Fatalf("comment update missing from payload: %q", got)
	}
	if value, present := payload["vrf"]; present {
		t.Fatalf("read-only IPv6 VRF replayed during unrelated update: %q", value)
	}
}

func TestBogonIPv6StaticAddressDiff(t *testing.T) {
	tests := []struct {
		name, old, desired, pool string
		eui64, suppress          bool
	}{
		{"different host", "2001:db8::100/64", "2001:db8::1/64", "", false, false},
		{"different prefix", "2001:db8::1/64", "::1/64", "", false, false},
		{"different prefix length", "2001:db8::1/64", "2001:db8::1/80", "", false, false},
		{"equivalent spelling", "2001:db8::1/64", "2001:0DB8:0:0:0:0:0:1/64", "", false, true},
		{"malformed value", "2001:db8::1/64", "invalid", "", false, false},
		{"EUI64 generation", "2001:db8::5c30:77ff:fe61:33ac/64", "2001:db8::/64", "", true, true},
		{"pool generation", "2001:db8::1/64", "::1/64", "probe", false, true},
	}
	res := ResourceIPv6Address()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
				"address": tc.desired, "interface": "ether2", "eui_64": tc.eui64, "from_pool": tc.pool,
			})
			if got := res.Schema["address"].DiffSuppressFunc("address", tc.old, tc.desired, d); got != tc.suppress {
				t.Fatalf("address diff suppressed=%v, want %v", got, tc.suppress)
			}
		})
	}
}
