# piihash

This is a small CLI utility that prints the SHA-256 hash used by the gateway for `messageHash` logging.
It exists to help operators and developers correlate logs without exposing message content.

Usage:

```sh
go run ./cmd/piihash -value "hello world"
```
