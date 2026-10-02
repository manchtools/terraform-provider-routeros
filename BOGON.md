# Bogon fork scope and review

This fork builds `manchtools/routeros` version `1.99.1-bogon.6` for Bogon. The baseline is upstream main at `0d8c069c20a012300dfeeb96cb343ad7a5e7ebfb`. It keeps upstream history and the MPL 2.0 license. Bogon pins a commit and archive checksum, builds an unsigned executable and installs it through a local OpenTofu mirror. Registry publication, signed releases and state migrations are deferred.

## Reviewed upstream contributions

These are selected patch integrations, with adaptations where needed, rather than unreviewed merges of entire contributor branches. All seven complete patches (46 file changes, 44 distinct files) and the selected PR #1009 mapping were read for correctness and security, including supporting serialization and lifecycle code. An independent review found no evidence of malicious changes: no added dependencies, external destinations, credential handling, executable hooks or elevated workflow permissions. This finding covers these revisions; it does not certify the full upstream backlog or dependencies.

| Contribution | Reviewed head | Author | Decision |
| --- | --- | --- | --- |
| [#992](https://github.com/terraform-routeros/terraform-provider-routeros/pull/992) modern schemas | `7d6a7c590c2479adfdbf7d5a49cdcbd4f2d76330` | glitchedmob | Adapt MLAG and BGP behavior; retain related schema additions. |
| [#984](https://github.com/terraform-routeros/terraform-provider-routeros/pull/984) address VRF | `27d180cb568675b1cf638aaff8051c6397abacc2` | SolAstrius | Accept computed-only VRF. |
| [#985](https://github.com/terraform-routeros/terraform-provider-routeros/pull/985) DHCP defaults | `dd1c8b2ec6ea485f3cdf4831885f85cc455c5042` | SolAstrius | Accept preservation of omitted identifiers and prefix pool. |
| [#1003](https://github.com/terraform-routeros/terraform-provider-routeros/pull/1003) IPv6 address lists | `02f74bf5a933e4ba2e70c8776d4065b352a20560` | simonostendorf | Accept filter and datasource fields. |
| [#1013](https://github.com/terraform-routeros/terraform-provider-routeros/pull/1013) comparison failures | `f5c736e928575962e9f59801926fc947b069c3c4` | toelke | Accept returning a diff rather than panicking on malformed values. |
| [#1014](https://github.com/terraform-routeros/terraform-provider-routeros/pull/1014) lowercase kilo | `47ccfe6d1dd00cad98a1d0dfd41cb2443a0a7c5c` | toelke | Accept parser change and tests. |
| [#980](https://github.com/terraform-routeros/terraform-provider-routeros/pull/980) bridge port path cost | `495311d40311d940d3a85c0642f306aaf39f7fac` | chramb | Accept computed-only observation. |
| [#1009](https://github.com/terraform-routeros/terraform-provider-routeros/pull/1009) service rename | `fa03e6c1df1c60be40b9a460174112d364f55937` | aaronmgn | Select only the RouterOS 7.24 `address` → `available-from` mapping. |

PR #992's singleton MLAG calls used ordinary CRUD and would fail on legacy firmware. Its BGP additions also left the legacy `add_path_out=none` default writable, causing unsupported writes on newer firmware. The fork corrects both and bounds version parsing to avoid an inherited malformed-version panic. BGP examples and docs reflect the computed-only legacy field.

PRs #1004 and the remaining #1009 changes are excluded because they suppress ownership of writable MLAG/RA settings. #983 changes service IDs without upgrading existing state. #894's WiFi clearing was not merged as written because it addresses API only; the fork implements and verifies equivalent direct-setting ownership over REST. #1007 makes unrelated schema breaks; #977 bulk caching is deferred. These exclusions are correctness and scope decisions, not claims of malicious intent. Remaining PRs require review before inclusion.

## Bogon fixes

- A dedicated `routeros_bridge_mlag` owns peer settings on both legacy singleton menus and modern bridges. Refresh detects drift; destroy disables MLAG and preserves the bridge. The bridge resource does not replay MLAG properties. Retaining separate ownership avoids the bridge/peer-port dependency cycle.
- IPv6 mangle accepts `jump_target`, allowing owned jump chains.
- WiFi datapath updates explicitly unset omitted bridge, VLAN and forwarding overrides, preserve native identity and support retry after partial failure. Explicit values remain distinct from omission.
- CAPsMAN destroy disables and verifies the singleton, retaining state on failure and preserving certificates/radios.
- Existing Bogon route blackhole normalization and 6 GHz validation are retained; address VRF and legacy BGP observations remain read-only.

- IPv6 address VRF is read-only; static address comparisons distinguish host and prefix changes while allowing equivalent IPv6 spelling.
- Firewall refresh clears omitted registered selectors, including source/destination addresses, input/output interfaces and protocols. Configuration removal unsets those selectors.
- Omitted BGP blocks no longer crash serialization or replay inherited settings; refresh clears groups that disappear. Explicitly configured blocks retain ownership.
- Rule ordering rejects fewer than two IDs before issuing native commands.
- WiFi interface/configuration maps use direct config reads and unset removed keys/references, preserving profile inheritance.
- Dependencies are updated and the build requires Go 1.26.6 or newer. `govulncheck ./...` reports zero affected imported packages or reachable symbols; an unused deprecated OpenPGP module advisory remains.

Router-owned DHCP-PD callbacks and HA election hooks remain necessary for autonomous behavior between applies.

## REST-only transport

Revision 6 removes the binary API client, Go driver dependency, protocol enum and transport branches. Provider endpoints accept HTTP/HTTPS and bare hosts (default HTTPS), with optional `/rest`; unsupported schemes return diagnostics before connecting. TLS verification, explicit CA/insecure options, environment credentials and version discovery remain supported. REST command paths and JSON query arrays remain available for singleton updates, unset, ordering, script execution and certificate operations. Native command replies can still carry IDs in `ret`.

Resource/data source schemas and native IDs are unchanged. REST state created with revision 5 was verified using the revision 6 executable without migration. Users of the removed `api://` or `apis://` schemes must switch to an HTTP(S) REST endpoint. The router’s binary API services and user policies are not changed by this provider migration.

## Verification and limits

The IP service and reverse proxy feature branch adds configuration-driven
service imports with stable name-based IDs, static-service filtering and the
`reverse-proxy` listener. One service resource manages one built-in service;
changing its selector replaces the managed resource. The new
`routeros_ip_reverse_proxy` resource manages static rules with IPv4 or IPv6
backends, certificate fallback, CRUD and imports. Dynamic container-app rules
remain owned by RouterOS. Rule-level VRF is omitted because RouterOS 7.24.2
ignored it in native probes.

Service import failed before the fix and passed afterward with an empty
follow-up plan. Native rule create, IPv6 update, SNI import, external deletion,
recreation, unchanged plans and destroy passed on RouterOS 7.24.2. Actual HTTPS
requests with verified certificates reached IPv4 and IPv6 loopback HTTP
backends, covering both listener certificate fallback and a rule certificate.
All static service settings were restored, and owned rules, certificates and
uploaded files were removed. These tests do not establish cross-VRF forwarding
or coverage of older firmware and unsupported hardware.

Offline Go tests, schema validation, vet, race checks for Bogon regressions and build pass. Regression tests cover the selected DHCP, address-list, parser and diff-comparison fixes. REST lifecycle fixtures cover MLAG on 7.21.5, 7.22.3, 7.23.7 and 7.24.5, WiFi omission/retry and CAPsMAN failure paths. REST wire tests verify CRUD, command suffixes, authentication, TLS and native command response IDs. Bogon tests use the actual built provider and OpenTofu 1.13.0; the MLAG test exercises apply, unchanged plan, native priority drift, correction and destroy.

The additional fixes were also tested sequentially over REST against a RouterOS 7.24.2 CHR test VM, with before/after reproduction, native readback, empty final plans and probe cleanup. [LIVE_VERIFICATION.md](LIVE_VERIFICATION.md) records the evidence and limits. WiFi profile inheritance and unset behavior are verified natively; CHR has no radios, so physical WiFi interface lifecycle, radio traffic and MLAG failover remain unverified. The multi-version fixtures establish API contracts rather than live coverage of those firmware versions. Current [MLAG documentation](https://help.mikrotik.com/docs/spaces/ROS/pages/67633179/Multi-chassis+Link+Aggregation+Group), [WiFi CLI reference](https://manual.mikrotik.com/docs/cli-reference/interface/wifi/datapath/) and [RouterOS changelogs](https://mikrotik.com/download/changelogs) describe the relevant interfaces. Newer versions should be checked against native observations before deployment.
