package routeros

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonFirewallClearedSelectors(t *testing.T) {
	resources := []*schema.Resource{
		ResourceIPFirewallFilter(), ResourceIPFirewallMangle(), ResourceIPFirewallNat(), ResourceIPFirewallRaw(),
		ResourceIPv6FirewallFilter(), ResourceIPv6FirewallMangle(), ResourceIPv6FirewallNat(),
	}
	for _, res := range resources {
		path := res.Schema[MetaResourcePath].Default.(string)
		for _, outcome := range []string{"absent", "present", "read error"} {
			t.Run(path+"/"+outcome, func(t *testing.T) {
				config := map[string]interface{}{"action": "accept", "chain": "bogon-probe", "log_prefix": "keep"}
				fields := loadSkipFields(res.Schema[MetaSetUnsetFields].Default.(string))
				for _, field := range []string{"src_address", "dst_address", "in_interface", "out_interface", "protocol"} {
					fields[field] = struct{}{}
				}
				for field := range fields {
					if res.Schema[field].Type == schema.TypeString {
						config[field] = "previous"
					}
				}
				d := bogonResourceData(t, res, config)
				d.SetId("*1")
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet || r.URL.Path != "/rest"+path {
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					}
					if outcome == "read error" {
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte(`{"error":500,"message":"probe failure"}`))
						return
					}
					row := MikrotikItem{".id": "*1", "action": "accept", "chain": "bogon-probe"}
					if outcome == "present" {
						for field := range fields {
							if res.Schema[field].Type == schema.TypeString {
								row[SnakeToKebab(field)] = "native"
							}
						}
					}
					json.NewEncoder(w).Encode([]MikrotikItem{row})
				}))
				defer server.Close()
				client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
				diags := res.ReadContext(context.Background(), d, client)
				if diags.HasError() != (outcome == "read error") {
					t.Fatalf("unexpected read diagnostics: %v", diags)
				}
				want := map[string]string{"absent": "", "present": "native", "read error": "previous"}[outcome]
				for field := range fields {
					if res.Schema[field].Type == schema.TypeString && d.Get(field) != want {
						t.Errorf("%s=%v, want %q", field, d.Get(field), want)
					}
				}
				if d.Get("log_prefix") != "keep" || d.Id() != "*1" {
					t.Fatal("refresh cleared unrelated fields or resource identity")
				}
			})
		}
	}
}
