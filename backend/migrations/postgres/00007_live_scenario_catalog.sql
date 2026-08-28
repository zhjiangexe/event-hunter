-- +goose Up
ALTER TABLE scenario_runs DROP CONSTRAINT IF EXISTS scenario_runs_scenario_id_chk;
ALTER TABLE scenario_runs
    ADD CONSTRAINT scenario_runs_scenario_id_chk CHECK (scenario_id ~ '^S([1-9]|1[0-4])$');

-- +goose Down
ALTER TABLE scenario_runs DROP CONSTRAINT IF EXISTS scenario_runs_scenario_id_chk;
ALTER TABLE scenario_runs
    ADD CONSTRAINT scenario_runs_scenario_id_chk CHECK (scenario_id ~ '^S([1-9]|1[01])$');
