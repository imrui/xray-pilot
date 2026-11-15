package entity

import "time"

const DefaultRoutingPolicy = `{"enable_ipv6_outbound":false,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_rules":[]}`

type RoutingTemplate struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex;not null" json:"name"`
	Description string    `json:"description"`
	Draft       string    `gorm:"type:text" json:"draft"`
	Version     uint      `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type RoutingTemplateRevision struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TemplateID    uint      `gorm:"uniqueIndex:idx_routing_revision,priority:1" json:"template_id"`
	Revision      uint      `gorm:"uniqueIndex:idx_routing_revision,priority:2" json:"revision"`
	SchemaVersion uint      `json:"schema_version"`
	Payload       string    `gorm:"type:text" json:"payload"`
	Actor         string    `json:"actor"`
	CreatedAt     time.Time `json:"created_at"`
}
type RoutingBinding struct {
	Scope        string    `gorm:"primaryKey;size:16" json:"scope"`
	TargetID     uint      `gorm:"primaryKey;autoIncrement:false" json:"target_id"`
	Mode         string    `json:"mode"`
	TemplateID   uint      `json:"template_id"`
	Revision     uint      `json:"revision"`
	Overrides    string    `gorm:"type:text" json:"overrides"`
	ConfigPath   string    `json:"config_path"`
	UDP443Policy string    `json:"udp443_policy"`
	Version      uint      `json:"version"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type ConfigDeployment struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	NodeID            uint      `gorm:"index" json:"node_id"`
	RequestKey        string    `gorm:"uniqueIndex" json:"request_key"`
	DesiredHash       string    `json:"desired_hash"`
	DesiredRevision   uint      `json:"desired_revision"`
	Status            string    `gorm:"index" json:"status"`
	Error             string    `json:"error"`
	Actor             string    `json:"actor"`
	Attempts          int       `json:"attempts"`
	ReconcileAttempts int       `json:"reconcile_attempts"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// RuntimeHash is an application receipt, not a hash read from the Xray API.
type NodeConfigState struct {
	NodeID                      uint       `gorm:"primaryKey;autoIncrement:false" json:"node_id"`
	ChangeRevision              uint       `json:"change_revision"`
	DesiredRevision             uint       `json:"desired_revision"`
	DesiredHash                 string     `json:"desired_hash"`
	DiskRevision                uint       `json:"disk_revision"`
	DiskHash                    string     `json:"disk_hash"`
	RuntimeRevision             uint       `json:"runtime_revision"`
	RuntimeHash                 string     `json:"runtime_hash"`
	LastHealthyRevision         uint       `json:"last_healthy_revision"`
	LastHealthyPolicy           string     `gorm:"type:text" json:"-"`
	LastHealthyTemplateID       uint       `json:"last_healthy_template_id"`
	LastHealthyTemplateRevision uint       `json:"last_healthy_template_revision"`
	VerifiedAt                  *time.Time `json:"verified_at"`
	ProcessID                   string     `json:"process_id"`
	Verification                string     `json:"verification"`
	RecoveryPending             bool       `gorm:"index" json:"recovery_pending"`
	RecoveryAttempts            int        `json:"recovery_attempts"`
	NextRetryAt                 *time.Time `json:"next_retry_at"`
	LeaseOwner                  string     `json:"-"`
	LeaseUntil                  *time.Time `json:"-"`
	LastError                   string     `json:"last_error"`
	UpdatedAt                   time.Time  `json:"updated_at"`
}
