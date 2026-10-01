package routeros

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

/*
{
    "ca-certificate": "auto",
    "certificate": "auto",
    "enabled": "yes",
    "generated-ca-certificate": "WiFi-CAPsMAN-CA-000000000000",
    "generated-certificate": "WiFi-CAPsMAN-000000000000",
    "interfaces": "LAN",
    "package-path": "/upgrade",
    "require-peer-certificate": "true",
    "upgrade-policy": "suggest-same-version"
}
*/

// https://help.mikrotik.com/docs/display/ROS/WiFi#WiFi-CAPsMANGlobalConfiguration
func ResourceWifiCapsman() *schema.Resource {
	resSchema := map[string]*schema.Schema{
		MetaResourcePath: PropResourcePath("/interface/wifi/capsman"),
		MetaId:           PropId(Name),

		"ca_certificate": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Device CA certificate.",
		},
		"certificate": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Device certificate.",
		},
		KeyEnabled: PropEnabled("Disable or enable CAPsMAN functionality."),
		"generated_ca_certificate": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Generated CA certificate.",
		},
		"generated_certificate": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Generated CAPsMAN certificate.",
		},
		"interfaces": {
			Type:        schema.TypeList,
			Optional:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
			Description: "List of interfaces on which CAPsMAN will listen for CAP connections.",
		},
		"package_path": {
			Type:     schema.TypeString,
			Optional: true,
			Description: "Folder location for the RouterOS packages. For example, use '/upgrade' to specify the " +
				"upgrade folder from the files section. If empty string is set, CAPsMAN can use built-in RouterOS " +
				"packages, note that in this case only CAPs with the same architecture as CAPsMAN will be upgraded.",
		},
		"require_peer_certificate": {
			Type:        schema.TypeBool,
			Optional:    true,
			Description: "Require all connecting CAPs to have a valid certificate.",
		},
		"upgrade_policy": {
			Type:         schema.TypeString,
			Optional:     true,
			Description:  "Upgrade policy options.",
			ValidateFunc: validation.StringInSlice([]string{"none", "require-same-version", "suggest-same-version"}, false),
		},
	}

	return &schema.Resource{
		Description: `*<span style="color:red">This resource requires a minimum version of RouterOS 7.13.</span>*

Destroy disables CAPsMAN and verifies that it stopped before removing the singleton from state. Other settings, generated certificates, and physical radios are preserved.`,
		CreateContext: DefaultSystemCreate(resSchema),
		ReadContext:   DefaultSystemRead(resSchema),
		UpdateContext: DefaultSystemUpdate(resSchema),
		DeleteContext: wifiCapsmanDelete(resSchema),

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: resSchema,
	}
}

func wifiCapsmanDelete(s map[string]*schema.Schema) schema.DeleteContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		c := m.(Client)
		path := GetMetadata(s).Path
		updatePath := path
		if c.GetTransport() == TransportREST {
			updatePath += "/set"
		}
		// CAPsMAN is a settings singleton. Deleting only state leaves a running
		// manager, while resetting all settings could affect unrelated certificates.
		if err := c.SendRequest(crudPost, &URL{Path: updatePath}, MikrotikItem{"enabled": "false"}, nil); err != nil {
			return diag.FromErr(err)
		}
		row := MikrotikItem{}
		if err := c.SendRequest(crudRead, &URL{Path: path}, nil, &row); err != nil {
			return diag.FromErr(err)
		}
		if row["enabled"] != "no" && row["enabled"] != "false" {
			return diag.Errorf("CAPsMAN disable could not be verified: enabled=%q", row["enabled"])
		}
		if err := d.Set(KeyEnabled, false); err != nil {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
}
