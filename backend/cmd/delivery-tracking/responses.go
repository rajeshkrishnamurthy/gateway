package main

import (
	"time"

	"gateway/deliverytracking"
	"gateway/submission"
)

type deliveryWebhookIngestionResponse struct {
	SourceRecordID         string `json:"sourceRecordId"`
	IntentID               string `json:"intentId"`
	ProviderDeliverySignal string `json:"providerDeliverySignal"`
	ProviderObservedAt     string `json:"providerObservedAt,omitempty"`
	ProviderEventID        string `json:"providerEventId,omitempty"`
	ReceivedAt             string `json:"receivedAt"`
	EffectiveAt            string `json:"effectiveAt"`
	IngressSource          string `json:"ingressSource"`
}

type deliveryCurrentResponse struct {
	IntentID             string `json:"intentId"`
	DeliveryTrackingMode string `json:"deliveryTrackingMode"`
	DeliveryStatus       string `json:"deliveryStatus,omitempty"`
	DeliveryFreshness    string `json:"deliveryFreshness,omitempty"`
}

type deliveryHistoryResponse struct {
	IntentID             string                         `json:"intentId"`
	DeliveryTrackingMode string                         `json:"deliveryTrackingMode"`
	Entries              []deliveryHistoryEntryResponse `json:"entries"`
}

type deliveryHistoryEntryResponse struct {
	RecordedAt             string `json:"recordedAt"`
	ProviderDeliverySignal string `json:"providerDeliverySignal"`
	DeliveryStatus         string `json:"deliveryStatus"`
	LateObservation        bool   `json:"lateObservation"`
}

func toDeliveryWebhookIngestionResponse(result deliverytracking.WebhookIngestionResult) deliveryWebhookIngestionResponse {
	response := deliveryWebhookIngestionResponse{
		SourceRecordID:         result.SourceRecordID,
		IntentID:               result.IntentID,
		ProviderDeliverySignal: string(result.ProviderDeliverySignal),
		ProviderEventID:        result.ProviderEventID,
		ReceivedAt:             result.ReceivedAt.UTC().Format(timeFormat),
		EffectiveAt:            result.EffectiveAt.UTC().Format(timeFormat),
		IngressSource:          result.IngressSource,
	}
	if result.ProviderObservedAt != nil {
		response.ProviderObservedAt = result.ProviderObservedAt.UTC().Format(timeFormat)
	}
	return response
}

func toDeliveryCurrentResponse(view deliverytracking.CurrentDeliveryView) deliveryCurrentResponse {
	response := deliveryCurrentResponse{
		IntentID:             view.IntentID,
		DeliveryTrackingMode: string(view.DeliveryTrackingMode),
	}
	if view.DeliveryTrackingMode == submission.DeliveryTrackingModeOn {
		if view.DeliveryStatus != nil {
			response.DeliveryStatus = string(*view.DeliveryStatus)
		}
		if view.DeliveryFreshness != nil {
			response.DeliveryFreshness = string(*view.DeliveryFreshness)
		}
	}
	return response
}

func toDeliveryHistoryResponse(view deliverytracking.DeliveryHistoryView) deliveryHistoryResponse {
	response := deliveryHistoryResponse{
		IntentID:             view.IntentID,
		DeliveryTrackingMode: string(view.DeliveryTrackingMode),
		Entries:              make([]deliveryHistoryEntryResponse, 0, len(view.Entries)),
	}
	for _, entry := range view.Entries {
		response.Entries = append(response.Entries, deliveryHistoryEntryResponse{
			RecordedAt:             entry.RecordedAt.UTC().Format(timeFormat),
			ProviderDeliverySignal: string(entry.ProviderDeliverySignal),
			DeliveryStatus:         string(entry.DeliveryStatus),
			LateObservation:        entry.LateObservation,
		})
	}
	return response
}

const timeFormat = time.RFC3339Nano
