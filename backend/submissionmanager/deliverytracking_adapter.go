package submissionmanager

import (
	"context"
	"errors"

	"gateway/deliverytracking"
)

// ApplyCorrelatedDeliveryRecord delegates delivery core processing to the deliverytracking package.
func (m *Manager) ApplyCorrelatedDeliveryRecord(ctx context.Context, record deliverytracking.CorrelatedDeliveryRecord) (deliverytracking.DeliveryApplyResult, error) {
	if m == nil || m.store == nil || m.store.db == nil {
		return deliverytracking.DeliveryApplyResult{}, errors.New("manager store is required")
	}
	processor, err := deliverytracking.NewProcessor(m.store.db)
	if err != nil {
		return deliverytracking.DeliveryApplyResult{}, err
	}
	return processor.ApplyCorrelatedDeliveryRecord(ctx, record)
}

// EvaluateDeliveryFreshnessStaleness delegates freshness staleness evaluation to deliverytracking.
func (m *Manager) EvaluateDeliveryFreshnessStaleness(ctx context.Context) (int64, error) {
	if m == nil || m.store == nil || m.store.db == nil {
		return 0, errors.New("manager store is required")
	}
	processor, err := deliverytracking.NewProcessor(m.store.db)
	if err != nil {
		return 0, err
	}
	return processor.EvaluateDeliveryFreshnessStaleness(ctx)
}
