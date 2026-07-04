-- +goose Up
CREATE TABLE template_statistic (
    code TEXT NOT NULL,
    use_cnt INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX tmpls_code_ui 
    ON template_statistic(code);

-- +goose Down
DROP INDEX IF EXISTS tmpls_code_ui;
DROP TABLE IF EXISTS template_statistic;