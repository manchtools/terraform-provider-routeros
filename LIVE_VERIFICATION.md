# Bogon provider live verification

Tests use OpenTofu 1.13.0 and RouterOS 7.24.2 (stable) on the supplied CHR test VM, Proxmox `proxmox1` VM 913. Authentication uses a private local credential file; credentials are excluded from this repository and logs. Test bridges have no ports, IPv6 probe addresses are disabled with advertisements off, and test firewall rules use isolated chains. Existing management configuration is preserved.

Each fix is tested before the next starts. Live command logs and temporary state are in Bogon's ignored `.local/routeros-live/` directory.

## IPv6 address VRF is read-only

The first baseline is fork commit `c3de3c08ee460e61d26fab3eaeb17ee59a7cdb51`. A new disabled IPv6 address on an isolated bridge was created successfully. An unrelated comment update then failed with HTTP 400, `unknown parameter vrf`.

The fix changes the IPv6 address VRF schema to computed-only. The same OpenTofu update succeeded against the same native resource; native readback confirmed the comment and `vrf=main`, and Terraform state retained the computed VRF. A subsequent plan returned exit 0 (no changes). Destroy succeeded; native reads confirmed the probe address and bridge were gone.

`TestBogonIPv6AddressVRFReadOnly` also failed before the fix because the update payload replayed `vrf=main`, then passed after the fix. The provider's full Go test suite and vet passed. No physical traffic test is claimed.
