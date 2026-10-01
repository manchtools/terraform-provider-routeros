package routeros

import "testing"

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
