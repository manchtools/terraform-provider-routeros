
resource "routeros_system_logging_action" "syslog" {
  name               = "app-syslog"
  target             = "remote"
  remote             = "192.168.1.1"
  remote_log_format  = "syslog"
  syslog_facility    = "user"
  syslog_severity    = "notice"
  syslog_time_format = "iso8601"
}
