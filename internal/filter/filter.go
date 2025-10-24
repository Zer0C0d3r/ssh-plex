// Package filter provides host filtering and grouping capabilities for ssh-plex.
package filter

import (
	"fmt"
	"regexp"
	"strings"

	"ssh-plex/internal/target"
)

// Filter represents a host filter condition
type Filter interface {
	// Match returns true if the target matches the filter condition
	Match(target target.Target) bool
	// String returns a human-readable description of the filter
	String() string
}

// TagFilter filters hosts by tags
type TagFilter struct {
	RequiredTags []string
	ExcludeTags  []string
}

// NewTagFilter creates a new tag-based filter
func NewTagFilter(required, excluded []string) *TagFilter {
	return &TagFilter{
		RequiredTags: required,
		ExcludeTags:  excluded,
	}
}

// Match checks if target has required tags and doesn't have excluded tags
func (f *TagFilter) Match(target target.Target) bool {
	targetTags := make(map[string]bool)
	for _, tag := range target.Tags {
		targetTags[strings.ToLower(tag)] = true
	}

	// Check required tags
	for _, required := range f.RequiredTags {
		if !targetTags[strings.ToLower(required)] {
			return false
		}
	}

	// Check excluded tags
	for _, excluded := range f.ExcludeTags {
		if targetTags[strings.ToLower(excluded)] {
			return false
		}
	}

	return true
}

// String returns a description of the tag filter
func (f *TagFilter) String() string {
	var parts []string
	if len(f.RequiredTags) > 0 {
		parts = append(parts, fmt.Sprintf("tags: %s", strings.Join(f.RequiredTags, ",")))
	}
	if len(f.ExcludeTags) > 0 {
		parts = append(parts, fmt.Sprintf("!tags: %s", strings.Join(f.ExcludeTags, ",")))
	}
	return strings.Join(parts, " AND ")
}

// PropertyFilter filters hosts by properties
type PropertyFilter struct {
	Property string
	Value    string
	Operator string // "equals", "contains", "regex"
}

// NewPropertyFilter creates a new property-based filter
func NewPropertyFilter(property, operator, value string) *PropertyFilter {
	return &PropertyFilter{
		Property: property,
		Value:    value,
		Operator: operator,
	}
}

// Match checks if target property matches the filter condition
func (f *PropertyFilter) Match(target target.Target) bool {
	propValue, exists := target.Properties[f.Property]
	if !exists {
		return false
	}

	switch f.Operator {
	case "equals":
		return strings.EqualFold(propValue, f.Value)
	case "contains":
		return strings.Contains(strings.ToLower(propValue), strings.ToLower(f.Value))
	case "regex":
		matched, err := regexp.MatchString(f.Value, propValue)
		return err == nil && matched
	default:
		return false
	}
}

// String returns a description of the property filter
func (f *PropertyFilter) String() string {
	return fmt.Sprintf("%s %s %s", f.Property, f.Operator, f.Value)
}

// HostFilter filters hosts by hostname patterns
type HostFilter struct {
	Pattern string
	IsRegex bool
}

// NewHostFilter creates a new hostname-based filter
func NewHostFilter(pattern string, isRegex bool) *HostFilter {
	return &HostFilter{
		Pattern: pattern,
		IsRegex: isRegex,
	}
}

// Match checks if target hostname matches the pattern
func (f *HostFilter) Match(target target.Target) bool {
	if f.IsRegex {
		matched, err := regexp.MatchString(f.Pattern, target.Host)
		return err == nil && matched
	}

	// Simple wildcard matching
	pattern := strings.ReplaceAll(f.Pattern, "*", ".*")
	matched, err := regexp.MatchString("^"+pattern+"$", target.Host)
	return err == nil && matched
}

// String returns a description of the host filter
func (f *HostFilter) String() string {
	if f.IsRegex {
		return fmt.Sprintf("host regex: %s", f.Pattern)
	}
	return fmt.Sprintf("host pattern: %s", f.Pattern)
}

