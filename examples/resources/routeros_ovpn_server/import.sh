# RouterOS 7.17+: import the existing server by native ID or its unique name.
terraform import routeros_ovpn_server.server '*1'
terraform import routeros_ovpn_server.server 'name=managed-ovpn'

# Older RouterOS versions have one server settings object.
terraform import routeros_ovpn_server.server .
