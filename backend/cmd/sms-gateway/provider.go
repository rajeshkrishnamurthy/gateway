package main

import (
	"errors"
	"gateway"
	"gateway/adapter"
	"os"
	"strings"
	"time"
)

func providerFromConfig(cfg fileConfig, providerConnectTimeout time.Duration) (gateway.ProviderCall, string, error) {
	switch cfg.SMSProvider {
	case "default":
		return adapter.DefaultProviderCall(cfg.SMSProviderURL, providerConnectTimeout), adapter.DefaultProviderName, nil
	case "model":
		return adapter.ModelProviderCall(cfg.SMSProviderURL, providerConnectTimeout), adapter.ModelProviderName, nil
	case "sms24x7":
		apiKey := strings.TrimSpace(os.Getenv("SMS24X7_API_KEY"))
		if apiKey == "" {
			return nil, "", errors.New("SMS24X7_API_KEY is required for sms24x7")
		}
		if strings.TrimSpace(cfg.SMSProviderServiceName) == "" {
			return nil, "", errors.New("smsProviderServiceName is required for sms24x7")
		}
		if strings.TrimSpace(cfg.SMSProviderSenderID) == "" {
			return nil, "", errors.New("smsProviderSenderId is required for sms24x7")
		}
		return adapter.Sms24X7ProviderCall(
			cfg.SMSProviderURL,
			apiKey,
			cfg.SMSProviderServiceName,
			cfg.SMSProviderSenderID,
			providerConnectTimeout,
		), adapter.Sms24X7ProviderName, nil
	case "smskarix":
		apiKey := strings.TrimSpace(os.Getenv("SMSKARIX_API_KEY"))
		if apiKey == "" {
			return nil, "", errors.New("SMSKARIX_API_KEY is required for smskarix")
		}
		if strings.TrimSpace(cfg.SMSProviderVersion) == "" {
			return nil, "", errors.New("smsProviderVersion is required for smskarix")
		}
		if strings.TrimSpace(cfg.SMSProviderSenderID) == "" {
			return nil, "", errors.New("smsProviderSenderId is required for smskarix")
		}
		return adapter.SmsKarixProviderCall(
			cfg.SMSProviderURL,
			apiKey,
			cfg.SMSProviderVersion,
			cfg.SMSProviderSenderID,
			providerConnectTimeout,
		), adapter.SmsKarixProviderName, nil
	case "smsinfobip":
		apiKey := strings.TrimSpace(os.Getenv("SMSINFOBIP_API_KEY"))
		if apiKey == "" {
			return nil, "", errors.New("SMSINFOBIP_API_KEY is required for smsinfobip")
		}
		if strings.TrimSpace(cfg.SMSProviderSenderID) == "" {
			return nil, "", errors.New("smsProviderSenderId is required for smsinfobip")
		}
		return adapter.SmsInfoBipProviderCall(
			cfg.SMSProviderURL,
			apiKey,
			cfg.SMSProviderSenderID,
			providerConnectTimeout,
		), adapter.SmsInfoBipProviderName, nil
	default:
		return nil, "", errors.New("smsProvider must be one of: default, model, sms24x7, smskarix, smsinfobip")
	}
}
