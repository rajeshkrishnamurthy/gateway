# Gateway configs
COMPLETED

## Common config rules

- Config files are JSON and allow full-line `#` comments only.
- Inline comments are not supported.
- Unknown fields are rejected.
- Trailing JSON data is rejected.

## SMS gateway config

Default path: `conf/sms/config.json`

### Common fields (all providers)

- `smsProvider` (optional, default `default`)
  - Allowed values: `default`, `model`, `sms24x7`, `smskarix`, `smsinfobip`.
- `smsProviderUrl` (required)
- `smsProviderTimeoutSeconds` (required, 15 to 60)
- `smsProviderConnectTimeoutSeconds` (optional; default 2; must be 2 to 10)
- `grafanaDashboardUrl` (optional)

### Provider-specific fields

- `smsProviderServiceName` (required for `sms24x7`)
- `smsProviderSenderId` (required for `sms24x7`, `smskarix`, `smsinfobip`)
- `smsProviderVersion` (required for `smskarix`)

### Provider credentials

SMS provider credentials are read from environment variables in `cmd/sms-gateway/main.go`.

- `sms24x7` requires `SMS24X7_API_KEY`.
- `smskarix` requires `SMSKARIX_API_KEY`.
- `smsinfobip` requires `SMSINFOBIP_API_KEY`.

No secrets are read from config files.

## Push gateway config

Default path: `conf/config_push.json`

### Fields

- `pushProvider` (optional, default `fcm`)
  - Allowed values: `fcm`.
- `pushProviderUrl` (required)
- `pushProviderTimeoutSeconds` (required, 15 to 60)
- `pushProviderConnectTimeoutSeconds` (optional; default 2; must be 2 to 10)
- `grafanaDashboardUrl` (optional)

### Provider credentials

Push provider credentials are read from environment variables in `cmd/push-gateway/main.go`.

- Preferred: `PUSH_FCM_CREDENTIAL_JSON_PATH` (service account JSON file path)
- Alternative: `PUSH_FCM_BEARER_TOKEN`
- Optional: `PUSH_FCM_SCOPE_URL`

No secrets are read from config files.