// CompositeFilter combines multiple filters with AND/OR logic
type CompositeFilter struct {
	Filters []Filter
	Logic   string // "AND" or "OR"
}

// NewCompositeFilter creates a new composite filter
func NewCompositeFilter(logic string, filters ...Filter) *CompositeFilter {
	return &CompositeFilter{
		Filters: filters,
		Logic:   strings.ToUpper(logic),
	}
}

// Match evaluates all filters with the specified logic
func (f *CompositeFilter) Match(target target.Target) bool {
	if len(f.Filters) == 0 {
		return true
	}

	switch f.Logic {
	case "AND":
		for _, filter := range f.Filters {
			if !filter.Match(target) {
				return false
			}
		}
		return true
	case "OR":
		for _, filter := range f.Filters {
			if filter.Match(target) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// String returns a description of the composite filter
func (f *CompositeFilter) String() string {
	if len(f.Filters) == 0 {
		return "no filters"
	}

	var descriptions []string
	for _, filter := range f.Filters {
		descriptions = append(descriptions, filter.String())
	}

	return fmt.Sprintf("(%s)", strings.Join(descriptions, " "+f.Logic+" "))
}

// FilterTargets applies filters to a list of targets and returns matching ones
func FilterTargets(targets []target.Target, filters ...Filter) []target.Target {
	if len(filters) == 0 {
		return targets
	}

	var filtered []target.Target
	for _, target := range targets {
		match := true
		for _, filter := range filters {
			if !filter.Match(target) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, target)
		}
	}

	return filtered
}

// GroupTargets groups targets by a specified property or tag
func GroupTargets(targets []target.Target, groupBy string) map[string][]target.Target {
	groups := make(map[string][]target.Target)

	for _, target := range targets {
		var groupKey string

		// Check if grouping by a property
		if propValue, exists := target.Properties[groupBy]; exists {
			groupKey = propValue
		} else {
			// Check if grouping by a tag (if target has the tag)
			hasTag := false
			for _, tag := range target.Tags {
				if strings.EqualFold(tag, groupBy) {
					groupKey = tag
					hasTag = true
					break
				}
			}
			if !hasTag {
				groupKey = "untagged"
			}
		}

		groups[groupKey] = append(groups[groupKey], target)
	}

	return groups
}

// ParseFilterExpression parses a filter expression string
// Format: "tag:web,prod property:env=production host:*.example.com"
func ParseFilterExpression(expression string) ([]Filter, error) {
	if strings.TrimSpace(expression) == "" {
		return nil, nil
	}

	var filters []Filter
	parts := strings.Fields(expression)

	for _, part := range parts {
		if strings.HasPrefix(part, "tag:") {
			tagSpec := strings.TrimPrefix(part, "tag:")
			tags := strings.Split(tagSpec, ",")
			filter := NewTagFilter(tags, nil)
			filters = append(filters, filter)
		} else if strings.HasPrefix(part, "!tag:") {
			tagSpec := strings.TrimPrefix(part, "!tag:")
			tags := strings.Split(tagSpec, ",")
			filter := NewTagFilter(nil, tags)
			filters = append(filters, filter)
		} else if strings.HasPrefix(part, "property:") {
			propSpec := strings.TrimPrefix(part, "property:")
			if strings.Contains(propSpec, "=") {
				propParts := strings.SplitN(propSpec, "=", 2)
				filter := NewPropertyFilter(propParts[0], "equals", propParts[1])
				filters = append(filters, filter)
			}
		} else if strings.HasPrefix(part, "host:") {
			hostPattern := strings.TrimPrefix(part, "host:")
			isRegex := strings.HasPrefix(hostPattern, "regex:")
			if isRegex {
				hostPattern = strings.TrimPrefix(hostPattern, "regex:")
			}
			filter := NewHostFilter(hostPattern, isRegex)
			filters = append(filters, filter)
		}
	}

	return filters, nil
}
