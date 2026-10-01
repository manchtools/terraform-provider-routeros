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
