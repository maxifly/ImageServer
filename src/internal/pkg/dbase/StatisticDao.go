package dbase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type StatisticDao struct {
	db *sql.DB
}

func NewStatisticDao(db *sql.DB) *StatisticDao {
	return &StatisticDao{db: db}
}

func (dao *StatisticDao) Increment(ctx context.Context, code string) (int64, error) {

	if code == "" {
		return 0, errors.New("code must not be empty")
	}

	var newValue int64

	updateQuery := `
UPDATE template_statistic
SET use_cnt = use_cnt + 1
WHERE code = ?
RETURNING use_cnt;
`

	err := dao.db.QueryRowContext(ctx, updateQuery, code).Scan(&newValue)
	if err == nil {
		return newValue, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("update counter %q: %w", code, err)
	}

	insertQuery := `
INSERT INTO template_statistic (code, use_cnt)
VALUES (?, 1)
ON CONFLICT(code) DO UPDATE SET use_cnt = use_cnt + 1
RETURNING use_cnt;
`

	err = dao.db.QueryRowContext(ctx, insertQuery, code).Scan(&newValue)
	if err != nil {
		return 0, fmt.Errorf("insert fallback counter %q: %w", code, err)
	}

	return newValue, nil
}

func (dao *StatisticDao) Delete(ctx context.Context, code string) error {
	if code == "" {
		return errors.New("code must not be empty")
	}

	_, err := dao.db.ExecContext(ctx, `
DELETE FROM template_statistic
WHERE code = ?;
`, code)

	if err != nil {
		return fmt.Errorf("delete counter %q: %w", code, err)
	}

	return nil
}

func (dao *StatisticDao) GetAll(ctx context.Context) (map[string]StatisticItem, error) {
	rows, err := dao.db.QueryContext(ctx, `
SELECT code, use_cnt
FROM template_statistic;
`)
	if err != nil {
		return nil, fmt.Errorf("get all counters: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	result := make(map[string]StatisticItem)

	for rows.Next() {
		var code string
		var useCnt int64

		if err := rows.Scan(&code, &useCnt); err != nil {
			return nil, fmt.Errorf("scan counter row: %w", err)
		}

		result[code] = StatisticItem{
			Code:   code,
			UseCnt: useCnt,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate counters: %w", err)
	}

	return result, nil
}
