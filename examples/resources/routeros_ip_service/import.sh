# Import by service name. The address matches the for_each example.
terraform import 'routeros_ip_service.tls["www-ssl"]' www-ssl

# Alternatively use a selector for one static service.
terraform import 'routeros_ip_service.tls["www-ssl"]' 'name=www-ssl'
