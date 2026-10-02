# Use the actual internal ID shown by /ip/reverse-proxy print as-value.
terraform import routeros_ip_reverse_proxy.app '*1'

# Alternatively select one static rule by its exact SNI.
terraform import routeros_ip_reverse_proxy.app 'sni=app.example.com'
