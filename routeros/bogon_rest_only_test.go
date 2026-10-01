package routeros

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBogonRESTOnlyEndpoints(t *testing.T) {
	prior := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = prior })
	for _, tc := range []struct{ name, endpoint, want string }{
		{"HTTP", "http://router.local:8080/", "http://router.local:8080"},
		{"HTTPS", "https://router.local", "https://router.local"},
		{"default HTTPS", "router.local:8443", "https://router.local:8443"},
		{"IPv6", "[2001:db8::1]:8443", "https://[2001:db8::1]:8443"},
		{"REST suffix", "https://router.local/rest/", "https://router.local"},
		{"encoded proxy prefix", "https://router.local/proxy%2Fsegment/rest", "https://router.local/proxy%2Fsegment"},
		{"embedded credentials", "https://user:fixture@router.local", ""},
		{"query", "https://router.local?x=1", ""},
		{"fragment", "https://router.local/#x", ""},
		{"proxy prefix", "https://router.local/proxy/rest", "https://router.local/proxy"},
		{"API removed", "api://127.0.0.1:1", ""},
		{"APIs removed", "apis://127.0.0.1:1", ""},
		{"unsupported", "ftp://router.local", ""},
		{"empty host", "https://", ""},
		{"empty endpoint", "", ""},
		{"invalid port", "https://router.local:bad", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("invalid endpoint caused a panic: %v", r)
				}
			}()
			d := schema.TestResourceDataRaw(t, Provider().Schema, map[string]interface{}{
				"hosturl": tc.endpoint, "username": "fixture", "password": "fixture", "routeros_version": "7.24.2",
			})
			client, diags := NewClient(context.Background(), d)
			if tc.want == "" {
				if !diags.HasError() || client != nil {
					t.Fatalf("unsupported endpoint accepted: %v", diags)
				}
				if strings.Contains(tc.endpoint, "api") && !strings.Contains(diags[0].Summary, "http:// or https://") {
					t.Fatalf("missing REST migration diagnostic: %v", diags)
				}
				return
			}
			if diags.HasError() {
				t.Fatal(diags)
			}
			rest, ok := client.(*RestClient)
			if !ok || rest.HostURL != tc.want {
				t.Fatalf("client=%v, want REST endpoint %s", client, tc.want)
			}
		})
	}
}

func TestBogonRESTAuthenticationAndTLS(t *testing.T) {
	prior := RouterOSVersion
	t.Cleanup(func() { RouterOSVersion = prior })
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "fixture" || pass != "fixture-password" {
			t.Error("REST authentication was not preserved")
		}
		if r.Method != http.MethodGet || r.URL.Path != "/rest/system/resource" {
			t.Errorf("unexpected version request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"7.24.2 (stable)"}`))
	})
	plain := httptest.NewServer(handler)
	defer plain.Close()
	secure := httptest.NewTLSServer(handler)
	defer secure.Close()
	ca := filepath.Join(t.TempDir(), "router-ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: secure.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, endpoint, ca  string
		insecure, wantError bool
	}{
		{"HTTP version discovery", plain.URL + "/rest/", "", false, false},
		{"HTTPS rejects untrusted CA", secure.URL, "", false, true},
		{"HTTPS explicit CA", secure.URL, ca, false, false},
		{"HTTPS explicit insecure", secure.URL, "", true, false},
		{"conflicting TLS settings", secure.URL, ca, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, Provider().Schema, map[string]interface{}{"hosturl": tc.endpoint, "username": "fixture", "password": "fixture-password", "insecure": tc.insecure, "ca_certificate": tc.ca})
			_, diags := NewClient(context.Background(), d)
			if diags.HasError() != tc.wantError {
				t.Fatalf("configure diagnostics=%v, wantError=%v", diags, tc.wantError)
			}
			if !tc.wantError && RouterOSVersion != "7.24.2" {
				t.Fatalf("version discovery failed: %s", RouterOSVersion)
			}
		})
	}
}

