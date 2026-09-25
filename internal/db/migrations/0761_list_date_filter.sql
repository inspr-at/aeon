-- SPDX-License-Identifier: AGPL-3.0-only
-- The ticket list filters by dates that imported work keeps in its fields
-- (start_date, end_date, accepted_at). Those are free text: this reads the
-- leading calendar date and returns NULL for anything that is not one, so a
-- malformed value never fails a list query.
CREATE FUNCTION aeon_field_date(value text) RETURNS date
LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE AS $$
BEGIN
    IF value IS NULL OR value !~ '^\d{4}-\d{2}-\d{2}' THEN
        RETURN NULL;
    END IF;
    RETURN make_date(substr(value, 1, 4)::int, substr(value, 6, 2)::int, substr(value, 9, 2)::int);
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$;
