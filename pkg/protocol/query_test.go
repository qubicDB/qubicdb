package protocol

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/core"
)

func TestFilterMatcherBasicEquality(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test content", 3)
	n.Energy = 0.8

	// Match by content
	if !fm.MatchNeuron(n, map[string]any{"content": "Test content"}) {
		t.Error("Should match exact content")
	}

	// No match
	if fm.MatchNeuron(n, map[string]any{"content": "Other"}) {
		t.Error("Should not match different content")
	}
}

func TestFilterMatcherComparisonOperators(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test", 3)
	n.Energy = 0.5

	// $gt
	if !fm.MatchNeuron(n, map[string]any{"energy": map[string]any{"$gt": 0.3}}) {
		t.Error("0.5 should be > 0.3")
	}
	if fm.MatchNeuron(n, map[string]any{"energy": map[string]any{"$gt": 0.7}}) {
		t.Error("0.5 should not be > 0.7")
	}

	// $gte
	if !fm.MatchNeuron(n, map[string]any{"energy": map[string]any{"$gte": 0.5}}) {
		t.Error("0.5 should be >= 0.5")
	}

	// $lt
	if !fm.MatchNeuron(n, map[string]any{"energy": map[string]any{"$lt": 0.7}}) {
		t.Error("0.5 should be < 0.7")
	}

	// $lte
	if !fm.MatchNeuron(n, map[string]any{"energy": map[string]any{"$lte": 0.5}}) {
		t.Error("0.5 should be <= 0.5")
	}

	// $ne
	if !fm.MatchNeuron(n, map[string]any{"energy": map[string]any{"$ne": 0.3}}) {
		t.Error("0.5 should be != 0.3")
	}
}

func TestFilterMatcherLogicalOperators(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test", 3)
	n.Energy = 0.5
	n.Depth = 1

	// $and
	filter := map[string]any{
		"$and": []any{
			map[string]any{"energy": map[string]any{"$gt": 0.3}},
			map[string]any{"depth": 1},
		},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("$and should match when both conditions true")
	}

	// $or
	filter = map[string]any{
		"$or": []any{
			map[string]any{"energy": map[string]any{"$gt": 0.9}},
			map[string]any{"depth": 1},
		},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("$or should match when one condition true")
	}

	// $not
	filter = map[string]any{
		"$not": map[string]any{"energy": map[string]any{"$gt": 0.9}},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("$not should match when inner condition false")
	}
}

func TestFilterMatcherInArray(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test", 3)
	n.Depth = 1

	// $in
	filter := map[string]any{
		"depth": map[string]any{"$in": []any{0, 1, 2}},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("1 should be in [0, 1, 2]")
	}

	// $nin
	filter = map[string]any{
		"depth": map[string]any{"$nin": []any{3, 4, 5}},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("1 should not be in [3, 4, 5]")
	}
}

func TestFilterMatcherRegex(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Hello World Test", 3)

	filter := map[string]any{
		"content": map[string]any{"$regex": "World"},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("Should match regex")
	}

	filter = map[string]any{
		"content": map[string]any{"$regex": "^Hello"},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("Should match start regex")
	}
}

func TestFilterMatcherContains(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("TypeScript Programming", 3)

	filter := map[string]any{
		"content": map[string]any{"$contains": "script"},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("Should match case-insensitive contains")
	}
}

func TestFilterMatcherExists(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test", 3)
	n.Metadata = map[string]any{"key1": "value1"}

	// Field exists
	filter := map[string]any{
		"key1": map[string]any{"$exists": true},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("key1 should exist in metadata")
	}

	// Field does not exist
	filter = map[string]any{
		"key2": map[string]any{"$exists": false},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("key2 should not exist")
	}
}

