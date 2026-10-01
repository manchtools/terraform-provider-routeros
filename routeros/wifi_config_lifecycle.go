package routeros

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func isWifiProfilePath(path string) bool {
	return path == "/interface/wifi" || path == "/interface/wifi/configuration"
}

func readWifiDirectConfig(id *ItemId, path string, c Client) (*[]MikrotikItem, error) {
	url := &URL{Path: path, Query: []string{"?" + id.Type.String() + "=" + id.Value}}
	if c.GetTransport() == TransportREST {
		url.Path += "/print"
	}
	var rows []MikrotikItem
	err := c.SendRequest(crudPrintConfig, url, MikrotikItem{"config": ""}, &rows)
	return &rows, err
}

func wifiDirectRead(s map[string]*schema.Schema) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		metadata := GetMetadata(s)
		rows, err := readWifiDirectConfig(&ItemId{metadata.IdType, d.Id()}, metadata.Path, m.(Client))
		if err != nil {
			return diag.FromErr(err)
		}
		if len(*rows) == 0 {
			d.SetId("")
			return nil
		}
		// Config print returns only direct settings. Rebuild profile maps so
		// a removed reference cannot survive from an earlier native read.
		for name, field := range s {
			if field.Type == schema.TypeMap {
				if err := d.Set(name, map[string]interface{}{}); err != nil {
					return diag.FromErr(err)
				}
			}
		}
		d.SetId((*rows)[0].GetID(metadata.IdType))
		return MikrotikResourceDataToTerraform((*rows)[0], s, d)
	}
}

func wifiDirectCreate(s map[string]*schema.Schema) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		diags := ResourceCreate(ctx, s, d, m)
		if diags.HasError() {
			return diags
		}
		return append(diags, wifiDirectRead(s)(ctx, d, m)...)
	}
}

func wifiDirectUpdate(s map[string]*schema.Schema) schema.UpdateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		metadata := GetMetadata(s)
		c := m.(Client)
		rows, err := readWifiDirectConfig(&ItemId{metadata.IdType, d.Id()}, metadata.Path, c)
		if err != nil {
			return diag.FromErr(err)
		}
		if len(*rows) == 0 {
			d.SetId("")
			return diag.FromErr(errorNoLongerExists)
		}
		item, _ := TerraformResourceDataToMikrotik(s, d)
		id := (*rows)[0].GetID(Id)
		if id == "" {
			return diag.Errorf("WiFi config read returned no native ID; refusing to unset settings")
		}
		for native := range (*rows)[0] {
			owned := false
			for name, field := range s {
				if field.Type == schema.TypeMap && (native == name || strings.HasPrefix(native, name+".")) {
					owned = true
					break
				}
			}
			if !owned {
				continue
			}
			if _, configured := item[native]; configured {
				continue
			}
			path := metadata.Path
			if c.GetTransport() == TransportREST {
				path += "/unset"
			}
			if err := c.SendRequest(crudUnset, &URL{Path: path}, MikrotikItem{"numbers": id, "value-name": native}, nil); err != nil {
				return diag.Errorf("Failed to unset direct WiFi setting %s: %s", native, err)
			}
		}
		diags := ResourceUpdate(ctx, s, d, m)
		if diags.HasError() {
			return diags
		}
		return append(diags, wifiDirectRead(s)(ctx, d, m)...)
	}
}
