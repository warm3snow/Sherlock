// Copyright 2024 Sherlock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hosts

import (
	"context"
	"database/sql"
	"fmt"
)

// CreateTag creates a new tag.
func (m *Manager) CreateTag(ctx context.Context, tag *Tag) error {
	if tag.Color == "" {
		tag.Color = "#808080"
	}

	result, err := m.db.ExecContext(ctx, `
		INSERT INTO tags (name, color)
		VALUES (?, ?)
	`, tag.Name, tag.Color)
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	tag.ID = id

	return nil
}

// UpdateTag updates an existing tag.
func (m *Manager) UpdateTag(ctx context.Context, tag *Tag) error {
	_, err := m.db.ExecContext(ctx, `
		UPDATE tags SET name = ?, color = ?
		WHERE id = ?
	`, tag.Name, tag.Color, tag.ID)
	if err != nil {
		return fmt.Errorf("failed to update tag: %w", err)
	}
	return nil
}

// DeleteTag deletes a tag by ID.
func (m *Manager) DeleteTag(ctx context.Context, id int64) error {
	result, err := m.db.ExecContext(ctx, "DELETE FROM tags WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("tag not found")
	}

	return nil
}

// GetTag returns a tag by ID.
func (m *Manager) GetTag(ctx context.Context, id int64) (*Tag, error) {
	var t Tag
	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, color FROM tags WHERE id = ?
	`, id).Scan(&t.ID, &t.Name, &t.Color)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tag not found")
		}
		return nil, err
	}
	return &t, nil
}

// GetTagByName returns a tag by name.
func (m *Manager) GetTagByName(ctx context.Context, name string) (*Tag, error) {
	var t Tag
	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, color FROM tags WHERE name = ?
	`, name).Scan(&t.ID, &t.Name, &t.Color)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tag not found")
		}
		return nil, err
	}
	return &t, nil
}

// ListTags returns all tags.
func (m *Manager) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, name, color FROM tags ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			continue
		}
		tags = append(tags, t)
	}

	return tags, nil
}

// AddTagToHost adds a tag to a host.
func (m *Manager) AddTagToHost(ctx context.Context, hostID, tagID int64) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO host_tags (host_id, tag_id)
		VALUES (?, ?)
	`, hostID, tagID)
	if err != nil {
		return fmt.Errorf("failed to add tag to host: %w", err)
	}
	return nil
}

// AddTagToHostByName adds a tag to a host by tag name.
func (m *Manager) AddTagToHostByName(ctx context.Context, hostID int64, tagName string) error {
	tag, err := m.GetTagByName(ctx, tagName)
	if err != nil {
		// Create tag if it doesn't exist
		tag = &Tag{Name: tagName}
		if err := m.CreateTag(ctx, tag); err != nil {
			return err
		}
	}
	return m.AddTagToHost(ctx, hostID, tag.ID)
}

// RemoveTagFromHost removes a tag from a host.
func (m *Manager) RemoveTagFromHost(ctx context.Context, hostID, tagID int64) error {
	_, err := m.db.ExecContext(ctx, `
		DELETE FROM host_tags
		WHERE host_id = ? AND tag_id = ?
	`, hostID, tagID)
	if err != nil {
		return fmt.Errorf("failed to remove tag from host: %w", err)
	}
	return nil
}

// RemoveTagFromHostByName removes a tag from a host by tag name.
func (m *Manager) RemoveTagFromHostByName(ctx context.Context, hostID int64, tagName string) error {
	tag, err := m.GetTagByName(ctx, tagName)
	if err != nil {
		return err
	}
	return m.RemoveTagFromHost(ctx, hostID, tag.ID)
}

// GetHostsByTag returns all hosts with a specific tag.
func (m *Manager) GetHostsByTag(ctx context.Context, tagID int64) ([]Host, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT h.id, h.host, h.port, h.user, h.alias, h.description, h.timestamp, h.has_pub_key, h.login_count
		FROM hosts h
		JOIN host_tags ht ON h.id = ht.host_id
		WHERE ht.tag_id = ?
		ORDER BY h.timestamp DESC
	`, tagID)
	if err != nil {
		return nil, fmt.Errorf("failed to get hosts by tag: %w", err)
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		host, err := m.scanHostFromRows(rows)
		if err != nil {
			continue
		}
		hosts = append(hosts, *host)
	}

	return hosts, nil
}

// GetHostsByTagName returns all hosts with a specific tag by tag name.
func (m *Manager) GetHostsByTagName(ctx context.Context, tagName string) ([]Host, error) {
	tag, err := m.GetTagByName(ctx, tagName)
	if err != nil {
		return nil, err
	}
	return m.GetHostsByTag(ctx, tag.ID)
}
