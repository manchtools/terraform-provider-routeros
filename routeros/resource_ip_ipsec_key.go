package routeros

import (
	"context"
	"fmt"
	"maps"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

/*
  {
    ".id": "*1",
    "key-size": "1024", <<<< !!! /ip/ipsec/key/generate-key name=new-key key-size=   2048     4096     8192
    "name": "new-key",
    "private-key": "true",
    "rsa": "true"
  }
*/

// https://help.mikrotik.com/docs/display/ROS/IPsec#IPsec-Keys
func ResourceIpIpsecKey() *schema.Resource {
	resSchema := map[string]*schema.Schema{
		MetaResourcePath: PropResourcePath("/ip/ipsec/key"),
		MetaId:           PropId(Id),
		MetaSkipFields:   PropSkipFields("private_key", "rsa"),

		"key_size": {
			Type:             schema.TypeInt,
			Required:         true,
			ForceNew:         true,
			Description:      "Size of this key.",
			ValidateFunc:     validation.IntInSlice([]int{1024, 2048, 4096}),
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		KeyName: PropName(""),
	}

	// Select the native menu at operation time, after provider configuration has
	// discovered the firmware version. Never change the shared provider schema.
	operation := func(action func(context.Context, map[string]*schema.Schema, *schema.ResourceData, interface{}) diag.Diagnostics) func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics {
		return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
			runtimeSchema, err := ipsecKeyResourceSchema(resSchema)
			if err != nil {
				return diag.FromErr(err)
			}
			return action(ctx, runtimeSchema, d, m)
		}
	}
	return &schema.Resource{
		Description: "Manages an IPsec RSA key. Uses /ip/ipsec/key/rsa on RouterOS 7.20 and newer, and /ip/ipsec/key on older firmware.",
		CreateContext: operation(func(ctx context.Context, s map[string]*schema.Schema, d *schema.ResourceData, m interface{}) diag.Diagnostics {
			return ResourceCreateAndWait(ctxSetCrudMethod(ctx, crudGenerateKey), s, d, m, d.Timeout(schema.TimeoutCreate))
		}),
		ReadContext:   operation(ResourceRead),
		UpdateContext: operation(ResourceUpdate),
		DeleteContext: operation(ResourceDelete),

		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
				runtimeSchema, err := ipsecKeyResourceSchema(resSchema)
				if err != nil {
					return nil, err
				}
				return ImportStateCustomContext(runtimeSchema)(ctx, d, m)
			},
		},

		Schema: resSchema,
	}
}

func ipsecKeyResourceSchema(s map[string]*schema.Schema) (map[string]*schema.Schema, error) {
	modern, err := routerOSVersionAtLeast("7.20")
	if err != nil {
		return nil, fmt.Errorf("select IPsec RSA key menu: %w", err)
	}
	path := "/ip/ipsec/key"
	if modern {
		path += "/rsa"
	}
	runtimeSchema := maps.Clone(s)
	runtimeSchema[MetaResourcePath] = PropResourcePath(path)
	return runtimeSchema, nil
}
