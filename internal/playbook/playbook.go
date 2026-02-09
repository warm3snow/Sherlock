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

// Package playbook provides automated operations playbook management and execution.
package playbook

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Category defines playbook categories.
type Category string

const (
	CategoryInspect Category = "inspect" // 巡检类
	CategoryCleanup Category = "cleanup" // 清理类
	CategoryDeploy  Category = "deploy"  // 部署类
	CategoryBackup  Category = "backup"  // 备份类
	CategoryRecover Category = "recover" // 恢复类
	CategoryCustom  Category = "custom"  // 自定义
)

// PlaybookStep represents a single step in a playbook.
type PlaybookStep struct {
	Name            string `json:"name"`
	Command         string `json:"command"`
	Description     string `json:"description,omitempty"`
	ContinueOnError bool   `json:"continue_on_error"`
	Timeout         int    `json:"timeout"` // seconds, 0 = default (60s)
	ExpectedExitCode int   `json:"expected_exit_code"`
	RetryCount      int    `json:"retry_count"`
}

// Playbook represents an automated operations playbook.
type Playbook struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    Category          `json:"category"`
	Steps       []PlaybookStep    `json:"steps"`
	Variables   map[string]string `json:"variables,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Author      string            `json:"author,omitempty"`
	Version     string            `json:"version,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	UsageCount  int               `json:"usage_count"`
	IsBuiltin   bool              `json:"is_builtin"`
}

// Manager manages playbooks.
type Manager struct {
	db      *sql.DB
	builtin map[string]*Playbook
}

// NewManager creates a new playbook manager.
func NewManager(db *sql.DB) (*Manager, error) {
	m := &Manager{
		db:      db,
		builtin: make(map[string]*Playbook),
	}

	if err := m.initTable(); err != nil {
		return nil, err
	}

	m.loadBuiltinPlaybooks()

	return m, nil
}

