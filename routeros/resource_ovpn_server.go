package routeros

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

/*
  {
    "auth": "sha1,md5,sha256,sha512",
    "certificate": "root-cert",
    "cipher": "blowfish128,aes128-cbc",
    "default-profile": "default",
    "enable-tun-ipv6": "false",
    "enabled": "true",
    "ipv6-prefix-len": "64",
    "keepalive-timeout": "60",
    "mac-address": "FE:01:63:24:35:19",
    "max-mtu": "1500",
    "mode": "ip",
    "netmask": "24",
    "port": "1194",
    "protocol": "tcp",
    "redirect-gateway": "disabled",
    "reneg-sec": "3600",
    "require-client-certificate": "true",
    "tls-version": "only-1.2",
    "tun-server-ipv6": "::"
  }
*/

// https://help.mikrotik.com/docs/display/ROS/OpenVPN
func ResourceOpenVPNServer() *schema.Resource {
	resSchema := map[string]*schema.Schema{
		MetaResourcePath: PropResourcePath("/interface/ovpn-server/server"),
		MetaId:           PropId(Id),

		"auth": {
			Type:     schema.TypeSet,
			Optional: true,
			Elem: &schema.Schema{
				Type:         schema.TypeString,
				ValidateFunc: validation.StringInSlice([]string{"md5", "sha1", "null", "sha256", "sha512"}, false),
			},
			Description:      "Authentication methods that the server will accept.",
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		"certificate": {
			Type:             schema.TypeString,
			Optional:         true,
			Description:      "Name of the certificate that the OVPN server will use.",
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		"cipher": {
			Type:     schema.TypeSet,
			Optional: true,
			Elem: &schema.Schema{
				Type: schema.TypeString,
				ValidateFunc: validation.StringInSlice([]string{
					"null", "aes128-cbc", "aes128-gcm", "aes192-cbc", "aes192-gcm", "aes256-cbc", "aes256-gcm", "blowfish128",
					// Backward compatibility with ROS v7.7
					"aes128", "aes192", "aes256",
				}, false),
			},
			Description:      `Allowed ciphers.`,
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		"default_profile": {
			Type:        schema.TypeString,
			Optional:    true,
			Default:     "default",
			Description: "Default profile to use.",
		},
		"enable_tun_ipv6": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     false,
			Description: "Specifies if IPv6 IP tunneling mode should be possible with this OVPN server.",
		},
		KeyEnabled: {
			Type: schema.TypeBool, Optional: true, Computed: true,
			Description:   "Compatibility alias for the inverse of disabled. Cannot be combined with disabled.",
			ConflictsWith: []string{KeyDisabled},
		},
		KeyDisabled: {
			Type: schema.TypeBool, Optional: true, Computed: true,
			Description:   "Whether the server is disabled. A new server defaults to disabled when neither enabled nor disabled is configured.",
			ConflictsWith: []string{KeyEnabled},
		},
		KeyName: {
			Type: schema.TypeString, Optional: true, Computed: true,
			Description:  "Server name. Available on RouterOS 7.17 and newer, which support multiple servers.",
			ValidateFunc: validation.StringIsNotWhiteSpace,
		},
		KeyVrf: {
			Type: schema.TypeString, Optional: true, Computed: true,
			Description:  "VRF in which the server listens. Available on RouterOS 7.17 and newer.",
			ValidateFunc: validation.StringIsNotWhiteSpace,
		},
		KeyInactive: {
			Type: schema.TypeBool, Computed: true,
			Description: "Whether this server is inactive.",
		},
		"ipv6_prefix_len": {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  64,
			Description: "Length of IPv6 prefix for IPv6 address which will be used when generating OVPN interface " +
				"on the server side.",
			ValidateFunc: validation.IntBetween(1, 128),
		},
		"keepalive_timeout": {
			Type:     schema.TypeString,
			Optional: true,
			Default:  "60",
			Description: "Defines  the time period (in seconds) after which the router is starting to send  " +
				"keepalive packets every second. If no traffic and no keepalive  responses have come for " +
				"that period of time (i.e. 2 *  keepalive-timeout), not responding client is proclaimed " +
				"disconnected",
			DiffSuppressFunc: TimeEqual,
		},
		// Computed only???
		"mac_address": {
			Type:        schema.TypeString,
			Optional:    true,
			Computed:    true,
			Description: "Automatically generated MAC address of the server.",
		},
		"max_mtu": {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  1500,
			Description: "Maximum Transmission Unit. Max packet size that the OVPN interface will be able to send " +
				"without packet fragmentation.",
			ValidateFunc: validation.IntBetween(64, 65535),
		},
		"mode": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "ip",
			Description:  "Layer3 or layer2 tunnel mode (alternatively tun, tap)",
			ValidateFunc: validation.StringInSlice([]string{"ip", "ethernet"}, false),
		},
		"netmask": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      24,
			Description:  "Subnet mask to be applied to the client.",
			ValidateFunc: validation.IntBetween(0, 32),
		},
		"port": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      1194,
			Description:  "Port to run the server on.",
			ValidateFunc: validation.IntBetween(1, 65535),
		},
		"protocol": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "tcp",
			Description:  "indicates the protocol to use when connecting with the remote endpoint.",
			ValidateFunc: validation.StringInSlice([]string{"tcp", "udp"}, false),
		},
		"push_routes": {
			Type:             schema.TypeSet,
			Optional:         true,
			Elem:             &schema.Schema{Type: schema.TypeString},
			Description:      "Push routes to the VPN client (available since RouterOS 7.14).",
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		"redirect_gateway": {
			Type:     schema.TypeSet,
			Optional: true,
			Elem: &schema.Schema{
				Type:         schema.TypeString,
				ValidateFunc: validation.StringInSlice([]string{"def1", "disabled", "ipv6"}, false),
			},
			Description: "Specifies what kind of routes the OVPN client must add to the routing table.\n  * def1 – Use " +
				"this flag to override the default gateway by using 0.0.0.0/1 and  128.0.0.0/1 rather " +
				"than 0.0.0.0/0. This has the benefit of overriding  but not wiping out the original " +
				"default gateway.\n  * disabled - Do not send redirect-gateway flags to the OVPN client.\n  * ipv6 " +
				"- Redirect IPv6 routing into the tunnel on the client side. This works  similarly to the " +
				"def1 flag, that is, more specific IPv6 routes are added  (2000::/4 and 3000::/4), " +
				"covering the whole IPv6 unicast space.",
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		"reneg_sec": {
			Type:        schema.TypeInt,
			Optional:    true,
			Default:     3600,
			Description: "Renegotiate data channel key after n seconds (default=3600).",
		},
		"require_client_certificate": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
			Description: "If set to yes, then the server checks whether the client's certificate belongs to the " +
				"same certificate chain.",
		},
		"tls_version": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "any",
			Description:  "Specifies which TLS versions to allow.",
			ValidateFunc: validation.StringInSlice([]string{"any", "only-1.2"}, false),
		},
		"tun_server_ipv6": {
			Type:     schema.TypeString,
			Optional: true,
			Default:  "::",
			Description: "IPv6 prefix address which will be used when generating the OVPN interface on the server " +
				"side.",
		},
	}

	return &schema.Resource{
		Description:   "Manages OpenVPN server configuration on RouterOS 7.8 and newer. RouterOS 7.17 and newer use named server entries with native IDs and ordinary CRUD. Older versions have a singleton whose deletion only removes it from state.",
		CreateContext: openVPNServerWrite(resSchema, true),
		ReadContext:   openVPNServerRead(resSchema),
		UpdateContext: openVPNServerWrite(resSchema, false),
		DeleteContext: func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
			modern, err := routerOSVersionAtLeast("7.17")
			if err != nil {
				return diag.FromErr(err)
			}
			if !modern {
				return SystemResourceDelete(ctx, resSchema, d, m)
			}
			if err := validateOpenVPNServerID(d.Id()); err != nil {
				return diag.FromErr(err)
			}
			return ResourceDelete(ctx, resSchema, d, m)
		},

		Importer: &schema.ResourceImporter{
			StateContext: openVPNServerImport(resSchema),
		},

		Schema:        resSchema,
		SchemaVersion: 1,
		StateUpgraders: []schema.StateUpgrader{
			{
				Type:    ResourceOpenVPNServerV0().CoreConfigSchema().ImpliedType(),
				Upgrade: stateMigrationScalarToList("auth", "cipher", "redirect_gateway"),
				Version: 0,
			},
		},
	}
}