func TestParseCommand(t *testing.T) {
	json := `{"type": "find", "collection": "neurons", "filter": {"energy": {"$gt": 0.5}}}`

	cmd, err := ParseCommand([]byte(json))
	if err != nil {
		t.Fatalf("ParseCommand failed: %v", err)
	}

	if cmd.Type != CmdFind {
		t.Errorf("Expected type 'find', got '%s'", cmd.Type)
	}
	if cmd.Collection != "neurons" {
		t.Error("Collection mismatch")
	}
}

func TestParseCommandMissingType(t *testing.T) {
	json := `{"collection": "neurons"}`

	_, err := ParseCommand([]byte(json))
	if err == nil {
		t.Error("Should fail for missing type")
	}
}

func TestNeuronToDocument(t *testing.T) {
	n := core.NewNeuron("Test content", 3)
	n.Energy = 0.8
	n.Depth = 1

	// Full document (no projection)
	doc := NeuronToDocument(n, nil)
	if doc["content"] != "Test content" {
		t.Error("Content mismatch")
	}
	if doc["energy"] != 0.8 {
		t.Error("Energy mismatch")
	}

	// With projection (inclusion mode)
	doc = NeuronToDocument(n, map[string]int{"content": 1, "energy": 1})
	if doc["content"] == nil {
		t.Error("Content should be included in inclusion mode")
	}

	// Full document returns all fields
	doc = NeuronToDocument(n, nil)
	if doc["depth"] == nil {
		t.Error("Depth should be in full document")
	}
}

func TestDocumentToNeuron(t *testing.T) {
	doc := map[string]any{
		"content": "Test content",
		"tags":    []any{"tag1", "tag2"},
	}

	n, err := DocumentToNeuron(doc, 3)
	if err != nil {
		t.Fatalf("DocumentToNeuron failed: %v", err)
	}

	if n.Content != "Test content" {
		t.Error("Content mismatch")
	}
	if len(n.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(n.Tags))
	}
}

func TestDocumentToNeuronMissingContent(t *testing.T) {
	doc := map[string]any{
		"tags": []any{"tag1"},
	}

	_, err := DocumentToNeuron(doc, 3)
	if err == nil {
		t.Error("Should fail for missing content")
	}
}

func TestMarshalResult(t *testing.T) {
	result := &Result{
		Success:    true,
		InsertedID: "test-id",
		Count:      5,
	}

	data, err := MarshalResult(result)
	if err != nil {
		t.Fatalf("MarshalResult failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Result should not be empty")
	}
}

func TestFilterMatcherAccessCount(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test", 3)
	n.AccessCount = 10

	filter := map[string]any{
		"accessCount": map[string]any{"$gte": float64(5)},
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("accessCount 10 should be >= 5")
	}
}

func TestFilterMatcherCreatedAt(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test", 3)
	n.CreatedAt = time.Now()

	// Just verify field is accessible
	val := fm.getFieldValue(n, "createdAt")
	if val == nil {
		t.Error("createdAt should be accessible")
	}
}

func TestFilterMatcherAllFields(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test content", 3)
	n.Energy = 0.5
	n.Depth = 2
	n.AccessCount = 10
	n.Tags = []string{"tag1", "tag2"}

	// Test all field accessors
	fields := []string{"_id", "id", "content", "energy", "depth", "accessCount", "tags", "createdAt", "lastFiredAt"}
	for _, field := range fields {
		val := fm.getFieldValue(n, field)
		if val == nil && field != "metadata" {
			t.Errorf("Field %s should be accessible", field)
		}
	}
}

