package routeros

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

var ipServiceNames = []string{"api", "api-ssl", "ftp", "ssh", "telnet", "winbox", "www", "www-ssl", "reverse-proxy"}

/*
  {
    ".id": "*0",
    "address": "",
    "disabled": "false",
    "invalid": "false",
    "name": "telnet",
    "port": "23",
    "vrf": "main"
  },
  {
    ".id": "*6",
    "address": "",
    "certificate": "https-cert",
    "disabled": "false",
    "invalid": "false",
    "name": "www-ssl",
    "port": "443",
    "tls-version": "any",
    "vrf": "main"
  },
*/

// https://help.mikrotik.com/docs/display/ROS/Services
func ResourceIpService() *schema.Resource {
	resSchema := map[string]*schema.Schema{
		MetaResourcePath: PropResourcePath("/ip/service"),
		MetaId:           PropId(Name),

		"address": {
			Type:        schema.TypeString,
			Optional:    true,
			Default:     "",
			Description: "List of IP/IPv6 prefixes from which the service is accessible.",
			DiffSuppressFunc: func(k, oldValue, newValue string, d *schema.ResourceData) bool {
				if oldValue == "" && newValue == "0.0.0.0/0" {
					return false
				}
				return oldValue == newValue
			},
		},
		"certificate": {
			Type:     schema.TypeString,
			Optional: true,
			Description: "The name of the certificate used by a particular service. Applicable only for services " +
				"that depend on certificates ( www-ssl, api-ssl, reverse-proxy ).",
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		KeyDisabled: PropDisabledRw,
		KeyDynamic:  PropDynamicRo,
		KeyInvalid:  PropInvalidRo,
		"max_sessions": {
			Type:             schema.TypeInt,
			Optional:         true,
			Description:      "Maximum number of concurrent connections to a particular service. This option is available in RouterOS starting from version 7.16.",
			ValidateFunc:     validation.IntAtLeast(1),
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		"name": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Service name.",
		},
		"numbers": {
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
			Description: "The name of the service whose settings will be changed ( api, api-ssl, ftp, ssh, telnet, " +
				"winbox, www, www-ssl, reverse-proxy ). One resource manages one built-in service.",
			ValidateFunc: validation.StringInSlice(ipServiceNames, false),
		},
		"port": {
			Type:         schema.TypeInt,
			Required:     true,
			Description:  "The port particular service listens on.",
			ValidateFunc: validation.IntBetween(1, 65535),
		},
		"proto": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"tls_version": {
			Type:             schema.TypeString,
			Optional:         true,
			Description:      "Specifies which TLS versions to allow by a particular service.",
			ValidateFunc:     validation.StringInSlice([]string{"any", "only-1.2"}, false),
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		KeyVrf: PropVrfRw,
	}

	read := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		row, err := lookupIPService(d.Id(), m.(Client))
		if errors.Is(err, errorNoLongerExists) {
			d.SetId("")
			return nil
		}
		if err != nil {
			return diag.FromErr(err)
		}
		return readIPService(row, resSchema, d)
	}
	resCreateUpdate := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		row, err := lookupIPService(d.Get("numbers").(string), m.(Client))
		if err != nil {
			return diag.FromErr(err)
		}
		item, metadata := TerraformResourceDataToMikrotik(resSchema, d)
		delete(item, "numbers")
		// Target the static record's internal ID so active connections with the
		// same name cannot become the write target. State remains name-based.
		if _, err := UpdateItem(&ItemId{Id, row[".id"]}, metadata.Path, item, m.(Client)); err != nil {
			return diag.FromErr(err)
		}
		d.SetId(row["name"])
		return read(ctx, d, m)
	}

	importService := func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
		row, err := lookupIPService(d.Id(), m.(Client))
		if err != nil {
			return nil, err
		}
		if diags := readIPService(row, resSchema, d); diags.HasError() {
			return nil, fmt.Errorf("read imported IP service: %v", diags)
		}
		return []*schema.ResourceData{d}, nil
	}

	return &schema.Resource{
		CreateContext: resCreateUpdate,
		ReadContext:   read,
		UpdateContext: resCreateUpdate,
		DeleteContext: DefaultSystemDelete(resSchema),

		Importer: &schema.ResourceImporter{
			StateContext: importService,
		},

		Schema: resSchema,
	}
}

func lookupIPService(selector string, client Client) (MikrotikItem, error) {
	field, value := "name", selector
	if strings.HasPrefix(selector, "*") {
		field = ".id"
	} else if key, selected, ok := strings.Cut(selector, "="); ok {
		field, value = SnakeToKebab(key), selected
	}
	if value == "" || !slices.Contains([]string{".id", "name", "port", "address", "certificate", "disabled", "vrf", "max-sessions", "tls-version", "proto"}, field) {
		return nil, fmt.Errorf("invalid IP service import selector %q", selector)
	}
	rows, err := ReadItemsFiltered([]string{field + "=" + url.QueryEscape(value)}, "/ip/service", client)
	if err != nil {
		return nil, fmt.Errorf("find IP service: %w", err)
	}
	var selected MikrotikItem
	for _, row := range *rows {
		// Older RouterOS versions omit dynamic. Missing means a static service.
		if row["dynamic"] == "true" || row["dynamic"] == "yes" {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("more than one static IP service matches %q", selector)
		}
		selected = row
	}
	if selected == nil {
		return nil, fmt.Errorf("IP service %q: %w", selector, errorNoLongerExists)
	}
	if selected[".id"] == "" || !slices.Contains(ipServiceNames, selected["name"]) {
		return nil, fmt.Errorf("unsupported IP service returned for %q", selector)
	}
	return selected, nil
}

func readIPService(row MikrotikItem, properties map[string]*schema.Schema, d *schema.ResourceData) diag.Diagnostics {
	d.SetId(row["name"])
	if err := d.Set("numbers", row["name"]); err != nil {
		return diag.FromErr(err)
	}
	return MikrotikResourceDataToTerraform(row, properties, d)
}