// initTable creates the playbooks table.
func (m *Manager) initTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS playbooks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		category TEXT DEFAULT 'custom',
		steps TEXT NOT NULL,
		variables TEXT,
		tags TEXT,
		author TEXT,
		version TEXT DEFAULT '1.0',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		usage_count INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_playbook_name ON playbooks(name);
	CREATE INDEX IF NOT EXISTS idx_playbook_category ON playbooks(category);
	`
	_, err := m.db.Exec(query)
	return err
}

// loadBuiltinPlaybooks loads built-in playbooks.
func (m *Manager) loadBuiltinPlaybooks() {
	// Daily inspection playbook
	m.builtin["daily-inspect"] = &Playbook{
		Name:        "daily-inspect",
		Description: "每日系统巡检",
		Category:    CategoryInspect,
		IsBuiltin:   true,
		Steps: []PlaybookStep{
			{Name: "系统信息", Command: "uname -a && hostname", Description: "获取系统基本信息"},
			{Name: "运行时间", Command: "uptime", Description: "查看系统运行时间和负载"},
			{Name: "磁盘使用", Command: "df -h", Description: "检查磁盘使用情况"},
			{Name: "内存使用", Command: "free -h", Description: "检查内存使用情况"},
			{Name: "CPU信息", Command: "top -bn1 | head -5", Description: "查看CPU使用情况"},
			{Name: "网络连接", Command: "ss -tuln | head -20", Description: "查看网络监听端口"},
			{Name: "最近登录", Command: "last -n 5", Description: "查看最近登录记录"},
		},
	}

	// Disk cleanup playbook
	m.builtin["disk-cleanup"] = &Playbook{
		Name:        "disk-cleanup",
		Description: "磁盘清理（清理日志和临时文件）",
		Category:    CategoryCleanup,
		IsBuiltin:   true,
		Steps: []PlaybookStep{
			{Name: "清理前", Command: "df -h /", Description: "查看清理前磁盘使用"},
			{Name: "清理日志", Command: "sudo find /var/log -type f -name '*.gz' -mtime +7 -delete 2>/dev/null || true", ContinueOnError: true},
			{Name: "清理临时文件", Command: "sudo find /tmp -type f -mtime +3 -delete 2>/dev/null || true", ContinueOnError: true},
			{Name: "清理包缓存", Command: "sudo apt-get clean 2>/dev/null || sudo yum clean all 2>/dev/null || true", ContinueOnError: true},
			{Name: "清理后", Command: "df -h /", Description: "查看清理后磁盘使用"},
		},
	}

	// Service check playbook
	m.builtin["service-check"] = &Playbook{
		Name:        "service-check",
		Description: "检查常用服务状态",
		Category:    CategoryInspect,
		IsBuiltin:   true,
		Variables:   map[string]string{"SERVICE": "nginx"},
		Steps: []PlaybookStep{
			{Name: "服务状态", Command: "systemctl status ${SERVICE} --no-pager 2>/dev/null || service ${SERVICE} status", ContinueOnError: true},
			{Name: "服务进程", Command: "pgrep -la ${SERVICE} || echo '服务未运行'", ContinueOnError: true},
			{Name: "服务日志", Command: "journalctl -u ${SERVICE} -n 20 --no-pager 2>/dev/null || tail -20 /var/log/${SERVICE}/*.log 2>/dev/null || true", ContinueOnError: true},
		},
	}

	// Security check playbook
	m.builtin["security-check"] = &Playbook{
		Name:        "security-check",
		Description: "安全检查",
		Category:    CategoryInspect,
		IsBuiltin:   true,
		Steps: []PlaybookStep{
			{Name: "失败登录", Command: "grep 'Failed password' /var/log/auth.log 2>/dev/null | tail -10 || grep 'Failed password' /var/log/secure 2>/dev/null | tail -10 || echo '无失败登录'", ContinueOnError: true},
			{Name: "当前用户", Command: "who", Description: "查看当前登录用户"},
			{Name: "可疑进程", Command: "ps aux --sort=-%cpu | head -10", Description: "高CPU进程"},
			{Name: "开放端口", Command: "ss -tuln", Description: "查看监听端口"},
			{Name: "最近修改", Command: "find /etc -type f -mtime -1 2>/dev/null | head -20", ContinueOnError: true},
		},
	}

	// Backup playbook
	m.builtin["backup-config"] = &Playbook{
		Name:        "backup-config",
		Description: "备份关键配置文件",
		Category:    CategoryBackup,
		IsBuiltin:   true,
		Variables: map[string]string{
			"BACKUP_DIR": "/tmp/backup",
		},
		Steps: []PlaybookStep{
			{Name: "创建目录", Command: "mkdir -p ${BACKUP_DIR}"},
			{Name: "备份hosts", Command: "cp /etc/hosts ${BACKUP_DIR}/", ContinueOnError: true},
			{Name: "备份passwd", Command: "cp /etc/passwd ${BACKUP_DIR}/", ContinueOnError: true},
			{Name: "备份crontab", Command: "crontab -l > ${BACKUP_DIR}/crontab.bak 2>/dev/null || true", ContinueOnError: true},
			{Name: "备份nginx", Command: "cp -r /etc/nginx ${BACKUP_DIR}/ 2>/dev/null || true", ContinueOnError: true},
			{Name: "压缩备份", Command: "cd ${BACKUP_DIR} && tar -czf backup_$(date +%Y%m%d).tar.gz * 2>/dev/null"},
			{Name: "显示备份", Command: "ls -lh ${BACKUP_DIR}/*.tar.gz 2>/dev/null || ls -lh ${BACKUP_DIR}/"},
		},
	}

	// Process management playbook
	m.builtin["process-top"] = &Playbook{
		Name:        "process-top",
		Description: "查看资源占用最高的进程",
		Category:    CategoryInspect,
		IsBuiltin:   true,
		Steps: []PlaybookStep{
			{Name: "CPU Top10", Command: "ps aux --sort=-%cpu | head -11"},
			{Name: "内存 Top10", Command: "ps aux --sort=-%mem | head -11"},
			{Name: "进程树", Command: "pstree -p | head -30"},
		},
	}
}

// Add adds a new playbook.
func (m *Manager) Add(pb *Playbook) error {
	stepsJSON, _ := json.Marshal(pb.Steps)
	varsJSON, _ := json.Marshal(pb.Variables)
	tagsJSON, _ := json.Marshal(pb.Tags)

	query := `
	INSERT INTO playbooks (name, description, category, steps, variables, tags, author, version)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := m.db.Exec(query, pb.Name, pb.Description, pb.Category,
		string(stepsJSON), string(varsJSON), string(tagsJSON), pb.Author, pb.Version)
	if err != nil {
		return err
	}

	pb.ID, _ = result.LastInsertId()
	return nil
}

// Get retrieves a playbook by name.
func (m *Manager) Get(name string) (*Playbook, error) {
	// Check builtin first
	if pb, ok := m.builtin[name]; ok {
		return pb, nil
	}

	query := `SELECT id, name, description, category, steps, variables, tags, author, version, created_at, updated_at, usage_count FROM playbooks WHERE name = ?`
	row := m.db.QueryRow(query, name)

	return m.scanPlaybook(row)
}

// GetByID retrieves a playbook by ID.
func (m *Manager) GetByID(id int64) (*Playbook, error) {
	query := `SELECT id, name, description, category, steps, variables, tags, author, version, created_at, updated_at, usage_count FROM playbooks WHERE id = ?`
	row := m.db.QueryRow(query, id)
	return m.scanPlaybook(row)
}

