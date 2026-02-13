package adapter

import (
	"log/slog"

	setulog "gateway/logging"
)

func providerLogger(providerName, referenceID string) *slog.Logger {
	return slog.Default().With(
		"component", "provider-adapter",
		"commProfile", setulog.CommProfileOutsideToSetu,
		"boundaryDirection", setulog.BoundaryDirectionEgress,
		"peerSystem", providerName,
		"operation", "provider_call",
		"provider", providerName,
		"referenceId", referenceID,
	)
}
