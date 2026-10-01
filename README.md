# manchtools RouterOS provider

A Bogon-focused fork of [terraform-routeros/terraform-provider-routeros](https://github.com/terraform-routeros/terraform-provider-routeros), based on upstream main at `0d8c069c20a012300dfeeb96cb343ad7a5e7ebfb`.

The source address is `manchtools/routeros`. Bogon builds version `1.99.1-bogon.6` into a local OpenTofu filesystem mirror; this fork has no registry publication or signed release pipeline. See [BOGON.md](BOGON.md) for scope, upstream contributions and verification limits.

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

You can build the provider locally to test fixes by following these intructions:
- Build and copy the provider where Terraform reads it
```
go build *.go && \
mkdir -p ~/.terraform.d/plugins/terraform.local/local/routeros/1.0.0/$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m) && \
mv main ~/.terraform.d/plugins/terraform.local/local/routeros/1.0.0/$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m)/terraform-provider-routeros_v1.0.0
```
- Change provider from 
```hcl
required_providers {
  routeros = {
    source  = "terraform-routeros/routeros"
    version = "1.85.1"
  }
}
```

to
```hcl
required_providers {
  routeros = {
    source  = "terraform.local/local/routeros"
    version = "1.0.0"
  }
}
```
- Clean your providers, init and apply
- Alternatively, you can edit/create ~/.terraformrc add add a provider installation block like:
```hcl
provider_installation {
  dev_overrides {
     "terraform-routeros/routeros" = "/path/to/your/git/clone"
  }

  direct {
  }
}
```

and then build the provider using
```
go build -o terraform-provider-routeros *.go
```
in order for Terraform to find it.

### Fixing RouterOS property drift

Sometimes RouterOS might introduce a breaking change on a property. You can easilfy contribute to the provider by following these intructions:

- Edit `routeros/mikrotik_resource_drift.yaml`. Add the resource used as well as the old property name and the new one
- Perform the generator. It should edit file `routeros/mikrotik_resource_drift.go`.
```bash
cd routeros/
go run ../tools/drift/main.go
```
- Submit your changes!

[Here](https://github.com/terraform-routeros/terraform-provider-routeros/pull/758/files) is a example of pull request.
