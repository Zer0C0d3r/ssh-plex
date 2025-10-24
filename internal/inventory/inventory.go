// Package inventory provides integration with external inventory systems for ssh-plex.
package inventory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ssh-plex/internal/target"

	"gopkg.in/yaml.v3"
)

// InventoryProvider defines the interface for inventory providers
type InventoryProvider interface {
	// LoadTargets loads targets from the inventory source
	LoadTargets() ([]target.Target, error)
	// GetGroups returns available groups in the inventory
	GetGroups() ([]string, error)
	// GetTargetsByGroup returns targets filtered by group
	GetTargetsByGroup(group string) ([]target.Target, error)
}

// AnsibleInventory represents an Ansible inventory
type AnsibleInventory struct {
	path string
}

// NewAnsibleInventory creates a new Ansible inventory provider
func NewAnsibleInventory(path string) *AnsibleInventory {
	return &AnsibleInventory{path: path}
}

// AnsibleInventoryData represents the structure of an Ansible inventory
type AnsibleInventoryData struct {
	All struct {
		Children map[string]*AnsibleGroup `yaml:"children" json:"children"`
		Hosts    map[string]*AnsibleHost  `yaml:"hosts" json:"hosts"`
		Vars     map[string]interface{}   `yaml:"vars" json:"vars"`
	} `yaml:"all" json:"all"`
	Groups map[string]*AnsibleGroup `yaml:",inline" json:",inline"`
}

// AnsibleGroup represents an Ansible inventory group
type AnsibleGroup struct {
	Hosts    map[string]*AnsibleHost  `yaml:"hosts" json:"hosts"`
	Children map[string]*AnsibleGroup `yaml:"children" json:"children"`
	Vars     map[string]interface{}   `yaml:"vars" json:"vars"`
}

// AnsibleHost represents an Ansible inventory host
type AnsibleHost struct {
	AnsibleHost     string                 `yaml:"ansible_host" json:"ansible_host"`
	AnsiblePort     int                    `yaml:"ansible_port" json:"ansible_port"`
	AnsibleUser     string                 `yaml:"ansible_user" json:"ansible_user"`
	AnsibleSSHKey   string                 `yaml:"ansible_ssh_private_key_file" json:"ansible_ssh_private_key_file"`
	AnsiblePassword string                 `yaml:"ansible_password" json:"ansible_password"`
	Vars            map[string]interface{} `yaml:",inline" json:",inline"`
}

// LoadTargets loads targets from the Ansible inventory
func (ai *AnsibleInventory) LoadTargets() ([]target.Target, error) {
	data, err := ai.loadInventoryData()
	if err != nil {
		return nil, err
	}

	var targets []target.Target

	// Process hosts from all groups
	processed := make(map[string]bool)

	// Process top-level hosts
	for hostname, host := range data.All.Hosts {
		if !processed[hostname] {
			target := ai.convertAnsibleHost(hostname, host, []string{})
			targets = append(targets, target)
			processed[hostname] = true
		}
	}

	// Process groups
	for groupName, group := range data.Groups {
		groupTargets := ai.processGroup(groupName, group, []string{groupName}, processed)
		targets = append(targets, groupTargets...)
	}

	return targets, nil
}

// GetGroups returns available groups in the inventory
func (ai *AnsibleInventory) GetGroups() ([]string, error) {
	data, err := ai.loadInventoryData()
	if err != nil {
		return nil, err
	}

	var groups []string
	for groupName := range data.Groups {
		groups = append(groups, groupName)
	}

	return groups, nil
}

// GetTargetsByGroup returns targets filtered by group
func (ai *AnsibleInventory) GetTargetsByGroup(group string) ([]target.Target, error) {
	data, err := ai.loadInventoryData()
	if err != nil {
		return nil, err
	}

	groupData, exists := data.Groups[group]
	if !exists {
		return nil, fmt.Errorf("group '%s' not found in inventory", group)
	}

	processed := make(map[string]bool)
	return ai.processGroup(group, groupData, []string{group}, processed), nil
}

// loadInventoryData loads and parses the inventory file
func (ai *AnsibleInventory) loadInventoryData() (*AnsibleInventoryData, error) {
	file, err := os.Open(ai.path)
	if err != nil {
		return nil, fmt.Errorf("failed to open inventory file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read inventory file: %w", err)
	}

	var data AnsibleInventoryData

	// Try to parse as YAML first, then JSON
	ext := strings.ToLower(filepath.Ext(ai.path))
	if ext == ".json" {
		err = json.Unmarshal(content, &data)
	} else {
		err = yaml.Unmarshal(content, &data)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse inventory file: %w", err)
	}

	return &data, nil
}

