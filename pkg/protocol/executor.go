package protocol

import (
	"sort"

	"github.com/denizumutdereli/qubicdb/pkg/concurrency"
	"github.com/denizumutdereli/qubicdb/pkg/core"
)

// CommandHandler is a function that handles a specific command type.
// Implementations receive the worker and parsed command, returning a result.
type CommandHandler func(worker *concurrency.BrainWorker, cmd *Command) *Result

// Executor executes commands against a brain worker using a pluggable
// command registry. Handlers are looked up by CommandType at dispatch time,
// so new command types can be registered without modifying Execute.
type Executor struct {
	matcher  *FilterMatcher
	handlers map[CommandType]CommandHandler
}

// NewExecutor creates a new command executor with the built-in command set.
func NewExecutor() *Executor {
	e := &Executor{
		matcher:  NewFilterMatcher(),
		handlers: make(map[CommandType]CommandHandler),
	}
	e.registerBuiltins()
	return e
}

// registerBuiltins wires up the default (built-in) command handlers.
func (e *Executor) registerBuiltins() {
	e.Register(CmdInsert, e.executeInsert)
	e.Register(CmdFind, e.executeFind)
	e.Register(CmdFindOne, e.executeFindOne)
	e.Register(CmdUpdate, e.executeUpdate)
	e.Register(CmdUpdateOne, e.executeUpdate)
	e.Register(CmdDelete, e.executeDelete)
	e.Register(CmdDeleteOne, e.executeDelete)
	e.Register(CmdCount, e.executeCount)
	e.Register(CmdActivate, e.executeActivate)
	e.Register(CmdSearch, e.executeSearch)
	e.Register(CmdStats, func(w *concurrency.BrainWorker, _ *Command) *Result {
		return e.executeStats(w)
	})
}

// Register adds or replaces a handler for the given command type.
func (e *Executor) Register(ct CommandType, h CommandHandler) {
	e.handlers[ct] = h
}

// Unregister removes a handler for the given command type.
func (e *Executor) Unregister(ct CommandType) {
	delete(e.handlers, ct)
}

// ListCommands returns all registered command types.
func (e *Executor) ListCommands() []CommandType {
	cmds := make([]CommandType, 0, len(e.handlers))
	for ct := range e.handlers {
		cmds = append(cmds, ct)
	}
	return cmds
}

// Execute dispatches a command to the appropriate registered handler.
func (e *Executor) Execute(worker *concurrency.BrainWorker, cmd *Command) *Result {
	if cmd.Type == CmdUpdate || cmd.Type == CmdUpdateOne || cmd.Type == CmdDelete || cmd.Type == CmdDeleteOne || cmd.Type == CmdActivate {
		return &Result{Success: false, Error: "direct neuron mutation is disabled; use high-level index operations"}
	}
	h, ok := e.handlers[cmd.Type]
	if !ok {
		return &Result{Success: false, Error: "unknown command type: " + string(cmd.Type)}
	}
	return h(worker, cmd)
}

// executeInsert handles insert command
func (e *Executor) executeInsert(worker *concurrency.BrainWorker, cmd *Command) *Result {
	if cmd.Collection != "neurons" {
		return &Result{Success: false, Error: "can only insert into neurons collection"}
	}

	neuron, err := DocumentToNeuron(cmd.Document, worker.Matrix().CurrentDim)
	if err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	// Use worker to add neuron
	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{
			Content: neuron.Content,
		},
	})

	if err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	n := result.(*core.Neuron)
	return &Result{
		Success:    true,
		InsertedID: string(n.ID),
		Data:       NeuronToDocument(n, nil),
	}
}

