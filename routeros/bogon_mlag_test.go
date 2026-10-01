package routeros

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBogonMLAGLifecycle(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	for _, version := range []string{"7.21.5", "7.22.3", "7.23.7", "7.24.5"} {
		t.Run(version, func(t *testing.T) {
			RouterOSVersion = version
			modern := version != "7.21.5"
			row := map[string]string{".id": "*B", "name": "bridge-site", "mlag-peer-port": "none", "mlag-priority": "128", "mlag-heartbeat": "5s"}
			legacy := map[string]string{"bridge": "none", "peer-port": "none", "priority": "128", "heartbeat": "5s"}
			missing, failWrite := false, false
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case modern && r.Method == http.MethodGet && r.URL.Path == "/rest/interface/bridge":
					rows := []map[string]string{}
					if !missing {
						rows = append(rows, row)
					}
					_ = json.NewEncoder(w).Encode(rows)
				case !modern && r.Method == http.MethodGet && r.URL.Path == "/rest/interface/bridge/mlag":
					_ = json.NewEncoder(w).Encode(legacy)
				case modern && r.Method == http.MethodPatch && r.URL.Path == "/rest/interface/bridge/*B",
					!modern && r.Method == http.MethodPost && r.URL.Path == "/rest/interface/bridge/mlag/set":
					if failWrite {
						w.WriteHeader(http.StatusBadRequest)
						_ = json.NewEncoder(w).Encode(map[string]any{"error": 400, "message": "Bad Request", "detail": "write rejected"})
						return
					}
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					allowed := map[string]bool{"bridge": true, "peer-port": true, "priority": true, "heartbeat": true}
					target := legacy
					if modern {
						allowed = map[string]bool{"mlag-peer-port": true, "mlag-priority": true, "mlag-heartbeat": true}
						target = row
					}
					for key, value := range body {
						if !allowed[key] {
							t.Errorf("MLAG wrote unrelated property %q", key)
							w.WriteHeader(400)
							return
						}
						target[key] = value
					}
					writes++
					_ = json.NewEncoder(w).Encode(target)
				default:
					t.Errorf("unexpected MLAG request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": 400, "message": "Bad Request", "detail": "unexpected operation"})
				}
			}))
			defer server.Close()
			client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client(), extra: &ExtraParams{}}
			res := ResourceInterfaceBridgeMlag()
			d := bogonResourceData(t, res, map[string]interface{}{"bridge": "bridge-site", "peer_port": "bond-peer", "heartbeat": "1s"})
			_ = d.Set("priority", 50)
			if diags := res.CreateContext(context.Background(), d, client); diags.HasError() {
				t.Fatal(diags)
			}
			if d.Id() == "" || d.Get("peer_port") != "bond-peer" || d.Get("priority") != 50 {
				t.Fatalf("MLAG create state: id=%q peer=%v priority=%v", d.Id(), d.Get("peer_port"), d.Get("priority"))
			}
			if modern {
				row["mlag-priority"] = "120"
			} else {
				legacy["priority"] = "120"
			}
			if diags := res.ReadContext(context.Background(), d, client); diags.HasError() {
				t.Fatal(diags)
			}
			if d.Get("priority") != 120 {
				t.Fatal("MLAG refresh did not observe out-of-band drift")
			}
			_ = d.Set("priority", 50)
			_ = d.Set("peer_port", "bond-next")
			if diags := res.UpdateContext(context.Background(), d, client); diags.HasError() {
				t.Fatal(diags)
			}
			if d.Get("peer_port") != "bond-next" || d.Get("priority") != 50 {
				t.Fatal("MLAG update did not correct drift")
			}
			failWrite = true
			if diags := res.DeleteContext(context.Background(), d, client); !diags.HasError() || d.Id() == "" {
				t.Fatal("failed teardown discarded managed MLAG state")
			}
			failWrite = false
			// Destroy carries a null raw configuration in the SDK.
			previousData := d
			d = res.Data(&terraform.InstanceState{RawConfig: cty.NullVal(res.CoreConfigSchema().ImpliedType())})
			d.SetId(previousData.Id())
			for _, key := range []string{"bridge", "peer_port", "priority", "heartbeat"} {
				if err := d.Set(key, previousData.Get(key)); err != nil {
					t.Fatal(err)
				}
			}
			if diags := res.DeleteContext(context.Background(), d, client); diags.HasError() {
				t.Fatal(diags)
			}
			if d.Id() != "" || writes != 3 {
				t.Fatalf("MLAG teardown: id=%q writes=%d", d.Id(), writes)
			}
			if modern && (row["mlag-peer-port"] != "none" || row["name"] != "bridge-site") {
				t.Fatal("MLAG teardown did not preserve and disable the bridge")
			}
			if !modern && legacy["peer-port"] != "none" {
				t.Fatal("legacy teardown did not disable MLAG")
			}
			if modern {
				d.SetId("*B")
				missing = true
				if diags := res.ReadContext(context.Background(), d, client); diags.HasError() || d.Id() != "" {
					t.Fatal("missing bridge did not clear MLAG state")
				}
			}
		})
	}
}

func TestBogonMLAGCreateRetainsOwnershipOnRefreshFailure(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	for _, version := range []string{"7.21.5", "7.24.5"} {
		t.Run(version, func(t *testing.T) {
			RouterOSVersion = version
			written := false
			row := MikrotikItem{".id": "*B", "name": "bridge-site", "mlag-peer-port": "none", "mlag-priority": "128", "mlag-heartbeat": "5s"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					if written {
						w.WriteHeader(400)
						_ = json.NewEncoder(w).Encode(map[string]any{"error": 400, "message": "Bad Request", "detail": "refresh unavailable"})
						return
					}
					_ = json.NewEncoder(w).Encode([]MikrotikItem{row})
					return
				}
				written = true
				_ = json.NewEncoder(w).Encode(row)
			}))
			defer server.Close()
			c := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client(), extra: &ExtraParams{}}
			res := ResourceInterfaceBridgeMlag()
			d := bogonResourceData(t, res, map[string]interface{}{"bridge": "bridge-site", "peer_port": "bond-peer", "heartbeat": "5s"})
			_ = d.Set("priority", 50)
			if diags := res.CreateContext(context.Background(), d, c); !diags.HasError() {
				t.Fatal("expected failed refresh")
			}
			if !written || d.Id() == "" {
				t.Fatal("successful MLAG write lost ownership on failed refresh")
			}
		})
	}
}
