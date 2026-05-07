package blockdiff_test

import (
	"testing"

	"github.com/harleenquinzell/nodin/internal/blockdiff"
	"github.com/harleenquinzell/nodin/internal/notion"
)

func para(id, text string) notion.Block {
	return notion.Block{
		ID:   id,
		Type: "paragraph",
		Content: &notion.ParagraphContent{
			RichText: []notion.RichText{notion.NewRichText(text)},
		},
	}
}

func TestDiff_NoChange(t *testing.T) {
	blocks := []notion.Block{para("aaa", "hello"), para("bbb", "world")}
	ops := blockdiff.Diff(blocks, blocks)
	if len(ops) != 0 {
		t.Errorf("expected 0 ops, got %d: %+v", len(ops), ops)
	}
}

func TestDiff_Update(t *testing.T) {
	from := []notion.Block{para("aaa", "old")}
	to := []notion.Block{para("aaa", "new")}
	ops := blockdiff.Diff(from, to)
	if len(ops) != 1 || ops[0].Kind != blockdiff.Update || ops[0].ID != "aaa" {
		t.Errorf("expected one Update op for aaa, got %+v", ops)
	}
}

func TestDiff_Insert(t *testing.T) {
	from := []notion.Block{para("aaa", "first")}
	to := []notion.Block{para("aaa", "first"), para("bbb", "second")}
	ops := blockdiff.Diff(from, to)
	if len(ops) != 1 || ops[0].Kind != blockdiff.Insert {
		t.Errorf("expected one Insert op, got %+v", ops)
	}
	if ops[0].AfterID != "aaa" {
		t.Errorf("expected AfterID=aaa, got %q", ops[0].AfterID)
	}
}

func TestDiff_Delete(t *testing.T) {
	from := []notion.Block{para("aaa", "first"), para("bbb", "gone")}
	to := []notion.Block{para("aaa", "first")}
	ops := blockdiff.Diff(from, to)
	if len(ops) != 1 || ops[0].Kind != blockdiff.Delete || ops[0].ID != "bbb" {
		t.Errorf("expected one Delete op for bbb, got %+v", ops)
	}
}

func TestDiff_UnanchoredAlwaysInserted(t *testing.T) {
	from := []notion.Block{para("aaa", "existing")}
	to := []notion.Block{para("aaa", "existing"), para("", "unanchored")}
	ops := blockdiff.Diff(from, to)
	if len(ops) != 1 || ops[0].Kind != blockdiff.Insert {
		t.Fatalf("expected one Insert, got %+v", ops)
	}
	if ops[0].AfterID != "aaa" {
		t.Errorf("expected AfterID=aaa, got %q", ops[0].AfterID)
	}
}

func TestDiff_MixedOps(t *testing.T) {
	from := []notion.Block{
		para("a1", "keep"),
		para("a2", "old"),
		para("a3", "delete me"),
	}
	to := []notion.Block{
		para("a1", "keep"),
		para("a2", "updated"),
		para("a4", "new block"),
	}
	ops := blockdiff.Diff(from, to)

	byKind := make(map[blockdiff.OpKind][]blockdiff.Op)
	for _, op := range ops {
		byKind[op.Kind] = append(byKind[op.Kind], op)
	}

	if len(byKind[blockdiff.Update]) != 1 || byKind[blockdiff.Update][0].ID != "a2" {
		t.Errorf("expected Update a2, got %+v", byKind[blockdiff.Update])
	}
	if len(byKind[blockdiff.Insert]) != 1 {
		t.Errorf("expected one Insert, got %+v", byKind[blockdiff.Insert])
	}
	if len(byKind[blockdiff.Delete]) != 1 || byKind[blockdiff.Delete][0].ID != "a3" {
		t.Errorf("expected Delete a3, got %+v", byKind[blockdiff.Delete])
	}
}
