package routeros

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

/*
{
    ".id": "*1",
    "bridge": "lan",
    "bridge-cost": "1",
    "bridge-horizon": "none",
    "client-isolation": "true",
    "disabled": "false",
    "interface-list": "LAN",
    "name": "datapath1",
    "vlan-id": "1"
}
*/

// https://help.mikrotik.com/docs/display/ROS/WiFi#WiFi-Datapathproperties
func ResourceWifiDatapath() *schema.Resource {
	resSchema := map[string]*schema.Schema{
		MetaResourcePath: PropResourcePath("/interface/wifi/datapath"),
		MetaId:           PropId(Id),

		"bridge": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Bridge interface to add the interface as a bridge port. Omit to remove the profile override; use `none` to explicitly disable bridge membership.",
		},
		"bridge_cost": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Spanning tree protocol cost of the bridge port.",
		},
		"bridge_horizon": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Bridge horizon to use when adding as a bridge port.",
		},
		"client_isolation": {
			Type:        schema.TypeBool,
			Optional:    true,
			Description: "An option to toggle communication between clients connected to the same AP.",
		},
		KeyComment:  PropCommentRw,
		KeyDisabled: PropDisabledRw,
		"interface_list": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "List to which add the interface as a member.",
		},
		KeyName: PropName("Name of the datapath."),
		"traffic_processing": {
			Type:         schema.TypeString,
			Optional:     true,
			Description:  "Where traffic is processed: on-cap or on-capsman (CAPsMAN forwarding requires RouterOS 7.21 or newer). Omit to remove the profile override.",
			ValidateFunc: validation.StringInSlice([]string{"on-cap", "on-capsman"}, false),
		},
		KeyVlanId: PropVlanIdRw("Default VLAN ID to assign to client devices connecting to this interface. Omit to remove the profile override.", false),
	}

	// An omitted VLAN is an unset profile override, not an implicit zero.
	resSchema[KeyVlanId].DiffSuppressFunc = nil

	return &schema.Resource{
		Description:   `*<span style="color:red">This resource requires a minimum version of RouterOS 7.13.</span>*`,
		CreateContext: DefaultCreate(resSchema),
		ReadContext:   wifiDatapathRead(resSchema),
		UpdateContext: wifiDatapathUpdate(resSchema),
		DeleteContext: DefaultDelete(resSchema),

		Importer: &schema.ResourceImporter{
			StateContext: ImportStateCustomContext(resSchema),
		},

		Schema: resSchema,
	}
}

// Unset profile properties explicitly so lower-priority WiFi configuration can
// take effect. RouterOS documents these properties as unsettable:
// https://manual.mikrotik.com/docs/cli-reference/interface/wifi/datapath/
var wifiDatapathUnsetFields = []string{"bridge", "traffic_processing", KeyVlanId}

func wifiDatapathRead(s map[string]*schema.Schema) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		metadata := GetMetadata(s)
		rows, err := ReadItems(&ItemId{metadata.IdType, d.Id()}, metadata.Path, m.(Client))
		if err != nil {
			return diag.FromErr(err)
		}
		if len(*rows) == 0 {
			d.SetId("")
			return nil
		}
		row := (*rows)[0]
		d.SetId(row.GetID(metadata.IdType))
		return wifiDatapathSetState(row, s, d)
	}
}

func wifiDatapathSetState(row MikrotikItem, s map[string]*schema.Schema, d *schema.ResourceData) diag.Diagnostics {
	// The generic reader visits only returned fields. Missing profile properties
	// must clear their prior values, otherwise a refresh restores stale overrides.
	for _, field := range wifiDatapathUnsetFields {
		if _, present := row[SnakeToKebab(field)]; present {
			continue
		}
		var zero interface{} = ""
		if field == KeyVlanId {
			zero = 0
		}
		if err := d.Set(field, zero); err != nil {
			return diag.FromErr(err)
		}
	}
	return MikrotikResourceDataToTerraform(row, s, d)
}

func wifiDatapathUpdate(s map[string]*schema.Schema) schema.UpdateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		c := m.(Client)
		item, metadata := TerraformResourceDataToMikrotik(s, d)
		rows, err := ReadItems(&ItemId{metadata.IdType, d.Id()}, metadata.Path, c)
		if err != nil {
			return diag.FromErr(err)
		}
		if len(*rows) == 0 {
			return diag.FromErr(errorNoLongerExists)
		}
		row := (*rows)[0]
		id := row.GetID(Id)
		if id == "" {
			return diag.FromErr(errEmptyId)
		}
		config := d.GetRawConfig()
		for _, field := range wifiDatapathUnsetFields {
			value := config.GetAttr(field)
			if !value.IsKnown() {
				continue
			}
			omitted := value.IsNull()
			if field != KeyVlanId && d.Get(field).(string) == "" {
				omitted = true
			}
			if !omitted {
				continue
			}
			wireField := SnakeToKebab(field)
			delete(item, wireField)
			delete(item, "!"+wireField)
			// Skip already-unset fields. Retrying a partially completed update remains
			// safe, including RouterOS versions that reject an unset of an absent value.
			if _, present := row[wireField]; !present {
				continue
			}
			path := metadata.Path
			if c.GetTransport() == TransportREST {
				path += "/unset"
			}
			if err := c.SendRequest(crudUnset, &URL{Path: path}, MikrotikItem{"numbers": id, "value-name": wireField}, nil); err != nil {
				return diag.Errorf("Failed to unset WiFi datapath %s on %s: %s", wireField, id, err)
			}
		}
		if _, err := UpdateItem(&ItemId{Id, id}, metadata.Path, item, c); err != nil {
			return diag.FromErr(err)
		}
		// API set returns no configuration; read both transports after all commands.
		return wifiDatapathRead(s)(ctx, d, c)
	}
}