const legacyOpenVPNServerID = "interface.ovpn-server.server"

func openVPNServerConfigured(d *schema.ResourceData, key string) bool {
	config := d.GetRawConfig()
	return !config.IsNull() && config.IsKnown() && config.Type().IsObjectType() &&
		config.Type().HasAttribute(key) && !config.GetAttr(key).IsNull()
}

// Keep the public schema stable while selecting the native properties after
// provider configuration has discovered the router's version.
func openVPNServerNativeSchema(properties map[string]*schema.Schema, modern bool) map[string]*schema.Schema {
	native := make(map[string]*schema.Schema, len(properties)+1)
	for key, value := range properties {
		native[key] = value
	}
	if modern {
		native[MetaSkipFields] = PropSkipFields(KeyEnabled)
	} else {
		native[MetaSkipFields] = PropSkipFields(KeyDisabled, KeyName, KeyVrf, KeyInactive)
	}
	return native
}

func validateOpenVPNServerID(id string) error {
	if !strings.HasPrefix(id, "*") {
		return fmt.Errorf("OpenVPN server ID %q does not identify a RouterOS 7.17+ server entry; import the existing server by its native ID or name", id)
	}
	return nil
}

func openVPNServerHydrate(row MikrotikItem, native map[string]*schema.Schema, d *schema.ResourceData, modern bool) diag.Diagnostics {
	diags := MikrotikResourceDataToTerraform(row, native, d)
	if diags.HasError() {
		return diags
	}
	if modern {
		if disabled, present := row[KeyDisabled]; present {
			if err := d.Set(KeyEnabled, !BoolFromMikrotikJSON(disabled)); err != nil {
				return append(diags, diag.FromErr(err)...)
			}
		}
	} else if enabled, present := row[KeyEnabled]; present {
		if err := d.Set(KeyDisabled, !BoolFromMikrotikJSON(enabled)); err != nil {
			return append(diags, diag.FromErr(err)...)
		}
	}
	return diags
}

