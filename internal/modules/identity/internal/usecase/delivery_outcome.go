package usecase

// DeliveryOutcome tells a delivery store how to finalize a queued message.
// Accepted means the provider accepted the message, not that it was delivered.
type DeliveryOutcome string

const (
	// DeliveryAccepted means the provider accepted the message for processing.
	DeliveryAccepted DeliveryOutcome = "ACCEPTED"
	// DeliveryTemporary means the store can schedule another attempt.
	DeliveryTemporary DeliveryOutcome = "TEMPORARY"
	// DeliveryPermanent means the store must stop retrying this message.
	DeliveryPermanent DeliveryOutcome = "PERMANENT"
)
