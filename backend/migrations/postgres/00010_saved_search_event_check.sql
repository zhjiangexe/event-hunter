-- +goose Up
ALTER TABLE saved_searches
    DROP CONSTRAINT saved_searches_target_chk;

ALTER TABLE saved_searches
    ADD CONSTRAINT saved_searches_target_chk
    CHECK (target IN ('TIMELINE', 'JOURNEY', 'EVENT_CHECK'));

-- +goose Down
DELETE FROM saved_searches WHERE target = 'EVENT_CHECK';

ALTER TABLE saved_searches
    DROP CONSTRAINT saved_searches_target_chk;

ALTER TABLE saved_searches
    ADD CONSTRAINT saved_searches_target_chk
    CHECK (target IN ('TIMELINE', 'JOURNEY'));
