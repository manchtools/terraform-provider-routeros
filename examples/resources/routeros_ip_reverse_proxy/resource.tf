# Supply a certificate and private key already installed on the router.
# Port 8443 avoids the default www-ssl management port, 443.
resource "routeros_ip_service" "reverse_proxy" {
  numbers     = "reverse-proxy"
  port        = 8443
  certificate = "wildcard-cert"
  disabled    = false
}

resource "routeros_ip_reverse_proxy" "app" {
  sni         = "app.example.com"
  ip_address  = "192.0.2.10"
  port        = 8080
  certificate = "none" # Inherit the listener's certificate.
  comment     = "Application HTTP backend"

  depends_on = [routeros_ip_service.reverse_proxy]
}

resource "routeros_ip_reverse_proxy" "app6" {
  sni        = "app6.example.com"
  ip_address = "2001:db8::10"
  port       = 8080

  depends_on = [routeros_ip_service.reverse_proxy]
}
