package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/joincode"
)

// join_codes.go: the codes a laptop joins with (design 0015). Like the
// elevation queue it stores facts and never a state; see
// 0020_join_codes.sql.

// keepClosed is how long a closed code's row stays, so the list of who made
// which code can still be asked about after it was used.
const keepClosed = 7 * 24 * time.Hour

// JoinCodes exposes the code store on the pool.
func (s *Store) JoinCodes() *JoinCodeStore { return &JoinCodeStore{s} }

// JoinCodeStore implements ports.JoinCodeStore.
type JoinCodeStore struct{ s *Store }

// Create stores a new code, and prunes the rows of codes closed for longer
// than a week: making a code is rare enough to carry that, and it means no
// timer has to run for the table to stay small.
func (j *JoinCodeStore) Create(ctx context.Context, tenant string, c joincode.Code) error {
	if _, err := j.s.pool.Exec(ctx, `DELETE FROM join_codes WHERE tenant=$1 AND expires < $2`,
		tenant, c.Created.Add(-keepClosed)); err != nil {
		return fmt.Errorf("prune join codes: %w", err)
	}
	_, err := j.s.pool.Exec(ctx, `
		INSERT INTO join_codes (tenant, id, hash, grp, tag, uses, created, created_by, expires)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		tenant, c.ID, c.Hash, c.Group, c.Tag, c.Uses, c.Created, c.CreatedBy, c.Expires)
	if err != nil {
		return fmt.Errorf("create join code %s: %w", c.ID, err)
	}
	return nil
}

// Spend takes one use of the open code with this hash and returns the code
// as it was before. ok is false when there is no such code, or it is spent,
// or it has expired - deliberately one answer for all three.
func (j *JoinCodeStore) Spend(ctx context.Context, tenant string, hash []byte, now time.Time) (joincode.Code, bool, error) {
	row := j.s.pool.QueryRow(ctx, `
		UPDATE join_codes SET uses = uses - 1
		WHERE tenant=$1 AND hash=$2 AND uses > 0 AND expires > $3
		RETURNING id, hash, grp, tag, uses + 1, created, created_by, expires`,
		tenant, hash, now)
	c, err := scanJoinCode(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return joincode.Code{}, false, nil
	}
	if err != nil {
		return joincode.Code{}, false, fmt.Errorf("spend join code: %w", err)
	}
	return c, true, nil
}

// Refund gives back the use a failed join spent.
func (j *JoinCodeStore) Refund(ctx context.Context, tenant, id string) error {
	if _, err := j.s.pool.Exec(ctx, `UPDATE join_codes SET uses = uses + 1 WHERE tenant=$1 AND id=$2`,
		tenant, id); err != nil {
		return fmt.Errorf("refund join code %s: %w", id, err)
	}
	return nil
}

// Open lists the codes that can still be used at now, newest first.
func (j *JoinCodeStore) Open(ctx context.Context, tenant string, now time.Time) ([]joincode.Code, error) {
	rows, err := j.s.pool.Query(ctx, `
		SELECT id, hash, grp, tag, uses, created, created_by, expires
		FROM join_codes WHERE tenant=$1 AND uses > 0 AND expires > $2
		ORDER BY created DESC`, tenant, now)
	if err != nil {
		return nil, fmt.Errorf("list join codes: %w", err)
	}
	defer rows.Close()
	var out []joincode.Code
	for rows.Next() {
		c, err := scanJoinCode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan join code: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Withdraw closes a code by spending all its uses. The row stays, so the
// list of who made which code keeps it until it is pruned.
func (j *JoinCodeStore) Withdraw(ctx context.Context, tenant, id string) error {
	if _, err := j.s.pool.Exec(ctx, `UPDATE join_codes SET uses = 0 WHERE tenant=$1 AND id=$2`,
		tenant, id); err != nil {
		return fmt.Errorf("withdraw join code %s: %w", id, err)
	}
	return nil
}

func scanJoinCode(row scannable) (joincode.Code, error) {
	var c joincode.Code
	err := row.Scan(&c.ID, &c.Hash, &c.Group, &c.Tag, &c.Uses, &c.Created, &c.CreatedBy, &c.Expires)
	return c, err
}
