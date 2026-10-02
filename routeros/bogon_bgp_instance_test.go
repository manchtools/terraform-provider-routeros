package routeros

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonBGPInstanceValidation(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	for _, resource := range []struct {
		name string
		res  *schema.Resource
	}{
		{"connection", ResourceRoutingBgpConnection()},
		{"vpn", ResourceRoutingBgpVpn()},
	} {
		if field := resource.res.Schema["instance"]; !field.Optional || field.Required {
			t.Fatalf("%s requires an instance before the firmware version is known", resource.name)
		}
		for _, tc := range []struct {
			name, version, instance string
			templates               []string
			blocked                 bool
			nativeFailure           bool
		}{
			{name: "legacy missing instance", version: "7.19.6"},
			{name: "modern missing instance", version: "7.20", blocked: true},
			{name: "latest missing instance", version: "7.24.5", blocked: true},
			{name: "explicit instance", version: "7.24.5", instance: "owned"},
			{name: "empty instance", version: "7.24.5", instance: " ", blocked: true},
			{name: "empty templates", version: "7.24.5", templates: []string{}, blocked: true},
			{name: "inherited instance", version: "7.24.5", templates: []string{"parent"}},
			{name: "unresolved template", version: "7.24.5", templates: []string{"missing"}, nativeFailure: true},
			{name: "native write failure", version: "7.24.5", instance: "owned", nativeFailure: true},
			{name: "invalid version", version: "invalid", instance: "owned", blocked: true},
			{name: "unknown version", instance: "owned", blocked: true},
		} {
			if resource.name == "vpn" && len(tc.templates) != 0 {
				continue
			}
			for _, operation := range []string{"create", "update"} {
				t.Run(resource.name+"/"+tc.name+"/"+operation, func(t *testing.T) {
					RouterOSVersion = tc.version
					values := map[string]interface{}{"name": "probe"}
					if resource.name == "connection" {
						values["as"] = "64512"
						if tc.templates != nil {
							values["templates"] = tc.templates
						}
					} else {
						values["route_distinguisher"] = "64512:1"
					}
					if tc.instance != "" {
						values["instance"] = tc.instance
					}
					d := bogonResourceData(t, resource.res, values)
					if operation == "update" {
						d.SetId("*1")
					}
					priorID := d.Id()
					requests, writes := 0, 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests++
						row := MikrotikItem{".id": "*1", "name": "probe"}
						if r.Method == http.MethodGet {
							json.NewEncoder(w).Encode([]MikrotikItem{row})
							return
						}
						writes++
						var payload MikrotikItem
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Error(err)
						}
						if tc.name == "inherited instance" && payload["templates"] != "parent" {
							t.Errorf("template inheritance was lost: %v", payload)
						}
						if tc.nativeFailure {
							w.WriteHeader(http.StatusBadRequest)
							w.Write([]byte(`{"error":400,"message":"instance rejected"}`))
							return
						}
						json.NewEncoder(w).Encode(row)
					}))
					defer server.Close()
					client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
					fn := resource.res.CreateContext
					if operation == "update" {
						fn = schema.CreateContextFunc(resource.res.UpdateContext)
					}
					diags := fn(context.Background(), d, client)
					if tc.blocked {
						if !diags.HasError() || requests != 0 || d.Id() != priorID {
							t.Fatalf("invalid operation reached RouterOS or changed ownership: diags=%v requests=%d ID=%q", diags, requests, d.Id())
						}
						if !strings.Contains(diags[0].Summary, "instance") && !strings.Contains(diags[0].Summary, "version") {
							t.Fatalf("validation did not explain the missing instance/version: %v", diags)
						}
					} else if tc.nativeFailure {
						if !diags.HasError() || writes != 1 || d.Id() != priorID {
							t.Fatalf("native rejection lost resource ownership: diags=%v writes=%d ID=%q", diags, writes, d.Id())
						}
					} else if diags.HasError() || writes != 1 || d.Id() != "*1" {
						t.Fatalf("valid operation was rejected: diags=%v writes=%d ID=%q", diags, writes, d.Id())
					}
				})
			}
		}
	}
}

func TestBogonBGPObservedInstanceAndWriteFailures(t *testing.T) {
	previous := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = previous })
	RouterOSVersion = "7.24.5"
	for _, res := range []*schema.Resource{ResourceRoutingBgpConnection(), ResourceRoutingBgpVpn()} {
		for _, source := range []string{"refresh", "import"} {
			for _, failWrite := range []bool{false, true} {
				name := res.Schema[MetaResourcePath].Default.(string) + "/" + source
				if failWrite {
					name += "/failed write"
				}
				t.Run(name, func(t *testing.T) {
					row := MikrotikItem{".id": "*1", "name": "probe", "instance": "observed"}
					writes := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method == http.MethodGet {
							json.NewEncoder(w).Encode([]MikrotikItem{row})
							return
						}
						writes++
						if failWrite {
							w.WriteHeader(http.StatusBadRequest)
							w.Write([]byte(`{"error":400,"message":"instance rejected"}`))
							return
						}
						json.NewEncoder(w).Encode(row)
					}))
					defer server.Close()
					client := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
					values := map[string]interface{}{"name": "probe"}
					if _, connection := res.Schema["as"]; connection {
						values["as"] = "64512"
					} else {
						values["route_distinguisher"] = "64512:1"
					}
					d := bogonResourceData(t, res, values)
					d.SetId("*1")
					if source == "import" {
						if _, err := res.Importer.StateContext(context.Background(), d, client); err != nil {
							t.Fatal(err)
						}
					}
					if diags := res.ReadContext(context.Background(), d, client); diags.HasError() {
						t.Fatal(diags)
					}
					if d.Get("instance") != "observed" {
						t.Fatal("native instance was not observed")
					}
					diags := res.UpdateContext(context.Background(), d, client)
					if diags.HasError() != failWrite || writes != 1 || d.Id() != "*1" {
						t.Fatalf("observed instance/update ownership was lost: diags=%v writes=%d ID=%q", diags, writes, d.Id())
					}
				})
			}
		}
	}
}

func TestBogonBGPInstanceDiffPreservesObserved(t *testing.T) {
	for _, res := range []*schema.Resource{ResourceRoutingBgpConnection(), ResourceRoutingBgpVpn()} {
		t.Run(res.Schema[MetaResourcePath].Default.(string), func(t *testing.T) {
			suppress := res.Schema["instance"].DiffSuppressFunc
			omitted := bogonResourceData(t, res, map[string]interface{}{"name": "probe"})
			if suppress == nil || !suppress("instance", "observed", "", omitted) {
				t.Fatal("omitting the configured instance would clear the observed native instance")
			}
			for _, desired := range []string{"changed", ""} {
				explicit := bogonResourceData(t, res, map[string]interface{}{"name": "probe", "instance": desired})
				if suppress("instance", "observed", desired, explicit) {
					t.Fatalf("explicit instance change to %q was suppressed", desired)
				}
			}
		})
	}
}
