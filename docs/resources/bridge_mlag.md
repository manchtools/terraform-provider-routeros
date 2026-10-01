# routeros_bridge_mlag

Owns the MLAG peer port, heartbeat and election priority. Before RouterOS 7.22 it configures `/interface/bridge/mlag`; on newer versions it resolves the bridge and updates its native `mlag-*` settings. The bridge resource does not manage those settings.

```hcl
resource "routeros_bridge_mlag" "site" {
  bridge = routeros_interface_bridge.site.name
  peer_port = routeros_interface_bonding.peer.name
  heartbeat = "5s"
  priority = 50
  depends_on = [routeros_interface_bridge_port.peer]
}
```

`bridge` and `peer_port` are required. Changing the bridge replaces the MLAG resource. `heartbeat` defaults to `5s`; `priority` defaults to `128` and accepts 0–128. Refresh observes native drift. Destroy resets peer-port to `none`, priority to 128 and heartbeat to 5s; legacy firmware also resets bridge to `none`. The underlying bridge is retained. Failed teardown keeps the resource in state for retry.

Import on RouterOS 7.22 and newer uses the bridge name:

```sh
terraform import routeros_bridge_mlag.site bridge-site
```

Older versions accept the singleton ID `interface.bridge.mlag`.