func openVPNServerRead(properties map[string]*schema.Schema) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		modern, err := routerOSVersionAtLeast("7.17")
		if err != nil {
			return diag.FromErr(err)
		}
		native := openVPNServerNativeSchema(properties, modern)
		path := native[MetaResourcePath].Default.(string)
		if !modern {
			row := MikrotikItem{}
			if err := m.(Client).SendRequest(crudRead, &URL{Path: path}, nil, &row); err != nil {
				return diag.FromErr(err)
			}
			d.SetId(legacyOpenVPNServerID)
			return openVPNServerHydrate(row, native, d, false)
		}
		if err := validateOpenVPNServerID(d.Id()); err != nil {
			return diag.FromErr(err)
		}
		rows, err := ReadItems(&ItemId{Id, d.Id()}, path, m.(Client))
		if err != nil {
			return diag.FromErr(err)
		}
		if len(*rows) == 0 {
			d.SetId("")
			return nil
		}
		return openVPNServerHydrate((*rows)[0], native, d, true)
	}
}

func openVPNServerWrite(properties map[string]*schema.Schema, create bool) func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		modern, err := routerOSVersionAtLeast("7.17")
		if err != nil {
			return diag.FromErr(err)
		}
		if openVPNServerConfigured(d, KeyEnabled) && openVPNServerConfigured(d, KeyDisabled) {
			return diag.Errorf("configure either enabled or disabled for the OpenVPN server, not both")
		}
		if !modern && (openVPNServerConfigured(d, KeyName) || openVPNServerConfigured(d, KeyVrf)) {
			return diag.Errorf("OpenVPN server name and VRF require RouterOS 7.17 or newer")
		}
		if modern && !create {
			if err := validateOpenVPNServerID(d.Id()); err != nil {
				return diag.FromErr(err)
			}
		}
		native := openVPNServerNativeSchema(properties, modern)
		item, metadata := TerraformResourceDataToMikrotik(native, d)
		enabled := d.Get(KeyEnabled).(bool)
		if openVPNServerConfigured(d, KeyDisabled) || (modern && !openVPNServerConfigured(d, KeyEnabled) && !create) {
			enabled = !d.Get(KeyDisabled).(bool)
		}
		if modern {
			item[KeyDisabled] = BoolToMikrotikJSON(!enabled)
		} else {
			item[KeyEnabled] = BoolToMikrotikJSON(enabled)
			if err := m.(Client).SendRequest(crudPost, &URL{Path: metadata.Path + "/set"}, item, nil); err != nil {
				return diag.FromErr(err)
			}
			// The write succeeded even if the following refresh fails.
			d.SetId(legacyOpenVPNServerID)
			return openVPNServerRead(properties)(ctx, d, m)
		}
		var row MikrotikItem
		if create {
			row, err = CreateItem(ctx, item, metadata.Path, m.(Client))
		} else {
			row, err = UpdateItem(&ItemId{Id, d.Id()}, metadata.Path, item, m.(Client))
		}
		if err != nil {
			return diag.FromErr(err)
		}
		if create {
			if row.GetID(Id) == "" {
				return diag.Errorf("OpenVPN server creation did not return a native ID")
			}
			d.SetId(row.GetID(Id))
		}
		return openVPNServerHydrate(row, native, d, true)
	}
}

