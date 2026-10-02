package routeros

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// ResourceIPReverseProxy manages static rules, leaving container-app rules owned
// by RouterOS. The HTTPS listener is configured separately in /ip/service.
func ResourceIPReverseProxy() *schema.Resource {
	properties := map[string]*schema.Schema{
		MetaResourcePath: PropResourcePath("/ip/reverse-proxy"),
		MetaId:           PropId(Id),
		"sni": {
			Type:         schema.TypeString,
			Required:     true,
			Description:  "Exact TLS Server Name Indication hostname. Wildcard routing is not supported.",
			ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[^*[:space:]]+$`), "must be a nonempty exact SNI hostname without wildcards or whitespace"),
		},
		"ip_address": {
			Type:         schema.TypeString,
			Required:     true,
			Description:  "IPv4 or IPv6 address of the HTTP backend. DNS names are not supported.",
			ValidateFunc: validation.IsIPAddress,
			DiffSuppressFunc: func(_ string, old, new string, _ *schema.ResourceData) bool {
				oldAddress, oldErr := netip.ParseAddr(old)
				newAddress, newErr := netip.ParseAddr(new)
				return oldErr == nil && newErr == nil && oldAddress == newAddress
			},
		},
		"port": {
			Type:         schema.TypeInt,
			Required:     true,
			Description:  "Backend TCP port. RouterOS forwards plain HTTP even when this is 443.",
			ValidateFunc: validation.IntBetween(1, 65535),
		},
		"certificate": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "none",
			Description:  "Certificate name for TLS termination. Use none to inherit the reverse-proxy service certificate.",
			ValidateFunc: validation.StringIsNotWhiteSpace,
		},
		"comment": {
			Type:     schema.TypeString,
			Optional: true,
			Default:  "",
		},
		"disabled": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
		},
		KeyDynamic: PropDynamicRo,
	}
	read := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		row, err := lookupReverseProxy(d.Id(), m.(Client))
		if errors.Is(err, errorNoLongerExists) {
			d.SetId("")
			return nil
		}
		if err != nil {
			return diag.FromErr(err)
		}
		return readReverseProxy(row, properties, d)
	}
	update := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		row, err := lookupReverseProxy(d.Id(), m.(Client))
		if err != nil {
			return diag.FromErr(err)
		}
		item, metadata := TerraformResourceDataToMikrotik(properties, d)
		// RouterOS omits cleared comments from readback. An omitted default
		// must still clear an earlier comment on update.
		item["comment"] = d.Get("comment").(string)
		result, err := UpdateItem(&ItemId{Id, row[".id"]}, metadata.Path, item, m.(Client))
		if err != nil {
			return diag.FromErr(err)
		}
		return readReverseProxy(result, properties, d)
	}
	deleteRule := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		row, err := lookupReverseProxy(d.Id(), m.(Client))
		if errors.Is(err, errorNoLongerExists) {
			d.SetId("")
			return nil
		}
		if err != nil {
			return diag.FromErr(err)
		}
		if err := DeleteItem(&ItemId{Id, row[".id"]}, "/ip/reverse-proxy", m.(Client)); err != nil {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
	importRule := func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
		row, err := lookupReverseProxy(d.Id(), m.(Client))
		if err != nil {
			return nil, err
		}
		if diags := readReverseProxy(row, properties, d); diags.HasError() {
			return nil, fmt.Errorf("read imported reverse proxy rule: %v", diags)
		}
		return []*schema.ResourceData{d}, nil
	}
	return &schema.Resource{
		Description:   "Manages a static RouterOS HTTPS-to-HTTP reverse proxy rule. Requires a router supporting /ip/reverse-proxy; SMIPS devices do not support it. Configure the reverse-proxy service listener and certificates separately.",
		Schema:        properties,
		CreateContext: DefaultCreate(properties),
		ReadContext:   read,
		UpdateContext: update,
		DeleteContext: deleteRule,
		Importer:      &schema.ResourceImporter{StateContext: importRule},
	}
}

func lookupReverseProxy(selector string, client Client) (MikrotikItem, error) {
	field, value := "sni", selector
	if strings.HasPrefix(selector, "*") {
		field = ".id"
	} else if key, selected, ok := strings.Cut(selector, "="); ok {
		field, value = SnakeToKebab(key), selected
	}
	if value == "" || !slices.Contains([]string{".id", "sni", "ip-address", "port", "certificate", "comment", "disabled"}, field) {
		return nil, fmt.Errorf("invalid reverse proxy import selector %q", selector)
	}
	rows, err := ReadItemsFiltered([]string{field + "=" + url.QueryEscape(value)}, "/ip/reverse-proxy", client)
	if err != nil {
		return nil, fmt.Errorf("find reverse proxy rule: %w", err)
	}
	var selected MikrotikItem
	for _, row := range *rows {
		if row["dynamic"] == "true" || row["dynamic"] == "yes" {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("more than one static reverse proxy rule matches %q; import by .id", selector)
		}
		selected = row
	}
	if selected == nil {
		if len(*rows) > 0 {
			return nil, fmt.Errorf("dynamic reverse proxy rules are owned by RouterOS and cannot be managed")
		}
		return nil, fmt.Errorf("reverse proxy rule %q: %w", selector, errorNoLongerExists)
	}
	if selected[".id"] == "" {
		return nil, fmt.Errorf("reverse proxy rule has no internal ID")
	}
	return selected, nil
}

func readReverseProxy(row MikrotikItem, properties map[string]*schema.Schema, d *schema.ResourceData) diag.Diagnostics {
	d.SetId(row[".id"])
	if err := d.Set("comment", row["comment"]); err != nil {
		return diag.FromErr(err)
	}
	return MikrotikResourceDataToTerraform(row, properties, d)
}
