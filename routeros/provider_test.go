package routeros

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var testAccProvider *schema.Provider
var testAccProviderFactories map[string]func() (*schema.Provider, error)
var testNames = []string{"REST"}

var reVersion = regexp.MustCompile(`\d+`)

var providerConfig = `
provider "routeros" {
	insecure = true
}
`

func init() {
	if os.Getenv("TF_ACC") != "1" {
		RouterOSVersion = "7.24.5"
	}
	testAccProvider = Provider()
	testAccProviderFactories = map[string]func() (*schema.Provider, error){
		"routeros": func() (*schema.Provider, error) {
			return testAccProvider, nil
		},
	}
}

func testCheckMinVersion(t *testing.T, version string) bool {
	// version: 6.39.1
	var current, min uint64

	if RouterOSVersion == "" {
		RouterOSVersion = os.Getenv("ROS_VERSION")
	}

	current, err := parseRouterOSVersion(RouterOSVersion)
	if err != nil {
		t.Fatal(err)
	}

	min, err = parseRouterOSVersion(version)
	if err != nil {
		t.Fatal(err)
	}

	return current >= min
}

func testCheckMaxVersion(t *testing.T, version string) bool {
	// version: 6.39.1
	var current, max uint64

	if RouterOSVersion == "" {
		RouterOSVersion = os.Getenv("ROS_VERSION")
	}

	current, err := parseRouterOSVersion(RouterOSVersion)
	if err != nil {
		t.Fatal(err)
	}

	max, err = parseRouterOSVersion(version)
	if err != nil {
		t.Fatal(err)
	}

	return current <= max
}

func TestCheckMinVersion(t *testing.T) {
	originalVersion := RouterOSVersion
	defer func() {
		RouterOSVersion = originalVersion
	}()

	type args struct {
		current string
		min     string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"Positive #1", args{"7", "6.2.53"}, true},
		{"Positive #2", args{"7.1", "6.2.53"}, true},
		{"Positive #3", args{"7.1.35", "6.2.53"}, true},
		{"Positive #4", args{"7.1.35", "6.2"}, true},
		{"Positive #5", args{"7.1.35", "6"}, true},
		{"Positive #6", args{"7", "7"}, true},
		{"Positive #7", args{"7.1", "7.1"}, true},
		{"Positive #8", args{"7.1.53", "7.1.53"}, true},
		{"Negative #1", args{"6", "7.1.35"}, false},
		{"Negative #2", args{"6.2", "7.1.35"}, false},
		{"Negative #3", args{"6.2.53", "7.1.35"}, false},
		{"Negative #4", args{"6.2.53", "7.1"}, false},
		{"Negative #5", args{"6.2.53", "7"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RouterOSVersion = tt.args.current
			if got := testCheckMinVersion(t, tt.args.min); got != tt.want {
				t.Errorf("TestCheckMinVersion() diag got = %v, want = %v", got, tt.want)
			}
		})
	}
}

// Keep the configured REST scheme and port, including HTTP lab endpoints.
func testSetTransportEnv(t *testing.T, testName string) {
	t.Helper()
	if !strings.Contains(testName, "REST") {
		t.Fatal("The test must have the suffix REST")
	}
	if _, err := parseRouterURL(os.Getenv("ROS_HOSTURL")); err != nil {
		t.Fatal(err)
	}
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv("ROS_HOSTURL") == "" ||
		os.Getenv("ROS_USERNAME") == "" {
		t.Fatal("Environment variables (ROS_HOSTURL & ROS_USERNAME) must be set for testing")
	}

	for _, v := range Provider().ResourcesMap {
		checkResourceSchema(v.Schema, t)
	}
}

func checkResourceSchema(s map[string]*schema.Schema, t *testing.T) {
	f, ok := s[MetaResourcePath]
	if !ok {
		t.Fatalf("the schema does not contain field '%v'", MetaResourcePath)
	}
	if f.Default.(string) == "" {
		t.Fatalf("the field '%v', contains no data", MetaResourcePath)
	}
	f, ok = s[MetaId]
	if !ok {
		t.Fatalf("the schema does not contain field '%v'", MetaId)
	}
	if f.Default.(int) < 1 {
		t.Fatalf("the field '%v' is not defined", MetaId)
	}
}

func testCheckResourceDestroy(resourcePath, resourceType string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := testAccProvider.Meta().(Client)
		idType := IdType(Provider().ResourcesMap[resourceType].Schema[MetaId].Default.(int))
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			rows, err := ReadItems(&ItemId{Type: idType, Value: rs.Primary.ID}, resourcePath, client)
			if err != nil {
				return err
			}
			if len(*rows) != 0 {
				return fmt.Errorf("resource %v %s has been found", resourceType, rs.Primary.ID)
			}
		}
		return nil
	}
}

// testCheckResourceExists queries the MikroTik API and retrieves the matching resource parameters
func testCheckResourceExists(name string, resourcePath string, resource *MikrotikItem) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s: resource not found in terraform state", name)
		}

		id := rs.Primary.ID
		if id == "" {
			return fmt.Errorf("%s: no id is set", name)
		}

		resourceType := strings.Split(name, ".")[0]
		resourceSchema, ok := testAccProvider.ResourcesMap[resourceType]
		if !ok {
			return fmt.Errorf("%s: schema for '%s' resource not found", name, resourceType)
		}
		idType := IdType(resourceSchema.Schema[MetaId].Default.(int))

		client := testAccProvider.Meta().(Client)
		resources, err := ReadItems(&ItemId{Type: idType, Value: id}, resourcePath, client)
		if err != nil {
			return err
		}

		if len(*resources) == 0 {
			return fmt.Errorf("%s: resource not found", name)
		}

		if resource != nil {
			*resource = (*resources)[0]
		}

		return nil
	}
}
