package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

var errAdmissionSpent = errors.New("anonymous admission budget exhausted")

// AdmissionStore records privacy-keyed fixed-window accounting in one
// transaction against the restricted Identity enrollment runtime credential.
// No raw remote IP, email, subject or bearer enters this table or SQL.
type AdmissionStore struct {
	db Transactor
}

func NewAdmissionStore(db Transactor) (*AdmissionStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &AdmissionStore{db: db}, nil
}

func validAdmissionBucket(action usecase.AnonymousAction, b usecase.AdmissionBucket) bool {
	if b.Key == ([32]byte{}) {
		return false
	}
	if action == usecase.AnonymousRecovery {
		switch b.Scope {
		case "GLOBAL_MINUTE":
			return b.WindowSeconds == 60 && b.Limit == 60
		case "CLIENT_MINUTE":
			return b.WindowSeconds == 60 && b.Limit == 5
		case "CLIENT_HOUR":
			return b.WindowSeconds == 3600 && b.Limit == 40
		case "EMAIL_HOUR":
			return b.WindowSeconds == 3600 && b.Limit == 2
		case "EMAIL_DAY":
			return b.WindowSeconds == 86400 && b.Limit == 4
		default:
			return false
		}
	}
	// LOGIN has a separate domain and deliberately different alias quotas.
	// All other actions preserve their immutable v5-v7 accounting policy.
	if action == usecase.AnonymousLogin {
		switch b.Scope {
		case "GLOBAL_MINUTE":
			return b.WindowSeconds == 60 && b.Limit == 80
		case "CLIENT_MINUTE":
			return b.WindowSeconds == 60 && b.Limit == 8
		case "CLIENT_HOUR":
			return b.WindowSeconds == 3600 && b.Limit == 80
		case "EMAIL_HOUR":
			return b.WindowSeconds == 3600 && b.Limit == 5
		case "EMAIL_DAY":
			return b.WindowSeconds == 86400 && b.Limit == 20
		default:
			return false
		}
	}
	switch b.Scope {
	case "CLIENT_MINUTE":
		return b.WindowSeconds == 60 && b.Limit == 8
	case "CLIENT_HOUR":
		return b.WindowSeconds == 3600 && b.Limit == 80
	case "EMAIL_HOUR":
		return b.WindowSeconds == 3600 && b.Limit == 3
	case "EMAIL_DAY":
		return b.WindowSeconds == 86400 && b.Limit == 5
	default:
		return false
	}
}

// Spend holds the database transaction through ALL four counter upserts.
// Every counter shares one DB clock sample and deterministic lock order.
// A denied request rolls back the previous three increments. This keeps the
// budget coherent across processes without in-memory mutexes or cache TTLs.
//
// Windows are intentionally FIXED UTC-aligned bins; boundaries can permit
// double-burst traffic. Deploy an edge WAF/global rate limiter separately.
func (s *AdmissionStore) Spend(ctx context.Context, action usecase.AnonymousAction, budgets []usecase.AdmissionBucket) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return false, ErrStorageUnavailable
	}
	// Exact ordered scopes enforce an action-specific, static policy. Do
	// not permit callers to omit a GLOBAL or per-alias bucket.
	var expected []string
	switch action {
	case usecase.AnonymousRegister, usecase.AnonymousResend:
		expected = []string{"CLIENT_MINUTE", "CLIENT_HOUR", "EMAIL_HOUR", "EMAIL_DAY"}
	case usecase.AnonymousConfirm:
		expected = []string{"CLIENT_MINUTE", "CLIENT_HOUR"}
	case usecase.AnonymousLogin:
		expected = []string{"GLOBAL_MINUTE", "CLIENT_MINUTE", "CLIENT_HOUR", "EMAIL_HOUR", "EMAIL_DAY"}
	case usecase.AnonymousRecovery:
		expected = []string{"GLOBAL_MINUTE", "CLIENT_MINUTE", "CLIENT_HOUR", "EMAIL_HOUR", "EMAIL_DAY"}
	default:
		return false, ErrStorageUnavailable
	}
	if len(budgets) != len(expected) {
		return false, ErrStorageUnavailable
	}
	for i, budget := range budgets {
		if budget.Scope != expected[i] || !validAdmissionBucket(action, budget) {
			return false, ErrStorageUnavailable
		}
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		var now time.Time
		if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
			return err
		}
		for _, budget := range budgets {
			var hits int
			err := tx.QueryRow(ctx, `INSERT INTO mamony_identity.anonymous_admission_buckets
					(action, scope, key_digest, window_start, hits)
				VALUES ($1, $2, $3,
					date_bin(make_interval(secs => $4::int), $5::timestamptz,
						timestamptz '1970-01-01 00:00:00+00'), 1)
				ON CONFLICT (action, scope, key_digest, window_start)
				DO UPDATE SET hits = mamony_identity.anonymous_admission_buckets.hits + 1
				WHERE mamony_identity.anonymous_admission_buckets.hits < $6
				RETURNING hits`,
				string(action), budget.Scope, budget.Key[:], budget.WindowSeconds, now, budget.Limit).
				Scan(&hits)
			if errors.Is(err, pgx.ErrNoRows) {
				return errAdmissionSpent
			}
			if err != nil {
				return err
			}
			if hits < 1 || int32(hits) > budget.Limit {
				return ErrStorageUnavailable
			}
		}
		return nil
	})
	if errors.Is(err, errAdmissionSpent) {
		return false, nil
	}
	if err != nil {
		return false, ErrStorageUnavailable
	}
	return true, nil
}
