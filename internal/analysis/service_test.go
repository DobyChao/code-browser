package analysis

import (
    "testing"
    "github.com/sourcegraph/scip/bindings/go/scip"
)

func TestGetDefinitionFromSCIP_LineNumberMapping_SingleLine(t *testing.T) {
    idx := &scip.Index{Documents: []*scip.Document{}}
    doc := &scip.Document{RelativePath: "a.go"}
    doc.Occurrences = append(doc.Occurrences, &scip.Occurrence{
        Range:  []int32{0, 1, 5}, // single-line: [startLine=0, startCol=1, endCol=5]
        Symbol: "symA",
        SymbolRoles: int32(scip.SymbolRole_Definition),
    })
    idx.Documents = append(idx.Documents, doc)

    // 构造服务并直接调用内部函数
    s := &Service{}
    defs, err := s.getDefinitionFromSCIPForTest(idx, "a.go", 0, 2, "1")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(defs) == 0 {
        t.Fatalf("expected at least one definition")
    }
    d := defs[0]
    if d.Range.StartLine != 1 || d.Range.EndLine != 1 { // 1-based
        t.Fatalf("expected 1-based line, got start=%d end=%d", d.Range.StartLine, d.Range.EndLine)
    }
    if d.Range.StartColumn != 1 || d.Range.EndColumn != 5 {
        t.Fatalf("columns mismatch: %d..%d", d.Range.StartColumn, d.Range.EndColumn)
    }
}

func TestGetDefinitionFromSCIP_LineNumberMapping_MultiLine(t *testing.T) {
    idx := &scip.Index{Documents: []*scip.Document{}}
    doc := &scip.Document{RelativePath: "b.py"}
    doc.Occurrences = append(doc.Occurrences, &scip.Occurrence{
        Range:  []int32{2, 0, 4, 3}, // multi-line: [startLine=2, startCol=0, endLine=4, endCol=3]
        Symbol: "symB",
        SymbolRoles: int32(scip.SymbolRole_Definition),
    })
    idx.Documents = append(idx.Documents, doc)

    s := &Service{}
    defs, err := s.getDefinitionFromSCIPForTest(idx, "b.py", 2, 1, "1")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(defs) == 0 {
        t.Fatalf("expected at least one definition")
    }
    d := defs[0]
    if d.Range.StartLine != 3 || d.Range.EndLine != 5 { // +1
        t.Fatalf("expected 1-based lines 3..5, got %d..%d", d.Range.StartLine, d.Range.EndLine)
    }
    if d.Range.StartColumn != 0 || d.Range.EndColumn != 3 {
        t.Fatalf("columns mismatch: %d..%d", d.Range.StartColumn, d.Range.EndColumn)
    }
}
