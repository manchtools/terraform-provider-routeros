package routeros

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestBogonWifiDirectProfileLifecycle(t *testing.T) {
	for _, res := range []*schema.Resource{ResourceWifiConfiguration(), ResourceWifi()} {
		path := res.Schema[MetaResourcePath].Default.(string)
		for _, action := range []string{"refresh", "create", "unrelated update", "remove inline", "remove reference"} {
			t.Run(path+"/"+action, func(t *testing.T) {
				direct := MikrotikItem{".id": "*1", "name": "probe", "datapath": "parent"}
				if action == "remove inline" || action == "remove reference" {
					direct["datapath.bridge"] = "previous-override"
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var payload map[string]interface{}
					if r.Body != nil && r.Method != http.MethodGet {
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Error(err)
						}
					}
					if r.Method == http.MethodPost && r.URL.Path == "/rest"+path+"/print" {
						query, ok := payload[".query"].([]interface{})
						if !ok || len(query) != 1 || query[0] != ".id=*1" || payload["config"] != "" {
							t.Errorf("config print missing native query array: %v", payload)
						}
						json.NewEncoder(w).Encode([]MikrotikItem{direct})
						return
					}
					if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/unset") {
						if payload["numbers"] != "*1" {
							t.Error("unset targeted incorrect item")
						}
						delete(direct, payload["value-name"].(string))
						w.Write([]byte(`[]`))
						return
					}
					if r.Method == http.MethodPatch || r.Method == http.MethodPut {
						for key, value := range payload {
							if key == "datapath.bridge" || key == "datapath.vlan-id" {
								t.Errorf("unconfigured/inherited override replayed: %s", key)
							}
							direct[key] = value.(string)
						}
					}
					row := MikrotikItem{}
					for key, value := range direct {
						row[key] = value
					}
					if _, inherited := row["datapath"]; inherited {
						if _, explicit := row["datapath.bridge"]; !explicit {
							row["datapath.bridge"] = "inherited-bridge"
						}
						row["datapath.vlan-id"] = "913"
					}
					if r.Method == http.MethodGet {
						json.NewEncoder(w).Encode([]MikrotikItem{row})
					} else {
						json.NewEncoder(w).Encode(row)
					}
				}))
				defer server.Close()
				base := bogonResourceData(t, res, map[string]interface{}{"name": "probe", "comment": "unrelated edit"})
				attrs := base.GetRawConfig().AsValueMap()
				if action != "remove reference" {
					attrs["datapath"] = cty.MapVal(map[string]cty.Value{"config": cty.StringVal("parent")})
				}
				d := res.Data(&terraform.InstanceState{RawConfig: cty.ObjectVal(attrs)})
				d.SetId("*1")
				d.Set("name", "probe")
				d.Set("comment", "unrelated edit")
				d.Set("datapath", map[string]interface{}{"config": "parent", "bridge": "previous-override", "vlan_id": "913"})
				client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
				fn := res.ReadContext
				if action == "create" {
					fn = schema.ReadContextFunc(res.CreateContext)
				} else if action != "refresh" {
					fn = schema.ReadContextFunc(res.UpdateContext)
				}
				if diags := fn(context.Background(), d, client); diags.HasError() {
					t.Fatal(diags)
				}
				got := d.Get("datapath").(map[string]interface{})
				if action == "remove reference" {
					if len(got) != 0 || direct["datapath"] != "" || direct["datapath.bridge"] != "" {
						t.Fatal("removed profile reference/override retained")
					}
				} else if len(got) != 1 || got["config"] != "parent" {
					t.Fatalf("inherited settings retained in managed map: %v", got)
				}
			})
		}
	}
}

func TestBogonWifiDirectFailuresRetainOwnership(t *testing.T) {
	for _, failure := range []string{"read", "missing ID", "unset", "create refresh"} {
		t.Run(failure, func(t *testing.T) {
			mutations := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					mutations++
					w.Write([]byte(`{".id":"*1","name":"probe"}`))
					return
				}
				if strings.HasSuffix(r.URL.Path, "/unset") {
					mutations++
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error":500,"message":"unset failure"}`))
					return
				}
				if failure == "read" || failure == "create refresh" {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error":500,"message":"read failure"}`))
					return
				}
				if failure == "missing ID" {
					w.Write([]byte(`[{"name":"probe","datapath":"parent"}]`))
					return
				}
				w.Write([]byte(`[{".id":"*1","name":"probe","datapath":"parent"}]`))
			}))
			defer server.Close()
			res := ResourceWifiConfiguration()
			d := bogonResourceData(t, res, map[string]interface{}{"name": "probe"})
			d.SetId("*1")
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
			fn := res.UpdateContext
			if failure == "create refresh" {
				d.SetId("")
				fn = schema.UpdateContextFunc(res.CreateContext)
			}
			if diags := fn(context.Background(), d, client); !diags.HasError() {
				t.Fatal("failed operation did not report an error")
			}
			if d.Id() != "*1" {
				t.Fatal("failed operation discarded ownership")
			}
			if (failure == "read" || failure == "missing ID") && mutations != 0 {
				t.Fatal("failed read triggered a mutation")
			}
		})
	}
}
