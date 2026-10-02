package main

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestGeneratedProviderUsesFork(t *testing.T) {
	configuration := fmt.Sprintf(providerTemplate, "router.example.test", "admin")
	file, diags := hclsyntax.ParseConfig([]byte(configuration), "generated.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	body := file.Body.(*hclsyntax.Body)
	for _, block := range body.Blocks {
		if block.Type != "terraform" {
			continue
		}
		for _, required := range block.Body.Blocks {
			if required.Type != "required_providers" {
				continue
			}
			provider, diags := required.Body.Attributes["routeros"].Expr.Value(nil)
			if diags.HasErrors() {
				t.Fatal(diags.Error())
			}
			if source := provider.GetAttr("source").AsString(); source != "manchtools/routeros" {
				t.Fatalf("generated configuration selects provider %q instead of the fork", source)
			}
			if version := provider.GetAttr("version").AsString(); version != "1.99.1-bogon.7" {
				t.Fatalf("generated constraint %q does not select the mirrored fork version", version)
			}
			return
		}
	}
	t.Fatal("generated configuration does not declare the required provider")
}

func TestGeneratedImportTargetIsResourceReference(t *testing.T) {
	for _, resource := range []string{"routeros_ip_service", "routeros_ip_reverse_proxy"} {
		t.Run(resource, func(t *testing.T) {
			configuration := fmt.Sprintf(importTemplate, resource, "existing", "*4")
			file, diags := hclsyntax.ParseConfig([]byte(configuration), "generated.tf", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatal(diags.Error())
			}
			block := file.Body.(*hclsyntax.Body).Blocks[0]
			target, diags := hcl.AbsTraversalForExpr(block.Body.Attributes["to"].Expr)
			if diags.HasErrors() {
				t.Fatalf("generated import target cannot identify a resource: %s", diags.Error())
			}
			if len(target) != 2 || target.RootName() != resource || target[1].(hcl.TraverseAttr).Name != "existing" {
				t.Fatalf("generated import targets an unexpected resource: %#v", target)
			}
			id, diags := block.Body.Attributes["id"].Expr.Value(nil)
			if diags.HasErrors() {
				t.Fatal(diags.Error())
			}
			if id.AsString() != "*4" {
				t.Fatalf("generated import has incorrect native ID: %s", id.AsString())
			}
		})
	}
}
