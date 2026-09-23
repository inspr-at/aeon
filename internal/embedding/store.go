// SPDX-License-Identifier: AGPL-3.0-only

package embedding

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/inspr-at/aeon/internal/db"
)

type claimedJob struct {
	NodeID   string
	QueuedAt time.Time
	Title    string
	Body     string
	Hash     string
}

func (w *Worker) claim(ctx context.Context, tenantID string) ([]claimedJob, int, error) {
	var live []claimedJob
	var dropped int
	err := db.InTenant(ctx, w.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH picked AS (
				SELECT tenant_id, node_id
				FROM node_embedding_jobs
				WHERE status IN ('queued', 'failed')
				  AND attempts < $1
				  AND (
				    $2::boolean
				    OR status = 'queued'
				    OR updated_at <= now() - (interval '1 second' * least(60::numeric, power(2::numeric, attempts::numeric)))
				  )
				ORDER BY queued_at, node_id
				FOR UPDATE SKIP LOCKED
				LIMIT $3
			)
			UPDATE node_embedding_jobs j
			SET status = 'running', attempts = j.attempts + 1, updated_at = now()
			FROM picked p
			WHERE j.tenant_id = p.tenant_id AND j.node_id = p.node_id
			RETURNING j.node_id::text, j.queued_at`,
			w.maxTries, w.immediate, w.batch)
		if err != nil {
			return err
		}
		defer rows.Close()
		type picked struct {
			id     string
			queued time.Time
		}
		var got []picked
		for rows.Next() {
			var item picked
			if err := rows.Scan(&item.id, &item.queued); err != nil {
				return err
			}
			got = append(got, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		for _, item := range got {
			var title, body string
			var deleted *time.Time
			err := tx.QueryRow(ctx, `
				SELECT title, body, deleted_at FROM nodes WHERE id = $1`, item.id).
				Scan(&title, &body, &deleted)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && deleted != nil) {
				tag, delErr := tx.Exec(ctx, `
					DELETE FROM node_embedding_jobs
					WHERE node_id = $1 AND queued_at = $2`, item.id, item.queued)
				if delErr != nil {
					return delErr
				}
				if tag.RowsAffected() == 1 {
					dropped++
				}
				continue
			}
			if err != nil {
				return err
			}
			live = append(live, claimedJob{
				NodeID:   item.id,
				QueuedAt: item.queued,
				Title:    title,
				Body:     body,
				Hash:     ContentHash(title, body),
			})
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return live, dropped, nil
}

func (w *Worker) finish(ctx context.Context, tenantID, model string, job claimedJob, vector []float32) (bool, error) {
	var done bool
	err := db.InTenant(ctx, w.pool, tenantID, func(tx pgx.Tx) error {
		var title, body string
		var deleted *time.Time
		err := tx.QueryRow(ctx, `
			SELECT title, body, deleted_at FROM nodes WHERE id = $1 FOR UPDATE`, job.NodeID).
			Scan(&title, &body, &deleted)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var matches bool
		err = tx.QueryRow(ctx, `
			SELECT queued_at = $2
			FROM node_embedding_jobs
			WHERE node_id = $1
			FOR UPDATE`, job.NodeID, job.QueuedAt).Scan(&matches)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !matches) {
			return nil
		}
		if err != nil {
			return err
		}
		if deleted != nil {
			tag, err := tx.Exec(ctx, `
				DELETE FROM node_embedding_jobs
				WHERE node_id = $1 AND queued_at = $2`, job.NodeID, job.QueuedAt)
			if err != nil {
				return err
			}
			done = tag.RowsAffected() == 1
			return nil
		}
		hash := ContentHash(title, body)
		if hash != job.Hash {
			_, err = tx.Exec(ctx, `
				UPDATE node_embedding_jobs
				SET status = 'queued', updated_at = now()
				WHERE node_id = $1 AND queued_at = $2`, job.NodeID, job.QueuedAt)
			return err
		}
		var prevModel, prevHash string
		err = tx.QueryRow(ctx, `
			SELECT model, content_hash FROM node_embeddings WHERE node_id = $1`, job.NodeID).
			Scan(&prevModel, &prevHash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if errors.Is(err, pgx.ErrNoRows) || prevModel != model || prevHash != hash {
			if _, err = tx.Exec(ctx, `
				INSERT INTO node_embeddings (tenant_id, node_id, model, content_hash, embedding)
				VALUES ($1, $2, $3, $4, $5::real[]::halfvec(1536))
				ON CONFLICT (tenant_id, node_id) DO UPDATE
				SET model = EXCLUDED.model,
				    content_hash = EXCLUDED.content_hash,
				    embedding = EXCLUDED.embedding,
				    updated_at = now()`,
				tenantID, job.NodeID, model, hash, pgtype.FlatArray[float32](vector)); err != nil {
				return err
			}
			actor, err := ensureActor(ctx, tx, tenantID)
			if err != nil {
				return err
			}
			nodeID := job.NodeID
			after := embeddingSnapshot{NodeID: nodeID, Model: model, ContentHash: hash}
			var before any
			if prevModel != "" {
				before = embeddingSnapshot{NodeID: nodeID, Model: prevModel, ContentHash: prevHash}
			}
			if err := w.append(ctx, tx, actor, Change{
				NodeID: &nodeID,
				Type:   eventEmbedded,
				Before: before,
				After:  after,
			}); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `
			DELETE FROM node_embedding_jobs
			WHERE node_id = $1 AND queued_at = $2`, job.NodeID, job.QueuedAt)
		if err != nil {
			return err
		}
		done = tag.RowsAffected() == 1
		return nil
	})
	return done, err
}
