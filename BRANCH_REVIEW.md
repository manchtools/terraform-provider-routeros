# Branch consolidation review — 2026-10-02

`main` is the single supported branch of the manchtools fork. This review
covers every remote branch present at the start: 15 branches, including the
original `main`. Requiredness means a behavior or tooling correction still
needed by the current implementation, rather than an unmerged commit count.
Changes were compared from their merge bases against candidate `57417ca` and
traced through current schemas, serialization, imports and native request paths.

## Branch decisions

| Branch | Reviewed tip | Decision | Reason |
| --- | --- | --- | --- |
| `bogon` | [6066bc9c](https://github.com/manchtools/terraform-provider-routeros/commit/6066bc9c0332f9ee2a8911f72ecda2e14d68cf92) | Merge | Required REST-only transport, protocol ownership fixes and previous verified regressions. |
| `bogon-ip-services-reverse-proxy` | [57417ca3](https://github.com/manchtools/terraform-provider-routeros/commit/57417ca372ea04d5fffcece6eb48d400f0bb46ca) | Merge | Required service import corrections and requested static reverse proxy support. |
| `dependabot/go_modules/github.com/fatih/color-1.19.0` | [3eb31e9c](https://github.com/manchtools/terraform-provider-routeros/commit/3eb31e9c158dbc8fdbcebdbe002241e43860914a) | Adapt upgrade | Adopt 1.19.0 with compatible API and writer/terminal fixes. |
| `dependabot/go_modules/github.com/hashicorp/terraform-plugin-docs-0.25.0` | [24246774](https://github.com/manchtools/terraform-provider-routeros/commit/2424677431142b21202174d3c08db779a18da0de) | Adapt upgrade | Adopt 0.25.0 and hc-install verification-key fix; retain newer security dependencies. |
| `dependabot/go_modules/github.com/hashicorp/terraform-plugin-testing-1.16.0` | [5f7918a5](https://github.com/manchtools/terraform-provider-routeros/commit/5f7918a5af8dc515d88f89e546d341590d0bc37c) | Adapt upgrade | Adopt 1.16.0 with compatible SDK constraints and testing/tool-install corrections. |
| `dependabot/go_modules/golang.org/x/crypto-0.50.0` | [282ff7e4](https://github.com/manchtools/terraform-provider-routeros/commit/282ff7e4ea1c13fff6d9dc6e23fd26f9f129f31a) | Superseded | Keep current 0.56.0; old branch would downgrade. |
| `dependabot/go_modules/google.golang.org/grpc-1.79.3` | [abf8fa40](https://github.com/manchtools/terraform-provider-routeros/commit/abf8fa409ccf74070f29057eb77158bb449bf5d8) | Superseded | Keep current 1.83.2; old branch would downgrade. |
| `devel` | [e662b6f5](https://github.com/manchtools/terraform-provider-routeros/commit/e662b6f56c6739387f996f152ac93d7963d37a8a) | Exclude | Obsolete container prototype and release workflow; modern container support already exists. |
| `dn/fix_nil_resources` | [830407b6](https://github.com/manchtools/terraform-provider-routeros/commit/830407b6a0519675e02cb1c13d4e22b1804be0bd) | Already covered | Current serializer handles empty, null, unknown and inherited nested blocks; BGP regressions cover them. |
| `dn/routeros_test_updates` | [8e4b24d5](https://github.com/manchtools/terraform-provider-routeros/commit/8e4b24d5de238593857a8bfd34cf2505cdfb6838) | Adapt | Retain IPsec menu routing, OpenVPN lifecycle, SSH cleanup, BGP instance validation and useful acceptance corrections. Exclude invalid BGP schema, Cloud bool, hardcoded dates and redundant offload assertions. |
| `main` | [0d8c069c](https://github.com/manchtools/terraform-provider-routeros/commit/0d8c069c20a012300dfeeb96cb343ad7a5e7ebfb) | Keep ancestry | Original upstream baseline; fast-forward to reviewed fork changes. |
| `revert-867-jbfavre/fix-importer` | [8d50d9c5](https://github.com/manchtools/terraform-provider-routeros/commit/8d50d9c5b074571c65a5c1fcac755a484ee14ac5) | Exclude; repair current helper | Revert loses quoting, identity matching and timeouts. Correct fork source and import traversal in current helper instead. |
| `vaerh/issue630` | [ffd2355a](https://github.com/manchtools/terraform-provider-routeros/commit/ffd2355a1967c277c7b8e7f79c0419737908054a) | Already covered; exclude unsafe alternative | Multi-passphrase suppression is already removed. Broad unsetting would cast integer DH groups to strings and panic; global default suppression change breaks omitted DHCP settings. |
| `vaerh/issue667` | [61899dfc](https://github.com/manchtools/terraform-provider-routeros/commit/61899dfc9d6003e3c0fd497b1163b104594f5812) | Already covered | Logging regex, disabled and invalid fields already exist. |
| `vaerh/issue769` | [3dbe5003](https://github.com/manchtools/terraform-provider-routeros/commit/3dbe5003840b4bcc9890ab2ef7dab00282b67271) | Superseded | New static-service lookup preserves state IDs, filters connections and handles imports; old patch uses unsafe version parsing and different IDs. |

## Adaptations

IPsec RSA keys choose `/ip/ipsec/key/rsa` at RouterOS 7.20 and newer, retaining
the older menu for earlier firmware. Every operation and import uses the same
version-selected path without mutating the provider schema. The asynchronous
key-generation helper is otherwise unchanged.

OpenVPN server configuration uses native named-entry CRUD/imports on RouterOS
7.17 and newer while preserving older singleton behavior. The public `enabled`
setting remains available as an inverse alias of modern `disabled`. Ambiguous
imports and synthetic legacy IDs after a firmware upgrade fail explicitly.
Names and VRF are supported, and examples describe the actual resource.

BGP connection and VPN instance fields remain optional in the shared schema for
older firmware. Modern create/update operations require an instance, with
connection template inheritance delegated to RouterOS. Existing inherited-block
serialization remains intact. The branch's `Optional: false` connection field
would fail SDK schema validation; globally requiring VPN instance would needlessly
break older firmware. Deprecated connection/template fields are retained for
compatibility instead of deleting only half of a shared contract.

SSH no longer exposes `allow_none_crypto`, replaced by RouterOS cipher settings.
The erroneous requirement to configure a crypto setting for unrelated SSH
updates is removed. Use `ciphers` or `strong_crypto` when configuring encryption.
Logging keeps the deprecated optional `bsd_syslog` field for older configurations;
modern examples and acceptance fixtures use `remote_log_format = "syslog"`.
The Ethernet acceptance assertion checks interface identity rather than assuming
a link is running. Clock date changes and physical offload claims were excluded.

The importer helper keeps its existing quoting, identity and timeout handling.
Generated configuration now uses `manchtools/routeros`, explicitly selects the
mirrored fork version, and uses an unquoted resource traversal in `import.to`.
OpenTofu initialization rejects the old `~> 1` constraint because it excludes
the fork's prerelease version. Explicit revision 7 initialization passes.

Dependency patches were reimplemented on the current module graph, preserving
the newer grpc and x/crypto selections. The docs/testing upgrades include
hc-install 0.9.4, which repairs expired verification-key handling. CI now targets
`main`, covers all packages with tests and the race detector, and checks vet,
formatting, module integrity and a static build. The old duplicate Go 1.25 cache
workflow was removed. Formatting was normalized; the drift generator now checks
errors and emits formatted Go without truncating output on malformed input.

## Preserving retired branches

Each non-main reviewed tip is preserved under
`archive/2026-10-02/<original-branch-name>` before its branch is removed.
Original history and upstream release tags remain. Rejected branch code is
recoverable through these archive tags but is not part of stable `main`.
The original main tip remains an ancestor of the consolidated main.

## Evidence and limits

Regressions fail before and pass after the fixes for SSH validation, importer
output, IPsec key menus, OpenVPN server lifecycle and modern BGP instances.
Versioned REST fixtures exercise lifecycle, imports, failure ownership and
external deletion. Full offline tests, race checks, vet, module verification,
static builds, documentation generation and Bogon's actual OpenTofu provider
compatibility checks validate the combined result.

No running application or router was used for new native tests during this
consolidation. [LIVE_VERIFICATION.md](LIVE_VERIFICATION.md) records earlier live
CHR evidence retained with the merged fixes and distinguishes it from fixtures.
Physical MLAG, switching offload and radio traffic still require hardware.

Sources: [OpenVPN](https://help.mikrotik.com/docs/spaces/ROS/pages/2031655/OpenVPN),
[IPsec RSA CLI](https://manual.mikrotik.com/docs/cli-reference/ip/ipsec/key/rsa/),
[BGP connection CLI](https://manual.mikrotik.com/docs/cli-reference/routing/bgp/connection/),
[SSH](https://help.mikrotik.com/docs/spaces/ROS/pages/132350014/SSH),
[docs 0.25.0](https://github.com/hashicorp/terraform-plugin-docs/releases/tag/v0.25.0),
[testing 1.16.0](https://github.com/hashicorp/terraform-plugin-testing/releases/tag/v1.16.0)
and [color 1.19.0](https://github.com/fatih/color/releases/tag/v1.19.0).
