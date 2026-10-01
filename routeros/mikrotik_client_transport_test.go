package routeros

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestRESTClientLiveRead(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run live RouterOS REST tests")
	}
	testAccPreCheck(t)
	d := schema.TestResourceDataRaw(t, Provider().Schema, map[string]interface{}{})
	client, diags := NewClient(context.Background(), d)
	if diags.HasError() {
		t.Fatal(diags)
	}
	rows, err := ReadItems(nil, "/system/resource", client.(Client))
	if err != nil {
		t.Fatal(err)
	}
	if len(*rows) != 1 || (*rows)[0]["version"] == "" {
		t.Fatalf("missing RouterOS system response: %v", rows)
	}
}
