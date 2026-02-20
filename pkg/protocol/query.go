package protocol

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

// Query represents a MongoDB-like query
type Query struct {
	Collection string                 `json:"collection"` // "neurons" or "synapses"
	Filter     map[string]any         `json:"filter"`
	Projection map[string]int         `json:"projection,omitempty"`
	Sort       map[string]int         `json:"sort,omitempty"`
	Limit      int                    `json:"limit,omitempty"`
	Skip       int                    `json:"skip,omitempty"`
}

// Command types
type CommandType string

const (
	CmdInsert     CommandType = "insert"
	CmdFind       CommandType = "find"
	CmdFindOne    CommandType = "findOne"
	CmdUpdate     CommandType = "update"
	CmdUpdateOne  CommandType = "updateOne"
	CmdDelete     CommandType = "delete"
	CmdDeleteOne  CommandType = "deleteOne"
	CmdAggregate  CommandType = "aggregate"
	CmdCount      CommandType = "count"
	CmdActivate   CommandType = "activate"   // Fire a neuron
	CmdSearch     CommandType = "search"     // Semantic-like search with spread
	CmdStats      CommandType = "stats"
)

// Command represents a database command
type Command struct {
	Type       CommandType    `json:"type"`
	Collection string         `json:"collection"`
	Document   map[string]any `json:"document,omitempty"`
	Filter     map[string]any `json:"filter,omitempty"`
	Update     map[string]any `json:"update,omitempty"`
	Pipeline   []any          `json:"pipeline,omitempty"`
	Options    CommandOptions `json:"options,omitempty"`
}

// CommandOptions for queries
type CommandOptions struct {
	Limit      int            `json:"limit,omitempty"`
	Skip       int            `json:"skip,omitempty"`
	Sort       map[string]int `json:"sort,omitempty"`
	Projection map[string]int `json:"projection,omitempty"`
	Depth      int            `json:"depth,omitempty"`  // For search spread
	Upsert     bool           `json:"upsert,omitempty"`
}

// Result represents a command result
type Result struct {
	Success     bool           `json:"success"`
	Data        any            `json:"data,omitempty"`
	Count       int            `json:"count,omitempty"`
	InsertedID  string         `json:"insertedId,omitempty"`
	ModifiedCnt int            `json:"modifiedCount,omitempty"`
	DeletedCnt  int            `json:"deletedCount,omitempty"`
	Error       string         `json:"error,omitempty"`
}

// FilterMatcher evaluates filters against neurons
type FilterMatcher struct{}

// NewFilterMatcher creates a new filter matcher
func NewFilterMatcher() *FilterMatcher {
	return &FilterMatcher{}
}

// MatchNeuron checks if a neuron matches the filter
func (fm *FilterMatcher) MatchNeuron(n *core.Neuron, filter map[string]any) bool {
	for key, value := range filter {
		if !fm.matchField(n, key, value) {
			return false
		}
	}
	return true
}

// matchField checks a single field condition
func (fm *FilterMatcher) matchField(n *core.Neuron, key string, condition any) bool {
	// Handle operators
	if strings.HasPrefix(key, "$") {
		return fm.matchOperator(n, key, condition)
	}

	// Get field value
	fieldValue := fm.getFieldValue(n, key)

	// Handle condition types
	switch cond := condition.(type) {
	case map[string]any:
		// Nested operators like {$gt: 5}
		return fm.matchNestedCondition(fieldValue, cond)
	default:
		// Direct equality
		return fm.equals(fieldValue, cond)
	}
}

// matchOperator handles top-level operators
func (fm *FilterMatcher) matchOperator(n *core.Neuron, op string, condition any) bool {
	switch op {
	case "$and":
		if conditions, ok := condition.([]any); ok {
			for _, c := range conditions {
				if cMap, ok := c.(map[string]any); ok {
					if !fm.MatchNeuron(n, cMap) {
						return false
					}
				}
			}
			return true
		}
	case "$or":
		if conditions, ok := condition.([]any); ok {
			for _, c := range conditions {
				if cMap, ok := c.(map[string]any); ok {
					if fm.MatchNeuron(n, cMap) {
						return true
					}
				}
			}
			return false
		}
	case "$not":
		if cMap, ok := condition.(map[string]any); ok {
			return !fm.MatchNeuron(n, cMap)
		}
	}
	return false
}