// List lists all playbooks.
func (m *Manager) List(category Category) ([]*Playbook, error) {
	var playbooks []*Playbook

	// Add builtin playbooks
	for _, pb := range m.builtin {
		if category == "" || pb.Category == category {
			playbooks = append(playbooks, pb)
		}
	}

	// Query user playbooks
	query := `SELECT id, name, description, category, steps, variables, tags, author, version, created_at, updated_at, usage_count FROM playbooks`
	args := []interface{}{}
	if category != "" {
		query += " WHERE category = ?"
		args = append(args, category)
	}
	query += " ORDER BY usage_count DESC, name"

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return playbooks, nil // Return builtin even if DB query fails
	}
	defer rows.Close()

	for rows.Next() {
		pb, err := m.scanPlaybookRow(rows)
		if err != nil {
			continue
		}
		playbooks = append(playbooks, pb)
	}

	return playbooks, nil
}

// Delete deletes a playbook.
func (m *Manager) Delete(name string) error {
	if _, ok := m.builtin[name]; ok {
		return fmt.Errorf("cannot delete builtin playbook: %s", name)
	}
	_, err := m.db.Exec("DELETE FROM playbooks WHERE name = ?", name)
	return err
}

// IncrementUsage increments the usage count.
func (m *Manager) IncrementUsage(name string) error {
	_, err := m.db.Exec("UPDATE playbooks SET usage_count = usage_count + 1, updated_at = CURRENT_TIMESTAMP WHERE name = ?", name)
	return err
}

// GetNames returns all playbook names (for autocomplete).
func (m *Manager) GetNames() []string {
	var names []string

	// Builtin names
	for name := range m.builtin {
		names = append(names, name)
	}

	// User playbook names
	rows, err := m.db.Query("SELECT name FROM playbooks ORDER BY name")
	if err != nil {
		return names
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			names = append(names, name)
		}
	}

	return names
}

// ExpandVariables expands variables in a command.
func ExpandVariables(command string, variables map[string]string) string {
	result := command
	for key, value := range variables {
		// Support both ${VAR} and $VAR syntax
		result = strings.ReplaceAll(result, "${"+key+"}", value)
		// Match $VAR but not ${VAR}
		re := regexp.MustCompile(`\$` + key + `(?:\b|$)`)
		result = re.ReplaceAllString(result, value)
	}
	return result
}

// ParseVariables extracts variable names from a command.
func ParseVariables(command string) []string {
	re := regexp.MustCompile(`\$\{?(\w+)\}?`)
	matches := re.FindAllStringSubmatch(command, -1)

	seen := make(map[string]bool)
	var vars []string
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			vars = append(vars, m[1])
			seen[m[1]] = true
		}
	}
	return vars
}

// scanPlaybook scans a single row into a Playbook.
func (m *Manager) scanPlaybook(row *sql.Row) (*Playbook, error) {
	var pb Playbook
	var stepsJSON, varsJSON, tagsJSON sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(&pb.ID, &pb.Name, &pb.Description, &pb.Category,
		&stepsJSON, &varsJSON, &tagsJSON, &pb.Author, &pb.Version,
		&createdAt, &updatedAt, &pb.UsageCount)
	if err != nil {
		return nil, err
	}

	if stepsJSON.Valid {
		json.Unmarshal([]byte(stepsJSON.String), &pb.Steps)
	}
	if varsJSON.Valid {
		json.Unmarshal([]byte(varsJSON.String), &pb.Variables)
	}
	if tagsJSON.Valid {
		json.Unmarshal([]byte(tagsJSON.String), &pb.Tags)
	}

	pb.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	pb.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	return &pb, nil
}

// scanPlaybookRow scans rows.Next() result into a Playbook.
func (m *Manager) scanPlaybookRow(rows *sql.Rows) (*Playbook, error) {
	var pb Playbook
	var stepsJSON, varsJSON, tagsJSON sql.NullString
	var createdAt, updatedAt string

	err := rows.Scan(&pb.ID, &pb.Name, &pb.Description, &pb.Category,
		&stepsJSON, &varsJSON, &tagsJSON, &pb.Author, &pb.Version,
		&createdAt, &updatedAt, &pb.UsageCount)
	if err != nil {
		return nil, err
	}

	if stepsJSON.Valid {
		json.Unmarshal([]byte(stepsJSON.String), &pb.Steps)
	}
	if varsJSON.Valid {
		json.Unmarshal([]byte(varsJSON.String), &pb.Variables)
	}
	if tagsJSON.Valid {
		json.Unmarshal([]byte(tagsJSON.String), &pb.Tags)
	}

	pb.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	pb.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	return &pb, nil
}
