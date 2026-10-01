resource "routeros_bridge_mlag" "example" {
  bridge    = "bridge-site"
  peer_port = "stack-link"
  heartbeat = "5s"
  priority  = 50
}
