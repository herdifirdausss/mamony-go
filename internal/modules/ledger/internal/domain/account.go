package domain

type BookID string
type AccountID string

type Direction string

const (
	Debit  Direction = "D"
	Credit Direction = "C"
)

type AccountType string

const (
	CollectionCash     AccountType = "COLLECTION_CASH"
	ContributedCapital AccountType = "CONTRIBUTED_CAPITAL"
	CustomerAvailable  AccountType = "CUSTOMER_AVAILABLE"
)

func (t AccountType) normalSide() Direction {
	switch t {
	case CollectionCash:
		return Debit
	case ContributedCapital, CustomerAvailable:
		return Credit
	default:
		return ""
	}
}

type AccountStatus string

const (
	Active    AccountStatus = "ACTIVE"
	Suspended AccountStatus = "SUSPENDED"
	Closed    AccountStatus = "CLOSED"
)

// AccountSnapshot is an input value, not a mutable persisted account.
type AccountSnapshot struct {
	ID       AccountID
	Book     BookID
	Currency CurrencyRef
	Type     AccountType
	Status   AccountStatus
	Balance  int64
	Sequence int64
}

func (a AccountSnapshot) valid() bool {
	return validIdentifier(string(a.ID)) && validIdentifier(string(a.Book)) &&
		a.Currency.valid() && a.Type.normalSide() != "" &&
		(a.Status == Active || a.Status == Suspended || a.Status == Closed) &&
		a.Balance >= 0 && a.Sequence >= 0
}
