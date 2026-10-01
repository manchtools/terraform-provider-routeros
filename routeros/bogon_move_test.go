package routeros

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type bogonMoveClient struct{ calls int }

func (c *bogonMoveClient) GetExtraParams() *ExtraParams { return &ExtraParams{} }
func (c *bogonMoveClient) GetTransport() TransportType  { return TransportREST }
func (c *bogonMoveClient) SendRequest(_ crudMethod, _ *URL, _ MikrotikItem, _ interface{}) error {
	c.calls++
	return nil
}

func TestBogonMoveRejectsShortSequences(t *testing.T) {
	res := ResourceMoveItems()
	for _, sequence := range [][]string{{}, {"*1"}} {
		for _, action := range []string{"create", "update"} {
			t.Run(action+"/"+strings.Join(sequence, ","), func(t *testing.T) {
				d := bogonResourceData(t, res, map[string]interface{}{"resource_path": "/ipv6/firewall/filter", "sequence": sequence})
				client := &bogonMoveClient{}
				fn := res.CreateContext
				if action == "update" {
					fn = schema.CreateContextFunc(res.UpdateContext)
				}
				if diags := fn(context.Background(), d, client); !diags.HasError() {
					t.Fatal("invalid sequence was accepted")
				}
				if client.calls != 0 {
					t.Fatal("invalid sequence sent a native command")
				}
			})
		}
	}
}
