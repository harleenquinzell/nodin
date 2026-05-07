package blockdiff

import (
	"github.com/harleenquinzell/nodin/internal/convert"
	"github.com/harleenquinzell/nodin/internal/notion"
)

// OpKind identifies the kind of block operation.
type OpKind int

const (
	Insert OpKind = iota
	Update
	Delete
)

// Op is a single block-level change.
type Op struct {
	Kind    OpKind
	ID      string       // existing block ID (for Update/Delete)
	Block   notion.Block // new block content (for Insert/Update)
	AfterID string       // for Insert: insert after this block ID ("" = first position)
}

// Diff computes operations to transform from into to.
//
// Blocks are matched by ID (set from anchor comments). Blocks without IDs are
// treated as new and always produce Insert ops. The result contains:
//   - Update ops for matched blocks whose rendered markdown changed
//   - Insert ops for new blocks, each carrying the ID of the preceding block
//   - Delete ops for blocks in from whose IDs do not appear in to
func Diff(from, to []notion.Block) []Op {
	fromByID := make(map[string]notion.Block, len(from))
	for _, b := range from {
		if b.ID != "" {
			fromByID[b.ID] = b
		}
	}

	seen := make(map[string]bool, len(from))

	var ops []Op
	// lastKnownID tracks the ID of the last block we matched in `from`, used as
	// the AfterID for subsequent inserts so ordering is preserved.
	lastKnownID := ""

	for _, b := range to {
		if b.ID == "" {
			ops = append(ops, Op{Kind: Insert, Block: b, AfterID: lastKnownID})
			continue
		}

		old, matched := fromByID[b.ID]
		if !matched {
			ops = append(ops, Op{Kind: Insert, Block: b, AfterID: lastKnownID})
			continue
		}

		seen[b.ID] = true
		lastKnownID = b.ID

		if convert.BlockMarkdown(b) != convert.BlockMarkdown(old) {
			ops = append(ops, Op{Kind: Update, ID: b.ID, Block: b})
		}
	}

	// Delete blocks present in from but absent in to.
	for _, b := range from {
		if b.ID != "" && !seen[b.ID] {
			ops = append(ops, Op{Kind: Delete, ID: b.ID})
		}
	}

	return ops
}
