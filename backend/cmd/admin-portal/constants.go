package main

import "time"

const version = "0.1.0"

const proxyTimeout = 8 * time.Second

const (
	navSMS           = "sms"
	navPush          = "push"
	navTroubleshoot  = "troubleshoot"
	navDashboards    = "dashboards"
	navHAProxy       = "haproxy"
	navCommandCenter = "command-center"
)
