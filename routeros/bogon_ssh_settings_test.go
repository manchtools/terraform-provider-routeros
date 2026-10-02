package routeros

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestBogonSSHSettingsWithoutCryptoOverride(t *testing.T) {
	res := ResourceIpSSHServer()
	config := map[string]interface{}{"forwarding_enabled": "local"}
	if diags := res.Validate(terraform.NewResourceConfigRaw(config)); diags.HasError() {
		t.Fatalf("unrelated SSH setting requires a crypto override: %v", diags)
	}
	if _, ok := res.Schema["allow_none_crypto"]; ok {
		t.Fatal("removed RouterOS allow-none-crypto property remains writable")
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			if r.URL.Path != "/rest/ip/ssh/set" {
				t.Errorf("unexpected command: %s", r.URL)
			}
			var body MikrotikItem
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["forwarding-enabled"] != "local" {
				t.Errorf("forwarding setting was not written: %v", body)
			}
			if _, ok := body["allow-none-crypto"]; ok {
				t.Error("write contains removed crypto property")
			}
			if _, ok := body["strong-crypto"]; ok {
				t.Error("unconfigured crypto setting was overwritten")
			}
			requests++
			_, _ = w.Write([]byte(`[]`))
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]MikrotikItem{{"forwarding-enabled": "local", "strong-crypto": "true"}})
		default:
			t.Errorf("unexpected request method: %s", r.Method)
		}
	}))
	defer server.Close()
	client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
	d := bogonResourceData(t, res, config)
	if diags := res.CreateContext(context.Background(), d, client); diags.HasError() {
		t.Fatal(diags)
	}
	if requests != 1 || d.Id() == "" || d.Get("strong_crypto") != true {
		t.Fatalf("SSH settings did not converge: writes=%d id=%q crypto=%v", requests, d.Id(), d.Get("strong_crypto"))
	}
}