// matchNestedCondition handles nested operators
func (fm *FilterMatcher) matchNestedCondition(fieldValue any, condition map[string]any) bool {
	for op, val := range condition {
		switch op {
		case "$eq":
			if !fm.equals(fieldValue, val) {
				return false
			}
		case "$ne":
			if fm.equals(fieldValue, val) {
				return false
			}
		case "$gt":
			if !fm.compare(fieldValue, val, ">") {
				return false
			}
		case "$gte":
			if !fm.compare(fieldValue, val, ">=") {
				return false
			}
		case "$lt":
			if !fm.compare(fieldValue, val, "<") {
				return false
			}
		case "$lte":
			if !fm.compare(fieldValue, val, "<=") {
				return false
			}
		case "$in":
			if !fm.inArray(fieldValue, val) {
				return false
			}
		case "$nin":
			if fm.inArray(fieldValue, val) {
				return false
			}
		case "$regex":
			if !fm.matchRegex(fieldValue, val) {
				return false
			}
		case "$exists":
			exists := fieldValue != nil
			if val.(bool) != exists {
				return false
			}
		case "$contains":
			if str, ok := fieldValue.(string); ok {
				if !strings.Contains(strings.ToLower(str), strings.ToLower(val.(string))) {
					return false
				}
			}
		}
	}
	return true
}

// getFieldValue extracts field value from neuron
func (fm *FilterMatcher) getFieldValue(n *core.Neuron, field string) any {
	switch field {
	case "id", "_id":
		return string(n.ID)
	case "content":
		return n.Content
	case "energy":
		return n.Energy
	case "depth":
		return n.Depth
	case "tags":
		return n.Tags
	case "accessCount":
		return n.AccessCount
	case "createdAt":
		return n.CreatedAt
	case "lastFiredAt":
		return n.LastFiredAt
	default:
		// Check metadata
		if n.Metadata != nil {
			if val, ok := n.Metadata[field]; ok {
				return val
			}
		}
		return nil
	}
}

// equals compares two values
func (fm *FilterMatcher) equals(a, b any) bool {
	return a == b
}

// compare performs numeric comparison
func (fm *FilterMatcher) compare(a, b any, op string) bool {
	aFloat, aOk := toFloat(a)
	bFloat, bOk := toFloat(b)
	
	if !aOk || !bOk {
		return false
	}

	switch op {
	case ">":
		return aFloat > bFloat
	case ">=":
		return aFloat >= bFloat
	case "<":
		return aFloat < bFloat
	case "<=":
		return aFloat <= bFloat
	}
	return false
}

// inArray checks if value is in array
func (fm *FilterMatcher) inArray(value any, arr any) bool {
	if arrSlice, ok := arr.([]any); ok {
		for _, item := range arrSlice {
			if fm.equals(value, item) {
				return true
			}
		}
	}
	return false
}

// matchRegex tests regex match
func (fm *FilterMatcher) matchRegex(value any, pattern any) bool {
	strVal, ok1 := value.(string)
	strPat, ok2 := pattern.(string)
	
	if !ok1 || !ok2 {
		return false
	}

	re, err := regexp.Compile(strPat)
	if err != nil {
		return false
	}
	return re.MatchString(strVal)
}

// toFloat converts value to float64
func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	}
	return 0, false
}

// ParseCommand parses a JSON command string
func ParseCommand(data []byte) (*Command, error) {
	var cmd Command
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil, err
	}
	
	if cmd.Type == "" {
		return nil, errors.New("command type required")
	}
	
	return &cmd, nil
}

// MarshalResult serializes a result
func MarshalResult(r *Result) ([]byte, error) {
	return json.Marshal(r)
}

// NeuronToDocument converts a neuron to a document map
func NeuronToDocument(n *core.Neuron, projection map[string]int) map[string]any {
	doc := make(map[string]any)
	
	// If no projection, return all fields
	includeAll := len(projection) == 0
	
	// Check for exclusion mode (any value is 0)
	exclusionMode := false
	for _, v := range projection {
		if v == 0 {
			exclusionMode = true
			break
		}
	}

	addField := func(name string, value any) {
		if includeAll {
			doc[name] = value
			return
		}
		if exclusionMode {
			if projection[name] != 0 {
				doc[name] = value
			}
		} else {
			if projection[name] == 1 {
				doc[name] = value
			}
		}
	}

	addField("_id", string(n.ID))
	addField("content", n.Content)
	addField("energy", n.Energy)
	addField("depth", n.Depth)
	addField("position", n.Position)
	addField("tags", n.Tags)
	addField("accessCount", n.AccessCount)
	addField("createdAt", n.CreatedAt)
	addField("lastFiredAt", n.LastFiredAt)
	addField("metadata", n.Metadata)

	return doc
}

// DocumentToNeuron creates a neuron from a document (for inserts)
func DocumentToNeuron(doc map[string]any, dim int) (*core.Neuron, error) {
	content, ok := doc["content"].(string)
	if !ok || content == "" {
		return nil, errors.New("content is required")
	}

	n := core.NewNeuron(content, dim)

	// Apply optional fields
	if tags, ok := doc["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				n.Tags = append(n.Tags, s)
			}
		}
	}

	if meta, ok := doc["metadata"].(map[string]any); ok {
		n.Metadata = meta
	}

	return n, nil
}
