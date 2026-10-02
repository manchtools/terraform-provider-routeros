package routeros

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const testIpIpsecKey = "routeros_ip_ipsec_key.test"

func TestAccIpIpsecKeyTest_basic(t *testing.T) {
	// t.Parallel()
	for _, name := range testNames {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck: func() {
					testAccPreCheck(t)
					testSetTransportEnv(t, name)
				},
				ProviderFactories: testAccProviderFactories,
				CheckDestroy: func(state *terraform.State) error {
					runtimeSchema, err := ipsecKeyResourceSchema(ResourceIpIpsecKey().Schema)
					if err != nil {
						return err
					}
					return testCheckResourceDestroy(GetMetadata(runtimeSchema).Path, "routeros_ip_ipsec_key")(state)
				},
				Steps: []resource.TestStep{
					{
						Config: testAccIpIpsecKeyConfig("test-key"),
						Check: resource.ComposeTestCheckFunc(
							testResourcePrimaryInstanceId(testIpIpsecKey),
							resource.TestCheckResourceAttr(testIpIpsecKey, "name", "test-key"),
							resource.TestCheckResourceAttr(testIpIpsecKey, "key_size", "2048"),
						),
					},
					{
						ResourceName:      testIpIpsecKey,
						ImportState:       true,
						ImportStateVerify: true,
					},
					{
						Config: testAccIpIpsecKeyConfig("renamed-key"),
						Check: resource.ComposeTestCheckFunc(
							testResourcePrimaryInstanceId(testIpIpsecKey),
							resource.TestCheckResourceAttr(testIpIpsecKey, "name", "renamed-key"),
							resource.TestCheckResourceAttr(testIpIpsecKey, "key_size", "2048"),
						),
					},
				},
			})
		})
	}
}

func testAccIpIpsecKeyConfig(name string) string {
	return fmt.Sprintf(`%v

resource "routeros_ip_ipsec_key" "test" {
  name     = %q
  key_size = 2048
}
`, providerConfig, name)
}
