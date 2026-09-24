-- SPDX-License-Identifier: AGPL-3.0-only
-- P1 quote documents are JSONB. Keep optional section formatting intact in
-- drafts and immutable version snapshots while rejecting malformed direct writes.
CREATE FUNCTION aeon_quote_section_format_valid(doc jsonb) RETURNS boolean
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    section jsonb;
    field text;
BEGIN
    IF jsonb_typeof(doc->'sections') IS DISTINCT FROM 'array' THEN RETURN false; END IF;
    FOR section IN SELECT value FROM jsonb_array_elements(doc->'sections') LOOP
        IF jsonb_typeof(section) IS DISTINCT FROM 'object' THEN RETURN false; END IF;
        IF section ? 'numbering_style' AND (
            jsonb_typeof(section->'numbering_style') IS DISTINCT FROM 'string' OR
            section->>'numbering_style' NOT IN ('decimal','upper-roman','lower-roman','upper-alpha','lower-alpha','none')
        ) THEN RETURN false; END IF;
        FOREACH field IN ARRAY ARRAY['page_break_before','keep_together'] LOOP
            IF section ? field AND jsonb_typeof(section->field) IS DISTINCT FROM 'boolean' THEN RETURN false; END IF;
        END LOOP;
        FOREACH field IN ARRAY ARRAY['spacing_before_mm','spacing_after_mm'] LOOP
            IF section ? field THEN
                IF jsonb_typeof(section->field) IS DISTINCT FROM 'string' THEN RETURN false; END IF;
                IF section->>field !~ '^(0|[1-9][0-9]?)(\.[0-9])?$' THEN RETURN false; END IF;
                IF (section->>field)::numeric > 40 THEN RETURN false; END IF;
            END IF;
        END LOOP;
    END LOOP;
    RETURN true;
END;
$$;

ALTER TABLE quote_drafts ADD CONSTRAINT quote_drafts_section_format_valid
    CHECK (aeon_quote_section_format_valid(document));
ALTER TABLE quote_version_snapshots ADD CONSTRAINT quote_version_snapshots_section_format_valid
    CHECK (aeon_quote_section_format_valid(document));
