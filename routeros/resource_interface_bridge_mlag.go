package routeros

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// MLAG owns only the peer settings, leaving the underlying bridge independently managed.
func ResourceInterfaceBridgeMlag() *schema.Resource {
	s := map[string]*schema.Schema{
		MetaResourcePath: PropResourcePath("/interface/bridge/mlag"), MetaId: PropId(Id),
		"bridge":    {Type: schema.TypeString, Required: true, ForceNew: true, Description: "Bridge hosting MLAG."},
		"peer_port": {Type: schema.TypeString, Required: true, Description: "Interface connecting the MLAG peers."},
		"heartbeat": {Type: schema.TypeString, Optional: true, Default: "5s", DiffSuppressFunc: TimeEqual, Description: "Peer heartbeat interval."},
		"priority":  {Type: schema.TypeInt, Optional: true, Default: 128, ValidateFunc: validation.IntBetween(0, 128), Description: "Primary election priority; lower wins."},
	}
	read := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		return readBridgeMlag(ctx, s, d, m.(Client))
	}
	write := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		if err := writeBridgeMlag(d, m.(Client), false); err != nil {
			return diag.FromErr(err)
		}
		return read(ctx, d, m)
	}
	return &schema.Resource{
		Description: "Owns MLAG settings on legacy singleton menus and on bridges in RouterOS 7.22 and newer. Destroy disables the peer port and retains the bridge.",
		Schema:      s, CreateContext: write, UpdateContext: write, ReadContext: read,
		DeleteContext: func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
			if err := writeBridgeMlag(d, m.(Client), true); err != nil {
				return diag.FromErr(err)
			}
			diags := read(ctx, d, m)
			if diags.HasError() {
				return diags
			}
			if d.Id() != "" {
				heartbeat, err := ParseDuration(d.Get("heartbeat").(string), time.Second)
				if err != nil || heartbeat != 5*time.Second || d.Get("peer_port").(string) != "none" || d.Get("priority").(int) != 128 {
					return diag.Errorf("MLAG reset could not be verified")
				}
			}
			d.SetId("")
			return diags
		},
		Importer: &schema.ResourceImporter{StateContext: func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
			modern, err := routerOSVersionAtLeast("7.22")
			if err != nil {
				return nil, err
			}
			if modern {
				if err := d.Set("bridge", d.Id()); err != nil {
					return nil, err
				}
			}
			return []*schema.ResourceData{d}, nil
		}},
	}
}

func mlagBridge(d *schema.ResourceData, c Client) (MikrotikItem, error) {
	name := url.QueryEscape(d.Get("bridge").(string))

	rows, err := ReadItems(&ItemId{Type: Name, Value: name}, "/interface/bridge", c)
	if err != nil {
		return nil, err
	}
	if len(*rows) == 0 {
		return nil, nil
	}
	if len(*rows) != 1 {
		return nil, fmt.Errorf("MLAG bridge name is not unique")
	}
	return (*rows)[0], nil
}

func readBridgeMlag(ctx context.Context, s map[string]*schema.Schema, d *schema.ResourceData, c Client) diag.Diagnostics {
	modern, err := routerOSVersionAtLeast("7.22")
	if err != nil {
		return diag.FromErr(err)
	}
	if !modern {
		return SystemResourceRead(ctx, s, d, c)
	}
	row, err := mlagBridge(d, c)
	if err != nil {
		return diag.FromErr(err)
	}
	if row == nil {
		d.SetId("")
		return nil
	}
	if row[".id"] == "" {
		return diag.Errorf("MLAG bridge response has no native ID")
	}
	d.SetId(row[".id"])
	return MikrotikResourceDataToTerraform(MikrotikItem{"bridge": row["name"], "peer-port": row["mlag-peer-port"], "priority": row["mlag-priority"], "heartbeat": row["mlag-heartbeat"]}, s, d)
}

func writeBridgeMlag(d *schema.ResourceData, c Client, reset bool) error {
	modern, err := routerOSVersionAtLeast("7.22")
	if err != nil {
		return err
	}
	item := MikrotikItem{"peer-port": d.Get("peer_port").(string), "priority": strconv.Itoa(d.Get("priority").(int)), "heartbeat": d.Get("heartbeat").(string)}
	if reset {
		item = MikrotikItem{"peer-port": "none", "priority": "128", "heartbeat": "5s"}
	}
	if modern {
		row, err := mlagBridge(d, c)
		if err != nil {
			return err
		}
		if row == nil {
			if reset {
				return nil
			}
			return fmt.Errorf("MLAG bridge %q does not exist", d.Get("bridge"))
		}
		if row[".id"] == "" {
			return fmt.Errorf("MLAG bridge response has no native ID")
		}
		body := MikrotikItem{}
		for k, v := range item {
			body["mlag-"+k] = v
		}
		_, err = UpdateItem(&ItemId{Type: Id, Value: row[".id"]}, "/interface/bridge", body, c)
		if err == nil {
			d.SetId(row[".id"])
		}
		return err
	}
	item["bridge"] = d.Get("bridge").(string)
	if reset {
		item["bridge"] = "none"
	}
	path := "/interface/bridge/mlag/set"

	err = c.SendRequest(crudPost, &URL{Path: path}, item, nil)
	if err == nil {
		d.SetId("interface.bridge.mlag")
	}
	return err
}
