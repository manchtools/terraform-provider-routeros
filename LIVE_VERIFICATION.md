# Bogon provider live verification

Tests use OpenTofu 1.13.0 and RouterOS 7.24.2 (stable) on the supplied CHR test VM, Proxmox `proxmox1` VM 913. Authentication uses a private local credential file; credentials are excluded from this repository and logs. Test bridges have no ports, IPv6 probe addresses are disabled with advertisements off, and test firewall rules use isolated chains. Existing management configuration is preserved.

Each fix is tested before the next starts. Live command logs and temporary state are in Bogon's ignored `.local/routeros-live/` directory.

## IPv6 address VRF is read-only

The first baseline is fork commit `c3de3c08ee460e61d26fab3eaeb17ee59a7cdb51`. A new disabled IPv6 address on an isolated bridge was created successfully. An unrelated comment update then failed with HTTP 400, `unknown parameter vrf`.

The fix changes the IPv6 address VRF schema to computed-only. The same OpenTofu update succeeded against the same native resource; native readback confirmed the comment and `vrf=main`, and Terraform state retained the computed VRF. A subsequent plan returned exit 0 (no changes). Destroy succeeded; native reads confirmed the probe address and bridge were gone.

`TestBogonIPv6AddressVRFReadOnly` also failed before the fix because the update payload replayed `vrf=main`, then passed after the fix. The provider's full Go test suite and vet passed. No physical traffic test is claimed.

## Static IPv6 address comparisons

Using the VRF-fixed provider as the next baseline, a native `2001:db8:ffff::100/64` address and a desired `2001:db8:ffff::1/64` incorrectly produced an unchanged plan. The new regression test also failed for different host addresses, different prefixes and equivalent IPv6 spellings.

The fix compares parsed full addresses and prefix lengths when neither EUI-64 nor pool generation is active. Live verification detected and corrected the configured address change and a subsequent native host-address drift. Expanded spelling of the same address produced no change. The final plan was empty and the probe resources were destroyed. Unit cases also preserve existing EUI-64 and pool-generated comparisons; those cases do not establish live generated-address behavior.

## Firewall selector refresh

Seven disabled rules in an isolated chain covered IPv4 filter, mangle, NAT and raw, and IPv6 filter, mangle and NAT. Native `/unset` removed each rule's `src-address-list`; native reads confirmed the key was absent. The previous provider incorrectly reported no changes.

The fix normalizes missing optional string fields explicitly registered as unsettable, only after a successful complete firewall resource read. The updated plan detected all seven changes, apply restored every native selector, the following plan was empty, and destroy removed every probe rule. Regression tests cover absent/present selectors and failed reads on all seven schemas; failed reads retain previous state and IDs.

A follow-up found incomplete unset metadata: IPv6 address selectors and filter interfaces, plus IPv4/IPv6 mangle and NAT protocols, could remain stale. An explicit regression matrix failed before expanding those declarations. Live tests removed source/destination addresses, input/output interfaces and protocol from all seven disabled rules. The fixed provider detected and restored every selector, then removal from configuration unset them natively; both final plans were empty and all rules were destroyed. This covers those selectors and the registered unsettable fields, not every optional firewall property.

## Omitted and inherited BGP blocks

Disabled native BGP connections and templates were imported with input/output/local/remote blocks omitted from configuration. An unrelated update crashed the baseline provider. Guarding serialization alone stopped the crash but left perpetual removal diffs; live testing also exposed stale state when an entire native block disappeared.

The fix checks empty, null and unknown nested values, skips unconfigured inherited blocks, marks inheritable BGP blocks optional/computed, and clears absent groups after complete native reads. Live tests updated comments without materializing inherited output overrides, then removed a parent's filter and confirmed both children followed it. Refresh removed the absent output from state. Explicit output blocks were subsequently applied, native removal was detected and corrected, the final plan was empty, and all owned BGP/filter probes were removed. Regression tests cover omitted inherited blocks, explicitly configured output and absent-block refresh. Omitting a block leaves its native settings unmanaged; it does not promise to erase every previous direct override.

## Short rule-ordering sequences

The baseline accepted an empty sequence in an OpenTofu plan and crashed during apply. The fix places the two-item minimum on the list schema and rejects short sequences in both runtime write callbacks before sending a command. Tests against the baseline implementation reproduce the slice panic; fixed runtime tests reject empty and one-item sequences without native calls.

Live OpenTofu plans reject both invalid lengths with a validation diagnostic. Three disabled IPv6 rules in an isolated chain were then ordered `a,b,c`, updated to `b,a,c`, and checked against native print order. Both subsequent plans were empty. Destroy removed every probe rule.

## WiFi profile inheritance over REST

An imported WiFi configuration referenced a datapath profile providing a bridge and VLAN. The baseline's unrelated comment update copied those inherited settings into direct overrides, confirmed using native `print config`. REST `POST .../print` with `config` and a JSON query array returns direct settings; GET flags and string/URL queries do not perform this selection reliably.