func TestBogonRESTCRUDAndCommands(t *testing.T) {
	// Exercise the real HTTP boundary, including empty DELETE bodies and native
	// POST command replies carrying an ID in ret rather than .id.
	expected := []struct{ method, path, query string }{
		{"PUT", "/rest/interface/bridge", ""},
		{"GET", "/rest/interface/bridge", ".id=*1"},
		{"GET", "/rest/interface/bridge", "name=probe&dynamic=false"},
		{"PATCH", "/rest/interface/bridge/*1", ""},
		{"DELETE", "/rest/interface/bridge/*1", ""},
		{"POST", "/rest/ip/ipsec/key/generate-key", ""},
		{"POST", "/rest/file/remove", ""},
		{"POST", "/rest/system/script/run", ""},
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls >= len(expected) {
			t.Errorf("unexpected extra request: %s %s", r.Method, r.URL)
			w.WriteHeader(500)
			return
		}
		want := expected[calls]
		calls++
		if r.Method != want.method || r.URL.Path != want.path || r.URL.RawQuery != want.query {
			t.Errorf("request=%s %s, want %v", r.Method, r.URL, want)
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost {
			var body MikrotikItem
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			switch r.URL.Path {
			case "/rest/ip/ipsec/key/generate-key":
				if body["name"] != "probe-key" {
					t.Errorf("lost command arguments: %v", body)
				}
				w.Write([]byte(`{"ret":"*key"}`))
			case "/rest/file/remove":
				if body[".id"] != "*file" {
					t.Errorf("lost file identity: %v", body)
				}
				w.Write([]byte(`[]`))
			case "/rest/system/script/run":
				if body[".id"] != "*script" {
					t.Errorf("lost script identity: %v", body)
				}
				w.Write([]byte(`[]`))
			}
			return
		}
		w.Write([]byte(`{".id":"*1","name":"probe"}`))
	}))
	defer server.Close()
	c := &RestClient{ctx: context.Background(), HostURL: server.URL, Client: server.Client()}
	row, err := CreateItem(context.Background(), MikrotikItem{"name": "probe"}, "/interface/bridge", c)
	if err != nil || row.GetID(Id) != "*1" {
		t.Fatalf("create failed: %v %v", row, err)
	}
	rows, err := ReadItems(&ItemId{Id, "*1"}, "/interface/bridge", c)
	if err != nil || len(*rows) != 1 {
		t.Fatalf("read failed: %v %v", rows, err)
	}
	filter := []string{"name=probe", "dynamic=false"}
	if _, err := ReadItemsFiltered(filter, "/interface/bridge", c); err != nil {
		t.Fatal(err)
	}
	if filter[0] != "name=probe" || filter[1] != "dynamic=false" {
		t.Fatal("read mutated filter arguments")
	}
	if _, err := UpdateItem(&ItemId{Id, "*1"}, "/interface/bridge", MikrotikItem{"comment": "updated"}, c); err != nil {
		t.Fatal(err)
	}
	if err := DeleteItem(&ItemId{Id, "*1"}, "/interface/bridge", c); err != nil {
		t.Fatal(err)
	}
	row, err = CreateItem(ctxSetCrudMethod(context.Background(), crudGenerateKey), MikrotikItem{"name": "probe-key"}, "/ip/ipsec/key", c)
	if err != nil || row.GetID(Id) != "*key" {
		t.Fatalf("REST command ID lost: %v %v", row, err)
	}
	if diags := fileDelete(context.Background(), "*file", c); diags.HasError() {
		t.Fatal(diags)
	}
	script := ResourceSystemScript()
	d := bogonResourceData(t, script, map[string]interface{}{"name": "probe-script", "source": ":put 1"})
	d.SetId("*script")
	if diags := startScript(context.Background(), script.Schema, d, c); diags.HasError() {
		t.Fatal(diags)
	}
	if calls != len(expected) {
		t.Fatalf("got %d requests, want %d", calls, len(expected))
	}
}
