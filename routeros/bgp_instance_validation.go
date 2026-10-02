package routeros

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func validateBGPInstance(allowTemplates bool) DataValidateFunc {
	return func(d *schema.ResourceData) diag.Diagnostics {
		modern, err := routerOSVersionAtLeast("7.20")
		if err != nil {
			return diag.FromErr(err)
		}
		if !modern || strings.TrimSpace(d.Get("instance").(string)) != "" {
			return nil
		}
		// RouterOS resolves instances inherited from connection templates. Let
		// it validate that resolution without requiring a redundant override.
		if allowTemplates && d.Get("templates").(*schema.Set).Len() != 0 {
			return nil
		}
		if allowTemplates {
			return diag.Errorf("RouterOS 7.20 and later requires a BGP instance or connection templates that supply one")
		}
		return diag.Errorf("RouterOS 7.20 and later requires a BGP VPN instance")
	}
}