// executeFind handles find command
func (e *Executor) executeFind(worker *concurrency.BrainWorker, cmd *Command) *Result {
	matrix := worker.Matrix()

	var results []map[string]any

	if cmd.Collection == "neurons" {
		// Filter neurons
		neurons := make([]*core.Neuron, 0)
		for _, n := range matrix.Neurons {
			if len(cmd.Filter) == 0 || e.matcher.MatchNeuron(n, cmd.Filter) {
				neurons = append(neurons, n)
			}
		}

		// Sort
		e.sortNeurons(neurons, cmd.Options.Sort)

		// Skip
		if cmd.Options.Skip > 0 && cmd.Options.Skip < len(neurons) {
			neurons = neurons[cmd.Options.Skip:]
		}

		// Limit
		if cmd.Options.Limit > 0 && cmd.Options.Limit < len(neurons) {
			neurons = neurons[:cmd.Options.Limit]
		}

		// Project
		for _, n := range neurons {
			results = append(results, NeuronToDocument(n, cmd.Options.Projection))
		}
	} else if cmd.Collection == "synapses" {
		// Return synapse data
		for _, s := range matrix.Synapses {
			doc := map[string]any{
				"_id":         string(s.ID),
				"fromId":      string(s.FromID),
				"toId":        string(s.ToID),
				"weight":      s.Weight,
				"coFireCount": s.CoFireCount,
				"lastCoFire":  s.LastCoFire,
				"createdAt":   s.CreatedAt,
			}
			results = append(results, doc)
		}
	}

	return &Result{
		Success: true,
		Data:    results,
		Count:   len(results),
	}
}

// executeFindOne handles findOne command
func (e *Executor) executeFindOne(worker *concurrency.BrainWorker, cmd *Command) *Result {
	cmd.Options.Limit = 1
	result := e.executeFind(worker, cmd)

	if result.Success && result.Count > 0 {
		if data, ok := result.Data.([]map[string]any); ok && len(data) > 0 {
			result.Data = data[0]
		}
	}

	return result
}

// executeUpdate handles update command
func (e *Executor) executeUpdate(worker *concurrency.BrainWorker, cmd *Command) *Result {
	matrix := worker.Matrix()
	modified := 0
	onlyOne := cmd.Type == CmdUpdateOne

	for _, n := range matrix.Neurons {
		if len(cmd.Filter) > 0 && !e.matcher.MatchNeuron(n, cmd.Filter) {
			continue
		}

		// Apply updates
		if err := e.applyUpdate(worker, n, cmd.Update); err != nil {
			return &Result{Success: false, Error: err.Error()}
		}
		modified++

		if onlyOne {
			break
		}
	}

	return &Result{
		Success:     true,
		ModifiedCnt: modified,
	}
}

// applyUpdate applies update operators to a neuron
func (e *Executor) applyUpdate(worker *concurrency.BrainWorker, n *core.Neuron, update map[string]any) error {
	for op, fields := range update {
		switch op {
		case "$set":
			if fieldMap, ok := fields.(map[string]any); ok {
				if content, ok := fieldMap["content"].(string); ok {
					_, err := worker.Submit(&concurrency.Operation{
						Type: concurrency.OpTouch,
						Payload: concurrency.UpdateNeuronRequest{
							ID:      n.ID,
							Content: content,
						},
					})
					if err != nil {
						return err
					}
				}
				if tags, ok := fieldMap["tags"].([]any); ok {
					n.Tags = make([]string, 0)
					for _, t := range tags {
						if s, ok := t.(string); ok {
							n.Tags = append(n.Tags, s)
						}
					}
				}
				if meta, ok := fieldMap["metadata"].(map[string]any); ok {
					for k, v := range meta {
						n.Metadata[k] = v
					}
				}
			}
		case "$inc":
			if fieldMap, ok := fields.(map[string]any); ok {
				if energyInc, ok := fieldMap["energy"].(float64); ok {
					n.Energy += energyInc
					if n.Energy > 1.0 {
						n.Energy = 1.0
					}
					if n.Energy < 0 {
						n.Energy = 0
					}
				}
			}
		case "$push":
			if fieldMap, ok := fields.(map[string]any); ok {
				if tag, ok := fieldMap["tags"].(string); ok {
					n.Tags = append(n.Tags, tag)
				}
			}
		}
	}
	return nil
}

