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

// CreateGroup creates a new host group.
func (m *Manager) CreateGroup(ctx context.Context, group *Group) error {
	var description sql.NullString
	if group.Description != "" {
		description = sql.NullString{String: group.Description, Valid: true}
	}

	result, err := m.db.ExecContext(ctx, `
		INSERT INTO host_groups (name, description)
		VALUES (?, ?)
	`, group.Name, description)
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
	var description sql.NullString
	if group.Description != "" {
		description = sql.NullString{String: group.Description, Valid: true}
	}

	_, err := m.db.ExecContext(ctx, `
		UPDATE host_groups SET name = ?, description = ?
		WHERE id = ?
	`, group.Name, description, group.ID)
	if err != nil {
		return fmt.Errorf("failed to update group: %w", err)
	}

	return nil
}

// DeleteGroup deletes a group by ID.
func (m *Manager) DeleteGroup(ctx context.Context, id int64) error {
	result, err := m.db.ExecContext(ctx, "DELETE FROM host_groups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("group not found")
	}

	return nil
}

// GetGroup returns a group by ID.
func (m *Manager) GetGroup(ctx context.Context, id int64) (*Group, error) {
	var g Group
	var description sql.NullString
	var createdAt string

	err := m.db.QueryRowContext(ctx, `
		SELECT g.id, g.name, g.description, g.created_at,
			   (SELECT COUNT(*) FROM host_group_members WHERE group_id = g.id) as host_count
		FROM host_groups g
		WHERE g.id = ?
	`, id).Scan(&g.ID, &g.Name, &description, &createdAt, &g.HostCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("group not found")
		}
		return nil, err
	}

	if description.Valid {
		g.Description = description.String
	}
	g.CreatedAt = parseTimestamp(createdAt)

	return &g, nil
}

// GetGroupByName returns a group by name.
func (m *Manager) GetGroupByName(ctx context.Context, name string) (*Group, error) {
	var g Group
	var description sql.NullString
	var createdAt string

	err := m.db.QueryRowContext(ctx, `
		SELECT g.id, g.name, g.description, g.created_at,
			   (SELECT COUNT(*) FROM host_group_members WHERE group_id = g.id) as host_count
		FROM host_groups g
		WHERE g.name = ?
	`, name).Scan(&g.ID, &g.Name, &description, &createdAt, &g.HostCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("group not found")
		}
		return nil, err
	}

	if description.Valid {
		g.Description = description.String
	}
	g.CreatedAt = parseTimestamp(createdAt)

	return &g, nil
}

// ListGroups returns all groups.
func (m *Manager) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT g.id, g.name, g.description, g.created_at,
			   (SELECT COUNT(*) FROM host_group_members WHERE group_id = g.id) as host_count
		FROM host_groups g
		ORDER BY g.name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		var description sql.NullString
		var createdAt string
		if err := rows.Scan(&g.ID, &g.Name, &description, &createdAt, &g.HostCount); err != nil {
			continue
		}
		if description.Valid {
			g.Description = description.String
		}
		g.CreatedAt = parseTimestamp(createdAt)
		groups = append(groups, g)
	}

	return groups, nil
}

// GetGroupHosts returns all hosts in a group.
func (m *Manager) GetGroupHosts(ctx context.Context, groupID int64) ([]Host, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT h.id, h.host, h.port, h.user, h.alias, h.description, h.timestamp, h.has_pub_key, h.login_count
		FROM hosts h
		JOIN host_group_members hgm ON h.id = hgm.host_id
		WHERE hgm.group_id = ?
		ORDER BY h.timestamp DESC
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group hosts: %w", err)
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

// GetGroupHostsByName returns all hosts in a group by group name.
func (m *Manager) GetGroupHostsByName(ctx context.Context, groupName string) ([]Host, error) {
	group, err := m.GetGroupByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	return m.GetGroupHosts(ctx, group.ID)
}

// AddHostToGroup adds a host to a group.
func (m *Manager) AddHostToGroup(ctx context.Context, hostID, groupID int64) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO host_group_members (host_id, group_id)
		VALUES (?, ?)
	`, hostID, groupID)
	if err != nil {
		return fmt.Errorf("failed to add host to group: %w", err)
	}
	return nil
}

// AddHostToGroupByName adds a host to a group by group name.
func (m *Manager) AddHostToGroupByName(ctx context.Context, hostID int64, groupName string) error {
	group, err := m.GetGroupByName(ctx, groupName)
	if err != nil {
		// Create group if it doesn't exist
		group = &Group{Name: groupName}
		if err := m.CreateGroup(ctx, group); err != nil {
			return err
		}
	}
	return m.AddHostToGroup(ctx, hostID, group.ID)
}

// RemoveHostFromGroup removes a host from a group.
func (m *Manager) RemoveHostFromGroup(ctx context.Context, hostID, groupID int64) error {
	_, err := m.db.ExecContext(ctx, `
		DELETE FROM host_group_members
		WHERE host_id = ? AND group_id = ?
	`, hostID, groupID)
	if err != nil {
		return fmt.Errorf("failed to remove host from group: %w", err)
	}
	return nil
}

// RemoveHostFromGroupByName removes a host from a group by group name.
func (m *Manager) RemoveHostFromGroupByName(ctx context.Context, hostID int64, groupName string) error {
	group, err := m.GetGroupByName(ctx, groupName)
	if err != nil {
		return err
	}
	return m.RemoveHostFromGroup(ctx, hostID, group.ID)
}
