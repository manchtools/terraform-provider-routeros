resource "routeros_routing_bgp_instance" "example" {
  name = "example-bgp"
  as   = "65550"
}

resource "routeros_routing_bgp_connection" "test" {
  name     = "neighbor-test"
  as       = "65550"
  instance = routeros_routing_bgp_instance.example.name
  output {
    as_override = true
  }
  remote {
    address = "172.17.0.1"
    as      = "12345"
  }
  local {
    role = "ebgp"
  }
}