The fix uses direct config reads for WiFi interfaces and configurations, writes only explicitly configured map keys, and unsets removed direct map fields/references. Live tests detected and removed the baseline's frozen bridge/VLAN overrides, confirmed a parent VLAN change propagated through another unrelated update, switched the profile reference, applied an explicit VLAN override and removed it to restore inheritance, then removed the entire reference. State contained direct map values only and final plans were empty. All profile/bridge probes were removed.

REST fixtures exercise both resource callbacks, JSON query arrays, create/read/update, map/reference removal and failure ownership. CHR has no radios: native profile behavior is verified, while physical interface lifecycle and radio traffic remain unverified.

## Dependency security refresh

Baseline `govulncheck` reported six findings with reachable symbols in the provider entry point and thirteen across the repository including its SSH importer tool. This establishes call reachability, not exploitability in a Bogon deployment.

The update selects gRPC 1.83.2, x/crypto 0.56.0, x/net 0.58.0, x/text 0.41.0 and x/mod 0.40.0, with their required transitive updates. The provider requires Go 1.26.6 or newer; the live test build uses Go 1.26.8. Bogon's Docker builder already uses Go 1.26. Provider tests, vet, race checks and module verification pass. The updated executable also passed live IPv6 create/comment/address updates and rule-ordering create/update/cleanup checks.

The final scan reports no affected imported packages or reachable symbols. One module-only advisory remains for the unused, deprecated `golang.org/x/crypto/openpgp` package, which has no fixed version; no OpenPGP package is imported by this repository. See the [Go advisory](https://pkg.go.dev/vuln/GO-2026-5932). Dependency verification and scans are not a complete audit of every upstream dependency change.

## IP service imports and reverse proxy (2026-10-02)

The baseline service importer failed a real OpenTofu 1.13.0 `import` block for
the existing SSH service. It resolved `ssh` to `*4` and then queried by name,
returning "Cannot import non-existent remote object". The fixed importer retained
`id=ssh` and `numbers=ssh`; the import plan contained no device changes and the
follow-up plan was empty. Offline regressions also cover internal IDs, attribute
selectors, dynamic connection duplicates, ambiguous matches and missing services.

The CHR ran RouterOS 7.24.2. An isolated reverse proxy listener on port 18443
used a temporary self-signed certificate trusted explicitly by the HTTPS client.
Verified HTTPS requests reached the router's own HTTP REST server through both
IPv4 (`127.0.0.1`) and IPv6 (`::1`) backends. The initial rule used certificate
`none` to inherit the listener certificate; the updated rule referenced the
certificate explicitly. Native readback matched both configurations.

OpenTofu create, IPv6 update with equivalent address spelling, comment removal,
SNI import, external deletion, recreation and destroy passed. Plans after create,
update, import and recreation were empty. Static service settings were restored
exactly; owned rules, certificates and uploaded files were removed. Provider
tests pass with the race detector, and vet passes. Documentation was generated
from the built provider schema with tfplugindocs. The exported registry key was
adapted for that tool; schema content was unchanged.

Rule-level `vrf` was ignored in native probes and is not exposed by the new
resource. Listener VRF remains available through `routeros_ip_service`.
Loopback backend tests establish actual IPv4 and IPv6 HTTP forwarding but do not
prove cross-VRF routing or external backend reachability. Dynamic container-rule
ownership is covered by REST contract tests; no container app was installed.
Older firmware and SMIPS hardware were not tested.

## REST-only migration

Baseline is revision 5 at `6f13c6803810d55cf6f14b1fb0adfa51d0ef1050`. Before removal, endpoint tests reproduced binary connection attempts, an unsupported-scheme panic, invalid endpoints accepted with an explicit version, and doubled `/rest` paths. Revision 6 accepts HTTP/HTTPS, bare hosts, IPv6 and optional REST/proxy prefixes; rejected schemes produce actionable diagnostics. HTTP fixtures exercise Basic Auth/version discovery, verified HTTPS with an explicit CA, default rejection of an untrusted CA, explicit insecure TLS and conflicting options.

The binary driver, protocol types and branches are removed. REST command suffixes, JSON query arrays and native `ret` IDs are retained and tested. The actual OpenTofu exports show identical resource and data source schemas before and after removal. Existing WiFi and ordering states created with revision 5 yielded empty revision 6 plans and retained IDs. Native updates, WiFi inheritance/unset and rule reordering succeeded and were cleaned up.

Live OpenTofu create/update/empty-plan/destroy tests on RouterOS 7.24.2 also cover a disabled isolated bridge, IPv4/IPv6 addresses, disabled firewall rule, disabled routing table, an owned small text file, and a harmless script. Native script run counters confirmed `/run` executed on create and update; file destruction verifies `/file/remove`. The endpoint included `/rest/`, so native version discovery and prefix normalization were exercised. No management setting, binary API service or user policy was modified. Certificate/container/physical switch paths are preserved and inspected but are not newly live-tested here; physical radio and MLAG limitations remain.
