# manchtools RouterOS provider

A Bogon-focused fork of [terraform-routeros/terraform-provider-routeros](https://github.com/terraform-routeros/terraform-provider-routeros), based on upstream main at `0d8c069c20a012300dfeeb96cb343ad7a5e7ebfb`.

The source address is `manchtools/routeros`. `main` is the stable default branch. Bogon builds version `1.99.1-bogon.7.1` into a local OpenTofu filesystem mirror; this fork has no registry publication or signed release pipeline. See [BOGON.md](BOGON.md) for scope, upstream contributions and verification limits.

## Purpose

This fork configures MikroTik routers through the [REST API](https://help.mikrotik.com/docs/display/ROS/REST+API) over HTTP or HTTPS. Binary `api://` and `apis://` transports were removed in `1.99.1-bogon.6`; replace those provider endpoints with the router’s REST URL. Bare hosts default to HTTPS, and a trailing `/rest` is optional. Resource names, IDs and state schemas are unchanged.
Compatibility testing is only performed within ROS version 7.x.

From version 1.0.0, the provider has been rewritten by [vaerh](https://github.com/vaerh), and their [fork](https://github.com/vaerh/terraform-provider-routeros) has now been merged. This version drastically improves adding new endpoints to the provider, enabling significantly easier development. [vaerh](https://github.com/vaerh) has been added as a maintainer to this project.

_We are not affiliated in any way with Mikrotik or the development of RouterOS_
## Using the provider

To get started with the provider, you first need to enable the REST API on your router. [You can follow the Mikrotik documentation on this](https://help.mikrotik.com/docs/display/ROS/REST+API), but the gist is to create an SSL cert (in `/system/certificates`) and enable the `www-ssl` service (in `/ip/service`) which uses that certificate. After that, include the following in your Terraform manifests:

```terraform
terraform {
  required_providers {
    routeros = {
      source = "manchtools/routeros"
    }
  }
}

provider "routeros" {
  hosturl  = "(http|https)://my.router.local[:port]"
  username = "my_username"
  password = "my_super_secret_password"
}

```

Resource and data source documentation is in [docs/](docs/). The upstream registry documentation can differ from this fork.

### Versions tested

- Go 1.26.6 or newer; isolated REST lifecycle contracts cover RouterOS 7.21.5, 7.22.3, 7.23.7 and 7.24.5. [Live verification](LIVE_VERIFICATION.md) covers the additional fixes on RouterOS 7.24.2. Physical MLAG and radio traffic require hardware verification.

## Changelog

For a detailed changelog, please see the [changelog.md](CHANGELOG.md).

## Contributing
This version of the module greatly simplifies the process of adding new resources.
You are welcome!

### Testing

Run the offline provider checks with Go 1.26.6 or newer:

```sh
make test
go test -mod=readonly -race ./...
go vet ./...
```

Live acceptance tests require an isolated RouterOS device and `TF_ACC=1`.
See [LIVE_VERIFICATION.md](LIVE_VERIFICATION.md) for completed native checks and
hardware limitations. CI runs offline tests, race checks, vet, module verification,
formatting and a static build on pushes and pull requests to `main`.

For local OpenTofu or Terraform development, build the provider:

```sh
mkdir -p /tmp/routeros-dev
go build -o /tmp/routeros-dev/terraform-provider-routeros .
```

Point a CLI configuration file at that directory:

```hcl
provider_installation {
  dev_overrides {
    "manchtools/routeros" = "/tmp/routeros-dev"
  }
  direct {}
}
```

Set `TF_CLI_CONFIG_FILE` to the configuration file when running plans or applies.
Keep the resource configuration's provider source as `manchtools/routeros`.
Bogon's build script creates an immutable filesystem mirror for normal builds;
the development override bypasses registry installation.

All fork branches were reviewed before consolidation. See
[BRANCH_REVIEW.md](BRANCH_REVIEW.md) for retained changes, exclusions and the
deletion of the retired branches.

### Fixing RouterOS property drift

Sometimes RouterOS might introduce a breaking change on a property. Update property mappings using the generator:

- Edit `routeros/mikrotik_resource_drift.yaml`. Add the resource used as well as the old property name and the new one
- Perform the generator. It should edit file `routeros/mikrotik_resource_drift.go`.
```bash
cd routeros/
go run ../tools/drift/main.go
```
- Submit your changes!

[Here](https://github.com/terraform-routeros/terraform-provider-routeros/pull/758/files) is a example of pull request.