func TestResultMarshalUnmarshal(t *testing.T) {
	result := &Result{
		Success:    true,
		InsertedID: "test-id",
		Count:      5,
		Data:       map[string]any{"key": "value"},
	}

	data, err := MarshalResult(result)
	if err != nil {
		t.Fatalf("MarshalResult failed: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Success != result.Success {
		t.Error("Success mismatch")
	}
	if decoded.InsertedID != result.InsertedID {
		t.Error("InsertedID mismatch")
	}
}

func TestParseCommandAllTypes(t *testing.T) {
	testCases := []struct {
		json     string
		cmdType  CommandType
	}{
		{`{"type": "insert", "collection": "neurons"}`, CmdInsert},
		{`{"type": "find", "collection": "neurons"}`, CmdFind},
		{`{"type": "findOne", "collection": "neurons"}`, CmdFindOne},
		{`{"type": "update", "collection": "neurons"}`, CmdUpdate},
		{`{"type": "delete", "collection": "neurons"}`, CmdDelete},
		{`{"type": "count", "collection": "neurons"}`, CmdCount},
		{`{"type": "search", "collection": "neurons"}`, CmdSearch},
		{`{"type": "stats", "collection": "neurons"}`, CmdStats},
	}

	for _, tc := range testCases {
		cmd, err := ParseCommand([]byte(tc.json))
		if err != nil {
			t.Errorf("ParseCommand failed for %s: %v", tc.cmdType, err)
			continue
		}
		if cmd.Type != tc.cmdType {
			t.Errorf("Expected type %s, got %s", tc.cmdType, cmd.Type)
		}
	}
}

func TestParseCommandInvalidJSON(t *testing.T) {
	_, err := ParseCommand([]byte("invalid json"))
	if err == nil {
		t.Error("Should fail for invalid JSON")
	}
}

func TestDocumentToNeuronWithTags(t *testing.T) {
	doc := map[string]any{
		"content": "Test content",
		"tags":    []any{"tag1", "tag2", "tag3"},
	}

	n, err := DocumentToNeuron(doc, 3)
	if err != nil {
		t.Fatalf("DocumentToNeuron failed: %v", err)
	}

	if len(n.Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(n.Tags))
	}
}

func TestDocumentToNeuronWithMetadata(t *testing.T) {
	doc := map[string]any{
		"content": "Test content",
		"metadata": map[string]any{
			"key1": "value1",
			"key2": 123,
		},
	}

	n, err := DocumentToNeuron(doc, 3)
	if err != nil {
		t.Fatalf("DocumentToNeuron failed: %v", err)
	}

	if n.Metadata["key1"] != "value1" {
		t.Error("Metadata key1 mismatch")
	}
}

func TestFilterMatcherEmptyFilter(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test", 3)

	// Empty filter should match everything
	if !fm.MatchNeuron(n, map[string]any{}) {
		t.Error("Empty filter should match")
	}
}

func TestFilterMatcherNilFilter(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test", 3)

	// Nil filter should match everything
	if !fm.MatchNeuron(n, nil) {
		t.Error("Nil filter should match")
	}
}

func TestFilterMatcherMultipleConditions(t *testing.T) {
	fm := NewFilterMatcher()
	n := core.NewNeuron("Test content", 3)
	n.Energy = 0.5
	n.Depth = 1

	// All conditions must match
	filter := map[string]any{
		"content": "Test content",
		"depth":   1,
	}
	if !fm.MatchNeuron(n, filter) {
		t.Error("Should match when all conditions are true")
	}

	// One condition fails
	filter = map[string]any{
		"content": "Test content",
		"depth":   2,
	}
	if fm.MatchNeuron(n, filter) {
		t.Error("Should not match when one condition fails")
	}
}

func TestCommandOptions(t *testing.T) {
	jsonStr := `{
		"type": "find",
		"collection": "neurons",
		"options": {
			"limit": 10,
			"skip": 5,
			"depth": 2
		}
	}`

	cmd, err := ParseCommand([]byte(jsonStr))
	if err != nil {
		t.Fatalf("ParseCommand failed: %v", err)
	}

	if cmd.Options.Limit != 10 {
		t.Errorf("Expected limit 10, got %d", cmd.Options.Limit)
	}
	if cmd.Options.Skip != 5 {
		t.Errorf("Expected skip 5, got %d", cmd.Options.Skip)
	}
	if cmd.Options.Depth != 2 {
		t.Errorf("Expected depth 2, got %d", cmd.Options.Depth)
	}
}
