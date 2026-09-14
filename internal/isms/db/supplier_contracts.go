package db

import "context"

// SupplierContractDue is a supplier sitting exactly on one of the contract
// expiry notification marks today.
//
// DaysUntil is the discrete mark itself (30, 7 or 0) rather than a range, which
// is what makes "fire once per threshold" hold with no stored notification
// state: the day has to land on the mark, and a day only lands on it once.
type SupplierContractDue struct {
	ID          int64
	Identifier  string
	Name        string
	Criticality string
	Owner       string // resolved owner email; "" when owner_id is NULL
	DaysUntil   int    // 30, 7 or 0
}

// The notification marks are 30, 7 and 0 days before expiry, inlined into the
// query below. Hardcoded on purpose (#44): no org setting, no threshold to
// configure, so there is nothing for a named variable to parameterise. After
// expiry (days_until < 0) nothing fires — the overdue summary carries the state
// from then on instead.

// SupplierContractsAtMarks returns suppliers whose contract_expiry falls exactly
// on one of the 30/7/0-day marks relative to today.
//
// The arithmetic is SQL-side for the same reason as in GetOverdueSummary:
// contract_expiry is a DATE and CURRENT_DATE is the only subtraction that does
// not drift by a day around a timezone boundary.
func (d *DB) SupplierContractsAtMarks(ctx context.Context, orgID int) ([]SupplierContractDue, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT s.id, s.identifier, s.name, COALESCE(s.criticality, ''),
			COALESCE((SELECT email FROM users WHERE id = s.owner_id), ''),
			(s.contract_expiry - CURRENT_DATE)::int
		FROM suppliers s
		WHERE s.organization_id = $1
			AND s.contract_expiry IS NOT NULL
			AND (s.contract_expiry - CURRENT_DATE)::int IN (30, 7, 0)
			AND COALESCE(s.status, 'active') <> 'terminated'
			AND s.deleted_at IS NULL
		ORDER BY s.contract_expiry ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SupplierContractDue
	for rows.Next() {
		var c SupplierContractDue
		if err := rows.Scan(&c.ID, &c.Identifier, &c.Name, &c.Criticality, &c.Owner, &c.DaysUntil); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