// executeDelete handles delete command
func (e *Executor) executeDelete(worker *concurrency.BrainWorker, cmd *Command) *Result {
	matrix := worker.Matrix()
	deleted := 0
	onlyOne := cmd.Type == CmdDeleteOne

	toDelete := make([]core.NeuronID, 0)
	for _, n := range matrix.Neurons {
		if len(cmd.Filter) > 0 && !e.matcher.MatchNeuron(n, cmd.Filter) {
			continue
		}
		toDelete = append(toDelete, n.ID)
		if onlyOne {
			break
		}
	}

	for _, id := range toDelete {
		_, err := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpForget,
			Payload: id,
		})
		if err == nil {
			deleted++
		}
	}

	return &Result{
		Success:    true,
		DeletedCnt: deleted,
	}
}

// executeCount handles count command
func (e *Executor) executeCount(worker *concurrency.BrainWorker, cmd *Command) *Result {
	matrix := worker.Matrix()
	count := 0

	for _, n := range matrix.Neurons {
		if len(cmd.Filter) == 0 || e.matcher.MatchNeuron(n, cmd.Filter) {
			count++
		}
	}

	return &Result{
		Success: true,
		Count:   count,
	}
}

// executeActivate fires a neuron
func (e *Executor) executeActivate(worker *concurrency.BrainWorker, cmd *Command) *Result {
	idStr, ok := cmd.Filter["_id"].(string)
	if !ok {
		return &Result{Success: false, Error: "_id filter required"}
	}

	_, err := worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpFire,
		Payload: core.NeuronID(idStr),
	})

	if err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	return &Result{Success: true}
}

// executeSearch performs semantic-like search with spread
func (e *Executor) executeSearch(worker *concurrency.BrainWorker, cmd *Command) *Result {
	query, ok := cmd.Filter["query"].(string)
	if !ok {
		return &Result{Success: false, Error: "query filter required"}
	}

	depth := cmd.Options.Depth
	if depth == 0 {
		depth = 2 // Default spread depth
	}

	limit := cmd.Options.Limit
	if limit == 0 {
		limit = 20 // Default limit
	}

	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{
			Query: query,
			Depth: depth,
			Limit: limit,
		},
	})

	if err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	neurons := result.([]*core.Neuron)
	docs := make([]map[string]any, 0, len(neurons))
	for _, n := range neurons {
		docs = append(docs, NeuronToDocument(n, cmd.Options.Projection))
	}

	return &Result{
		Success: true,
		Data:    docs,
		Count:   len(docs),
	}
}

// executeStats returns matrix statistics
func (e *Executor) executeStats(worker *concurrency.BrainWorker) *Result {
	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpGetStats,
	})

	if err != nil {
		return &Result{Success: false, Error: err.Error()}
	}

	return &Result{
		Success: true,
		Data:    result,
	}
}

// sortNeurons sorts neurons based on sort specification
func (e *Executor) sortNeurons(neurons []*core.Neuron, sortSpec map[string]int) {
	if len(sortSpec) == 0 {
		// Default: sort by energy descending
		sort.Slice(neurons, func(i, j int) bool {
			return neurons[i].Energy > neurons[j].Energy
		})
		return
	}

	sort.Slice(neurons, func(i, j int) bool {
		for field, order := range sortSpec {
			vi := e.matcher.getFieldValue(neurons[i], field)
			vj := e.matcher.getFieldValue(neurons[j], field)

			cmp := compare(vi, vj)
			if cmp != 0 {
				if order < 0 {
					return cmp > 0 // Descending
				}
				return cmp < 0 // Ascending
			}
		}
		return false
	})
}

// compare compares two values, returns -1, 0, or 1
func compare(a, b any) int {
	aFloat, aOk := toFloat(a)
	bFloat, bOk := toFloat(b)

	if aOk && bOk {
		if aFloat < bFloat {
			return -1
		}
		if aFloat > bFloat {
			return 1
		}
		return 0
	}

	// String comparison
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		if aStr < bStr {
			return -1
		}
		if aStr > bStr {
			return 1
		}
		return 0
	}

	return 0
}