func openVPNServerImport(properties map[string]*schema.Schema) schema.StateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
		modern, err := routerOSVersionAtLeast("7.17")
		if err != nil {
			return nil, err
		}
		if !modern {
			if d.Id() != "." && d.Id() != legacyOpenVPNServerID {
				return nil, fmt.Errorf("legacy OpenVPN singleton import requires . or %s", legacyOpenVPNServerID)
			}
			if diags := openVPNServerRead(properties)(ctx, d, m); diags.HasError() {
				return nil, fmt.Errorf("read imported OpenVPN singleton: %v", diags)
			}
			return []*schema.ResourceData{d}, nil
		}
		field, value := "name", d.Id()
		if strings.HasPrefix(value, "*") {
			field = ".id"
		} else if key, selected, ok := strings.Cut(value, "="); ok {
			field, value = key, selected
		}
		if value == "" || (field != "name" && field != ".id") || value == legacyOpenVPNServerID || value == "." {
			return nil, fmt.Errorf("import an OpenVPN server by native ID, name or name=server-name")
		}
		rows, err := ReadItemsFiltered([]string{field + "=" + url.QueryEscape(value)}, properties[MetaResourcePath].Default.(string), m.(Client))
		if err != nil {
			return nil, fmt.Errorf("find OpenVPN server: %w", err)
		}
		if len(*rows) != 1 {
			return nil, fmt.Errorf("OpenVPN import %q matched %d servers; use a unique native ID or name", d.Id(), len(*rows))
		}
		row := (*rows)[0]
		if err := validateOpenVPNServerID(row.GetID(Id)); err != nil {
			return nil, err
		}
		d.SetId(row.GetID(Id))
		if diags := openVPNServerHydrate(row, openVPNServerNativeSchema(properties, true), d, true); diags.HasError() {
			return nil, fmt.Errorf("read imported OpenVPN server: %v", diags)
		}
		return []*schema.ResourceData{d}, nil
	}
}
