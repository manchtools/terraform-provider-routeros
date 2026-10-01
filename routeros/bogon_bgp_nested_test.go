package routeros

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestBogonOmittedNestedBGPBlocks(t *testing.T) {
	for _, res := range []*schema.Resource{ResourceRoutingBgpConnection(), ResourceRoutingBgpTemplate()} {
		for _, state := range []string{"empty", "inherited"} {
			t.Run(res.Schema[MetaResourcePath].Default.(string)+"/"+state, func(t *testing.T) {
				d := bogonResourceData(t, res, map[string]interface{}{"name": "probe", "as": "64512", "comment": "unrelated edit"})
				if state == "inherited" {
					if err := d.Set("output", []interface{}{map[string]interface{}{"filter_chain": "inherited"}}); err != nil {
						t.Fatal(err)
					}
				}
				payload, _ := TerraformResourceDataToMikrotik(res.Schema, d)
				if payload["comment"] != "unrelated edit" {
					t.Fatal("comment update was lost")
				}
				if _, ok := payload["output.filter-chain"]; ok {
					t.Fatal("omitted inherited block was replayed as a direct override")
				}
			})
		}
	}
}

func TestBogonBGPAbsentNestedBlockRefresh(t *testing.T) {
	for _, res := range []*schema.Resource{ResourceRoutingBgpConnection(), ResourceRoutingBgpTemplate()} {
		t.Run(res.Schema[MetaResourcePath].Default.(string), func(t *testing.T) {
			d := bogonResourceData(t, res, map[string]interface{}{"name": "probe", "as": "64512"})
			if err := d.Set("output", []interface{}{map[string]interface{}{"filter_chain": "previous"}}); err != nil {
				t.Fatal(err)
			}
			d.SetId("*1")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`[{".id":"*1","name":"probe","as":"64512"}]`))
			}))
			defer server.Close()
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
			if diags := res.ReadContext(context.Background(), d, client); diags.HasError() {
				t.Fatal(diags)
			}
			if len(d.Get("output").([]interface{})) != 0 {
				t.Fatal("removed native BGP block retained in state")
			}
		})
	}
}

func TestBogonExplicitNestedBGPBlock(t *testing.T) {
	res := ResourceRoutingBgpConnection()
	base := bogonResourceData(t, res, map[string]interface{}{"name": "probe", "as": "64512"})
	attrs := base.GetRawConfig().AsValueMap()
	ty := attrs["output"].Type().ElementType()
	block := map[string]cty.Value{}
	for name, fieldType := range ty.AttributeTypes() {
		block[name] = cty.NullVal(fieldType)
	}
	block["filter_chain"] = cty.StringVal("owned")
	attrs["output"] = cty.ListVal([]cty.Value{cty.ObjectVal(block)})
	d := res.Data(&terraform.InstanceState{RawConfig: cty.ObjectVal(attrs)})
	if err := d.Set("output", []interface{}{map[string]interface{}{"filter_chain": "owned"}}); err != nil {
		t.Fatal(err)
	}
	payload, _ := TerraformResourceDataToMikrotik(res.Schema, d)
	if payload["output.filter-chain"] != "owned" {
		t.Fatal("explicit BGP output configuration was dropped")
	}
}