// processGroup recursively processes an Ansible group
func (ai *AnsibleInventory) processGroup(groupName string, group *AnsibleGroup, tags []string, processed map[string]bool) []target.Target {
	var targets []target.Target

	// Process hosts in this group
	for hostname, host := range group.Hosts {
		if !processed[hostname] {
			target := ai.convertAnsibleHost(hostname, host, tags)
			targets = append(targets, target)
			processed[hostname] = true
		}
	}

	// Process child groups
	for childName, childGroup := range group.Children {
		childTags := append(tags, childName)
		childTargets := ai.processGroup(childName, childGroup, childTags, processed)
		targets = append(targets, childTargets...)
	}

	return targets
}

// convertAnsibleHost converts an Ansible host to a ssh-plex target
func (ai *AnsibleInventory) convertAnsibleHost(hostname string, host *AnsibleHost, groups []string) target.Target {
	target := target.Target{
		Host:       hostname,
		Port:       22,
		Properties: make(map[string]string),
		Tags:       groups,
		Original:   hostname,
	}

	// Use ansible_host if specified
	if host.AnsibleHost != "" {
		target.Host = host.AnsibleHost
	}

	// Set port
	if host.AnsiblePort > 0 {
		target.Port = host.AnsiblePort
	}

	// Set user
	if host.AnsibleUser != "" {
		target.User = host.AnsibleUser
	}

	// Set SSH key
	if host.AnsibleSSHKey != "" {
		target.IdentityFile = host.AnsibleSSHKey
	}

	// Set password
	if host.AnsiblePassword != "" {
		target.Password = host.AnsiblePassword
	}

	// Convert other variables to properties
	for key, value := range host.Vars {
		if !isAnsibleBuiltin(key) {
			target.Properties[key] = fmt.Sprintf("%v", value)
		}
	}

	return target
}

// isAnsibleBuiltin checks if a variable is an Ansible builtin
func isAnsibleBuiltin(key string) bool {
	builtins := []string{
		"ansible_host", "ansible_port", "ansible_user",
		"ansible_ssh_private_key_file", "ansible_password",
		"ansible_connection", "ansible_ssh_host", "ansible_ssh_port",
		"ansible_ssh_user", "ansible_ssh_pass", "ansible_sudo_pass",
		"ansible_become", "ansible_become_method", "ansible_become_user",
		"ansible_become_pass", "ansible_python_interpreter",
	}

	for _, builtin := range builtins {
		if key == builtin {
			return true
		}
	}

	return false
}

// KubernetesInventory provides Kubernetes node inventory
type KubernetesInventory struct {
	kubeconfig string
	namespace  string
}

// NewKubernetesInventory creates a new Kubernetes inventory provider
func NewKubernetesInventory(kubeconfig, namespace string) *KubernetesInventory {
	return &KubernetesInventory{
		kubeconfig: kubeconfig,
		namespace:  namespace,
	}
}

// LoadTargets loads Kubernetes nodes as targets
func (ki *KubernetesInventory) LoadTargets() ([]target.Target, error) {
	// This would integrate with Kubernetes API
	// For now, return a placeholder implementation
	return nil, fmt.Errorf("Kubernetes inventory not yet implemented")
}

// GetGroups returns Kubernetes node groups
func (ki *KubernetesInventory) GetGroups() ([]string, error) {
	return []string{"master", "worker"}, nil
}

// GetTargetsByGroup returns Kubernetes nodes by group
func (ki *KubernetesInventory) GetTargetsByGroup(group string) ([]target.Target, error) {
	return nil, fmt.Errorf("Kubernetes inventory not yet implemented")
}

// StaticInventory provides a simple static inventory
type StaticInventory struct {
	targets []target.Target
	groups  map[string][]target.Target
}

// NewStaticInventory creates a new static inventory
func NewStaticInventory() *StaticInventory {
	return &StaticInventory{
		targets: make([]target.Target, 0),
		groups:  make(map[string][]target.Target),
	}
}

// AddTarget adds a target to the static inventory
func (si *StaticInventory) AddTarget(target target.Target) {
	si.targets = append(si.targets, target)

	// Add to groups based on tags
	for _, tag := range target.Tags {
		si.groups[tag] = append(si.groups[tag], target)
	}
}

// LoadTargets returns all targets in the static inventory
func (si *StaticInventory) LoadTargets() ([]target.Target, error) {
	return si.targets, nil
}

// GetGroups returns available groups
func (si *StaticInventory) GetGroups() ([]string, error) {
	var groups []string
	for group := range si.groups {
		groups = append(groups, group)
	}
	return groups, nil
}

// GetTargetsByGroup returns targets by group
func (si *StaticInventory) GetTargetsByGroup(group string) ([]target.Target, error) {
	targets, exists := si.groups[group]
	if !exists {
		return nil, fmt.Errorf("group '%s' not found", group)
	}
	return targets, nil
}

// LoadInventoryFromFile loads inventory from a file based on its extension
func LoadInventoryFromFile(path string) (InventoryProvider, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yml", ".yaml", ".json":
		return NewAnsibleInventory(path), nil
	default:
		return nil, fmt.Errorf("unsupported inventory file format: %s", ext)
	}
}
