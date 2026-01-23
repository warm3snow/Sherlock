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

package database

import (
	"context"
	"database/sql"
	"fmt"
)

// CreateGroup creates a new database group.
func (m *Manager) CreateGroup(ctx context.Context, group *Group) error {
	result, err := m.db.ExecContext(ctx, `
		INSERT INTO db_groups (name, description) VALUES (?, ?)
	`, group.Name, group.Description)
	if err != nil {
		return fmt.Errorf("failed to create group: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	group.ID = id

	return nil
}

// UpdateGroup updates an existing group.
func (m *Manager) UpdateGroup(ctx context.Context, group *Group) error {
	_, err := m.db.ExecContext(ctx, `
		UPDATE db_groups SET name = ?, description = ? WHERE id = ?
	`, group.Name, group.Description, group.ID)
	return err
}

// DeleteGroup deletes a group.
func (m *Manager) DeleteGroup(ctx context.Context, id int64) error {
	result, err := m.db.ExecContext(ctx, `DELETE FROM db_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("group not found")
	}

	return nil
}

// GetGroup returns a group by ID.
func (m *Manager) GetGroup(ctx context.Context, id int64) (*Group, error) {
	group := &Group{}
	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, description FROM db_groups WHERE id = ?
	`, id).Scan(&group.ID, &group.Name, &group.Description)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("group not found")
		}
		return nil, err
	}
	return group, nil
}

// GetGroupByName returns a group by name.
func (m *Manager) GetGroupByName(ctx context.Context, name string) (*Group, error) {
	group := &Group{}
	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, description FROM db_groups WHERE name = ?
	`, name).Scan(&group.ID, &group.Name, &group.Description)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("group not found")
		}
		return nil, err
	}
	return group, nil
}

// ListGroups returns all groups.
func (m *Manager) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, name, description FROM db_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description); err != nil {
			continue
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// AddConnectionToGroup adds a connection to a group.
func (m *Manager) AddConnectionToGroup(ctx context.Context, connID, groupID int64) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO db_group_members (db_id, group_id) VALUES (?, ?)
	`, connID, groupID)
	return err
}

// AddConnectionToGroupByName adds a connection to a group by name.
func (m *Manager) AddConnectionToGroupByName(ctx context.Context, connID int64, groupName string) error {
	group, err := m.GetGroupByName(ctx, groupName)
	if err != nil {
		return err
	}
	return m.AddConnectionToGroup(ctx, connID, group.ID)
}

// RemoveConnectionFromGroup removes a connection from a group.
func (m *Manager) RemoveConnectionFromGroup(ctx context.Context, connID, groupID int64) error {
	result, err := m.db.ExecContext(ctx, `
		DELETE FROM db_group_members WHERE db_id = ? AND group_id = ?
	`, connID, groupID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("connection not in group")
	}
	return nil
}

// GetConnectionGroups returns all groups for a connection.
func (m *Manager) GetConnectionGroups(ctx context.Context, connID int64) ([]Group, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT g.id, g.name, g.description
		FROM db_groups g
		JOIN db_group_members gm ON g.id = gm.group_id
		WHERE gm.db_id = ?
		ORDER BY g.name
	`, connID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description); err != nil {
			continue
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// GetGroupConnections returns all connections in a group.
func (m *Manager) GetGroupConnections(ctx context.Context, groupID int64) ([]Connection, error) {
	return m.ListConnections(ctx, &ListFilter{GroupID: groupID})
}

// CreateTag creates a new tag.
func (m *Manager) CreateTag(ctx context.Context, tag *Tag) error {
	if tag.Color == "" {
		tag.Color = "#808080"
	}

	result, err := m.db.ExecContext(ctx, `
		INSERT INTO tags (name, color) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET color = excluded.color
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

// DeleteTag deletes a tag.
func (m *Manager) DeleteTag(ctx context.Context, id int64) error {
	result, err := m.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("tag not found")
	}
	return nil
}

// GetTag returns a tag by ID.
func (m *Manager) GetTag(ctx context.Context, id int64) (*Tag, error) {
	tag := &Tag{}
	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, color FROM tags WHERE id = ?
	`, id).Scan(&tag.ID, &tag.Name, &tag.Color)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tag not found")
		}
		return nil, err
	}
	return tag, nil
}

// GetTagByName returns a tag by name.
func (m *Manager) GetTagByName(ctx context.Context, name string) (*Tag, error) {
	tag := &Tag{}
	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, color FROM tags WHERE name = ?
	`, name).Scan(&tag.ID, &tag.Name, &tag.Color)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tag not found")
		}
		return nil, err
	}
	return tag, nil
}

// ListTags returns all tags.
func (m *Manager) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, name, color FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
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

// AddTagToConnection adds a tag to a connection.
func (m *Manager) AddTagToConnection(ctx context.Context, connID, tagID int64) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO db_tags (db_id, tag_id) VALUES (?, ?)
	`, connID, tagID)
	return err
}

// AddTagToConnectionByName adds a tag to a connection by name.
func (m *Manager) AddTagToConnectionByName(ctx context.Context, connID int64, tagName string) error {
	tag, err := m.GetTagByName(ctx, tagName)
	if err != nil {
		// Create tag if not exists
		tag = &Tag{Name: tagName}
		if err := m.CreateTag(ctx, tag); err != nil {
			return err
		}
	}
	return m.AddTagToConnection(ctx, connID, tag.ID)
}

// RemoveTagFromConnection removes a tag from a connection.
func (m *Manager) RemoveTagFromConnection(ctx context.Context, connID, tagID int64) error {
	result, err := m.db.ExecContext(ctx, `
		DELETE FROM db_tags WHERE db_id = ? AND tag_id = ?
	`, connID, tagID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("tag not on connection")
	}
	return nil
}

// GetConnectionTags returns all tags for a connection.
func (m *Manager) GetConnectionTags(ctx context.Context, connID int64) ([]Tag, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.color
		FROM tags t
		JOIN db_tags dt ON t.id = dt.tag_id
		WHERE dt.db_id = ?
		ORDER BY t.name
	`, connID)
	if err != nil {
		return nil, err
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
