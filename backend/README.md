# Gateway Phase 5

## REST contract

Gateway request (JSON):

```json
{
  "referenceId": "string",
  "to": "string",
  "message": "string",
  "tenantId": "string (optional)"
}
```

Gateway response (JSON):

```json
{
  "referenceId": "string",
  "status": "accepted|rejected",
  "gatewayMessageId": "string (present when accepted)",
  "reason": "invalid_request|duplicate_reference|invalid_recipient|invalid_message|provider_failure (present when rejected)"
}
```

## Local fake provider + gateway smoke test

Start the fake provider:

```sh
go run ./cmd/fakeprovider -addr :9090
```

Start the gateway with the provider URL:

```sh
go run ./cmd/gateway -sms-provider-url http://localhost:9090/sms/send
```

Send requests through the gateway (use a fresh referenceId each time):

```sh
curl -i -X POST http://localhost:8080/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"referenceId":"ref-1","to":"15551234567","message":"hello"}'

curl -i -X POST http://localhost:8080/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"referenceId":"ref-2","to":"abc","message":"hello"}'

curl -i -X POST http://localhost:8080/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"referenceId":"ref-3","to":"15551234567","message":"                     "}'

curl -i -X POST http://localhost:8080/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"referenceId":"ref-4FAIL","to":"15551234567","message":"hello"}'
```
